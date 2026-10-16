package db

import (
	"errors"
	"github.com/Autumn-27/artex/chatstate"
	"testing"
)

func TestChatContextStaleHistoryRunAndSequence(t *testing.T) {
	d, ref := controlFixture(t)
	queueInput(t, d, ref, "one")
	run, err := d.ClaimNextChatInput(t.Context(), ref.GateKey())
	if err != nil {
		t.Fatal(err)
	}
	view := chatstate.ContextView{HistoryRevision: 0, Sequence: 2, Model: "fixture", Used: 100, Window: 32000, OutputReserve: 4096, Estimated: true}
	if err = d.SaveChatContext(t.Context(), ref, run.ID, view); err != nil {
		t.Fatal(err)
	}
	older := view
	older.Sequence = 1
	if err = d.SaveChatContext(t.Context(), ref, run.ID, older); !errors.Is(err, chatstate.ErrConflict) {
		t.Fatal("stale sequence accepted", err)
	}
	if err = d.FinishChatRun(t.Context(), run.ID, "completed"); err != nil {
		t.Fatal(err)
	}
	queueInput(t, d, ref, "two")
	next, err := d.ClaimNextChatInput(t.Context(), ref.GateKey())
	if err != nil {
		t.Fatal(err)
	}
	if err = d.SaveChatContext(t.Context(), ref, next.ID, view); err != nil {
		t.Fatal(err)
	}
	view.Sequence = 100
	if err = d.SaveChatContext(t.Context(), ref, run.ID, view); !errors.Is(err, chatstate.ErrConflict) {
		t.Fatal("old run overwrote context", err)
	}
	view.HistoryRevision = 9
	if err = d.SaveChatContext(t.Context(), ref, next.ID, view); !errors.Is(err, chatstate.ErrConflict) {
		t.Fatal("old history overwrote context", err)
	}
	saved, err := d.ChatContext(t.Context(), ref)
	if err != nil || saved.Used != 100 {
		t.Fatal(saved, err)
	}
}

func TestChatContextBillingRetainsSupersededAttempts(t *testing.T) {
	d, ref := controlFixture(t)
	inactive := false
	ptr := func(n int) *int { return &n }
	for _, a := range []Activity{
		{Kind: "usage", RunID: "old", InputTokens: ptr(10)},
		{Kind: "result", RunID: "old", ActiveAttempt: &inactive, InputTokens: ptr(10), OutputTokens: ptr(2)},
		{Kind: "usage", RunID: "new", InputTokens: ptr(20)},
		{Kind: "usage", RunID: "new", InputTokens: ptr(30)},
	} {
		if _, err := d.AppendConvActivity(ref.ConversationID, a); err != nil {
			t.Fatal(err)
		}
	}
	state, err := d.ChatControlState(t.Context(), ref)
	if err != nil || state.BilledUsage == nil || state.BilledUsage.InputTokens != 40 || state.BilledUsage.OutputTokens != 2 {
		t.Fatal(state.BilledUsage, err)
	}
	if _, err = d.AppendConvActivity(ref.ConversationID, Activity{Kind: "result", RunID: "new", InputTokens: ptr(30), OutputTokens: ptr(4)}); err != nil {
		t.Fatal(err)
	}
	state, err = d.ChatControlState(t.Context(), ref)
	if err != nil || state.BilledUsage.InputTokens != 40 || state.BilledUsage.OutputTokens != 6 {
		t.Fatal("live usage double-counted", state.BilledUsage, err)
	}
}
