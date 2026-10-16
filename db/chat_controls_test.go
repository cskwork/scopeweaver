package db

import (
	"errors"
	"github.com/Autumn-27/artex/chatstate"
	"testing"
)

func TestChatAdmission(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer d.Close()
	c, err := d.CreateConversation("mainagent", "admission", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer d.DeleteConversation(c.ID)
	ref := chatstate.SessionRef{ConversationID: c.ID}
	ctx := t.Context()
	prompt := chatstate.Prompt{Text: "review", Attachments: []chatstate.Attachment{{Name: "report.pdf", Path: "uploads/report.pdf", Size: 4}}}
	first, err := d.AdmitChatInput(ctx, ref, "one", "queue", "", prompt)
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := d.AdmitChatInput(ctx, ref, "one", "queue", "", prompt)
	if err != nil || first.ID != duplicate.ID {
		t.Fatalf("duplicate %+v %v", duplicate, err)
	}
	if _, err = d.AdmitChatInput(ctx, ref, "one", "queue", "", chatstate.Prompt{Text: "changed"}); !errors.Is(err, chatstate.ErrConflict) {
		t.Fatalf("changed duplicate %v", err)
	}
	if _, err = d.UpdateChatInput(ctx, ref, first.ID, first.Revision+1, prompt); !errors.Is(err, chatstate.ErrConflict) {
		t.Fatalf("stale edit %v", err)
	}
	reopened, err := Open(testDSN(t))
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	// Both Open calls reapply the idempotent schema.
	state, err := reopened.ChatControlState(ctx, ref)
	if err != nil || len(state.Inputs) != 1 {
		t.Fatalf("state %+v %v", state, err)
	}
	if state.Inputs[0].Prompt.Attachments[0].Path != "uploads/report.pdf" {
		t.Fatal("manifest lost")
	}
}
