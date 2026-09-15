package tools

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// CAB (Clinical Analytics Backend / nightingale) integration. These tools
// mirror the /cab/* proxy handlers (routers/api/v1/AutomapperController.go)
// but call the upstream directly from the agent process. Both endpoints are
// non-destructive: fastqQuery is a lookup, and pipeline submit kicks off an
// external run the user explicitly asked for (same spirit as submit_job).
const (
	// defaultCABBaseURL is the fallback upstream when no base URL is supplied
	// to the tool constructors. The configured value (cab.base-url /
	// ANTELOPE_CAB_BASE_URL) is threaded in via tools.Deps.CabBaseURL.
	defaultCABBaseURL = "http://nightingale-dev.stjude.org:8080"

	// cabMaxRespBytes caps the upstream response we buffer, mirroring the
	// proxy's request-body guard so a misbehaving upstream cannot OOM us.
	cabMaxRespBytes = 10 << 20 // 10 MiB
)

// cabHTTPClient has an explicit timeout so a slow upstream cannot pin a
// goroutine, matching the proxy controller's shared client.
var cabHTTPClient = &http.Client{Timeout: 30 * time.Second}

// ── query_cab_fastq ───────────────────────────────────────────────────────────

type queryCabFastqTool struct{ baseURL string }

// NewQueryCabFastqTool returns a tool that queries the CAB FASTQ lookup
// service. Read-only. An empty baseURL falls back to defaultCABBaseURL.
func NewQueryCabFastqTool(baseURL string) tool.CallableTool {
	if baseURL == "" {
		baseURL = defaultCABBaseURL
	}
	return &queryCabFastqTool{baseURL: baseURL}
}

func (t *queryCabFastqTool) Declaration() *tool.Declaration {
	return &tool.Declaration{
		Name: "query_cab_fastq",
		Description: "Query the St. Jude CAB (nightingale) service for FASTQ files matching " +
			"the given filters (e.g. sample name, sequencing identifiers). Read-only lookup.",
		InputSchema: &tool.Schema{
			Type: "object",
			Properties: map[string]*tool.Schema{
				"params": {
					Type:                 "object",
					Description:          "Query parameters forwarded verbatim as the request query string (key-value pairs).",
					AdditionalProperties: true,
				},
			},
		},
	}
}

type queryCabFastqInput struct {
	Params map[string]any `json:"params,omitempty"`
}

func (t *queryCabFastqTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	var in queryCabFastqInput
	if len(jsonArgs) > 0 {
		if err := json.Unmarshal(jsonArgs, &in); err != nil {
			return nil, fmt.Errorf("invalid args: %w", err)
		}
	}

	q := url.Values{}
	for k, v := range in.Params {
		q.Set(k, fmt.Sprintf("%v", v))
	}
	target := t.baseURL + "/cab/fastqQuery"
	if encoded := q.Encode(); encoded != "" {
		target += "?" + encoded
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	return doCABRequest(req)
}

// ── submit_cab_pipeline ───────────────────────────────────────────────────────

type submitCabPipelineTool struct{ baseURL string }

// NewSubmitCabPipelineTool returns a tool that submits a pipeline run to the
// CAB (nightingale) service. Only call after the user has confirmed. An empty
// baseURL falls back to defaultCABBaseURL.
func NewSubmitCabPipelineTool(baseURL string) tool.CallableTool {
	if baseURL == "" {
		baseURL = defaultCABBaseURL
	}
	return &submitCabPipelineTool{baseURL: baseURL}
}

func (t *submitCabPipelineTool) Declaration() *tool.Declaration {
	return &tool.Declaration{
		Name: "submit_cab_pipeline",
		Description: "Submit a pipeline run to the St. Jude CAB (nightingale) service. " +
			"Only call this AFTER the user has confirmed the submission.",
		InputSchema: &tool.Schema{
			Type:     "object",
			Required: []string{"payload"},
			Properties: map[string]*tool.Schema{
				"payload": {
					Type:                 "object",
					Description:          "Submission payload, sent verbatim as the JSON request body.",
					AdditionalProperties: true,
				},
			},
		},
	}
}

type submitCabPipelineInput struct {
	Payload json.RawMessage `json:"payload"`
}

func (t *submitCabPipelineTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	var in submitCabPipelineInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	if len(in.Payload) == 0 || string(in.Payload) == "null" {
		return nil, errors.New("payload is required")
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, t.baseURL+"/cab/pipeline", bytes.NewReader(in.Payload),
	)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	return doCABRequest(req)
}

// doCABRequest executes a prepared CAB request and decodes the response. A
// non-2xx upstream status is returned as a structured error map (not a Go
// error) so the model can read and react to it. JSON bodies are returned as
// parsed objects; anything else is returned as a raw string.
func doCABRequest(req *http.Request) (any, error) {
	resp, err := cabHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("cab request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, cabMaxRespBytes))
	if err != nil {
		return nil, fmt.Errorf("read cab response: %w", err)
	}

	var parsed any
	if json.Unmarshal(body, &parsed) != nil {
		parsed = string(body)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return map[string]any{
			"error":       fmt.Sprintf("CAB returned HTTP %d", resp.StatusCode),
			"status_code": resp.StatusCode,
			"body":        parsed,
		}, nil
	}
	return parsed, nil
}
