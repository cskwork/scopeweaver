package agent

import (
	"os"
	"testing"

	"github.com/Autumn-27/norma/llm"
)

// TestConfigFromOpenCodeGo locks the opencode-go alias wiring: the OpenCode Go
// subscription gateway is OpenAI-compatible, so it resolves to the openai wire
// format at the documented zen/go/v1 root, with glm-5.3-flash as the default
// model. A full endpoint URL is tolerated like the openai provider, and an
// explicit model or base URL always wins.
func TestConfigFromOpenCodeGo(t *testing.T) {
	c := ConfigFrom("opencode-go", "", "", "", "")
	if c.Provider() != "openai" {
		t.Errorf("opencode-go resolved provider %q, want openai (OpenAI-compatible gateway)", c.Provider())
	}
	if c.Format != llm.FormatOpenAI {
		t.Errorf("opencode-go resolved format %q, want %q", c.Format, llm.FormatOpenAI)
	}
	if c.BaseURL != "https://opencode.ai/zen/go/v1" {
		t.Errorf("opencode-go default base URL %q, want https://opencode.ai/zen/go/v1", c.BaseURL)
	}
	if c.Model != "glm-5.3-flash" {
		t.Errorf("opencode-go default model %q, want glm-5.3-flash", c.Model)
	}
	if c.SessionHeaderKey != "x-opencode-session" {
		t.Errorf("opencode-go session header %q, want x-opencode-session (gateway 400 MissingSessionID without it)", c.SessionHeaderKey)
	}

	c = ConfigFrom("opencode-go", "qwen3.7-plus", "https://opencode.ai/zen/go/v1/chat/completions", "", "")
	if c.BaseURL != "https://opencode.ai/zen/go/v1" {
		t.Errorf("opencode-go normalized base URL %q, want https://opencode.ai/zen/go/v1", c.BaseURL)
	}
	if c.Model != "qwen3.7-plus" {
		t.Errorf("opencode-go explicit model %q, want qwen3.7-plus", c.Model)
	}
}

// TestFromEnvOpenCodeGoCredential verifies the credential path: an opencode-go
// provider must take OPENCODE_API_KEY — not OPENAI_API_KEY, even when that cloud
// key is also present in the environment.
func TestFromEnvOpenCodeGoCredential(t *testing.T) {
	isolateLLMEnvironment(t)
	t.Setenv("SCOPEWEAVER_LLM_PROVIDER", "opencode-go")
	t.Setenv("OPENCODE_API_KEY", "opencode-subscription-key")
	cfg, ok := FromEnv()
	if !ok {
		t.Fatal("opencode-go environment configuration is not ready")
	}
	if cfg.Provider() != "openai" || cfg.BaseURL != "https://opencode.ai/zen/go/v1" || cfg.Model != "glm-5.3-flash" {
		t.Fatalf("opencode-go resolved provider=%q base=%q model=%q", cfg.Provider(), cfg.BaseURL, cfg.Model)
	}
	if cfg.APIKey != "opencode-subscription-key" {
		t.Fatalf("opencode-go configured key %q, want the OPENCODE_API_KEY value", cfg.APIKey)
	}
}

// TestFromEnvOpenCodeGoInferred verifies provider inference: OPENCODE_API_KEY
// alone (no Anthropic/OpenAI key) selects the opencode-go provider.
func TestFromEnvOpenCodeGoInferred(t *testing.T) {
	isolateLLMEnvironment(t)
	os.Unsetenv("ANTHROPIC_API_KEY")
	os.Unsetenv("OPENAI_API_KEY")
	t.Setenv("OPENCODE_API_KEY", "opencode-subscription-key")
	cfg, ok := FromEnv()
	if !ok {
		t.Fatal("OPENCODE_API_KEY alone did not resolve an LLM provider")
	}
	if cfg.Provider() != "openai" || cfg.BaseURL != "https://opencode.ai/zen/go/v1" || cfg.APIKey != "opencode-subscription-key" {
		t.Fatalf("inferred provider=%q base=%q key=%q, want openai / zen go root / OPENCODE_API_KEY", cfg.Provider(), cfg.BaseURL, cfg.APIKey)
	}
}

// TestFromEnvOpenCodeGoExplicitBlankKey verifies the API_KEY override contract:
// an explicitly empty SCOPEWEAVER_LLM_API_KEY suppresses the OPENCODE_API_KEY
// fallback. A cloud endpoint with no key is then not ready (Ready=false), the
// same contract as Anthropic/OpenAI; a keyless run needs a local endpoint.
func TestFromEnvOpenCodeGoExplicitBlankKey(t *testing.T) {
	isolateLLMEnvironment(t)
	t.Setenv("SCOPEWEAVER_LLM_PROVIDER", "opencode-go")
	t.Setenv("OPENCODE_API_KEY", "opencode-subscription-key")
	t.Setenv("SCOPEWEAVER_LLM_API_KEY", "")
	cfg, ok := FromEnv()
	if ok {
		t.Fatal("explicit blank API key must suppress the OPENCODE_API_KEY fallback and leave the cloud provider not ready")
	}
	if cfg.APIKey != "" {
		t.Fatalf("explicit blank API key suppressed fallback, got %q", cfg.APIKey)
	}
}
