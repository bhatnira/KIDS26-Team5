package agent

import (
	"context"
	"sync"
)

// requestContextKey carries the per-request identity through the
// trpc-agent-go runner pipeline so the AgentFactory (called inside
// Runner.Run before any Invocation is attached) can look up user-scoped
// configuration. Service-layer callers inject the value with WithRequest
// immediately before invoking Run.
type requestContextKey struct{}

// requestCtx is the in-context payload. Carries both the Antelope user id
// and the session id so the factory can resolve per-user and per-session
// configuration without re-deriving them.
type requestCtx struct {
	UserID    uint
	SessionID string
}

// WithRequest stamps the user and session identity onto ctx. It is the
// agent module's contract that runner.Run is called with a ctx that
// went through this helper.
func WithRequest(ctx context.Context, userID uint, sessionID string) context.Context {
	return context.WithValue(ctx, requestContextKey{}, requestCtx{UserID: userID, SessionID: sessionID})
}

// RequestFromContext returns the (userID, sessionID, ok) tuple previously
// set with WithRequest. ok is false when either value is missing.
func RequestFromContext(ctx context.Context) (uint, string, bool) {
	v, ok := ctx.Value(requestContextKey{}).(requestCtx)
	if !ok || v.UserID == 0 || v.SessionID == "" {
		return 0, "", false
	}
	return v.UserID, v.SessionID, true
}

// WithUserID is the backwards-compatible single-value setter. Prefer
// WithRequest when the session id is also known.
func WithUserID(ctx context.Context, userID uint) context.Context {
	return WithRequest(ctx, userID, "")
}

// UserIDFromContext returns just the user id from the request context.
func UserIDFromContext(ctx context.Context) (uint, bool) {
	v, ok := ctx.Value(requestContextKey{}).(requestCtx)
	if !ok {
		return 0, false
	}
	return v.UserID, v.UserID > 0
}

// stageAttachmentsCtxKey carries the chat attachments to stage into the
// sandbox for one turn. Manager.Run stamps it before invoking the runner so
// the agent factory (buildExecutor) can stage them before the model runs.
type stageAttachmentsCtxKey struct{}

// WithStageAttachments stamps the turn's attachments onto ctx so the factory
// can stage them into the workspace. Empty/nil is a no-op for readers.
func WithStageAttachments(ctx context.Context, atts []AttachmentRef) context.Context {
	if len(atts) == 0 {
		return ctx
	}
	return context.WithValue(ctx, stageAttachmentsCtxKey{}, atts)
}

// StageAttachmentsFromContext returns the attachments to stage for this turn,
// or nil when none were set.
func StageAttachmentsFromContext(ctx context.Context) []AttachmentRef {
	v, _ := ctx.Value(stageAttachmentsCtxKey{}).([]AttachmentRef)
	return v
}

// turnCleanupCtxKey carries a TurnCleanup registry for the lifetime of one
// chat turn. Manager.Run installs an empty registry before invoking the
// runner and drains it after the event channel closes.
type turnCleanupCtxKey struct{}

// TurnCleanup collects best-effort cleanup callbacks registered by the
// agent factory during one turn. Used to release per-turn resources whose
// lifecycle is shorter than the manager's (Daytona ephemeral sandboxes
// are the motivating case — they must be deleted as soon as the agent
// stops producing events so the user is not billed for idle compute).
//
// All methods are safe for concurrent use.
type TurnCleanup struct {
	mu      sync.Mutex
	closers []func() error
	done    bool
}

// Register adds a cleanup callback. Callbacks registered after the
// registry has been drained run immediately (this only happens on
// pathological orderings; the helper keeps the contract simple).
func (t *TurnCleanup) Register(fn func() error) {
	if fn == nil {
		return
	}
	t.mu.Lock()
	if t.done {
		t.mu.Unlock()
		_ = fn()
		return
	}
	t.closers = append(t.closers, fn)
	t.mu.Unlock()
}

// Drain runs every registered callback exactly once, returning the first
// error (subsequent errors are passed to errLog if non-nil so they can
// still be observed). Idempotent.
func (t *TurnCleanup) Drain(errLog func(error)) error {
	t.mu.Lock()
	if t.done {
		t.mu.Unlock()
		return nil
	}
	t.done = true
	closers := t.closers
	t.closers = nil
	t.mu.Unlock()

	var firstErr error
	for _, c := range closers {
		if err := c(); err != nil {
			if firstErr == nil {
				firstErr = err
			} else if errLog != nil {
				errLog(err)
			}
		}
	}
	return firstErr
}

// WithTurnCleanup installs an empty TurnCleanup registry on ctx and
// returns both the new context and a pointer to the registry so the
// caller (Manager.Run) can drain it once the turn finishes.
func WithTurnCleanup(ctx context.Context) (context.Context, *TurnCleanup) {
	t := &TurnCleanup{}
	return context.WithValue(ctx, turnCleanupCtxKey{}, t), t
}

// TurnCleanupFromContext returns the registry installed by WithTurnCleanup,
// or nil when none is in scope.
func TurnCleanupFromContext(ctx context.Context) *TurnCleanup {
	t, _ := ctx.Value(turnCleanupCtxKey{}).(*TurnCleanup)
	return t
}
