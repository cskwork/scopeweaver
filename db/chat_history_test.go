package db

import (
	"errors"
	"os"
	"testing"

	"github.com/Autumn-27/artex/chatstate"
	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/transcript"
)

func TestChatTranscriptPublishCrashWindows(t *testing.T) {
	d, ref := controlFixture(t)
	ctx := t.Context()
	store := transcript.NewStore(t.TempDir())
	legacy, err := d.ActiveChatTranscript(ctx, ref)
	if err != nil || legacy.TranscriptID != ref.LegacyTranscriptID() {
		t.Fatal(legacy, err)
	}
	prompt := llm.UserText("first")
	answer := llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock("answer")}}
	records := []transcript.Record{{UUID: "first", Type: "message", Message: &prompt}, {UUID: "answer", Type: "message", Message: &answer, Usage: &llm.Usage{InputTokens: 10}}}
	prepared, err := chatstate.PrepareTranscript(ctx, store, ref, records)
	if err != nil {
		t.Fatal(err)
	}
	if err = d.PublishChatHistory(ctx, ref, 0, prepared); err != nil {
		t.Fatal(err)
	}
	before, err := d.ActiveChatTranscript(ctx, ref)
	if err != nil || before.TranscriptID != prepared.TranscriptID {
		t.Fatal(before, err)
	}
	next, err := chatstate.PrepareTranscript(ctx, store, ref, records)
	if err != nil {
		t.Fatal(err)
	}
	if err = d.PublishChatHistory(ctx, ref, 0, next); !errors.Is(err, chatstate.ErrConflict) {
		t.Fatalf("stale publication %v", err)
	}
	after, err := d.ActiveChatTranscript(ctx, ref)
	if err != nil || after.TranscriptID != before.TranscriptID {
		t.Fatal(after, err)
	}
	bad := next
	bad.SHA256 = "mismatch"
	if err = d.PublishChatHistory(ctx, ref, 1, bad); err == nil {
		t.Fatal("published mismatched checksum")
	}
	if err = os.WriteFile(store.MainPath(next.TranscriptID), []byte("{partial"), 0600); err != nil {
		t.Fatal(err)
	}
	if err = d.PublishChatHistory(ctx, ref, 1, next); err == nil {
		t.Fatal("published partial transcript")
	}
	unchanged, err := d.ActiveChatTranscript(ctx, ref)
	if err != nil || unchanged.TranscriptID != before.TranscriptID {
		t.Fatal(unchanged, err)
	}
	// Preparing/copying history consumes no model usage and never writes billing rows.
	for _, cost := range []int{10, 20} {
		_, err = d.AppendConvActivity(ref.ConversationID, Activity{Kind: "result", InputTokens: &cost})
		if err != nil {
			t.Fatal(err)
		}
	}
	var billed int
	if err = d.QueryRow(`SELECT sum(input_tokens) FROM conversation_activities WHERE conversation_id=$1`, ref.ConversationID).Scan(&billed); err != nil || billed != 30 {
		t.Fatal(billed, err)
	}
}
