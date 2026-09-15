package agent

import (
	"trpc.group/trpc-go/trpc-agent-go/artifact"
	"trpc.group/trpc-go/trpc-agent-go/event"
)

// AppName is the application identifier passed to the framework's session
// and artifact services. All sessions for all users are namespaced under
// this constant in the framework-owned tables.
const AppName = "antelope"

// AttachmentRef references a file the user uploaded into their workspace
// bucket. Every attachment is staged into the sandbox workspace at its
// StagedInputPaths entry before the agent runs; small text-like files are
// additionally fetched and inlined into the user message (see
// services/agent.buildUserMessage).
//
// Size is client-reported on the wire and must not be trusted for memory
// decisions until the chat service has verified it against the real object
// (StatObject) — see normalizeAttachments in services/agent.
type AttachmentRef struct {
	Bucket   string `json:"bucket"`
	Key      string `json:"key"`
	Name     string `json:"name"`
	MimeType string `json:"mime_type,omitempty"`
	Size     int64  `json:"size,omitempty"`

	// StagedPath is the workspace-relative path the file is staged at. The
	// chat service pins it (from StagedInputPaths) before building the user
	// message, so the staged location and the message manifest can never
	// drift — and so attachments restored from earlier turns (see
	// SessionStateStagedAttachments) keep the path their manifest advertised.
	// Empty means "compute from StagedInputPaths".
	StagedPath string `json:"staged_path,omitempty"`

	// Data holds the object bytes when the chat service already fetched
	// them for inlining; staging reuses them instead of re-downloading.
	// Never serialized.
	Data []byte `json:"-"`

	// Unavailable marks an attachment whose object could not be found or
	// stat'ed in storage; it is excluded from inlining and staging and
	// flagged in the message manifest. Never serialized.
	Unavailable bool `json:"-"`
}

// ArtifactRef is the wire-shape used by the chat HTTP layer when listing
// artifacts produced during a session. It is derived from the framework
// artifact.Service ListArtifactKeys + LoadArtifact pair.
type ArtifactRef struct {
	Name        string `json:"name"`
	Version     int    `json:"version"`
	MimeType    string `json:"mime_type,omitempty"`
	SizeBytes   int64  `json:"size_bytes,omitempty"`
	DownloadURL string `json:"download_url,omitempty"`
}

// frameworkArtifact is a type-alias used by callers of this package to
// avoid leaking the upstream import. Step 2's artifact_s3.go implementation
// returns *artifact.Artifact from its Save/Load entry points.
type frameworkArtifact = artifact.Artifact

// frameworkEvent is a type-alias for runner channel events. Step 4's SSE
// translator consumes a <-chan *event.Event from runner.Runner.Run.
type frameworkEvent = event.Event

// Compile-time references so go.mod tidy keeps trpc-agent-go in require.
// These are removed once factory.go / runner.go / artifact_s3.go land in
// step 2 and use the imports for real.
var (
	_ *frameworkArtifact
	_ *frameworkEvent
)
