// Package daytona provides a CodeExecutor backed by a persistent Daytona
// sandbox, implementing the "persistent workspace model".
//
// # Dependency
//
// This package imports the Daytona Go SDK. Add it to your module:
//
//	go get github.com/daytonaio/daytona/libs/sdk-go@v0.179.0
//
// # Persistent workspace model
//
// Unlike E2B or the container executor (which create a fresh directory per
// invocation), this executor:
//
//   - Keeps ONE sandbox per user/session (identified by WithSandboxName).
//   - CreateWorkspace is idempotent: it runs "mkdir -p" on the stable layout
//     and returns the same physical path every time.
//   - Cleanup only prunes stale runs/ entries; it never deletes work/ or out/.
//   - StageInputs skips re-downloading artifacts that are already present
//     (detected via workspace metadata) when InputSpec.Pin is true.
//
// # Typical data-analysis agent setup
//
//	// Create or reconnect to a named sandbox.
//	exec, err := daytona.New(
//	    daytona.WithSandboxName("analyst-user123-session456"),
//	    daytona.WithAutoStopInterval(30), // archive after 30 min idle
//	)
//
//	// Wrap with a one-time init hook (runs once per workspace, skipped on repeat).
//	exec, err = codeexecutor.NewWorkspaceInitExecutor(exec,
//	    codeexecutor.NewWorkspaceInitHook(codeexecutor.WorkspaceInitSpec{
//	        Commands: []codeexecutor.WorkspaceInitCommand{{
//	            Key: "pip-install-data",
//	            Cmd: "pip", Args: []string{"install", "-q", "pandas", "matplotlib", "seaborn"},
//	        }},
//	    }),
//	)
//
//	// Wire into an llmagent.
//	agent := llmagent.New(
//	    llmagent.WithCodeExecutor(exec),
//	    llmagent.WithArtifactService(s3svc),
//	    // workspace_exec + workspace_save_artifact tools are auto-registered.
//	)
//
// # Artifact ↔ workspace data flow
//
//	User uploads CSV → artifact.Service.SaveArtifact → S3
//	    artifact://data.csv@0
//
//	LLM calls workspace_exec → reconcileWorkspace:
//	    StageInputs([{From: "artifact://data.csv@0", Pin: true}])
//	        → already staged? skip (persistent model opt.)
//	        → not staged? LoadArtifactHelper → DownloadFile from S3 → UploadFile to Daytona
//	    RunProgram("python3 analyze.py") → ExecuteCommand in sandbox
//
//	LLM calls workspace_save_artifact("out/result.png"):
//	    CollectOutputs → DownloadFile from Daytona → SaveArtifactHelper → S3
//	    → artifact://out/result.png@0
package daytona
