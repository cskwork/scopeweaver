package server

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"github.com/Autumn-27/artex/db"
)

func TestInterceptJudgeUsageEndpoint(t *testing.T) {
	m := backportTestManager(t)
	var err error
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	td := t.TempDir()
	s := New(ctx, m, td, td, td)
	h := s.Handler()
	token, err := signJWT(s.jwtKey)
	if err != nil {
		t.Fatal(err)
	}
	// Use an isolated ledger for exact, hand-derived totals.
	if _, err = m.pg.Exec(`DELETE FROM llm_usage`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = m.pg.Exec(`DELETE FROM llm_usage`) })
	fixtures := []db.LLMUsage{
		{Worker: "judge", ProfileName: "judge-old", InputTokens: 10, OutputTokens: 4, CacheRead: 3, CacheWrite: 2, Status: "ok"},
		{Worker: "judge", ProfileName: "judge-new", InputTokens: 20, OutputTokens: 6, CacheRead: 5, CacheWrite: 1, Status: "error"},
		{Worker: "planner", ProfileName: "other", InputTokens: 1000, OutputTokens: 1000, Status: "ok"},
	}
	for _, row := range fixtures {
		if err = m.pg.InsertLLMUsage(&row); err != nil {
			t.Fatal(err)
		}
	}
	if _, err = m.pg.Exec(`UPDATE llm_usage SET ts=now()-interval '40 days' WHERE profile_name='judge-old'`); err != nil {
		t.Fatal(err)
	}
	for _, days := range []string{"30", "0", "invalid"} {
		r := httptest.NewRequest("GET", "/api/intercept/judge/usage?days="+days, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 200 {
			t.Fatalf("status=%d want 200; body=%s", w.Code, w.Body.String())
		}
		var got struct {
			Calls      int `json:"calls"`
			Input      int `json:"input_tokens"`
			Output     int `json:"output_tokens"`
			CacheRead  int `json:"cache_read_tokens"`
			CacheWrite int `json:"cache_write_tokens"`
			Daily      []struct {
				Calls  int `json:"calls"`
				Input  int `json:"input_tokens"`
				Output int `json:"output_tokens"`
			} `json:"daily"`
		}
		if err = json.Unmarshal(w.Body.Bytes(), &got); err != nil {
			t.Fatal(err)
		}
		if got.Calls != 2 || got.Input != 30 || got.Output != 10 || got.CacheRead != 8 || got.CacheWrite != 3 {
			t.Fatalf("wrong all-time judge totals: %+v", got)
		}
		if len(got.Daily) != 1 || got.Daily[0].Calls != 1 || got.Daily[0].Input != 20 || got.Daily[0].Output != 6 {
			t.Fatalf("wrong recent judge series: %+v", got.Daily)
		}
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/api/intercept/judge/usage", nil))
	if w.Code != 401 {
		t.Fatalf("unauthenticated status=%d want 401", w.Code)
	}
}
