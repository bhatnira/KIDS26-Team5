package agent

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"path"
	"strconv"
	"sync"
	"time"

	"antelope/internal/modules/agent/daytona"

	"golang.org/x/sync/errgroup"
	"trpc.group/trpc-go/trpc-agent-go/artifact"
)

const (
	// harvestTimeout bounds the whole end-of-turn harvest (hashing plus the
	// in-sandbox uploads). It runs on context.Background because the turn
	// context may already be cancelled by a client disconnect.
	harvestTimeout = 15 * time.Minute

	// harvestPresignTTL keeps each upload URL valid for the full harvest
	// window with margin.
	harvestPresignTTL = 30 * time.Minute

	// harvestMaxBytes is the single-object presigned PUT ceiling (the S3
	// single-part limit). Larger files are skipped with an error so the
	// gap is visible in logs instead of failing mid-upload.
	harvestMaxBytes = 5 << 30 // 5 GiB

	// bufferedHarvestMaxBytes caps the streaming fallback. The fallback pipes
	// the body through the API process without buffering it whole, so memory
	// is not the concern — this is a safety valve bounding how much sandbox→
	// API→storage transfer one degraded harvest will attempt. Above it we skip
	// with a visible error.
	bufferedHarvestMaxBytes = 5 << 30 // 5 GiB

	// harvestConcurrency bounds how many output files upload at once. The
	// uploads are I/O-bound (sandbox → storage, or sandbox → API → storage),
	// so a small fan-out shrinks the post-turn gap without overwhelming the
	// API process on the streaming-fallback path.
	harvestConcurrency = 4
)

// harvestStats reports per-path outcomes so the caller can log how each file
// was persisted. Direct = presigned PUT straight from the sandbox; Streamed =
// buffered fallback through the API process.
type harvestStats struct {
	Direct   int // saved via presigned PUT (sandbox → storage)
	Streamed int // saved via streaming fallback (sandbox → API → storage)
}

// Saved is the total number of files persisted this harvest.
func (h harvestStats) Saved() int { return h.Direct + h.Streamed }

// harvestOutputs persists every file under the workspace's out/ directory as
// a session artifact before the ephemeral sandbox is deleted. This is what
// makes out/ durable without the model having to call a save tool: the next
// turn's restore (sessionArtifactSpecs) stages the harvested files back at
// the same paths.
//
// Transfer path: each file is uploaded straight from the sandbox to a
// presigned PUT URL, so multi-GB outputs never pass through the API process.
// When that fails — most commonly because the sandbox cannot reach the
// storage endpoint directly (e.g. an internal MinIO host it can't resolve, or
// an untrusted TLS cert) — it falls back to STREAMING the file through the API
// process (sandbox → API → storage) via the artifact service. The fallback
// pipes the body without buffering it whole, so heap stays flat regardless of
// size; it is still capped at bufferedHarvestMaxBytes as a safety valve.
//
// Change detection: a file is skipped when the latest stored version has the
// same size and an ETag equal to the file's in-sandbox MD5 — that covers
// both "saved via workspace_save_artifact earlier this turn" and "restored
// from a previous turn and not modified", so unchanged files do not grow a
// new version every turn. We force single-part PUTs for every artifact write
// (storage.PutObject/PutObjectStream set DisableMultipart) precisely so the
// ETag is always the content MD5 — otherwise a file larger than the multipart
// threshold (~16 MiB) gets a "<hash>-<parts>" ETag that never matches the MD5,
// and would be re-uploaded as a duplicate version every turn. (A server-side-
// encrypted bucket still defeats the comparison; there the file is re-saved —
// duplicated storage, never lost data.)
//
// It returns the artifacts it persisted (for the runner to persist as one
// consolidated event) and per-path stats. Progress is surfaced live through
// sink.Emit: a single pending event up front (so the UI shows skeleton cards
// the instant the listing completes, before the slow uploads), one artifact
// event per file as it finishes (uploads run in parallel, so cards resolve as
// they land), and a settled event at the end (so the UI can drop any
// placeholder whose upload failed).
//
// Best-effort: per-file failures are collected and reported to the caller
// for logging; they never block the sandbox teardown. warn is called for each
// file whose direct presigned upload failed (before the streaming fallback)
// so the operator can observe the degraded path being taken.
func harvestOutputs(
	ctx context.Context,
	arts *PerUserS3Artifact,
	exec *daytona.CodeExecutor,
	sink *ArtifactSink,
	appName string,
	userID uint,
	sessionID string,
	warn func(rel string, directErr error),
) ([]HarvestedArtifact, harvestStats, error) {
	var stats harvestStats
	files, err := exec.ListOutputFiles(ctx)
	if err != nil {
		return nil, stats, fmt.Errorf("list output files: %w", err)
	}
	if len(files) == 0 {
		return nil, stats, nil
	}

	info := artifact.SessionInfo{
		AppName:   appName,
		UserID:    strconv.FormatUint(uint64(userID), 10),
		SessionID: sessionID,
	}
	existing, err := arts.ListSessionArtifacts(ctx, info)
	if err != nil {
		return nil, stats, fmt.Errorf("list session artifacts: %w", err)
	}
	latest := make(map[string]SessionArtifactObject, len(existing))
	for _, a := range existing {
		latest[a.Name] = a
	}

	// Decide the upload set first (cheap): drop unchanged files (dedup) and
	// oversized ones. Only this set gets a placeholder + card.
	type uploadItem struct {
		file    daytona.WorkspaceOutputFile
		version int
	}
	var (
		toUpload []uploadItem
		errs     []error
	)
	for _, f := range files {
		if f.Size > harvestMaxBytes {
			errs = append(errs, fmt.Errorf("skip %s: %d bytes exceeds the %d-byte harvest limit", f.Rel, f.Size, int64(harvestMaxBytes)))
			continue
		}
		cur, ok := latest[f.Rel]
		if ok && cur.Size == f.Size && cur.ETag == f.MD5 {
			continue // content already stored at the latest version
		}
		version := 0
		if ok {
			version = cur.Version + 1
		}
		toUpload = append(toUpload, uploadItem{file: f, version: version})
	}
	if len(toUpload) == 0 {
		return nil, stats, errors.Join(errs...)
	}

	// Announce the placeholders before the slow uploads start.
	pending := make([]PendingArtifact, len(toUpload))
	for i, u := range toUpload {
		pending[i] = PendingArtifact{Name: u.file.Rel, SizeBytes: u.file.Size}
	}
	sink.Emit(newArtifactsPendingEvent(pending))

	// Upload in parallel; emit each card as its file lands.
	var (
		mu    sync.Mutex
		saved []HarvestedArtifact
	)
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(harvestConcurrency)
	for _, u := range toUpload {
		g.Go(func() error {
			art, streamed, upErr := harvestOne(gctx, arts, exec, info, u.file, u.version, warn)
			mu.Lock()
			defer mu.Unlock()
			if upErr != nil {
				errs = append(errs, upErr)
				return nil // best-effort: one failure must not cancel the rest
			}
			saved = append(saved, art)
			if streamed {
				stats.Streamed++
			} else {
				stats.Direct++
			}
			sink.Emit(newHarvestEvent([]HarvestedArtifact{art}))
			return nil
		})
	}
	_ = g.Wait() // per-file errors are already collected; Go funcs never return non-nil

	// Tell the UI which files actually saved so it can clear failed placeholders.
	savedNames := make([]string, len(saved))
	for i, a := range saved {
		savedNames[i] = a.Name
	}
	sink.Emit(newArtifactsSettledEvent(savedNames))

	return saved, stats, errors.Join(errs...)
}

// harvestOne persists a single output file, preferring the direct
// sandbox→storage presigned PUT and falling back to a streaming upload through
// the API process when the direct path is unavailable. It returns the saved
// artifact's metadata and whether the streaming fallback was used. version is
// the slot the presigned PUT targets; the streaming fallback lets the artifact
// service allocate its own next version (which equals version, since a failed
// PUT writes nothing).
func harvestOne(
	ctx context.Context,
	arts *PerUserS3Artifact,
	exec *daytona.CodeExecutor,
	info artifact.SessionInfo,
	f daytona.WorkspaceOutputFile,
	version int,
	warn func(rel string, directErr error),
) (HarvestedArtifact, bool, error) {
	contentType := harvestContentType(ctx, exec, f.Rel)
	art := HarvestedArtifact{
		Name:      f.Rel,
		SavedAs:   f.Rel,
		Version:   version,
		MimeType:  contentType,
		SizeBytes: f.Size,
	}

	// Fast path: upload straight from the sandbox to a presigned PUT URL.
	url, directErr := arts.PresignedPutURL(ctx, info, f.Rel, version, harvestPresignTTL)
	if directErr == nil {
		directErr = exec.UploadOutputFile(ctx, f.Rel, url, contentType)
	}
	if directErr == nil {
		return art, false, nil
	}
	if warn != nil {
		warn(f.Rel, directErr)
	}

	// Direct upload failed — stream through the API process instead.
	if f.Size > bufferedHarvestMaxBytes {
		return HarvestedArtifact{}, false, fmt.Errorf(
			"upload %s: direct upload failed (%v) and the file is too large (%d bytes) to stream through the server",
			f.Rel, directErr, f.Size)
	}
	rc, openErr := exec.OpenOutputFile(ctx, f.Rel)
	if openErr != nil {
		return HarvestedArtifact{}, false, fmt.Errorf("upload %s: direct upload failed (%v); streaming open also failed: %w", f.Rel, directErr, openErr)
	}
	defer func() { _ = rc.Close() }()

	ver, saveErr := arts.SaveArtifactStream(ctx, info, f.Rel, rc, f.Size, contentType)
	if saveErr != nil {
		return HarvestedArtifact{}, false, fmt.Errorf("upload %s: streaming save failed: %w", f.Rel, saveErr)
	}
	art.Version = ver
	return art, true, nil
}

// harvestContentType resolves a file's MIME type the same way the
// workspace_save_artifact tool does — sniffing the content with
// http.DetectContentType — so harvested cards and tool-saved cards show a
// consistent type. It falls back to the filename extension, then
// application/octet-stream, if the in-sandbox sniff fails, so the type is
// never blank.
func harvestContentType(ctx context.Context, exec *daytona.CodeExecutor, rel string) string {
	if ct, err := exec.SniffOutputFile(ctx, rel); err == nil && ct != "" {
		return ct
	}
	if ct := mime.TypeByExtension(path.Ext(rel)); ct != "" {
		return ct
	}
	return "application/octet-stream"
}
