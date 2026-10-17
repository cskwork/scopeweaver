package server

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Autumn-27/artex/agent"
	"github.com/Autumn-27/artex/db"
	"github.com/Autumn-27/artex/internal/dbtest"
	"github.com/Autumn-27/artex/locale"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// TestStockMetadataRuntimePostgres is a database-backed integration receipt, not a live
// inference test. It uses the actual catalog HTTP handlers, wireTools resolver,
// and PostgreSQL readback. Its owned child database isolates it from other tests.
func TestStockMetadataRuntimePostgres(t *testing.T) {
	dsn := dbtest.DSN(t)
	cfg, err := pgconn.ParseConfig(dsn)
	if err != nil || !strings.HasPrefix(cfg.Database, "scopeweaver_test_") {
		t.Fatal("ARTEX_TEST_DSN must select a disposable scopeweaver_test_* database")
	}
	childDSN, childDatabase := proofCreateStockDatabase(t, dsn)
	pg, err := db.Open(childDSN)
	if err != nil {
		t.Fatalf("open disposable PostgreSQL fixture: %v", err)
	}
	defer pg.Close()
	var database string
	if err := pg.QueryRow("SELECT current_database()").Scan(&database); err != nil || database != childDatabase {
		t.Fatal("database readback does not match the guarded test database")
	}
	for _, table := range []string{"tools", "agent_prompts", "llm_profiles"} {
		var n int
		if err := pg.QueryRow("SELECT count(*) FROM " + table).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Fatalf("requires fresh fixture: %s already has %d rows", table, n)
		}
	}
	// db.Open seeds disabled browser/ScopeSentry MCP rows; no enabled external connector is allowed.
	var enabledMCP int
	if err := pg.QueryRow("SELECT count(*) FROM mcp_servers WHERE enabled").Scan(&enabledMCP); err != nil {
		t.Fatal(err)
	}
	if enabledMCP != 0 {
		t.Fatal("fixture contains an enabled external MCP connector")
	}
	t.Setenv("AGENT_CORE_DISABLE_INTERACTIVE_SHELL", "1")
	priorResolve, priorTraffic := agent.ToolResolve, agent.FindingTrafficBindingEnabled
	defer func() { agent.ToolResolve, agent.FindingTrafficBindingEnabled = priorResolve, priorTraffic }()

	// Only the real metadata handlers are exposed, on an ephemeral loopback port.
	// No scheduler, inference provider, external MCP client, or platform action runs.
	s := &Server{m: &Manager{pg: pg}}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/tools", s.pgListTools)
	mux.HandleFunc("GET /api/agents/{key}/prompts", s.pgListPromptVersions)
	mux.HandleFunc("POST /api/agents/{key}/prompt/preview", s.pgPreviewPrompt)
	httpFixture := httptest.NewServer(withLocale(mux))
	defer httpFixture.Close()
	variants := s.knownToolLanguages()

	raw, err := os.ReadFile("../agent/testdata/legacy_add_intent_schemas.json")
	if err != nil {
		t.Fatal(err)
	}
	var legacy struct {
		AddIntent map[string]struct {
			InputSchema map[string]any `json:"input_schema"`
		} `json:"add_intent"`
	}
	if err := json.Unmarshal(raw, &legacy); err != nil {
		t.Fatal(err)
	}
	for _, lang := range locale.Languages() {
		if legacy.AddIntent[string(lang)].InputSchema == nil {
			t.Fatalf("missing %s schema fixture", lang)
		}
	}
	// Bootstrap the real catalog, then replace add_intent with exact historical
	// fixtures. Re-running wireTools must not overwrite these stored values.
	wireTools(pg, nil, variants)
	const customDescription = "runtime proof: user tool description"
	for _, sourceLang := range locale.Languages() {
		historical := legacy.AddIntent[string(sourceLang)].InputSchema
		cases := []struct {
			name        string
			change      func(map[string]any)
			description string
			bindings    []string
			enabled     bool
			corrected   bool
		}{
			{name: "stock", description: customDescription, bindings: []string{"planner"}, enabled: true, corrected: true},
			{name: "schema_edit", description: customDescription, bindings: []string{"planner"}, enabled: true, change: func(sc map[string]any) {
				sc["properties"].(map[string]any)["proof_extra"] = map[string]any{"type": "string", "description": "user extra field"}
			}},
			{name: "default_edit", description: customDescription, bindings: []string{"planner"}, enabled: true, change: func(sc map[string]any) {
				sc["properties"].(map[string]any)["priority"].(map[string]any)["default"] = float64(8)
			}},
			{name: "parameter_description_edit", description: customDescription, bindings: []string{"planner"}, enabled: true, change: func(sc map[string]any) {
				sc["properties"].(map[string]any)["summary"].(map[string]any)["description"] = "user summary instructions"
			}},
			{name: "description_edit", description: "independent edited tool description", bindings: []string{"planner"}, enabled: true, corrected: true},
			{name: "agent_binding_edit", description: customDescription, bindings: []string{"proof_custom_agent"}, enabled: true, corrected: true},
			{name: "disabled", description: customDescription, bindings: []string{"planner"}, enabled: false},
		}
		for _, tc := range cases {
			t.Run(string(sourceLang)+"/"+tc.name, func(t *testing.T) {
				persisted := proofClone(t, historical)
				if tc.change != nil {
					tc.change(persisted)
				}
				schemaBytes, err := json.Marshal(persisted)
				if err != nil {
					t.Fatal(err)
				}
				agentsBytes, err := json.Marshal(tc.bindings)
				if err != nil {
					t.Fatal(err)
				}
				if err := pg.UpdateTool("add_intent", tc.description, schemaBytes, agentsBytes, tc.enabled); err != nil {
					t.Fatal(err)
				}
				before := proofToolRow(t, pg)
				wireTools(pg, nil, variants)
				for _, runtimeLang := range locale.Languages() {
					ctx := locale.WithLang(context.Background(), runtimeLang)
					base := agent.NewToolSet(nil, "", runtimeLang).PlannerTools()
					resolved := names(agent.ToolResolve(ctx, tc.bindings[0], base))["add_intent"]
					if !tc.enabled {
						if resolved != nil {
							t.Fatal("disabled tool reached runtime")
						}
					} else {
						if resolved == nil {
							t.Fatal("bound tool missing from runtime")
						}
						if resolved.Description() != tc.description {
							t.Fatal("custom tool description changed at runtime")
						}
						got := resolved.InputSchema()
						if tc.corrected {
							want := variants["add_intent"][runtimeLang].InputSchema()
							proofEqual(t, got, want, "runtime must expose complete current localized factory schema")
							proofCompleteIntentItems(t, got)
						} else {
							// Localization is allowed to replace exact stock prose only.
							proofEqual(t, proofShape(got), proofShape(persisted), "custom schema structure/defaults changed")
							if tc.name == "parameter_description_edit" && got["properties"].(map[string]any)["summary"].(map[string]any)["description"] != "user summary instructions" {
								t.Fatal("custom parameter description changed")
							}
							items := got["properties"].(map[string]any)["intents"].(map[string]any)["items"].(map[string]any)
							if _, ok := items["properties"]; ok {
								t.Fatal("custom prior schema was automatically upgraded")
							}
						}
					}
					if tc.name == "agent_binding_edit" {
						if names(agent.ToolResolve(ctx, "planner", base))["add_intent"] != nil {
							t.Fatal("planner regained a user-removed binding")
						}
					}
					var api struct {
						Tools []*db.Tool `json:"tools"`
					}
					proofHTTP(t, httpFixture.URL, http.MethodGet, "/api/tools?lang="+string(runtimeLang), nil, &api)
					var storedAPI *db.Tool
					for _, row := range api.Tools {
						if row.Key == "add_intent" {
							storedAPI = row
							break
						}
					}
					if storedAPI == nil {
						t.Fatal("HTTP catalog omitted add_intent")
					}
					var apiSchema map[string]any
					if err := json.Unmarshal(storedAPI.Schema, &apiSchema); err != nil {
						t.Fatal(err)
					}
					// Catalog API still describes the stored row, including prior-stock
					// structure; only runtime resolution substitutes corrected stock.
					proofEqual(t, proofShape(apiSchema), proofShape(persisted), "HTTP API rewrote stored schema structure/defaults")
					if storedAPI.Description != tc.description || !reflect.DeepEqual(storedAPI.Agents, tc.bindings) || storedAPI.Enabled != tc.enabled {
						t.Fatal("HTTP API changed custom description, agent binding, or enabled flag")
					}
					if tc.name == "parameter_description_edit" && apiSchema["properties"].(map[string]any)["summary"].(map[string]any)["description"] != "user summary instructions" {
						t.Fatal("HTTP API changed custom parameter description")
					}
					if after := proofToolRow(t, pg); after != before {
						t.Fatal("wireTools/runtime/HTTP mutated the stored tool row")
					}
				}
				t.Log("real wireTools runtime, loopback HTTP catalog, exact full stored-row readback verified in en/ko/zh-Hans")
			})
		}
	}
	t.Run("stored_stock_and_custom_prompts", func(t *testing.T) {
		proofStoredPrompts(t, pg, httpFixture.URL)
	})
}

// proofCreateStockDatabase owns only the child name whose CREATE succeeded. The
// existing test DSN is guarded by dbtest before this function is called; retain
// all its connection options while changing only the selected database.
func proofCreateStockDatabase(t *testing.T, parentDSN string) (string, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	admin, err := pgx.Connect(ctx, parentDSN)
	if err != nil {
		t.Fatal("connect to guarded parent test database failed")
	}
	name := fmt.Sprintf("scopeweaver_test_stock_%d_%d", os.Getpid(), time.Now().UnixNano())
	identifier := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+identifier); err != nil {
		_ = admin.Close(context.Background())
		t.Fatalf("create owned disposable stock database failed (test role needs CREATE DATABASE): %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		defer admin.Close(cleanupCtx)
		// db.Close runs in the test body's defer before testing invokes Cleanup.
		// No FORCE: a leaked connection must fail visibly rather than be killed.
		if _, err := admin.Exec(cleanupCtx, "DROP DATABASE "+identifier); err != nil {
			t.Errorf("drop owned disposable database %s failed: %v", name, err)
		}
	})
	childDSN := parentDSN
	if parsed, err := url.Parse(parentDSN); err == nil && (parsed.Scheme == "postgres" || parsed.Scheme == "postgresql") {
		parsed.Path = "/" + name
		parsed.RawPath = ""
		query := parsed.Query()
		query.Set("dbname", name)
		parsed.RawQuery = query.Encode()
		childDSN = parsed.String()
	} else {
		// Generated name has only ASCII letters, digits, and underscores. A final
		// keyword overrides an earlier dbname without reconstructing credentials.
		childDSN += " dbname=" + name
	}
	childConfig, err := pgconn.ParseConfig(childDSN)
	if err != nil || childConfig.Database != name {
		t.Fatal("owned child DSN did not select the newly created database")
	}
	return childDSN, name
}

func proofClone(t *testing.T, value map[string]any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatal(err)
	}
	return out
}

func proofEqual(t *testing.T, got, want any, message string) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(gotJSON, wantJSON) {
		t.Fatalf("%s\ngot: %s\nwant: %s", message, gotJSON, wantJSON)
	}
}

// Ignore localized prose while comparing all executable schema metadata.
func proofShape(value any) any {
	switch v := value.(type) {
	case map[string]any:
		out := map[string]any{}
		for key, child := range v {
			if key != "description" {
				out[key] = proofShape(child)
			}
		}
		return out
	case []any:
		out := make([]any, len(v))
		for i, child := range v {
			out[i] = proofShape(child)
		}
		return out
	default:
		return value
	}
}

func proofToolRow(t *testing.T, pg *db.DB) string {
	t.Helper()
	var snapshot string
	if err := pg.QueryRow("SELECT row_to_json(t)::text FROM tools t WHERE key='add_intent'").Scan(&snapshot); err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func proofCompleteIntentItems(t *testing.T, schema map[string]any) {
	t.Helper()
	items := schema["properties"].(map[string]any)["intents"].(map[string]any)["items"].(map[string]any)
	props, ok := items["properties"].(map[string]any)
	if !ok {
		t.Fatal("intents.items has no object properties")
	}
	for _, key := range []string{"summary", "asset_ids", "parent_ids", "priority"} {
		if _, exists := props[key]; !exists {
			t.Fatalf("intents.items missing %s", key)
		}
	}
	required, ok := items["required"].([]any)
	if !ok {
		t.Fatal("intents.items missing required list")
	}
	found := false
	for _, value := range required {
		if value == "summary" {
			found = true
		}
	}
	if !found {
		t.Fatal("intents.items does not require summary")
	}
}

func proofHTTP(t *testing.T, baseURL, method, path string, body any, target any) {
	t.Helper()
	var payload []byte
	var err error
	if body != nil {
		payload, err = json.Marshal(body)
		if err != nil {
			t.Fatal(err)
		}
	}
	request, err := http.NewRequest(method, baseURL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		t.Fatalf("%s %s: HTTP %d", method, path, response.StatusCode)
	}
	if err := json.NewDecoder(response.Body).Decode(target); err != nil {
		t.Fatal(err)
	}
}

func proofStoredPrompts(t *testing.T, pg *db.DB, baseURL string) {
	raw, err := os.ReadFile("../agent/testdata/legacy_stock_prompts.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixtures struct {
		Planner map[string]string `json:"planner"`
		Worker  map[string]string `json:"worker"`
	}
	if err := json.Unmarshal(raw, &fixtures); err != nil {
		t.Fatal(err)
	}
	for key, bodies := range map[string]map[string]string{"planner": fixtures.Planner, "worker": fixtures.Worker} {
		row, err := pg.GetAgentByKey(key)
		if err != nil || row == nil {
			t.Fatalf("missing built-in %s: %v", key, err)
		}
		for _, sourceLang := range locale.Languages() {
			stock := bodies[string(sourceLang)]
			if stock == "" {
				t.Fatalf("missing %s %s prompt fixture", key, sourceLang)
			}
			for _, custom := range []bool{false, true} {
				body := stock
				if custom {
					body = "CUSTOM USER PROMPT {{.DataDir}}"
				}
				version, err := pg.SavePrompt(row.ID, body, "disposable runtime proof", "test")
				if err != nil {
					t.Fatal(err)
				}
				before, err := pg.ListPromptVersions(row.ID)
				if err != nil {
					t.Fatal(err)
				}
				seedPrompts(pg)
				for _, runtimeLang := range locale.Languages() {
					path := fmt.Sprintf("/api/agents/%s/prompts?lang=%s", key, runtimeLang)
					var history struct {
						Versions []db.PromptVersion `json:"versions"`
					}
					proofHTTP(t, baseURL, http.MethodGet, path, nil, &history)
					if len(history.Versions) != len(before) || history.Versions[0].Version != version || history.Versions[0].Template != body {
						t.Fatal("HTTP prompt history changed stored template/version")
					}
					var preview struct {
						Rendered string `json:"rendered"`
						Error    string `json:"error"`
					}
					path = fmt.Sprintf("/api/agents/%s/prompt/preview?lang=%s", key, runtimeLang)
					// Empty template means read the current prompt through the actual API.
					proofHTTP(t, baseURL, http.MethodPost, path, map[string]any{"sample": map[string]string{"DataDir": "proof-data"}}, &preview)
					if preview.Error != "" {
						t.Fatalf("prompt preview failed: %s", preview.Error)
					}
					if custom {
						if preview.Rendered != "CUSTOM USER PROMPT proof-data" {
							t.Fatal("custom prompt preview changed")
						}
					} else {
						if agent.CanonicalBuiltinPrompt(body) != agent.BuiltinPromptSeeds()[key] {
							t.Fatal("stored prior stock prompt not recognized")
						}
						catalog, err := pg.PromptVars(row.ID)
						if err != nil {
							t.Fatal(err)
						}
						expected, err := renderPrompt(agent.BuiltinPromptText(agent.BuiltinPromptSeeds()[key], runtimeLang), withGlobalVars(catalog, runtimeLang), map[string]string{"DataDir": "proof-data"})
						if err != nil {
							t.Fatal(err)
						}
						if preview.Rendered != expected {
							t.Fatal("stored prior stock preview differs from current canonical localized prompt")
						}
					}
					stored, err := pg.CurrentPrompt(row.ID)
					if err != nil || stored != body {
						t.Fatal("prompt preview rewrote stored prompt")
					}
					after, err := pg.ListPromptVersions(row.ID)
					if err != nil {
						t.Fatal(err)
					}
					proofEqual(t, after, before, "prompt resolution/HTTP/seeding mutated prompt history")
				}
			}
		}
	}
	t.Log("planner/worker prior stock and custom prompt history, real HTTP preview, and stored readback verified; actual agent renderSystem is covered separately in agent package")
}
