package server

import (
	"bytes"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Autumn-27/artex/chatstate"
	"github.com/Autumn-27/artex/locale"
)

func (s *Server) chatControlRef(w http.ResponseWriter, r *http.Request) (chatstate.SessionRef, bool) {
	if strings.HasPrefix(r.URL.Path, "/api/conversations/") {
		_, c, ok := s.convByID(w, r)
		if !ok {
			return chatstate.SessionRef{}, false
		}
		return chatstate.SessionRef{ConversationID: c.ID}, true
	}
	t := s.m.ResolveTask(r.PathValue("id"))
	if t == nil {
		writeErr(w, 404, "task not found")
		return chatstate.SessionRef{}, false
	}
	seg, err := strconv.Atoi(r.URL.Query().Get("seg"))
	if err != nil || seg < 0 {
		writeErr(w, 400, "seg is required")
		return chatstate.SessionRef{}, false
	}
	id, _ := strconv.ParseInt(t.ID, 10, 64)
	return chatstate.SessionRef{TaskID: id, ExplorationID: t.ExpID, MainSeg: seg}, true
}
func chatControlError(w http.ResponseWriter, err error) {
	status := 400
	switch {
	case errors.Is(err, chatstate.ErrGone):
		status = 404
	case errors.Is(err, chatstate.ErrBusy), errors.Is(err, chatstate.ErrConflict), errors.Is(err, chatstate.ErrUnknownRun), errors.Is(err, chatstate.ErrAmbiguousHistory):
		status = 409
	}
	writeError(w, status, err)
}
func (s *Server) chatPromptDir(ref chatstate.SessionRef) string {
	if ref.ConversationID > 0 {
		return filepath.Join(s.m.dir, "sessions", ref.Key())
	}
	return filepath.Join(s.m.dir, "tasks", strconv.FormatInt(ref.TaskID, 10))
}
func (s *Server) validateChatPrompt(ref chatstate.SessionRef, prompt chatstate.Prompt) error {
	if strings.TrimSpace(prompt.Text) == "" && len(prompt.Attachments) == 0 {
		return fmt.Errorf("message is required")
	}
	base := s.chatPromptDir(ref)
	for _, a := range prompt.Attachments {
		cleaned := filepath.ToSlash(filepath.Clean(a.Path))
		if cleaned != a.Path || !strings.HasPrefix(cleaned, "uploads/") || strings.Contains(cleaned, "../") || filepath.IsAbs(a.Path) {
			return fmt.Errorf("invalid attachment path")
		}
		path := filepath.Join(base, filepath.FromSlash(cleaned))
		resolved, err := filepath.EvalSymlinks(path)
		if err != nil {
			return fmt.Errorf("attachment unavailable: %s", a.Name)
		}
		root, err := filepath.EvalSymlinks(filepath.Join(base, "uploads"))
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, resolved)
		if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			return fmt.Errorf("attachment outside chat")
		}
		info, err := os.Stat(resolved)
		if err != nil || !info.Mode().IsRegular() || info.Size() != a.Size {
			return fmt.Errorf("attachment unavailable: %s", a.Name)
		}
		if a.Abs != "" && filepath.Clean(a.Abs) != filepath.Clean(path) {
			return fmt.Errorf("attachment owner mismatch")
		}
	}
	_, err := composeChatMentionMessage(s.m.pg, prompt.Text)
	return err
}
func (s *Server) chatControls(w http.ResponseWriter, r *http.Request) {
	if s.pg(w) == nil {
		return
	}
	ref, ok := s.chatControlRef(w, r)
	if !ok {
		return
	}
	rest := strings.Trim(r.PathValue("rest"), "/")
	parts := strings.Split(rest, "/")
	if r.Method == http.MethodGet && rest == "" {
		state, err := s.m.pg.ChatControlState(r.Context(), ref)
		if err != nil {
			chatControlError(w, err)
			return
		}
		writeJSON(w, 200, state)
		return
	}
	if r.Method == http.MethodPost && rest == "inputs" {
		s.admitChatHTTP(w, r, ref)
		return
	}
	if len(parts) >= 2 && parts[0] == "inputs" {
		id := parts[1]
		if r.Method == http.MethodPatch {
			var req struct {
				Expected    int64            `json:"expected_revision,string"`
				Message     string           `json:"message"`
				Attachments []chatAttachment `json:"attachments"`
			}
			if !decodeConversationRequest(w, r, &req) {
				return
			}
			prompt := chatstate.Prompt{Text: req.Message, Attachments: req.Attachments}
			if err := s.validateChatPrompt(ref, prompt); err != nil {
				chatControlError(w, err)
				return
			}
			a, err := s.m.pg.UpdateChatInput(r.Context(), ref, id, req.Expected, prompt)
			if err != nil {
				chatControlError(w, err)
				return
			}
			writeJSON(w, 200, a)
			return
		}
		if r.Method == http.MethodDelete {
			expected, err := strconv.ParseInt(r.URL.Query().Get("expected_revision"), 10, 64)
			if err == nil {
				err = s.m.pg.CancelChatInput(r.Context(), ref, id, expected)
			}
			if err != nil {
				chatControlError(w, err)
				return
			}
			writeJSON(w, 200, map[string]bool{"ok": true})
			return
		}
		if r.Method == http.MethodPost && len(parts) == 3 && parts[2] == "discard" {
			var req struct {
				Expected int64 `json:"expected_revision,string"`
			}
			if !decodeConversationRequest(w, r, &req) {
				return
			}
			if err := s.m.pg.DiscardUnknownChatRun(r.Context(), ref, id, req.Expected); err != nil {
				chatControlError(w, err)
				return
			}
			state, err := s.m.pg.ChatControlState(r.Context(), ref)
			if err != nil {
				chatControlError(w, err)
				return
			}
			writeJSON(w, 200, state)
			return
		}
	}
	if r.Method == http.MethodPost && (rest == "pause" || rest == "resume" || rest == "queue/order") {
		var req struct {
			Expected int64    `json:"expected_revision,string"`
			IDs      []string `json:"input_ids"`
		}
		if !decodeConversationRequest(w, r, &req) {
			return
		}
		var err error
		switch rest {
		case "pause":
			err = s.m.pg.PauseChatGate(r.Context(), ref, req.Expected)
		case "resume":
			err = s.m.pg.ResumeChatGate(r.Context(), ref, req.Expected)
		case "queue/order":
			err = s.m.pg.ReorderChatInputs(r.Context(), ref, req.Expected, req.IDs)
		}
		if err != nil {
			chatControlError(w, err)
			return
		}
		if rest == "resume" {
			s.dispatchChatGate(ref.GateKey())
		}
		state, err := s.m.pg.ChatControlState(r.Context(), ref)
		if err != nil {
			chatControlError(w, err)
			return
		}
		writeJSON(w, 200, state)
		return
	}
	http.NotFound(w, r)
}
func (s *Server) admitChatHTTP(w http.ResponseWriter, r *http.Request, ref chatstate.SessionRef) {
	var req struct {
		ClientID    string           `json:"client_request_id"`
		Mode        string           `json:"mode"`
		Message     string           `json:"message"`
		Attachments []chatAttachment `json:"attachments"`
		TargetRunID string           `json:"target_run_id"`
	}
	if !decodeConversationRequest(w, r, &req) {
		return
	}
	if ref.TaskID > 0 && s.engine.IsDeleting(strconv.FormatInt(ref.TaskID, 10)) {
		chatControlError(w, chatstate.ErrGone)
		return
	}
	prompt := chatstate.Prompt{Text: req.Message, Attachments: req.Attachments}
	if err := s.validateChatPrompt(ref, prompt); err != nil {
		chatControlError(w, err)
		return
	}
	if ref.ConversationID > 0 {
		c, err := s.m.pg.GetConversation(ref.ConversationID)
		if err != nil {
			chatControlError(w, err)
			return
		}
		if c == nil {
			chatControlError(w, chatstate.ErrGone)
			return
		}
		if s.resolveChatAgent(c) == nil {
			writeErr(w, 503, s.chatUnavailableReason(locale.FromRequest(r)))
			return
		}
	}
	a, err := s.m.pg.AdmitChatInput(r.Context(), ref, req.ClientID, req.Mode, req.TargetRunID, prompt)
	if err != nil {
		chatControlError(w, err)
		return
	}
	s.dispatchChatGate(ref.GateKey())
	writeJSON(w, 202, a)
}

// Existing send endpoints enter exactly the same durable admission path. A
// server-generated ID preserves their pre-idempotency compatibility contract.
func (s *Server) admitCompatibilityChat(w http.ResponseWriter, r *http.Request, ref chatstate.SessionRef, message string, attachments []chatAttachment) {
	body, _ := json.Marshal(map[string]any{"client_request_id": rand.Text(), "mode": "send", "message": message, "attachments": attachments})
	next := r.Clone(r.Context())
	next.Body = io.NopCloser(bytes.NewReader(body))
	s.admitChatHTTP(w, next, ref)
}
