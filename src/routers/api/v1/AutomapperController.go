package v1

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

	"antelope/internal/modules/log"
	"antelope/pkg/response"
)

const (
	// defaultCABBaseURL is the fallback upstream used when cab.base-url /
	// ANTELOPE_CAB_BASE_URL is unset. Override at startup via SetCABBaseURL.
	defaultCABBaseURL = "http://nightingale-dev.stjude.org:8080"

	// ARCH-4: cap proxied request bodies to prevent OOM from malicious clients.
	maxProxyBodyBytes = 10 << 20 // 10 MiB
)

// cabBaseURL is the upstream CAB (nightingale) base URL. It is a package var so
// it can be configured once at startup (SetCABBaseURL) rather than baked into
// the image; the handlers read it at request time.
var cabBaseURL = defaultCABBaseURL

// SetCABBaseURL overrides the upstream CAB base URL. Called once during router
// wiring from config; an empty url leaves the current value untouched.
func SetCABBaseURL(url string) {
	if url != "" {
		cabBaseURL = url
	}
}

// ARCH-4: shared HTTP client with an explicit timeout so a slow upstream cannot
// hold a goroutine indefinitely.
var proxyHTTPClient = &http.Client{Timeout: 30 * time.Second}

// CORS header names to skip when copying proxied response (middleware sets these)
var proxySkipCORSHeaders = map[string]bool{
	"Access-Control-Allow-Origin":      true,
	"Access-Control-Allow-Methods":     true,
	"Access-Control-Allow-Headers":     true,
	"Access-Control-Expose-Headers":    true,
	"Access-Control-Max-Age":           true,
	"Access-Control-Allow-Credentials": true,
}

// ProxyFastqQuery proxies the FASTQ query request to nightingale.stjude.org
// @Summary Proxy FASTQ query to CAB
// @Tags proxy
// @Produce json
// @Security BearerAuth
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /cab/fastqQuery [get]
func ProxyFastqQuery(c *gin.Context) {
	// Build the target URL with query parameters
	targetURL, err := url.Parse(cabBaseURL + "/cab/fastqQuery")
	if err != nil {
		log.L().Error("failed to parse target URL", zap.Error(err))
		response.ServerError(c, nil, response.SystemError)
		return
	}

	// Copy query parameters
	targetURL.RawQuery = c.Request.URL.RawQuery

	// Create proxy request. SEC: bind to the client's request context so a
	// client disconnect cancels the upstream call instead of pinning it.
	proxyReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodGet, targetURL.String(), nil)
	if err != nil {
		log.L().Error("failed to create proxy request", zap.Error(err))
		response.ServerError(c, nil, response.SystemError)
		return
	}

	// Copy headers from original request
	copyHeaders(c.Request.Header, proxyReq.Header)

	// Execute the request using the shared client with timeout.
	resp, err := proxyHTTPClient.Do(proxyReq)
	if err != nil {
		log.L().Error("failed to execute proxy request", zap.Error(err))
		response.ServerError(c, nil, response.SystemError)
		return
	}
	defer resp.Body.Close()

	// Copy response headers (skip CORS so only our middleware's CORS headers are sent)
	copyProxiedResponseHeaders(c.Writer, resp.Header)

	// Set status code and copy body
	c.Status(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}

// ProxyPipelineSubmit proxies the pipeline submission request to nightingale.stjude.org
// @Summary Proxy pipeline submit to CAB
// @Tags proxy
// @Accept json
// @Produce json
// @Security BearerAuth
// @Param body body map[string]interface{} true "Pipeline submit payload"
// @Success 200 {object} map[string]interface{}
// @Failure 400 {object} map[string]interface{}
// @Failure 401 {object} map[string]interface{}
// @Router /cab/pipeline [post]
func ProxyPipelineSubmit(c *gin.Context) {
	// ARCH-4: limit request body size to prevent OOM from large payloads.
	body, err := io.ReadAll(io.LimitReader(c.Request.Body, maxProxyBodyBytes))
	if err != nil {
		log.L().Error("failed to read request body", zap.Error(err))
		response.ServerError(c, nil, response.SystemError)
		return
	}

	// Build the target URL
	targetURL := cabBaseURL + "/cab/pipeline"

	// Create proxy request. SEC: bind to the client's request context so a
	// client disconnect cancels the upstream call instead of pinning it.
	proxyReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, targetURL, strings.NewReader(string(body)))
	if err != nil {
		log.L().Error("failed to create proxy request", zap.Error(err))
		response.ServerError(c, nil, response.SystemError)
		return
	}

	// Copy headers from original request
	copyHeaders(c.Request.Header, proxyReq.Header)
	proxyReq.Header.Set("Content-Type", "application/json")

	// Execute the request using the shared client with timeout.
	resp, err := proxyHTTPClient.Do(proxyReq)
	if err != nil {
		log.L().Error("failed to execute proxy request", zap.Error(err))
		response.ServerError(c, nil, response.SystemError)
		return
	}
	defer resp.Body.Close()

	// Copy response headers (skip CORS so only our middleware's CORS headers are sent)
	copyProxiedResponseHeaders(c.Writer, resp.Header)

	// Set status code and copy body
	c.Status(resp.StatusCode)
	io.Copy(c.Writer, resp.Body)
}

// copyProxiedResponseHeaders copies response headers from the upstream response,
// skipping CORS headers so only the Gin CORS middleware's values are sent.
func copyProxiedResponseHeaders(dst http.ResponseWriter, src http.Header) {
	for key, values := range src {
		if proxySkipCORSHeaders[http.CanonicalHeaderKey(key)] {
			continue
		}
		for _, value := range values {
			dst.Header().Add(key, value)
		}
	}
}

// copyHeaders copies headers from source to destination, excluding hop-by-hop
// headers and sensitive credentials.
//
// SEC: Authorization and Cookie carry the caller's Antelope access JWT / session
// and must NEVER be forwarded to the third-party CAB upstream — doing so leaks a
// live, full-access credential into another system's logs and request path. If
// the upstream ever needs authentication, inject a dedicated upstream credential
// here instead of the user's token.
func copyHeaders(src, dst http.Header) {
	skipHeaders := map[string]bool{
		// hop-by-hop headers (RFC 7230 §6.1)
		"Connection":          true,
		"Keep-Alive":          true,
		"Proxy-Authenticate":  true,
		"Proxy-Authorization": true,
		"Te":                  true,
		"Trailers":            true,
		"Transfer-Encoding":   true,
		"Upgrade":             true,
		// sensitive credentials — never forward to a third-party upstream
		"Authorization": true,
		"Cookie":        true,
	}

	for key, values := range src {
		if skipHeaders[http.CanonicalHeaderKey(key)] {
			continue
		}
		for _, value := range values {
			dst.Add(key, value)
		}
	}
}
