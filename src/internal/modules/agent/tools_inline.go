package agent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	frameworkagent "trpc.group/trpc-go/trpc-agent-go/agent"
	"trpc.group/trpc-go/trpc-agent-go/artifact"
	"trpc.group/trpc-go/trpc-agent-go/codeexecutor"
	"trpc.group/trpc-go/trpc-agent-go/tool"
)

// pythonInlineTool runs ad-hoc Python in the sandbox's Jupyter kernel and
// captures any inline display output (matplotlib figures, HTML, etc.) as
// artifacts. Ported from antelope-agent's pythonInlineTool.
//
// This complements workspace_exec: workspace_exec is the file-based skill
// path (run.py reads/writes files under out/), while run_python_inline is
// for snippets where the value of the cell is the plot or object itself,
// not files on disk. The python kernel does not share a filesystem with
// the skill workspace, so this tool is only useful for self-contained code.
//
// The tool is constructed per request inside Factory.Build with the user's
// own executor, then registered on the LLMAgent for that one turn.
type pythonInlineTool struct {
	exec codeexecutor.CodeExecutor
}

// NewPythonInlineTool returns the run_python_inline tool wired to the
// given executor. The executor is normally the per-user Daytona sandbox.
func NewPythonInlineTool(exec codeexecutor.CodeExecutor) tool.CallableTool {
	return &pythonInlineTool{exec: exec}
}

func (t *pythonInlineTool) Declaration() *tool.Declaration {
	return &tool.Declaration{
		Name: "run_python_inline",
		Description: "Execute a self-contained Python snippet in the sandbox's " +
			"Jupyter python kernel and return stdout plus any inline display " +
			"outputs (matplotlib figures, HTML, etc.) as artifacts. Use this " +
			"for ad-hoc exploration or plotting WHERE NO WORKSPACE FILES ARE " +
			"READ OR WRITTEN — the python kernel does not share the skill " +
			"workspace's filesystem. For file-based processing (reading an " +
			".h5ad, saving outputs under out/, etc.) use a skill via " +
			"workspace_exec instead.",
		InputSchema: &tool.Schema{
			Type:     "object",
			Required: []string{"code"},
			Properties: map[string]*tool.Schema{
				"code": {
					Type:        "string",
					Description: "Python source to execute. End with plt.show() / a display() call to capture figures.",
				},
			},
		},
		OutputSchema: &tool.Schema{
			Type: "object",
			Properties: map[string]*tool.Schema{
				"stdout": {Type: "string"},
				"artifacts": {
					Type: "array",
					Items: &tool.Schema{
						Type: "object",
						Properties: map[string]*tool.Schema{
							"name":       {Type: "string"},
							"mime_type":  {Type: "string"},
							"ref":        {Type: "string"},
							"size_bytes": {Type: "integer"},
						},
					},
				},
			},
		},
	}
}

type pythonInlineInput struct {
	Code string `json:"code"`
}

type pythonInlineArtifact struct {
	Name      string `json:"name"`
	MimeType  string `json:"mime_type,omitempty"`
	Ref       string `json:"ref,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
}

type pythonInlineResult struct {
	Stdout    string                 `json:"stdout"`
	Artifacts []pythonInlineArtifact `json:"artifacts,omitempty"`
}

func (t *pythonInlineTool) Call(ctx context.Context, jsonArgs []byte) (any, error) {
	var in pythonInlineInput
	if err := json.Unmarshal(jsonArgs, &in); err != nil {
		return nil, fmt.Errorf("invalid args: %w", err)
	}
	code := strings.TrimSpace(in.Code)
	if code == "" {
		return nil, errors.New("code is required")
	}
	if t.exec == nil {
		return nil, errors.New("no code executor configured")
	}

	res, err := t.exec.ExecuteCode(ctx, codeexecutor.CodeExecutionInput{
		CodeBlocks: []codeexecutor.CodeBlock{
			{Language: "python", Code: code},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("execute: %w", err)
	}

	out := &pythonInlineResult{Stdout: res.Output}
	if len(res.OutputFiles) == 0 {
		return out, nil
	}

	inv, ok := frameworkagent.InvocationFromContext(ctx)
	if !ok || inv == nil || inv.ArtifactService == nil ||
		inv.Session == nil || inv.Session.ID == "" {
		// No artifact service available — return file metadata only.
		for _, f := range res.OutputFiles {
			out.Artifacts = append(out.Artifacts, pythonInlineArtifact{
				Name:      f.Name,
				MimeType:  f.MIMEType,
				SizeBytes: f.SizeBytes,
			})
		}
		return out, nil
	}

	for _, f := range res.OutputFiles {
		key := path.Join("inline", fmt.Sprintf("%d_%s", time.Now().UnixNano(), f.Name))
		ver, err := inv.ArtifactService.SaveArtifact(ctx, artifact.SessionInfo{
			AppName:   inv.Session.AppName,
			UserID:    inv.Session.UserID,
			SessionID: inv.Session.ID,
		}, key, &artifact.Artifact{
			Data:     []byte(f.Content),
			MimeType: f.MIMEType,
			Name:     f.Name,
		})
		if err != nil {
			return nil, fmt.Errorf("save artifact %s: %w", f.Name, err)
		}
		out.Artifacts = append(out.Artifacts, pythonInlineArtifact{
			Name:      f.Name,
			MimeType:  f.MIMEType,
			Ref:       fmt.Sprintf("artifact://%s@%d", key, ver),
			SizeBytes: f.SizeBytes,
		})
	}
	return out, nil
}
