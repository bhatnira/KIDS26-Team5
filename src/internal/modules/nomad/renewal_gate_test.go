package nomad

import "testing"

func actionName(a tickerAction) string {
	switch a {
	case tickerStart:
		return "start"
	case tickerStop:
		return "stop"
	default:
		return "noop"
	}
}

// step is one driver event applied to the gate: either a reconcile against a
// leader flag, or a renew-failure. wantStart/wantStop are ignored for failures.
type step struct {
	failed   bool // onRenewFailed instead of reconcile
	isLeader bool // arg to reconcile (when !failed)
	want     tickerAction
}

func runSteps(t *testing.T, name string, steps []step) {
	t.Helper()
	g := renewalGate{}
	for i, s := range steps {
		if s.failed {
			g.onRenewFailed()
			if g.renewing {
				t.Fatalf("%s step %d: renewing should be false after onRenewFailed", name, i)
			}
			continue
		}
		if got := g.reconcile(s.isLeader); got != s.want {
			t.Fatalf("%s step %d: reconcile(%v) = %s, want %s",
				name, i, s.isLeader, actionName(got), actionName(s.want))
		}
	}
}

func TestRenewalGate(t *testing.T) {
	// follower -> leader starts the ticker; leader -> follower stops it.
	runSteps(t, "basic edges", []step{
		{isLeader: true, want: tickerStart},
		{isLeader: false, want: tickerStop},
	})

	// Redundant "still leader" notifications must NOT re-Start the ticker —
	// otherwise repeated Resets could push the next renewal past the lease TTL.
	runSteps(t, "redundant leader signals", []step{
		{isLeader: true, want: tickerStart},
		{isLeader: true, want: tickerNoop},
		{isLeader: true, want: tickerNoop},
	})

	// Redundant follower notifications must NOT re-Stop the ticker.
	runSteps(t, "redundant follower signals", []step{
		{isLeader: false, want: tickerNoop}, // already idle at start
		{isLeader: true, want: tickerStart},
		{isLeader: false, want: tickerStop},
		{isLeader: false, want: tickerNoop},
	})

	// Regression for the signal-coalescing bug: after a renew failure the gate
	// is "not renewing", so a fresh leader reconcile (re-acquired leadership)
	// MUST restart the ticker. The earlier channel-carries-bool design could
	// drop that restart and silently stop renewing while still leader.
	runSteps(t, "lost then reacquired restarts", []step{
		{isLeader: true, want: tickerStart},
		{failed: true},
		{isLeader: true, want: tickerStart},
	})

	// A realistic flap: lead, fail, immediately reacquire, then lose for real.
	runSteps(t, "flap sequence", []step{
		{isLeader: true, want: tickerStart},
		{failed: true},
		{isLeader: true, want: tickerStart},
		{isLeader: false, want: tickerStop},
		{isLeader: false, want: tickerNoop},
	})
}

// TestRenewalGateZeroValueIsFollower documents that a fresh gate is not renewing
// (so the loop starts idle and only renews after becoming leader).
func TestRenewalGateZeroValueIsFollower(t *testing.T) {
	g := renewalGate{}
	if got := g.reconcile(false); got != tickerNoop {
		t.Fatalf("zero-value gate reconcile(false) = %s, want noop", actionName(got))
	}
	if g.renewing {
		t.Fatal("zero-value gate must not be renewing")
	}
}
