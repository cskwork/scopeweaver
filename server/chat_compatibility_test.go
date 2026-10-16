package server

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/chatstate"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/locale"
	"github.com/Autumn-27/norma/llm"
	"github.com/Autumn-27/norma/transcript"
)

func compatibilityTask(t *testing.T) (*Server, *Task, http.Handler) {
	t.Helper()
	s, _ := newRetestServer(t)
	s.m.assets = s.m.pg.Assets()
	s.engine = NewEngine(s.m)
	task, err := s.m.CreateTask("legacy chat fixture", "local compatibility checks", nil, 0, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		waitRetestIdle(t, s)
		if _, err := s.m.DeleteTask(task.ID, DeleteTaskOptions{}); err != nil {
			t.Error(err)
		}
	})
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/chat", s.chat)
	mux.HandleFunc("POST /api/tasks/{id}/chat/controls/{rest...}", s.chatControls)
	mux.HandleFunc("POST /api/conversations/{id}/messages", s.pgSendConversationMessage)
	return s, task, mux
}

func compatibilityRequest(h http.Handler, path, body string) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	r = r.WithContext(locale.WithLang(r.Context(), locale.Lang("en")))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func compatibilityJSON(t *testing.T, w *httptest.ResponseRecorder, status int, want map[string]string) {
	t.Helper()
	var got map[string]string
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Errorf("response is not the legacy string object: HTTP %d %s: %v", w.Code, w.Body.String(), err)
		return
	}
	if w.Code != status || fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("HTTP %d %v; want HTTP %d %v", w.Code, got, status, want)
	}
}

func TestChatCompatibilityRuleAndSegments(t *testing.T) {
	s, task, h := compatibilityTask(t)
	current, err := task.Store.NewMainSession()
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, segment string
		want          int
	}{
		{"omitted", "", current.Seq},
		{"null", `,"seg":null`, current.Seq},
		{"negative-one", `,"seg":-1`, current.Seq},
		{"other-negative", `,"seg":-7`, current.Seq},
		{"explicit-zero", `,"seg":0`, 0},
		{"explicit-current", fmt.Sprintf(`,"seg":%d`, current.Seq), current.Seq},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text := "compatibility-" + tc.name
			w := compatibilityRequest(h, "/api/chat?task="+task.ID, `{"message":"hint `+text+`"`+tc.segment+`}`)
			compatibilityJSON(t, w, 200, map[string]string{"reply": "Hint saved for the planner's next round: " + text, "mode": "rule"})
			waitRetestIdle(t, s)
			nodes, err := task.Store.ListByKind(db.KindHint, 100)
			if err != nil {
				t.Fatal(err)
			}
			count := 0
			for _, node := range nodes {
				var payload struct{ Text string }
				if err := json.Unmarshal(node.Payload, &payload); err != nil {
					t.Fatal(err)
				}
				if payload.Text == text {
					count++
				}
			}
			if count != 1 {
				t.Errorf("rule side effects=%d; want exactly one saved hint", count)
			}
			ref := chatstate.SessionRef{TaskID: mustTaskID(t, task.ID), ExplorationID: task.ExpID, MainSeg: tc.want}
			runs, err := s.m.pg.ChatHistoryRuns(t.Context(), ref)
			if err != nil {
				t.Fatal(err)
			}
			var matched []chatstate.Run
			for _, run := range runs {
				if run.Prompt.Text == "hint "+text {
					matched = append(matched, run)
				}
			}
			if len(matched) != 1 || matched[0].Status != "completed" {
				t.Fatalf("durable run for segment %d: %+v", tc.want, matched)
			}
			var users, replies int
			if err := s.m.pg.QueryRow(`SELECT count(*) FILTER (WHERE kind='user'),count(*) FILTER (WHERE kind='text') FROM activity WHERE exploration_id=$1 AND run_id=$2 AND main_seg=$3`, task.ExpID, matched[0].ID, tc.want).Scan(&users, &replies); err != nil {
				t.Fatal(err)
			}
			if users != 1 || replies != 1 {
				t.Errorf("persisted user/reply rows=%d/%d; want 1/1", users, replies)
			}
		})
	}
}

func TestChatCompatibilityLLMResponseAndBusy(t *testing.T) {
	s, task, h := compatibilityTask(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	t.Cleanup(func() { close(release) })
	var calls atomic.Int32
	p := retestProvider{complete: func(ctx context.Context, _ llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
		calls.Add(1)
		started <- struct{}{}
		select {
		case <-release:
		case <-ctx.Done():
			return llm.Message{}, "", llm.Usage{}, ctx.Err()
		}
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock("done")}}, "end_turn", llm.Usage{}, nil
	}}
	main := agent.NewMainAgent(p, "compatibility", s.m.dir, transcript.NewStore(filepath.Join(s.m.dir, "transcripts")), 10000, 1)
	main.SetNonStreaming(func() bool { return true })
	s.llmOn, s.llmProv = true, p
	s.taskAgents = map[string]*taskAgentBundle{task.ID: {main: main}}
	w := compatibilityRequest(h, "/api/chat?task="+task.ID, `{"message":"first"}`)
	compatibilityJSON(t, w, 202, map[string]string{"status": "accepted", "mode": "llm"})
	select {
	case <-started:
	case <-time.After(5 * time.Second):
		t.Fatal("accepted legacy request did not start the model")
	}
	w = compatibilityRequest(h, "/api/chat?task="+task.ID, `{"message":"hint must-not-run"}`)
	if w.Code != 409 {
		t.Fatalf("busy legacy send: HTTP %d %s; want 409", w.Code, w.Body.String())
	}
	var inputs int
	ref := chatstate.SessionRef{TaskID: mustTaskID(t, task.ID), ExplorationID: task.ExpID}
	if err := s.m.pg.QueryRow(`SELECT count(*) FROM chat_control_inputs WHERE gate_key=$1`, ref.GateKey()).Scan(&inputs); err != nil || inputs != 1 || calls.Load() != 1 {
		t.Fatalf("busy send admitted/executed extra work: inputs=%d calls=%d err=%v", inputs, calls.Load(), err)
	}
}

func TestChatCompatibilityConversationResponse(t *testing.T) {
	s, _, h := compatibilityTask(t)
	setRetestProvider(s, retestProvider{complete: func(context.Context, llm.CompletionRequest) (llm.Message, string, llm.Usage, error) {
		return llm.Message{Role: llm.RoleAssistant, Content: []llm.ContentBlock{llm.TextBlock("done")}}, "end_turn", llm.Usage{}, nil
	}})
	c, err := s.m.pg.CreateConversation("auto", "compatibility", nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { waitRetestIdle(t, s); s.m.pg.DeleteConversation(c.ID) })
	w := compatibilityRequest(h, fmt.Sprintf("/api/conversations/%d/messages", c.ID), `{"message":"hello"}`)
	compatibilityJSON(t, w, 202, map[string]string{"status": "accepted"})
}

func TestChatCompatibilityQueuedAndDeletingTask(t *testing.T) {
	s, task, h := compatibilityTask(t)
	ref := chatstate.SessionRef{TaskID: mustTaskID(t, task.ID), ExplorationID: task.ExpID}
	if err := s.m.pg.PauseChatGate(t.Context(), ref, -1); err != nil {
		t.Fatal(err)
	}
	w := compatibilityRequest(h, "/api/tasks/"+task.ID+"/chat/controls/inputs?seg=0", `{"client_request_id":"durable-queue","mode":"queue","message":"hint queued-once"}`)
	var admission chatstate.Admission
	if err := json.Unmarshal(w.Body.Bytes(), &admission); err != nil || w.Code != 202 || admission.ID == "" || admission.Status != "queued" {
		t.Fatalf("new control endpoint lost durable admission: HTTP %d %s err=%v", w.Code, w.Body.String(), err)
	}
	w = compatibilityRequest(h, "/api/chat?task="+task.ID, `{"message":"hint cannot-jump-queue"}`)
	if w.Code != 409 {
		t.Fatalf("legacy send jumped paused queue: HTTP %d %s", w.Code, w.Body.String())
	}
	if err := s.m.pg.ResumeChatGate(t.Context(), ref, -1); err != nil {
		t.Fatal(err)
	}
	s.dispatchChatGate(ref.GateKey())
	waitRetestIdle(t, s)
	if !s.engine.BeginDelete(task.ID) {
		t.Fatal("could not install deletion barrier")
	}
	defer s.engine.AbortDelete(task.ID, false)
	w = compatibilityRequest(h, "/api/chat?task="+task.ID, `{"message":"hint cannot-run-after-delete"}`)
	if w.Code < 400 || w.Code >= 500 {
		t.Fatalf("deleting task accepted work: HTTP %d %s", w.Code, w.Body.String())
	}
	nodes, err := task.Store.ListByKind(db.KindHint, 100)
	if err != nil || len(nodes) != 1 || !strings.Contains(string(nodes[0].Payload), `"queued-once"`) {
		t.Fatalf("rejected or queued work executed extra side effects: %+v err=%v", nodes, err)
	}
}
