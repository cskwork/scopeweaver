package server

import (
	"testing"

	"github.com/Autumn-27/artex/chatstate"
	"github.com/Autumn-27/norma/llm"
)

func TestChatContextNULFlushClearsPending(t *testing.T) {
	s, ref, _ := newShutdownFlushFixture(t)
	view := chatstate.ContextView{Sequence: 2, Request: llm.CompletionRequest{
		Messages: []llm.Message{llm.UserText("binary\x00payload")},
	}}
	if err := s.captureChatContext(t.Context(), ref, "", view); err != nil {
		t.Fatal(err)
	}
	if err := s.flushChatContexts(); err != nil {
		t.Fatal(err)
	}
	if len(s.side.contextPending) != 0 {
		t.Fatal("successful NUL snapshot remained pending for retry")
	}
	got, err := s.m.pg.ChatContext(t.Context(), ref)
	if err != nil {
		t.Fatal(err)
	}
	if got.Sequence != 2 || got.Request.Messages[0].Text() != "binarypayload" {
		t.Fatalf("snapshot not persisted: %+v", got)
	}
	if err := s.flushChatContexts(); err != nil {
		t.Fatal("empty retry flush failed", err)
	}
}
