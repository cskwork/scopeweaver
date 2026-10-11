package db

import "testing"

// TestProfileOpenRouterRoutingRoundTrip pins the OpenRouter routing columns
// through the whole read/write surface: both column lists (list vs keyed loads)
// must carry them, the tri-state fallbacks pointer must survive NULL, and the
// blank-key UPDATE branch must not drop them. A miscounted column list silently
// shifts every later Scan target, so this is the test that catches it.
func TestProfileOpenRouterRoutingRoundTrip(t *testing.T) {
	d, err := Open(testDSN(t))
	if err != nil {
		t.Skipf("postgres unavailable (%v) — skipping", err)
	}
	// Close via Cleanup, registered FIRST so it runs LAST: cleanups are LIFO, and a
	// plain `defer d.Close()` would fire before them — the row-deleting cleanups
	// would then run against a closed pool and silently leave test rows behind.
	t.Cleanup(func() { d.Close() })

	fb := false
	id, err := d.SaveProfile(&LLMProfile{
		Name: "t-openrouter", Format: "openrouter", Model: "anthropic/claude-sonnet-4.5", APIKey: "k",
		BaseURL:             "https://openrouter.ai/api/v1",
		OpenRouterOrder:     "anthropic,openai",
		OpenRouterFallbacks: &fb,
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM llm_profiles WHERE id=$1`, id) })

	assertRouting := func(where string, p *LLMProfile) {
		t.Helper()
		if p.OpenRouterOrder != "anthropic,openai" || p.OpenRouterFallbacks == nil || *p.OpenRouterFallbacks {
			t.Fatalf("%s: order=%q fallbacks=%v", where, p.OpenRouterOrder, p.OpenRouterFallbacks)
		}
	}

	// Keyed single-row load (profileColsKey).
	p, err := d.ProfileByID(id)
	if err != nil || p == nil {
		t.Fatalf("ProfileByID: %v, p=%v", err, p)
	}
	assertRouting("keyed load", p)

	// List load (profileCols — the hint variant, a separate column list).
	ps, err := d.ListProfiles()
	if err != nil {
		t.Fatal(err)
	}
	var found *LLMProfile
	for _, x := range ps {
		if x.ID == id {
			found = x
		}
	}
	if found == nil {
		t.Fatal("profile missing from ListProfiles")
	}
	assertRouting("list load", found)

	// Update with a blank key takes the "keep existing key" UPDATE branch, which
	// has its own column list and is the easiest one to forget.
	p.APIKey = ""
	p.OpenRouterOrder = "google"
	p.OpenRouterFallbacks = nil
	if _, err := d.SaveProfile(p); err != nil {
		t.Fatal(err)
	}
	after, err := d.ProfileByID(id)
	if err != nil || after == nil {
		t.Fatalf("reload: %v, p=%v", err, after)
	}
	if after.OpenRouterOrder != "google" || after.OpenRouterFallbacks != nil {
		t.Fatalf("after update: order=%q fallbacks=%v, want google/nil", after.OpenRouterOrder, after.OpenRouterFallbacks)
	}

	// A brand-new profile with no routing values stores the tri-state NULL and an
	// empty order — the historical behavior for every other format.
	plainID, err := d.SaveProfile(&LLMProfile{Name: "t-openrouter-plain", Format: "openai", Model: "m", APIKey: "k"})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { d.Exec(`DELETE FROM llm_profiles WHERE id=$1`, plainID) })
	plain, err := d.ProfileByID(plainID)
	if err != nil || plain == nil {
		t.Fatalf("ProfileByID(plain): %v, p=%v", err, plain)
	}
	if plain.OpenRouterOrder != "" || plain.OpenRouterFallbacks != nil {
		t.Fatalf("plain profile: order=%q fallbacks=%v", plain.OpenRouterOrder, plain.OpenRouterFallbacks)
	}
}
