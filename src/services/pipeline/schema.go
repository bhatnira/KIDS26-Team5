package pipeline

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"regexp"
	"strings"
	"time"

	"antelope/internal/modules/log"
	"antelope/pkg/apperr"
	"antelope/pkg/response"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
)

const (
	schemaFileName = "nextflow_schema.json"
	// Tags and commit SHAs are immutable on GitHub, so their schema can be
	// cached for a long time. Branches are mutable, so they get a short TTL to
	// bound how stale a freshly-pushed schema can be.
	schemaTTLImmutable = 24 * time.Hour
	schemaTTLMutable   = 5 * time.Minute
	schemaMaxBytes     = 5 << 20 // 5 MiB
	schemaFetchTries   = 3
	// schemaOpTimeout is the hard cap on the entire GetSchema operation,
	// covering every candidate URL, retry and backoff combined.
	schemaOpTimeout = 20 * time.Second
)

// schemaHTTPClient has a per-request timeout as a secondary guard; the overall
// operation is bounded by the request context (schemaOpTimeout).
//
// It forces IPv4 ("tcp4") when dialing: GitHub's CDN is dual-stack, and on hosts
// with broken IPv6 routing the dialer can commit to a dead IPv6 path and fail
// with "write: socket is not connected". GitHub raw content is always reachable
// over IPv4, so pinning to it avoids that failure mode entirely.
var schemaHTTPClient = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			if network == "tcp" {
				network = "tcp4"
			}
			d := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
			return d.DialContext(ctx, network, addr)
		},
		ForceAttemptHTTP2:     true,
		MaxIdleConns:          100,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
	},
}

var (
	// ownerRepoRe restricts the GitHub owner/repo segments we interpolate into
	// the raw URL, preventing path traversal / request smuggling.
	ownerRepoRe = regexp.MustCompile(`^[A-Za-z0-9._-]+$`)
	// schemaVersionRe rejects versions containing path separators or control
	// characters (mirrors the rule in internal/modules/git).
	schemaVersionRe = regexp.MustCompile(`^[^\x00-\x1f /\\'"]+$`)
	// schemaCommitHashRe detects a full/short commit SHA, which raw.githubusercontent
	// serves directly as a ref (without the refs/tags|heads prefix).
	schemaCommitHashRe = regexp.MustCompile(`^[0-9a-fA-F]{7,40}$`)
)

// GetSchema returns the nextflow_schema.json for a pipeline, fetched from GitHub
// server-side and cached in Redis. Proxying through the backend avoids the
// browser-side rate limiting, CORS variance and lack of retry that made the
// direct raw.githubusercontent.com fetch fail intermittently.
func (s *pipelineService) GetSchema(repository, version string) (gin.H, error) {
	owner, repo, err := parseGitHubRepo(repository)
	if err != nil {
		return nil, apperr.CheckFail(response.CheckFailCode, response.SchemaInvalidRepo)
	}
	if version == "" || !schemaVersionRe.MatchString(version) {
		return nil, apperr.CheckFail(response.CheckFailCode, response.SchemaInvalidVersion)
	}

	ctx, cancel := context.WithTimeout(context.Background(), schemaOpTimeout)
	defer cancel()
	cacheKey := fmt.Sprintf("pipeline:schema:%s:%s:%s", owner, repo, version)

	if s.redis != nil {
		if cached, err := s.redis.Get(ctx, cacheKey).Result(); err == nil && cached != "" {
			var schema map[string]any
			if jsonErr := json.Unmarshal([]byte(cached), &schema); jsonErr == nil {
				return gin.H{"schema": schema}, nil
			}
			// corrupt cache entry: fall through and re-fetch
		}
	}

	raw, immutable, err := fetchSchemaRaw(ctx, owner, repo, version)
	if err != nil {
		return nil, err
	}

	var schema map[string]any
	if err := json.Unmarshal(raw, &schema); err != nil {
		log.L().Error("pipeline schema is not valid json",
			zap.String("owner", owner), zap.String("repo", repo),
			zap.String("version", version), zap.Error(err))
		return nil, apperr.ServerError(response.SchemaFetchError)
	}

	if s.redis != nil {
		ttl := schemaTTLMutable
		if immutable {
			ttl = schemaTTLImmutable
		}
		if err := s.redis.Set(ctx, cacheKey, string(raw), ttl).Err(); err != nil {
			log.L().Warn("cache pipeline schema failed", zap.String("key", cacheKey), zap.Error(err))
		}
	}

	return gin.H{"schema": schema}, nil
}

// fetchSchemaRaw tries each candidate ref until one returns the file. A 404 on a
// candidate is expected (the ref might be a tag vs. branch); only when every
// candidate is a definitive 404 do we report SchemaNotFound. Transient failures
// (network errors, 429, 5xx) are retried inside tryFetch. The returned bool
// reports whether the matched ref is immutable (tag/SHA), which drives the cache
// TTL chosen by the caller.
func fetchSchemaRaw(ctx context.Context, owner, repo, version string) (body []byte, immutable bool, err error) {
	candidates := schemaURLCandidates(owner, repo, version)

	var lastErr error
	notFound := 0
	for _, cand := range candidates {
		b, status, err := tryFetch(ctx, cand.url)
		if err != nil {
			lastErr = err
			continue
		}
		switch status {
		case http.StatusOK:
			return b, cand.immutable, nil
		case http.StatusNotFound:
			notFound++
		default:
			lastErr = fmt.Errorf("unexpected status %d from %s", status, cand.url)
		}
	}

	if notFound == len(candidates) {
		return nil, false, apperr.NotFound(response.SchemaNotFound)
	}
	log.L().Error("fetch pipeline schema from github failed",
		zap.String("owner", owner), zap.String("repo", repo),
		zap.String("version", version), zap.Error(lastErr))
	return nil, false, apperr.ServerError(response.SchemaFetchError)
}

// tryFetch performs up to schemaFetchTries attempts against a single URL,
// retrying transient failures with exponential backoff. A 404 (or other 4xx)
// returns immediately without retrying.
func tryFetch(ctx context.Context, rawURL string) ([]byte, int, error) {
	var lastErr error
	backoff := 200 * time.Millisecond
	for attempt := range schemaFetchTries {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-time.After(backoff):
			}
			backoff *= 2
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
		if err != nil {
			return nil, 0, err
		}

		resp, err := schemaHTTPClient.Do(req)
		if err != nil {
			lastErr = err // network error → retry
			continue
		}

		if resp.StatusCode == http.StatusOK {
			body, readErr := io.ReadAll(io.LimitReader(resp.Body, schemaMaxBytes))
			resp.Body.Close()
			if readErr != nil {
				lastErr = readErr
				continue
			}
			return body, http.StatusOK, nil
		}

		status := resp.StatusCode
		resp.Body.Close()

		if status == http.StatusTooManyRequests || status >= 500 {
			lastErr = fmt.Errorf("transient status %d", status)
			continue // rate limited / server error → retry
		}
		return nil, status, nil // 404 and other 4xx are definitive
	}
	return nil, 0, lastErr
}

// schemaCandidate is a raw URL to try plus whether the ref it points at is
// immutable (tag/commit SHA) versus mutable (branch).
type schemaCandidate struct {
	url       string
	immutable bool
}

// schemaURLCandidates builds the raw.githubusercontent.com URLs to try, in
// priority order: commit SHA (if applicable), tag, v-prefixed tag, branch.
func schemaURLCandidates(owner, repo, version string) []schemaCandidate {
	const base = "https://raw.githubusercontent.com"
	build := func(ref string, immutable bool) schemaCandidate {
		return schemaCandidate{
			url:       fmt.Sprintf("%s/%s/%s/%s/%s", base, owner, repo, ref, schemaFileName),
			immutable: immutable,
		}
	}

	candidates := make([]schemaCandidate, 0, 4)
	if schemaCommitHashRe.MatchString(version) {
		candidates = append(candidates, build(version, true))
	}
	candidates = append(candidates,
		build("refs/tags/"+version, true),
		build("refs/tags/v"+version, true),
		build("refs/heads/"+version, false),
	)
	return candidates
}

// parseGitHubRepo extracts and validates the owner/repo from a GitHub URL.
// Only github.com is supported (matching the previous frontend behaviour).
func parseGitHubRepo(repository string) (owner, repo string, err error) {
	r := strings.TrimSpace(repository)
	r = strings.TrimSuffix(r, ".git")
	for _, prefix := range []string{"https://github.com/", "http://github.com/", "git@github.com:"} {
		r = strings.TrimPrefix(r, prefix)
	}

	parts := strings.SplitN(r, "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid github repository url: %q", repository)
	}
	owner, repo = parts[0], parts[1]
	if !ownerRepoRe.MatchString(owner) || !ownerRepoRe.MatchString(repo) {
		return "", "", fmt.Errorf("invalid github owner/repo: %q/%q", owner, repo)
	}
	return owner, repo, nil
}
