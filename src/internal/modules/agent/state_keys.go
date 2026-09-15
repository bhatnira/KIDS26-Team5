package agent

// Session state keys persisted on the framework session.State map. Read +
// written by the chat service via session.Service. Stored as []byte (per
// the framework's StateMap contract).
const (
	// SessionStateTitle holds the conversation's display title.
	SessionStateTitle = "title"

	// SessionStateStagedAttachments holds the JSON-encoded []AttachmentRef of
	// every attachment staged in earlier turns, with the StagedPath each one
	// was advertised at. The chat service re-stages them into the fresh
	// per-turn sandbox so paths promised by old message manifests stay valid.
	SessionStateStagedAttachments = "staged_attachments"
)
