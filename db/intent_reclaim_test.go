package db

// Regression tests for the intent re-claim loop.
//
// Observed in run 852: one intent (#23538) was claimed five times by the same
// worker. A failed terminal CAS leaves the row in whatever state the winning
// actor chose; when that actor returns the intent to 'open', the frontier offers
// it again and the pool loop re-runs work that already happened. These tests pin
// the DB invariants the engine relies on:
//
//  1. ClaimIntent is a single-winner CAS.
//  2. A stale terminal CAS never overwrites the winner's state.
//  3. ResetRunningIntents reopens only rows still in 'running'.

import (
	"sync"
	"sync/atomic"
	"testing"
)

// reClaimFixture opens a store on a throwaway exploration and returns it cleaned up.
func reClaimFixture(t *testing.T) *ExplorationStore {
	t.Helper()
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	t.Cleanup(func() { d.Close() })
	expID, err := d.CreateExploration("intent reclaim", "re-claim loop regression")
	if err != nil {
		t.Fatalf("create exploration: %v", err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM explorations WHERE id=$1`, expID) })
	return d.Exploration(expID)
}

func TestClaimIntentIsSingleWinner(t *testing.T) {
	es := reClaimFixture(t)
	id := mustIntent(t, es, "single-winner")

	const racers = 3
	var wins int64
	var wg sync.WaitGroup
	start := make(chan struct{})
	for i := 0; i < racers; i++ {
		wg.Add(1)
		go func(name string) {
			defer wg.Done()
			<-start // release all goroutines together to widen the race window
			if ok, err := es.ClaimIntent(id, name); err != nil {
				t.Errorf("ClaimIntent(%s): %v", name, err)
			} else if ok {
				atomic.AddInt64(&wins, 1)
			}
		}(string(rune('a' + i)))
	}
	close(start)
	wg.Wait()

	if wins != 1 {
		t.Fatalf("ClaimIntent winners = %d, want exactly 1", wins)
	}
}

func TestClaimIntentFailsOnceRunning(t *testing.T) {
	es := reClaimFixture(t)
	id := mustIntent(t, es, "second-claim")

	ok, err := es.ClaimIntent(id, "work#1")
	if err != nil || !ok {
		t.Fatalf("first claim: ok=%v err=%v", ok, err)
	}
	ok, err = es.ClaimIntent(id, "work#2")
	if err != nil {
		t.Fatalf("second claim err: %v", err)
	}
	if ok {
		t.Fatal("second claim won while the intent was already running")
	}
}

// The winner settles first; the loser's stale CAS must fail and leave state alone.
// This is the exact case whose log line ("could not persist terminal state done")
// preceded every re-claim in run 852.
func TestCompareAndSetTerminalDoesNotOverwriteWinner(t *testing.T) {
	es := reClaimFixture(t)
	id := mustIntent(t, es, "lost-terminal-cas")

	if ok, err := es.ClaimIntent(id, "work#1"); err != nil || !ok {
		t.Fatalf("claim: ok=%v err=%v", ok, err)
	}
	changed, err := es.CompareAndSetIntentState(id, "running", "stopped")
	if err != nil || !changed {
		t.Fatalf("winner settle: changed=%v err=%v", changed, err)
	}
	changed, err = es.CompareAndSetIntentState(id, "running", "done")
	if err != nil {
		t.Fatalf("loser settle err: %v", err)
	}
	if changed {
		t.Fatal("stale terminal CAS overwrote the winner's state")
	}
	node, err := es.GetNode(id)
	if err != nil || node == nil {
		t.Fatalf("reload: node=%v err=%v", node, err)
	}
	if node.State != "stopped" {
		t.Fatalf("state = %q, want stopped (winner wins)", node.State)
	}
}

func TestResetRunningIntentsOnlyTouchesRunning(t *testing.T) {
	es := reClaimFixture(t)
	running := mustIntent(t, es, "left-running")
	done := mustIntent(t, es, "already-done")

	if ok, err := es.ClaimIntent(running, "work#1"); err != nil || !ok {
		t.Fatalf("claim running: ok=%v err=%v", ok, err)
	}
	if ok, err := es.ClaimIntent(done, "work#2"); err != nil || !ok {
		t.Fatalf("claim done: ok=%v err=%v", ok, err)
	}
	if changed, err := es.CompareAndSetIntentState(done, "running", "done"); err != nil || !changed {
		t.Fatalf("settle done: changed=%v err=%v", changed, err)
	}

	n, err := es.ResetRunningIntents()
	if err != nil {
		t.Fatalf("reset: %v", err)
	}
	if n != 1 {
		t.Fatalf("reset count = %d, want 1", n)
	}

	r, err := es.GetNode(running)
	if err != nil || r == nil {
		t.Fatalf("reload running: node=%v err=%v", r, err)
	}
	if r.State != "open" {
		t.Fatalf("running intent state = %q, want open", r.State)
	}
	d, err := es.GetNode(done)
	if err != nil || d == nil {
		t.Fatalf("reload done: node=%v err=%v", d, err)
	}
	if d.State != "done" {
		t.Fatalf("done intent state = %q, want done (must not be reopened)", d.State)
	}
}
