package daytona

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"path"
	"strconv"
	"strings"
	"time"
)

// outputListTimeout bounds the in-sandbox enumeration + MD5 pass over out/.
// Hashing is disk-bound; multi-GB outputs need more headroom than the
// generic stage timeout.
const outputListTimeout = 10 * time.Minute

// WorkspaceOutputFile describes one regular file under the workspace's out/
// directory. MD5 is computed in-sandbox so callers can compare it against
// object-store ETags for change detection without transferring any bytes.
type WorkspaceOutputFile struct {
	Rel  string // workspace-relative path, e.g. "out/result.png"
	Size int64
	MD5  string // lower-case hex content MD5
}

// ListOutputFiles enumerates the regular files under out/ with their size and
// content MD5. Returns nil when no sandbox was provisioned this turn — it
// never creates one, so a text-only turn stays sandbox-free through harvest.
func (c *CodeExecutor) ListOutputFiles(ctx context.Context) ([]WorkspaceOutputFile, error) {
	c.mu.Lock()
	sbx := c.sbx
	c.mu.Unlock()
	if sbx == nil {
		return nil, nil
	}

	// One pass per file: "<size> <md5>  <path>". `wc -c <` keeps the output
	// a bare number on GNU and busybox alike; md5sum appends "hash  path".
	script := fmt.Sprintf(
		"cd %s 2>/dev/null || exit 0; [ -d out ] || exit 0; "+
			"find out -type f -exec sh -c "+
			"'for f; do printf \"%%s %%s\\n\" \"$(wc -c < \"$f\")\" \"$(md5sum \"$f\")\"; done' _ {} +",
		shellQuote(c.workspacePath),
	)
	res, err := c.ensureRuntime().execBash(ctx, script, outputListTimeout)
	if err != nil {
		return nil, fmt.Errorf("daytona: list output files: %w", err)
	}
	if res.ExitCode != 0 {
		return nil, fmt.Errorf("daytona: list output files failed (exit %d)", res.ExitCode)
	}

	var out []WorkspaceOutputFile
	for line := range strings.SplitSeq(res.Result, "\n") {
		f, ok := parseOutputFileLine(line)
		if !ok {
			continue
		}
		out = append(out, f)
	}
	return out, nil
}

// parseOutputFileLine decodes one "<size> <32-hex-md5><sep><path>" line where
// sep is md5sum's two-character separator ("  " or " *"). Positional parsing
// keeps paths containing spaces intact.
func parseOutputFileLine(line string) (WorkspaceOutputFile, bool) {
	line = strings.TrimRight(line, "\r")
	sp := strings.IndexByte(line, ' ')
	if sp <= 0 {
		return WorkspaceOutputFile{}, false
	}
	size, err := strconv.ParseInt(line[:sp], 10, 64)
	if err != nil || size < 0 {
		return WorkspaceOutputFile{}, false
	}
	rest := line[sp+1:]
	if len(rest) < 32+2+1 {
		return WorkspaceOutputFile{}, false
	}
	md5 := strings.ToLower(rest[:32])
	for _, r := range md5 {
		if (r < '0' || r > '9') && (r < 'a' || r > 'f') {
			return WorkspaceOutputFile{}, false
		}
	}
	rel := path.Clean(rest[32+2:])
	if rel != "out" && !strings.HasPrefix(rel, "out/") {
		return WorkspaceOutputFile{}, false
	}
	return WorkspaceOutputFile{Rel: rel, Size: size, MD5: md5}, true
}

// SniffOutputFile returns the content type of one out/ file by reading its
// first bytes and running http.DetectContentType — the SAME detection the
// framework's workspace_save_artifact tool uses — so harvested-file cards
// show the identical MIME type (e.g. application/octet-stream for an .h5ad,
// text/plain for a CSV) instead of an extension-only guess that is often
// blank. Only the leading 512 bytes are downloaded; the stream is closed
// immediately after.
func (c *CodeExecutor) SniffOutputFile(ctx context.Context, rel string) (string, error) {
	rc, err := c.OpenOutputFile(ctx, rel)
	if err != nil {
		return "", err
	}
	defer func() { _ = rc.Close() }()

	buf := make([]byte, 512)
	n, err := io.ReadFull(rc, buf)
	switch err {
	case nil, io.EOF, io.ErrUnexpectedEOF:
		return http.DetectContentType(buf[:n]), nil
	default:
		return "", err
	}
}

// UploadOutputFile streams one workspace-relative file from inside the
// sandbox to a presigned PUT URL (curl, then wget fallback), so multi-GB
// outputs never pass through the API process. contentType is optional.
func (c *CodeExecutor) UploadOutputFile(ctx context.Context, rel, url, contentType string) error {
	c.mu.Lock()
	sbx := c.sbx
	c.mu.Unlock()
	if sbx == nil {
		return fmt.Errorf("daytona: upload %s: sandbox not initialized", rel)
	}
	clean := path.Clean(strings.TrimSpace(rel))
	if clean != rel || !strings.HasPrefix(clean, "out/") {
		return fmt.Errorf("daytona: upload refused for non-output path %q", rel)
	}

	curlHeader, wgetHeader := "", ""
	if ct := strings.TrimSpace(contentType); ct != "" {
		curlHeader = "-H " + shellQuote("Content-Type: "+ct) + " "
		wgetHeader = "--header=" + shellQuote("Content-Type: "+ct) + " "
	}
	curlInsecure, wgetInsecure := "", ""
	if c.insecureTLS {
		curlInsecure = "-k "
		wgetInsecure = "--no-check-certificate "
	}
	// --connect-timeout bounds name resolution + connect so an unreachable
	// storage endpoint (e.g. an internal MinIO host the sandbox can't
	// resolve) fails fast and the caller can fall back to a buffered upload
	// instead of stalling the whole harvest.
	script := fmt.Sprintf(
		"set -e; cd %s; "+
			"if command -v curl >/dev/null 2>&1; then curl -fsS %s--connect-timeout 15 -X PUT %s--upload-file %s %s; "+
			"elif command -v wget >/dev/null 2>&1; then wget -q %s--connect-timeout=15 --method=PUT %s--body-file=%s -O /dev/null %s; "+
			"else echo 'no curl or wget in sandbox' >&2; exit 127; fi",
		shellQuote(c.workspacePath),
		curlInsecure, curlHeader, shellQuote(clean), shellQuote(url),
		wgetInsecure, wgetHeader, shellQuote(clean), shellQuote(url),
	)
	res, err := c.ensureRuntime().execBash(ctx, script, s3StageTimeout)
	if err != nil {
		return fmt.Errorf("daytona: upload %s: %w", rel, err)
	}
	if res.ExitCode != 0 {
		return fmt.Errorf("daytona: upload %s failed (exit %d)", rel, res.ExitCode)
	}
	return nil
}

// OpenOutputFile opens one workspace-relative out/ file in the sandbox for
// streaming download. It is the buffered fallback for environments where the
// sandbox cannot reach the storage endpoint directly (so UploadOutputFile
// fails): the API process pipes the body straight to storage, so heap usage
// stays flat regardless of file size. The caller must Close the reader.
func (c *CodeExecutor) OpenOutputFile(ctx context.Context, rel string) (io.ReadCloser, error) {
	c.mu.Lock()
	sbx := c.sbx
	c.mu.Unlock()
	if sbx == nil {
		return nil, fmt.Errorf("daytona: open %s: sandbox not initialized", rel)
	}
	clean := path.Clean(strings.TrimSpace(rel))
	if clean != rel || !strings.HasPrefix(clean, "out/") {
		return nil, fmt.Errorf("daytona: open refused for non-output path %q", rel)
	}
	abs := path.Join(c.workspacePath, clean)
	rc, err := sbx.FileSystem.DownloadFileStream(ctx, abs)
	if err != nil {
		return nil, fmt.Errorf("daytona: open %s: %w", rel, err)
	}
	return rc, nil
}
