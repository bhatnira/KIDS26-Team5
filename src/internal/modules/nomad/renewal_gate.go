package nomad

// tickerAction is what the renewal loop must do to its renew ticker after the
// gate reconciles against the authoritative leader flag.
type tickerAction int

const (
	tickerNoop  tickerAction = iota // already in the right state
	tickerStart                     // (re)start the renew ticker
	tickerStop                      // stop the renew ticker
)

// renewalGate decides whether the leader-lease renewal ticker should be running
// given the authoritative isLeader flag. It is pure and side-effect free: it
// holds no timer, no Redis client, and no goroutine, so the follower<->leader
// edge behavior can be unit-tested without either. runRenewalLoop owns the real
// ticker and applies the actions the gate returns.
//
// The gate exists to make the ticker only (re)start on the follower->leader
// edge: a leader that receives many redundant "still leader" notifications must
// not Reset the ticker each time, or the next renewal could be pushed past the
// lease TTL and leadership would be lost while still leader.
type renewalGate struct {
	renewing bool
}

// reconcile aligns the gate with the current leader flag and returns the ticker
// action the caller must apply. It returns tickerNoop when the ticker is
// already in the correct state (e.g. a redundant leader notification).
func (g *renewalGate) reconcile(isLeader bool) tickerAction {
	switch {
	case isLeader && !g.renewing:
		g.renewing = true
		return tickerStart
	case !isLeader && g.renewing:
		g.renewing = false
		return tickerStop
	default:
		return tickerNoop
	}
}

// onRenewFailed records that renewal has stopped after a failed renew (or a tick
// observed while no longer leader). The ticker is stopped by the caller; the
// gate stays "not renewing" until the next follower->leader edge restarts it.
func (g *renewalGate) onRenewFailed() {
	g.renewing = false
}
