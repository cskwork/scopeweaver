package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Autumn-27/norma/llm"
)

// openRouterFixture serves one Chat Completions round trip and reports the body
// the transport actually sent, so tests assert the routed wire request.
type openRouterFixture struct {
	srv    *httptest.Server
	mu     sync.Mutex
	got    map[string]json.RawMessage
	header http.Header
}

func newOpenRouterFixture(t *testing.T) *openRouterFixture {
	t.Helper()
	f := &openRouterFixture{}
	f.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.got = map[string]json.RawMessage{}
		_ = json.Unmarshal(body, &f.got)
		f.header = r.Header.Clone()
		f.mu.Unlock()
		w.Header().Set("content-type", "application/json")
		_, _ = io.WriteString(w, `{"id":"t","object":"chat.completion","model":"anthropic/claude-sonnet-4.5","choices":[{"index":0,"message":{"role":"assistant","content":"OK"},"finish_reason":"stop"}],"usage":{"prompt_tokens":1,"completion_tokens":1,"total_tokens":2}}`)
	}))
	t.Cleanup(f.srv.Close)
	return f
}

// request snapshots the body fields and headers the last request carried.
func (f *openRouterFixture) request() (map[string]json.RawMessage, http.Header) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.got, f.header
}

// routing decodes the sent "provider" object, nil when the request carried none.
func (f *openRouterFixture) routing(t *testing.T) map[string]any {
	t.Helper()
	got, _ := f.request()
	raw, sent := got["provider"]
	if !sent {
		return nil
	}
	var routing map[string]any
	if err := json.Unmarshal(raw, &routing); err != nil {
		t.Fatalf("decode provider object: %v", err)
	}
	return routing
}

// providerFor builds an openrouter-format provider against the fixture with the
// given routing settings.
func providerFor(t *testing.T, f *openRouterFixture, order string, fallbacks *bool) llm.Provider {
	t.Helper()
	c := ConfigFrom("openrouter", "anthropic/claude-sonnet-4.5", f.srv.URL, "test-key", "")
	c.SetOpenRouterRouting(order, fallbacks)
	p, err := c.NewProvider()
	if err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	return p
}

func completeOnce(t *testing.T, p llm.Provider) llm.Message {
	t.Helper()
	msg, _, _, err := p.Complete(context.Background(), llm.CompletionRequest{
		Messages: []llm.Message{{Role: llm.RoleUser, Content: []llm.ContentBlock{llm.TextBlock("hi")}}},
	})
	if err != nil {
		t.Fatalf("Complete: %v", err)
	}
	return msg
}

func msgText(t *testing.T, m llm.Message) string {
	t.Helper()
	var b strings.Builder
	for _, block := range m.Content {
		if block.Type == llm.BlockText {
			b.WriteString(block.Text)
		}
	}
	return b.String()
}

func boolPtr[T any](v T) *T { return &v }

// A pinned route with disallowed fallbacks must reach OpenRouter as
// provider.order + provider.allow_fallbacks=false on the same body the SDK built.
func TestOpenRouterRoutingInjection(t *testing.T) {
	t.Parallel()
	f := newOpenRouterFixture(t)
	p := providerFor(t, f, "Anthropic, openai, Anthropic", boolPtr(false))
	msg := completeOnce(t, p)
	if got := msgText(t, msg); got != "OK" {
		t.Fatalf("reply=%q", got)
	}
	got, header := f.request()
	var model string
	if err := json.Unmarshal(got["model"], &model); err != nil || model != "anthropic/claude-sonnet-4.5" {
		t.Fatalf("model=%q err=%v", got["model"], err)
	}
	routing := f.routing(t)
	if routing == nil {
		t.Fatal("no provider object sent")
	}
	order, _ := routing["order"].([]any)
	if len(order) != 2 || order[0] != "Anthropic" || order[1] != "openai" {
		t.Fatalf("order=%v, want [Anthropic openai] (deduped, case preserved)", order)
	}
	if fb, ok := routing["allow_fallbacks"].(bool); !ok || fb {
		t.Fatalf("allow_fallbacks=%v, want false", routing["allow_fallbacks"])
	}
	if _, sent := header["Authorization"]; !sent {
		t.Fatal("Bearer auth header missing")
	}
}

// Without routing settings the body must stay exactly norma's — no provider key,
// so OpenRouter's default load balancing applies.
func TestOpenRouterNoRoutingObjectWithoutConfig(t *testing.T) {
	t.Parallel()
	f := newOpenRouterFixture(t)
	p := providerFor(t, f, "", nil)
	completeOnce(t, p)
	got, _ := f.request()
	if _, sent := got["provider"]; sent {
		t.Fatalf("unexpected provider object: %s", got["provider"])
	}
}

// Pinning with fallbacks disallowed but no order is meaningful: OpenRouter should
// not fall back away from its default primary. The injector must send the object.
func TestOpenRouterFallbacksOnlyWithoutOrder(t *testing.T) {
	t.Parallel()
	f := newOpenRouterFixture(t)
	p := providerFor(t, f, "", boolPtr(false))
	completeOnce(t, p)
	routing := f.routing(t)
	if len(routing) != 1 {
		t.Fatalf("routing=%v, want only allow_fallbacks", routing)
	}
	if fb, ok := routing["allow_fallbacks"].(bool); !ok || fb {
		t.Fatalf("allow_fallbacks=%v, want false", routing["allow_fallbacks"])
	}
}

// The provider object is OpenRouter-specific: any other endpoint must receive the
// untouched body even when routing is configured.
func TestOpenRouterRoutingLeavesOtherEndpointsUntouched(t *testing.T) {
	t.Parallel()
	const body = `{"model":"m","messages":[],"stream":true}`
	var got []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	rt := quotaAwareTransport{base: http.DefaultTransport, routing: openrouterRouting{
		order:     []string{"anthropic"},
		fallbacks: boolPtr(false),
	}}
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/v1/responses", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: rt}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if string(got) != body {
		t.Fatalf("non-chat endpoint body=%q want %q", got, body)
	}
}

// norma rebuilds retries from GetBody. The merged body must be what retries read,
// otherwise a retry would silently drop the pinned routing.
func TestOpenRouterRoutingSurvivesRetryBodyReads(t *testing.T) {
	t.Parallel()
	const body = `{"model":"m","messages":[],"stream":false}`
	rt := quotaAwareTransport{base: roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		sent, err := io.ReadAll(r.Body)
		if err != nil {
			return nil, err
		}
		if !bytes.Contains(sent, []byte(`"provider"`)) {
			t.Fatalf("sent body missing provider object: %s", sent)
		}
		replayed, err := r.GetBody()
		if err != nil {
			t.Fatalf("GetBody: %v", err)
		}
		replayedBody, _ := io.ReadAll(replayed)
		replayed.Close()
		if !bytes.Equal(sent, replayedBody) {
			t.Fatalf("GetBody %q != sent %q", replayedBody, sent)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Status:     "200 OK",
			Body:       io.NopCloser(strings.NewReader(`{"choices":[{"message":{"content":"OK"},"finish_reason":"stop"}]}`)),
			Header:     make(http.Header),
		}, nil
	}), routing: openrouterRouting{fallbacks: boolPtr(false)}}
	req, err := http.NewRequest(http.MethodPost, "https://openrouter.invalid/api/v1/chat/completions", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
}

// A body that is not a JSON object must fail the request rather than send it
// without the pinned route.
func TestOpenRouterRoutingFailsClosedOnUnroutableBody(t *testing.T) {
	t.Parallel()
	rt := quotaAwareTransport{base: roundTripperFunc(func(*http.Request) (*http.Response, error) {
		t.Error("request must not reach the wire without its routing object")
		return nil, io.EOF
	}), routing: openrouterRouting{fallbacks: boolPtr(false)}}
	req, err := http.NewRequest(http.MethodPost, "https://openrouter.invalid/api/v1/chat/completions", strings.NewReader("not json"))
	if err != nil {
		t.Fatal(err)
	}
	req.GetBody = nil
	req.Header.Set("Content-Type", "application/json")
	if _, err := rt.RoundTrip(req); err == nil || !strings.Contains(err.Error(), "not a JSON object") {
		t.Fatalf("err=%v, want a not-a-JSON-object failure", err)
	}
}

// The openrouter profile format resolves to the OpenAI Chat Completions wire
// format on OpenRouter's base URL, and round-trips through Provider().
func TestConfigFromOpenRouter(t *testing.T) {
	t.Parallel()
	c := ConfigFrom("openrouter", "", "", "sk-or-x", "")
	if !c.OpenRouter || c.Format != llm.FormatOpenAI {
		t.Fatalf("format=%v openrouter=%v", c.Format, c.OpenRouter)
	}
	if c.BaseURL != OpenRouterBaseURL {
		t.Fatalf("base_url=%q, want %q", c.BaseURL, OpenRouterBaseURL)
	}
	if c.Model != OpenRouterDefaultModel {
		t.Fatalf("model=%q", c.Model)
	}
	if c.Provider() != "openrouter" {
		t.Fatalf("Provider()=%q", c.Provider())
	}
	// A full endpoint URL is tolerated like the other Chat Completions formats.
	c = ConfigFrom("openrouter", "vendor/model", "https://openrouter.example/api/v1/chat/completions", "k", "")
	if c.BaseURL != "https://openrouter.example/api/v1" {
		t.Fatalf("base_url=%q, want the /chat/completions suffix stripped", c.BaseURL)
	}
	if _, err := c.NewProvider(); err != nil {
		t.Fatalf("NewProvider: %v", err)
	}
	// Plain openai must not grow the flag or the routing setters' effect.
	c = ConfigFrom("openai", "gpt-4o", "", "k", "")
	c.SetOpenRouterRouting("anthropic", boolPtr(false))
	if c.OpenRouter || c.Provider() != "openai" || c.OpenRouterOrder != "" || c.OpenRouterFallbacks != "" {
		t.Fatalf("routing leaked into openai format: %+v", c)
	}
	// Setter canonicalization and the tri-state conversion.
	c = ConfigFrom("openrouter", "vendor/model", "", "k", "")
	c.SetOpenRouterRouting(" Anthropic, ,openai;;google\tAnthropic ", boolPtr(false))
	if c.OpenRouterOrder != "Anthropic,openai,google" || c.OpenRouterFallbacks != "false" {
		t.Fatalf("routing=%q/%q", c.OpenRouterOrder, c.OpenRouterFallbacks)
	}
	c.SetOpenRouterRouting("x", nil)
	if c.OpenRouterFallbacks != "false" {
		t.Fatalf("nil fallbacks must keep the previous setting, got %q", c.OpenRouterFallbacks)
	}
}

// Env resolution: explicit provider uses OPENROUTER_API_KEY; the key alone is
// enough to infer the format without stepping on the other keys' precedence.
func TestFromEnvOpenRouter(t *testing.T) {
	t.Setenv("ARTEX_LLM_PROVIDER", "openrouter")
	t.Setenv("OPENROUTER_API_KEY", "sk-or-env")
	c, ok := FromEnv()
	if !ok || !c.OpenRouter || c.APIKey != "sk-or-env" {
		t.Fatalf("ok=%v cfg=%+v", ok, c)
	}
	if c.BaseURL != OpenRouterBaseURL {
		t.Fatalf("base_url=%q", c.BaseURL)
	}
	t.Setenv("ARTEX_LLM_PROVIDER", "")
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("OPENAI_API_KEY", "")
	t.Setenv("OPENROUTER_API_KEY", "sk-or-infer")
	c, ok = FromEnv()
	if !ok || c.Provider() != "openrouter" || c.APIKey != "sk-or-infer" {
		t.Fatalf("inference: ok=%v provider=%q key=%q", ok, c.Provider(), c.APIKey)
	}
}

// Storage-string parsing: separators, blanks, and duplicates, case preserved.
func TestSplitOpenRouterOrder(t *testing.T) {
	t.Parallel()
	got := SplitOpenRouterOrder(" Anthropic, ,openai;;google\tAnthropic ")
	want := []string{"Anthropic", "openai", "google"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("slugs=%v want %v", got, want)
	}
	if n := NormalizeOpenRouterRouting(" a , b ;b a"); n != "a,b" {
		t.Fatalf("normalized=%q", n)
	}
	if n := NormalizeOpenRouterRouting("  "); n != "" {
		t.Fatalf("normalized blank=%q", n)
	}
}
