package daytona

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"time"

	dtoptions "github.com/daytonaio/daytona/libs/sdk-go/pkg/options"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	atrace "trpc.group/trpc-go/trpc-agent-go/telemetry/trace"
)

// Compile-time interface checks.
var (
	_ codeexecutor.WorkspaceManager = (*workspaceRuntime)(nil)
	_ codeexecutor.WorkspaceFS      = (*workspaceRuntime)(nil)
	_ codeexecutor.ProgramRunner    = (*workspaceRuntime)(nil)
)

const (
	defaultExecTimeout    = 5 * time.Minute
	defaultStageTimeout   = 60 * time.Second
	defaultCollectTimeout = 30 * time.Second
	defaultCreateTimeout  = 15 * time.Second

	// maxReadSizeBytes is the per-file cap when collecting outputs (4 MiB).
	maxReadSizeBytes = 4 * 1024 * 1024

	// sentinelSep separates stdout from stderr in the RunProgram wrapper.
	// Uses a null-byte prefix to minimise collision probability.
	sentinelSep = "\x00__DAYTONA_STDERR_SEP__\x00"

	// Input URI scheme prefixes — mirror e2b/container for consistency.
	inputSchemeArtifact  = "artifact://"
	inputSchemeHost      = "host://"
	inputSchemeWorkspace = "workspace://"
	inputSchemeSkill     = "skill://"

	// s3 schemes carry a user-uploaded chat attachment as "<scheme>bucket/key".
	// The suffix selects the transfer mechanism (decided by the caller from
	// the file size): s3buf:// buffers via the API process + SDK upload;
	// s3url:// hands the sandbox a presigned URL to download itself.
	inputSchemeS3Buf = SchemeS3Buf
	inputSchemeS3URL = SchemeS3URL

	// s3StageTimeout bounds an in-sandbox attachment download. Large genomics
	// files over a presigned URL can take minutes, so this is far longer than
	// the generic stage timeout.
	s3StageTimeout = 15 * time.Minute

	// s3PresignTTL is how long a generated download URL stays valid — long
	// enough to cover s3StageTimeout with margin.
	s3PresignTTL = 30 * time.Minute
)

// Exported scheme prefixes so the agent layer can construct InputSpec.From
// values without duplicating the literals.
const (
	SchemeS3Buf = "s3buf://"
	SchemeS3URL = "s3url://"
)

// workspaceRuntime implements WorkspaceManager, WorkspaceFS, and ProgramRunner
// for the Daytona persistent sandbox model.
//
// Persistent model invariant: CreateWorkspace always returns the same physical
// path (ce.workspacePath) regardless of execID. The standard layout
// (skills/, work/, runs/, out/, src/) is created once and survives across
// agent invocations within the same sandbox.
type workspaceRuntime struct {
	ce *CodeExecutor
}

func newWorkspaceRuntime(ce *CodeExecutor) *workspaceRuntime {
	return &workspaceRuntime{ce: ce}
}

// ─── WorkspaceManager ────────────────────────────────────────────────────────

// CreateWorkspace ensures the standard workspace layout exists inside the
// sandbox and returns a Workspace descriptor. Idempotent: subsequent calls
// with any execID return the same physical path.
func (r *workspaceRuntime) CreateWorkspace(
	ctx context.Context,
	execID string,
	pol codeexecutor.WorkspacePolicy,
) (codeexecutor.Workspace, error) {
	_, span := atrace.Tracer.Start(ctx, codeexecutor.SpanWorkspaceCreate)
	span.SetAttributes(attribute.String(codeexecutor.AttrExecID, execID))
	defer span.End()

	if err := r.ce.ensureSandbox(ctx); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return codeexecutor.Workspace{}, err
	}

	_ = pol // persistent model: ignore isolation toggle
	wsPath := r.ce.workspacePath

	// One bash call creates all required subdirectories (idempotent mkdir -p).
	var mkdirScript strings.Builder
	mkdirScript.WriteString("set -e; mkdir -p")
	for _, sub := range []string{
		wsPath,
		path.Join(wsPath, codeexecutor.DirSkills),
		path.Join(wsPath, codeexecutor.DirWork),
		path.Join(wsPath, codeexecutor.DirRuns),
		path.Join(wsPath, codeexecutor.DirOut),
		path.Join(wsPath, codeexecutor.InlineSourceDir),
		path.Join(wsPath, codeexecutor.DirWork, "inputs"),
	} {
		mkdirScript.WriteByte(' ')
		mkdirScript.WriteString(shellQuote(sub))
	}
	// Initialise metadata.json only if absent.
	metaPath := path.Join(wsPath, codeexecutor.MetaFileName)
	mkdirScript.WriteString(
		"; [ -f " + shellQuote(metaPath) +
			" ] || echo '{}' > " + shellQuote(metaPath),
	)

	if _, err := r.execBash(ctx, mkdirScript.String(), defaultCreateTimeout); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return codeexecutor.Workspace{}, err
	}

	ws := codeexecutor.Workspace{ID: execID, Path: wsPath}
	// Stage user attachments into the just-created layout, once per turn,
	// before any code runs. Doing it here (rather than in the agent factory)
	// keeps it lazy: a turn that never creates a workspace never stages, and
	// it covers every execution path — run_python_inline (ExecuteCode) and
	// the workspace_exec tools both funnel through CreateWorkspace.
	r.ce.stageInputsOnce(ctx, r, ws)
	return ws, nil
}

// Cleanup prunes stale per-run directories (runs/run_* older than 24 h) but
// never removes the workspace root or any user data. This preserves the
// persistent workspace invariant.
func (r *workspaceRuntime) Cleanup(
	ctx context.Context,
	ws codeexecutor.Workspace,
) error {
	_, span := atrace.Tracer.Start(ctx, codeexecutor.SpanWorkspaceCleanup)
	span.SetAttributes(attribute.String(codeexecutor.AttrPath, ws.Path))
	defer span.End()

	// Never provision a sandbox just to clean it: if none was created this
	// turn there is nothing to prune.
	if r.ce.sbx == nil {
		return nil
	}

	runsDir := path.Join(ws.Path, codeexecutor.DirRuns)
	script := fmt.Sprintf(
		"find %s -maxdepth 1 -name 'run_*' -mtime +1 -exec rm -rf {} + 2>/dev/null; true",
		shellQuote(runsDir),
	)
	if _, err := r.execBash(ctx, script, defaultStageTimeout); err != nil {
		// Non-fatal: stale run dirs are cosmetic.
		span.SetStatus(codes.Error, err.Error())
	}
	return nil
}

// ─── WorkspaceFS ─────────────────────────────────────────────────────────────

// PutFiles writes files into the sandbox workspace.
// Parent directories are created automatically before upload.
func (r *workspaceRuntime) PutFiles(
	ctx context.Context,
	ws codeexecutor.Workspace,
	files []codeexecutor.PutFile,
) error {
	_, span := atrace.Tracer.Start(ctx, codeexecutor.SpanWorkspaceStageFiles)
	span.SetAttributes(attribute.Int(codeexecutor.AttrCount, len(files)))
	defer span.End()

	if len(files) == 0 {
		return nil
	}
	if err := r.ce.ensureSandbox(ctx); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	// Collect unique parent dirs and create them in one bash call.
	dirs := make(map[string]struct{})
	for _, f := range files {
		clean := path.Clean(filepath.ToSlash(f.Path))
		absPath := path.Join(ws.Path, clean)
		if d := path.Dir(absPath); d != "" && d != "." && d != "/" {
			dirs[d] = struct{}{}
		}
	}
	if len(dirs) > 0 {
		var mkScript strings.Builder
		mkScript.WriteString("mkdir -p")
		for d := range dirs {
			mkScript.WriteByte(' ')
			mkScript.WriteString(shellQuote(d))
		}
		if _, err := r.execBash(ctx, mkScript.String(), defaultStageTimeout); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}

	// Upload each file via the native Daytona FileSystem API.
	for _, f := range files {
		clean := path.Clean(filepath.ToSlash(f.Path))
		absPath := path.Join(ws.Path, clean)

		if err := r.ce.sbx.FileSystem.UploadFile(ctx, f.Content, absPath); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return fmt.Errorf("daytona: upload %s: %w", f.Path, err)
		}
		if f.Mode != 0 {
			chmod := fmt.Sprintf("chmod %o %s", f.Mode, shellQuote(absPath))
			if _, err := r.execBash(ctx, chmod, defaultStageTimeout); err != nil {
				span.SetStatus(codes.Error, err.Error())
				return err
			}
		}
	}
	return nil
}

// StageDirectory copies a host-side directory into ws/to inside the sandbox.
// When opt.ReadOnly is true the destination tree is made non-writable.
func (r *workspaceRuntime) StageDirectory(
	ctx context.Context,
	ws codeexecutor.Workspace,
	src, to string,
	opt codeexecutor.StageOptions,
) error {
	_, span := atrace.Tracer.Start(ctx, codeexecutor.SpanWorkspaceStageDir)
	span.SetAttributes(
		attribute.String(codeexecutor.AttrHostPath, src),
		attribute.String(codeexecutor.AttrTo, to),
	)
	defer span.End()

	if err := r.ce.ensureSandbox(ctx); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	abs, err := filepath.Abs(src)
	if err != nil {
		return err
	}
	dest := ws.Path
	if to != "" {
		dest = path.Join(ws.Path, filepath.ToSlash(to))
	}

	// Walk host directory and upload each file.
	err = filepath.WalkDir(abs, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(abs, p)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		remote := path.Join(dest, rel)

		if d.IsDir() {
			_, e := r.execBash(ctx, "mkdir -p "+shellQuote(remote), defaultStageTimeout)
			return e
		}
		if !d.Type().IsRegular() {
			return nil
		}
		data, e := os.ReadFile(p) //nolint:gosec // p comes from walking a trusted local staging dir, not untrusted input
		if e != nil {
			return e
		}
		return r.ce.sbx.FileSystem.UploadFile(ctx, data, remote)
	})
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return err
	}

	if opt.ReadOnly {
		script := "chmod -R a-w " + shellQuote(dest)
		if _, err := r.execBash(ctx, script, defaultStageTimeout); err != nil {
			span.SetStatus(codes.Error, err.Error())
			return err
		}
	}
	span.SetAttributes(attribute.Bool(codeexecutor.AttrMountUsed, false))
	return nil
}

// Collect returns files matching the supplied glob patterns using bash globstar
// semantics (same as the E2B runtime) for ** support.
func (r *workspaceRuntime) Collect(
	ctx context.Context,
	ws codeexecutor.Workspace,
	patterns []string,
) ([]codeexecutor.File, error) {
	_, span := atrace.Tracer.Start(ctx, codeexecutor.SpanWorkspaceCollect)
	span.SetAttributes(attribute.Int(codeexecutor.AttrPatterns, len(patterns)))
	defer span.End()

	patterns = codeexecutor.NormalizeGlobs(patterns)
	if len(patterns) == 0 {
		return nil, nil
	}
	if err := r.ce.ensureSandbox(ctx); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	matched, err := r.listFilesByGlob(ctx, ws.Path, patterns)
	if err != nil {
		span.SetStatus(codes.Error, err.Error())
		return nil, err
	}

	out := make([]codeexecutor.File, 0, len(matched))
	seen := map[string]bool{}
	for _, full := range matched {
		rel := strings.TrimPrefix(full, ws.Path+"/")
		if rel == full {
			rel = filepath.ToSlash(full)
		}
		if codeexecutor.IsRootMetadataTempPath(rel) {
			continue
		}
		if seen[rel] {
			continue
		}
		seen[rel] = true

		data, size, truncated, err := r.readFile(ctx, full, maxReadSizeBytes)
		if err != nil {
			span.SetStatus(codes.Error, err.Error())
			return nil, err
		}
		mime := http.DetectContentType(data)
		out = append(out, codeexecutor.File{
			Name:      rel,
			Content:   string(data),
			MIMEType:  mime,
			SizeBytes: size,
			Truncated: truncated,
		})
	}
	span.SetAttributes(attribute.Int(codeexecutor.AttrCount, len(out)))
	return out, nil
}

// StageInputs maps external inputs (artifact://, host://, workspace://, skill://)
// into the workspace, recording each in metadata for idempotent re-staging.
//
// Persistent model optimisation: when sp.Pin is true and metadata already
// records the same artifact@version at the same destination, the download is
// skipped entirely. This avoids re-transferring files across conversation turns.
func (r *workspaceRuntime) StageInputs(
	ctx context.Context,
	ws codeexecutor.Workspace,
	specs []codeexecutor.InputSpec,
) error {
	if len(specs) == 0 {
		return nil
	}
	if err := r.ce.ensureSandbox(ctx); err != nil {
		return err
	}
	return codeexecutor.WithWorkspaceMetadataLock(
		ctx, ws.Path,
		func(ctx context.Context) error {
			return r.stageInputsLocked(ctx, ws, specs)
		},
	)
}

func (r *workspaceRuntime) stageInputsLocked(
	ctx context.Context,
	ws codeexecutor.Workspace,
	specs []codeexecutor.InputSpec,
) error {
	md, err := r.loadWorkspaceMetadata(ctx, ws)
	if err != nil {
		return err
	}

	// One failing input must not strand the rest of the batch, and — just
	// as important — must not skip the metadata save: records for inputs
	// already transferred in this batch exist only in memory until the
	// save, and losing them forces a full re-download on every later turn.
	var errs []error
	for _, sp := range specs {
		mode := strings.ToLower(strings.TrimSpace(sp.Mode))
		if mode == "" {
			mode = "copy"
		}
		to := strings.TrimSpace(sp.To)
		if to == "" {
			to = path.Join(codeexecutor.DirWork, "inputs", inputBase(sp.From))
		}
		dest := path.Join(ws.Path, filepath.ToSlash(to))

		// Persistent model: skip re-download if already staged at this version.
		if sp.Pin && isAlreadyStaged(md, sp.From, to) {
			continue
		}

		resolved, ver, stageErr := r.stageInput(ctx, ws, md, sp, mode, dest)
		if stageErr != nil {
			errs = append(errs, fmt.Errorf("stage %s: %w", sp.From, stageErr))
			continue
		}
		md.Inputs = append(md.Inputs, codeexecutor.InputRecord{
			From:      sp.From,
			To:        to,
			Resolved:  resolved,
			Version:   ver,
			Mode:      mode,
			Timestamp: time.Now(),
		})
	}
	if saveErr := r.saveWorkspaceMetadata(ctx, ws, md); saveErr != nil {
		errs = append(errs, saveErr)
	}
	return errors.Join(errs...)
}

func (r *workspaceRuntime) stageInput(
	ctx context.Context,
	ws codeexecutor.Workspace,
	md codeexecutor.WorkspaceMetadata,
	sp codeexecutor.InputSpec,
	mode, dest string,
) (string, *int, error) {
	switch {
	case strings.HasPrefix(sp.From, inputSchemeArtifact):
		return r.stageArtifactInput(ctx, md, sp, dest)

	case strings.HasPrefix(sp.From, inputSchemeHost):
		hostPath := strings.TrimPrefix(sp.From, inputSchemeHost)
		to := strings.TrimPrefix(dest, ws.Path+"/")
		if err := r.StageDirectory(ctx, ws, hostPath, path.Dir(to), codeexecutor.StageOptions{}); err != nil {
			return "", nil, err
		}
		return hostPath, nil, nil

	case strings.HasPrefix(sp.From, inputSchemeWorkspace):
		rest := strings.TrimPrefix(sp.From, inputSchemeWorkspace)
		src := path.Join(ws.Path, filepath.ToSlash(rest))
		return src, nil, r.copyInsideSandbox(ctx, src, dest, mode)

	case strings.HasPrefix(sp.From, inputSchemeSkill):
		rest := strings.TrimPrefix(sp.From, inputSchemeSkill)
		src := path.Join(ws.Path, codeexecutor.DirSkills, filepath.ToSlash(rest))
		return src, nil, r.copyInsideSandbox(ctx, src, dest, mode)

	case strings.HasPrefix(sp.From, inputSchemeS3Buf):
		return r.stageS3Buffered(ctx, sp.From, dest)

	case strings.HasPrefix(sp.From, inputSchemeS3URL):
		resolved, ver, presErr := r.stageS3Presigned(ctx, sp.From, dest)
		if presErr != nil && r.ce != nil && r.ce.fetcher != nil {
			// The sandbox may be unable to reach the storage endpoint
			// directly (e.g. an internal MinIO host it can't resolve). Fall
			// back to buffering the object through the API process via the
			// same bucket/key.
			return r.stageS3Buffered(ctx, inputSchemeS3Buf+canonicalS3Ref(sp.From), dest)
		}
		return resolved, ver, presErr

	default:
		return "", nil, fmt.Errorf("daytona: unsupported input scheme: %s", sp.From)
	}
}

// stageS3Buffered downloads the object through the API process and uploads it
// into the sandbox via the Daytona SDK. Used for small attachments where
// buffering in memory is cheap.
func (r *workspaceRuntime) stageS3Buffered(ctx context.Context, from, dest string) (string, *int, error) {
	if r.ce == nil || r.ce.fetcher == nil {
		return "", nil, errors.New("daytona: no object fetcher configured for s3buf:// input")
	}
	bucket, key, err := parseS3Ref(from, inputSchemeS3Buf)
	if err != nil {
		return "", nil, err
	}
	data, err := r.ce.fetcher.GetObject(ctx, bucket, key)
	if err != nil {
		return "", nil, fmt.Errorf("daytona: fetch s3://%s/%s: %w", bucket, key, err)
	}
	if _, mkErr := r.execBash(ctx, "mkdir -p "+shellQuote(path.Dir(dest)), defaultStageTimeout); mkErr != nil {
		return "", nil, mkErr
	}
	if err := r.ce.sbx.FileSystem.UploadFile(ctx, data, dest); err != nil {
		return "", nil, fmt.Errorf("daytona: upload attachment to %s: %w", dest, err)
	}
	return fmt.Sprintf("s3://%s/%s", bucket, key), nil, nil
}

// stageS3Presigned hands the sandbox a presigned GET URL and downloads the
// object in-sandbox (curl, then wget fallback). The object never passes
// through the API process, so multi-GB files stage without memory pressure.
// Requires sandbox network egress to the storage endpoint.
func (r *workspaceRuntime) stageS3Presigned(ctx context.Context, from, dest string) (string, *int, error) { //nolint:unparam // (string, *int, error) matches the stager-family signature; this stager never sets the *int
	if r.ce == nil || r.ce.fetcher == nil {
		return "", nil, errors.New("daytona: no object fetcher configured for s3url:// input")
	}
	bucket, key, err := parseS3Ref(from, inputSchemeS3URL)
	if err != nil {
		return "", nil, err
	}
	url, err := r.ce.fetcher.PresignGet(ctx, bucket, key, s3PresignTTL)
	if err != nil {
		return "", nil, fmt.Errorf("daytona: presign s3://%s/%s: %w", bucket, key, err)
	}
	// curl first, wget as a fallback for minimal images. -f makes curl exit
	// non-zero on HTTP errors so a failed download doesn't leave a 0-byte file
	// the agent would mistake for real data.
	curlInsecure, wgetInsecure := "", ""
	if r.ce.insecureTLS {
		curlInsecure = "-k "
		wgetInsecure = "--no-check-certificate "
	}
	// --connect-timeout bounds name resolution + connect so an unreachable
	// storage endpoint fails fast and the caller can fall back to a buffered
	// download instead of stalling staging.
	script := fmt.Sprintf(
		"set -e; mkdir -p %s; "+
			"if command -v curl >/dev/null 2>&1; then curl -fsSL %s--connect-timeout 15 -o %s %s; "+
			"elif command -v wget >/dev/null 2>&1; then wget -q %s--connect-timeout=15 -O %s %s; "+
			"else echo 'no curl or wget in sandbox' >&2; exit 127; fi",
		shellQuote(path.Dir(dest)),
		curlInsecure, shellQuote(dest), shellQuote(url),
		wgetInsecure, shellQuote(dest), shellQuote(url),
	)
	res, err := r.execBash(ctx, script, s3StageTimeout)
	if err != nil {
		return "", nil, fmt.Errorf("daytona: download s3://%s/%s into sandbox: %w", bucket, key, err)
	}
	if res.ExitCode != 0 {
		return "", nil, fmt.Errorf(
			"daytona: download s3://%s/%s failed (exit %d)", bucket, key, res.ExitCode,
		)
	}
	return fmt.Sprintf("s3://%s/%s", bucket, key), nil, nil
}

// canonicalS3Ref reduces an s3buf:// or s3url:// input ref to its
// scheme-independent identity "bucket/key". Returns "" for non-s3 refs.
func canonicalS3Ref(from string) string {
	switch {
	case strings.HasPrefix(from, inputSchemeS3Buf):
		return strings.TrimPrefix(from, inputSchemeS3Buf)
	case strings.HasPrefix(from, inputSchemeS3URL):
		return strings.TrimPrefix(from, inputSchemeS3URL)
	}
	return ""
}

// parseS3Ref splits "<scheme>bucket/key..." into bucket and key. The key may
// contain further slashes; only the first separates bucket from key.
func parseS3Ref(from, scheme string) (bucket, key string, err error) {
	rest := strings.TrimPrefix(from, scheme)
	i := strings.IndexByte(rest, '/')
	if i <= 0 || i == len(rest)-1 {
		return "", "", fmt.Errorf("daytona: malformed s3 ref %q (want bucket/key)", from)
	}
	return rest[:i], rest[i+1:], nil
}

func (r *workspaceRuntime) stageArtifactInput(
	ctx context.Context,
	md codeexecutor.WorkspaceMetadata,
	sp codeexecutor.InputSpec,
	dest string,
) (string, *int, error) {
	name := strings.TrimPrefix(sp.From, inputSchemeArtifact)
	aname, aver, err := codeexecutor.ParseArtifactRef(name)
	if err != nil {
		return "", nil, err
	}
	useVer := aver
	if useVer == nil && sp.Pin {
		useVer = pinnedArtifactVersion(md, aname, dest)
	}

	data, _, actual, err := codeexecutor.LoadArtifactHelper(ctx, aname, useVer)
	if err != nil {
		return "", nil, err
	}
	ver := useVer
	if ver == nil {
		v := actual
		ver = &v
	}

	if _, mkErr := r.execBash(ctx, "mkdir -p "+shellQuote(path.Dir(dest)), defaultStageTimeout); mkErr != nil {
		return "", nil, mkErr
	}
	if err := r.ce.sbx.FileSystem.UploadFile(ctx, data, dest); err != nil {
		return "", nil, fmt.Errorf("daytona: upload artifact %s to %s: %w", aname, dest, err)
	}
	return aname, ver, nil
}

func (r *workspaceRuntime) copyInsideSandbox(ctx context.Context, src, dest, mode string) error {
	var script strings.Builder
	script.WriteString("mkdir -p ")
	script.WriteString(shellQuote(path.Dir(dest)))
	script.WriteString(" && ")
	if mode == "link" {
		script.WriteString("ln -sfn ")
	} else {
		script.WriteString("cp -a ")
	}
	script.WriteString(shellQuote(src))
	script.WriteByte(' ')
	script.WriteString(shellQuote(dest))
	_, err := r.execBash(ctx, script.String(), defaultStageTimeout)
	return err
}

// CollectOutputs applies workspace-side globs and optionally persists files as
// artifacts via the artifact service in ctx.
func (r *workspaceRuntime) CollectOutputs(
	ctx context.Context,
	ws codeexecutor.Workspace,
	spec codeexecutor.OutputSpec,
) (codeexecutor.OutputManifest, error) {
	if err := r.ce.ensureSandbox(ctx); err != nil {
		return codeexecutor.OutputManifest{}, err
	}
	globs := codeexecutor.NormalizeGlobs(spec.Globs)
	paths, err := r.listFilesByGlob(ctx, ws.Path, globs)
	if err != nil {
		return codeexecutor.OutputManifest{}, err
	}

	maxFiles := spec.MaxFiles
	if maxFiles <= 0 {
		maxFiles = 100
	}
	maxFileBytes := spec.MaxFileBytes
	if maxFileBytes <= 0 {
		maxFileBytes = maxReadSizeBytes
	}
	maxTotal := spec.MaxTotalBytes
	if maxTotal <= 0 {
		maxTotal = 64 * 1024 * 1024
	}
	left := maxTotal

	mf := codeexecutor.OutputManifest{}
	count := 0
	for _, full := range paths {
		if count >= maxFiles || left <= 0 {
			mf.LimitsHit = true
			break
		}
		rel := strings.TrimPrefix(full, ws.Path+"/")
		if codeexecutor.IsRootMetadataTempPath(rel) {
			continue
		}
		limit := min(maxFileBytes, left)

		data, size, truncated, err := r.readFile(ctx, full, limit)
		if err != nil {
			return codeexecutor.OutputManifest{}, err
		}
		mime := http.DetectContentType(data)
		if truncated {
			mf.LimitsHit = true
		}
		if truncated && spec.Save {
			return codeexecutor.OutputManifest{}, fmt.Errorf(
				"daytona: cannot save truncated output file: %s", rel,
			)
		}
		left -= int64(len(data))

		ref := codeexecutor.FileRef{
			Name:      rel,
			MIMEType:  mime,
			SizeBytes: size,
			Truncated: truncated,
		}
		if spec.Inline {
			ref.Content = string(data)
		}
		if spec.Save {
			saveName := rel
			if spec.NameTemplate != "" {
				saveName = spec.NameTemplate + rel
			}
			ver, saveErr := codeexecutor.SaveArtifactHelper(ctx, saveName, data, mime)
			if saveErr != nil {
				return codeexecutor.OutputManifest{}, saveErr
			}
			ref.SavedAs = saveName
			ref.Version = ver
		}
		mf.Files = append(mf.Files, ref)
		count++
	}
	return mf, nil
}

// ─── ProgramRunner ───────────────────────────────────────────────────────────

// RunProgram executes a command inside the persistent workspace.
//
// stdout and stderr are separated via a sentinel wrapper script:
//
//	{ cmd ... } 2>$tmpfile
//	printf SENTINEL
//	cat $tmpfile
//	exit $inner_exit_code
//
// The exit code comes directly from ExecuteResponse.ExitCode.
func (r *workspaceRuntime) RunProgram(
	ctx context.Context,
	ws codeexecutor.Workspace,
	spec codeexecutor.RunProgramSpec,
) (codeexecutor.RunResult, error) {
	_, span := atrace.Tracer.Start(ctx, codeexecutor.SpanWorkspaceRun)
	span.SetAttributes(
		attribute.String(codeexecutor.AttrCmd, spec.Cmd),
		attribute.String(codeexecutor.AttrCwd, spec.Cwd),
	)
	defer span.End()

	if err := r.ce.ensureSandbox(ctx); err != nil {
		span.SetStatus(codes.Error, err.Error())
		return codeexecutor.RunResult{}, err
	}

	timeout := spec.Timeout
	if timeout <= 0 {
		timeout = defaultExecTimeout
	}

	// Resolve working directory.
	cwd := ws.Path
	if spec.Cwd != "" {
		cwd = path.Join(ws.Path, filepath.ToSlash(spec.Cwd))
	}

	// Build well-known workspace environment variables.
	runDir := path.Join(
		ws.Path, codeexecutor.DirRuns,
		"run_"+time.Now().Format("20060102T150405.000"),
	)
	baseEnv := map[string]string{
		codeexecutor.WorkspaceEnvDirKey: ws.Path,
		codeexecutor.EnvSkillsDir:       path.Join(ws.Path, codeexecutor.DirSkills),
		codeexecutor.EnvWorkDir:         path.Join(ws.Path, codeexecutor.DirWork),
		codeexecutor.EnvOutputDir:       path.Join(ws.Path, codeexecutor.DirOut),
		codeexecutor.EnvRunDir:          runDir,
	}
	// Merge: spec.Env overrides baseEnv.
	mergedEnv := make(map[string]string, len(baseEnv)+len(spec.Env))
	maps.Copy(mergedEnv, baseEnv)
	maps.Copy(mergedEnv, spec.Env)

	// Build env prefix string: "KEY='VAL' KEY2='VAL2' "
	var envParts []string
	for k, v := range mergedEnv {
		envParts = append(envParts, k+"="+shellQuote(v))
	}
	envAssign := ""
	if len(envParts) > 0 {
		envAssign = strings.Join(envParts, " ") + " "
	}

	// Build quoted command + args.
	quotedCmd := shellQuote(spec.Cmd)
	var quotedArgs strings.Builder
	for _, a := range spec.Args {
		quotedArgs.WriteByte(' ')
		quotedArgs.WriteString(shellQuote(a))
	}

	// Optional stdin via base64-decoded process substitution.
	stdinRedir := ""
	if spec.Stdin != "" {
		b64 := base64.StdEncoding.EncodeToString([]byte(spec.Stdin))
		stdinRedir = " < <(printf %s " + shellQuote(b64) + " | base64 -d)"
	}

	// Inner command: create run/out dirs, cd to cwd, set env, run program.
	inner := fmt.Sprintf(
		"mkdir -p %s %s && cd %s && %s%s%s%s",
		shellQuote(runDir),
		shellQuote(path.Join(ws.Path, codeexecutor.DirOut)),
		shellQuote(cwd),
		envAssign, quotedCmd, quotedArgs.String(),
		stdinRedir,
	)

	// Wrapper: capture stderr to temp file, then emit sentinel + stderr in stdout.
	// The wrapper exits with the inner command's exit code.
	wrapper := "bash -c " + shellQuote(
		"__dt_ef=$(mktemp) || __dt_ef=/dev/null; "+
			"{ "+inner+"; } 2>\"$__dt_ef\"; __dt_ec=$?; "+
			"printf '%s' "+shellQuote(sentinelSep)+"; "+
			"cat \"$__dt_ef\"; "+
			"rm -f \"$__dt_ef\"; "+
			"exit $__dt_ec",
	)

	start := time.Now()
	resp, err := r.ce.sbx.Process.ExecuteCommand(ctx, wrapper,
		func(o *dtoptions.ExecuteCommand) {
			o.Timeout = &timeout
		},
	)
	dur := time.Since(start)

	timedOut := false
	if err != nil {
		if isTimeoutErr(err) {
			timedOut = true
		} else {
			span.SetStatus(codes.Error, err.Error())
			return codeexecutor.RunResult{}, err
		}
	}

	var result string
	var exitCode int
	if resp != nil {
		result = resp.Result
		exitCode = resp.ExitCode
	}

	stdout, stderr := parseSentinelOutput(result)
	res := codeexecutor.RunResult{
		Stdout:   stdout,
		Stderr:   stderr,
		ExitCode: exitCode,
		Duration: dur,
		TimedOut: timedOut,
	}
	span.SetAttributes(
		attribute.Int(codeexecutor.AttrExitCode, res.ExitCode),
		attribute.Bool(codeexecutor.AttrTimedOut, res.TimedOut),
	)
	return res, nil
}

// parseSentinelOutput splits the wrapper's Result into stdout and stderr.
func parseSentinelOutput(result string) (stdout, stderr string) {
	before, after, ok := strings.Cut(result, sentinelSep)
	if !ok {
		return strings.TrimRight(result, "\n"), ""
	}
	return strings.TrimRight(before, "\n"),
		strings.TrimRight(after, "\n")
}

// ─── Metadata helpers ────────────────────────────────────────────────────────

func (r *workspaceRuntime) loadWorkspaceMetadata(
	ctx context.Context,
	ws codeexecutor.Workspace,
) (codeexecutor.WorkspaceMetadata, error) {
	md := codeexecutor.NewWorkspaceMetadata()
	metaPath := path.Join(ws.Path, codeexecutor.MetaFileName)

	data, _, _, err := r.readFile(ctx, metaPath, maxReadSizeBytes)
	if err != nil {
		return md, nil // treat missing as empty
	}
	trimmed := strings.TrimSpace(string(data))
	if trimmed == "" || trimmed == "{}" {
		return md, nil
	}
	if jsonErr := json.Unmarshal(data, &md); jsonErr != nil {
		if codeexecutor.IsMetadataCorruptError(jsonErr) {
			return codeexecutor.NewWorkspaceMetadata(), nil
		}
		return codeexecutor.WorkspaceMetadata{}, jsonErr
	}
	if md.Skills == nil {
		md.Skills = map[string]codeexecutor.SkillMeta{}
	}
	md.LastAccess = time.Now()
	return md, nil
}

func (r *workspaceRuntime) saveWorkspaceMetadata(
	ctx context.Context,
	ws codeexecutor.Workspace,
	md codeexecutor.WorkspaceMetadata,
) error {
	now := time.Now()
	if md.Version == 0 {
		md.Version = 1
	}
	if md.CreatedAt.IsZero() {
		md.CreatedAt = now
	}
	md.UpdatedAt = now
	md.LastAccess = now
	if md.Skills == nil {
		md.Skills = map[string]codeexecutor.SkillMeta{}
	}

	buf, err := json.MarshalIndent(md, "", "  ")
	if err != nil {
		return err
	}

	// Atomic write: upload to temp path then rename inside sandbox.
	tmpFile := codeexecutor.MetadataTempFileName()
	tmpPath := path.Join(ws.Path, tmpFile)
	metaPath := path.Join(ws.Path, codeexecutor.MetaFileName)

	if uploadErr := r.ce.sbx.FileSystem.UploadFile(ctx, buf, tmpPath); uploadErr != nil {
		return fmt.Errorf("daytona: upload metadata tmp: %w", uploadErr)
	}
	mvScript := "mv -f " + shellQuote(tmpPath) + " " + shellQuote(metaPath)
	resp, err := r.execBash(ctx, mvScript, defaultStageTimeout)
	if err != nil {
		return err
	}
	if resp.ExitCode != 0 {
		return fmt.Errorf("daytona: metadata commit failed (exit %d)", resp.ExitCode)
	}
	return nil
}

// ─── Low-level primitives ────────────────────────────────────────────────────

// execBashResult holds the output of a single execBash call.
type execBashResult struct {
	ExitCode int
	Result   string
	TimedOut bool
}

// execBash runs a bash script via Daytona's ExecuteCommand. It is the
// foundational primitive for all sandbox interactions (analogous to E2B's
// runBash / runBashStreaming).
func (r *workspaceRuntime) execBash(
	ctx context.Context,
	script string,
	timeout time.Duration,
) (*execBashResult, error) {
	if r.ce == nil || r.ce.sbx == nil {
		return nil, errors.New("daytona: sandbox not initialized")
	}
	if timeout <= 0 {
		timeout = defaultExecTimeout
	}

	resp, err := r.ce.sbx.Process.ExecuteCommand(ctx, script,
		func(o *dtoptions.ExecuteCommand) {
			o.Timeout = &timeout
		},
	)
	if err != nil {
		if isTimeoutErr(err) {
			return &execBashResult{ExitCode: -1, TimedOut: true}, nil
		}
		return nil, err
	}
	if resp == nil {
		return &execBashResult{}, nil
	}
	return &execBashResult{ExitCode: resp.ExitCode, Result: resp.Result}, nil
}

// readFile downloads a file from the sandbox and returns its content. When the
// file exceeds maxBytes it is truncated and truncated=true is returned.
func (r *workspaceRuntime) readFile(
	ctx context.Context,
	remotePath string,
	maxBytes int64,
) (data []byte, size int64, truncated bool, err error) {
	raw, dlErr := r.ce.sbx.FileSystem.DownloadFile(ctx, remotePath, nil)
	if dlErr != nil {
		return nil, 0, false, dlErr
	}
	size = int64(len(raw))
	if maxBytes > 0 && size > maxBytes {
		return raw[:maxBytes], size, true, nil
	}
	return raw, size, false, nil
}

// listFilesByGlob resolves glob patterns inside the sandbox using bash globstar
// and returns absolute file paths. Mirrors the E2B runtime for ** support and
// path-escape defence.
func (r *workspaceRuntime) listFilesByGlob(
	ctx context.Context,
	wsPath string,
	patterns []string,
) ([]string, error) {
	if len(patterns) == 0 {
		return nil, nil
	}

	var cmd bytes.Buffer
	cmd.WriteString("cd ")
	cmd.WriteString(shellQuote(wsPath))
	cmd.WriteString(
		" && __dt_base=$(readlink -f . 2>/dev/null || realpath . 2>/dev/null || pwd); " +
			"printf '__DT_BASE__=%s\\n' \"$__dt_base\"; " +
			"shopt -s globstar nullglob dotglob; for p in",
	)
	for _, p := range patterns {
		cmd.WriteByte(' ')
		cmd.WriteString(shellQuote(filepath.ToSlash(p)))
	}
	cmd.WriteString(
		"; do for f in $p; do " +
			"if [ -f \"$f\" ]; then " +
			"__dt_rp=$(readlink -f \"$f\" 2>/dev/null || realpath \"$f\" 2>/dev/null || echo \"$(pwd)/$f\"); " +
			"case \"$__dt_rp\" in " +
			"\"$__dt_base\"/*|\"$__dt_base\") printf '%s\\n' \"$__dt_rp\";; esac; " +
			"fi; done; done",
	)

	resp, err := r.execBash(ctx, cmd.String(), defaultCollectTimeout)
	if err != nil {
		return nil, err
	}

	resolvedBase := wsPath
	var out []string
	seen := map[string]bool{}
	for line := range strings.SplitSeq(resp.Result, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if after, ok := strings.CutPrefix(line, "__DT_BASE__="); ok {
			resolvedBase = after
			continue
		}
		clean := path.Clean(line)
		if !pathUnder(clean, wsPath) && !pathUnder(clean, resolvedBase) {
			continue
		}
		if seen[clean] {
			continue
		}
		seen[clean] = true
		out = append(out, clean)
	}
	return out, nil
}

// ─── Persistent-model helpers ─────────────────────────────────────────────────

// isAlreadyStaged returns true when metadata records that the given from-spec
// is already staged at destPath with a pinned version. This is the key
// optimisation that avoids re-downloading S3 artifacts across conversation turns.
func isAlreadyStaged(md codeexecutor.WorkspaceMetadata, from, destPath string) bool {
	// s3 attachments have no version; a chat upload's bucket/key is immutable,
	// so a bucket/key match on the destination means the file is already on
	// the volume. This is what keeps a large attachment from re-downloading
	// every turn. Two subtleties:
	//   - Only the LATEST record for this destination counts. A newer record
	//     with a different source means another file has since overwritten
	//     the path, so the bytes on disk are not ours — restage.
	//   - The comparison strips the s3buf://, s3url:// scheme prefixes: the
	//     transfer mechanism is not part of the file's identity, so a size
	//     reclassification across turns must not defeat the pin.
	if canon := canonicalS3Ref(from); canon != "" {
		for _, v := range slices.Backward(md.Inputs) {
			rec := v
			if rec.To != destPath {
				continue
			}
			return canonicalS3Ref(rec.From) == canon
		}
		return false
	}
	if !strings.HasPrefix(from, inputSchemeArtifact) {
		return false
	}
	name, ver, err := codeexecutor.ParseArtifactRef(
		strings.TrimPrefix(from, inputSchemeArtifact),
	)
	if err != nil {
		return false
	}
	for _, v := range slices.Backward(md.Inputs) {
		rec := v
		if rec.To != destPath || rec.Version == nil {
			continue
		}
		if ver != nil && *ver != *rec.Version {
			continue
		}
		if rec.Resolved == name {
			return true
		}
		if after, ok := strings.CutPrefix(rec.From, inputSchemeArtifact); ok {
			rn, _, _ := codeexecutor.ParseArtifactRef(
				after,
			)
			if rn == name {
				return true
			}
		}
	}
	return false
}

func pinnedArtifactVersion(md codeexecutor.WorkspaceMetadata, name, dest string) *int {
	if name == "" || dest == "" {
		return nil
	}
	for _, v := range slices.Backward(md.Inputs) {
		rec := v
		if rec.To != dest || rec.Version == nil {
			continue
		}
		if rec.Resolved == name {
			return rec.Version
		}
		if after, ok := strings.CutPrefix(rec.From, inputSchemeArtifact); ok {
			rn, _, _ := codeexecutor.ParseArtifactRef(
				after,
			)
			if rn == name {
				return rec.Version
			}
		}
	}
	return nil
}

func inputBase(from string) string {
	s := strings.TrimSpace(from)
	if after, ok := strings.CutPrefix(s, inputSchemeArtifact); ok {
		n, _, err := codeexecutor.ParseArtifactRef(
			after,
		)
		if err == nil {
			b := path.Base(strings.TrimSpace(n))
			if b != "." && b != "/" && b != ".." && b != "" {
				return b
			}
		}
	}
	if i := strings.LastIndex(s, "/"); i >= 0 && i+1 < len(s) {
		return s[i+1:]
	}
	return s
}

// ─── Utility ─────────────────────────────────────────────────────────────────

func pathUnder(p, base string) bool {
	if base == "" || p == "" {
		return false
	}
	base = strings.TrimRight(base, "/")
	return p == base || strings.HasPrefix(p, base+"/")
}

func isTimeoutErr(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "timeout") ||
		strings.Contains(msg, "deadline exceeded") ||
		strings.Contains(msg, "context deadline")
}

func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	return "'" + strings.ReplaceAll(s, "'", "'\\''") + "'"
}
