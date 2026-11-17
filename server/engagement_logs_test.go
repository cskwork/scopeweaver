package server

import (
	"context"
	"database/sql"
	"os"
	"testing"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// testDB opens the live artex PG. The engagement collector reads real tables
// (activity, findings, tasks), so an integration test against a real database
// is the only honest check; it skips cleanly when no DSN is configured.
func testDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("ARTEX_PG_DSN")
	if dsn == "" {
		// fall back to the runtime DSN used by the running service
		dsn = "postgres://artex:artex@127.0.0.1:5432/artex?sslmode=disable"
	}
	db, err := sql.Open("pgx", dsn)
	if err != nil {
		t.Skipf("no pg: %v", err)
	}
	if err := db.Ping(); err != nil {
		t.Skipf("pg unreachable: %v", err)
	}
	return db
}

// pickTask returns a task id that has exploration activity, so the collector has
// something to chew on. Prefers the most recent task with activity rows.
func pickTask(t *testing.T, db *sql.DB) int64 {
	t.Helper()
	var id int64
	err := db.QueryRow(`
		SELECT t.id FROM tasks t
		 JOIN explorations e ON e.id=t.exploration_id
		 JOIN activity a ON a.exploration_id=e.id
		 GROUP BY t.id ORDER BY count(a.id) DESC LIMIT 1`).Scan(&id)
	if err != nil {
		t.Skipf("no task with activity: %v", err)
	}
	return id
}

func TestCollectEngagementLogsRealTask(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	taskID := pickTask(t, db)

	c := &EngagementCollector{DB: db, TemplateRev: "test-rev", EvidenceDir: t.TempDir()}
	ctx := context.Background()

	runID, err := c.CollectEngagementLogs(ctx, taskID)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if runID == 0 {
		t.Fatal("runID is 0")
	}

	// every one of the five logs must have rows for a task with activity
	for _, tbl := range []string{
		"eng_activity_analysis", "eng_agent_activities", "eng_command_log",
		"eng_engagement_activities", "eng_attack_mapping",
	} {
		var n int64
		q := "SELECT count(*) FROM " + tbl + " WHERE run_id=$1"
		if err := db.QueryRow(q, runID).Scan(&n); err != nil {
			t.Fatalf("%s count: %v", tbl, err)
		}
		t.Logf("%-30s rows=%d", tbl, n)
		if n == 0 {
			t.Errorf("%s has zero rows — the log must be populated for every run", tbl)
		}
	}

	// manifest must have one row per log
	var mrows int64
	if err := db.QueryRow(`SELECT count(*) FROM eng_log_manifest WHERE run_id=$1`, runID).Scan(&mrows); err != nil {
		t.Fatal(err)
	}
	if mrows < 5 {
		t.Errorf("manifest rows=%d, want >=5", mrows)
	}

	// chain must verify intact for each log
	for _, tbl := range []string{
		"eng_activity_analysis", "eng_agent_activities", "eng_command_log",
		"eng_engagement_activities", "eng_attack_mapping",
	} {
		bad, err := c.VerifyChain(ctx, runID, tbl)
		if err != nil {
			t.Errorf("verify %s: %v", tbl, err)
			continue
		}
		if bad != 0 {
			t.Errorf("verify %s: chain broken at row %d", tbl, bad)
		}
	}

	// re-collect must be safe (idempotent replace) and keep the chain intact
	runID2, err := c.CollectEngagementLogs(ctx, taskID)
	if err != nil {
		t.Fatalf("re-collect: %v", err)
	}
	if runID2 == runID {
		t.Log("re-collect reused run id (delete+insert got a new serial; check)")
	}
	bad, err := c.VerifyChain(ctx, runID2, "eng_agent_activities")
	if err != nil || bad != 0 {
		t.Errorf("re-collect chain broken: row=%d err=%v", bad, err)
	}
}

func TestAttackMappingTaxonomy(t *testing.T) {
	cases := []struct {
		class      string
		wantMitre  string
		wantWSTG   string
	}{
		{"Broken Access Control / BFLA", "T1190", "WSTG-ATHZ-02"},
		{"SQL Injection — bounded scope", "T1190", "WSTG-INPV-05"},
		{"Missing Rate Limiting / Resource Exhaustion", "T1499", "WSTG-BUSL-05"},
		{"Cross-tenant IDOR on education records", "T1190", "WSTG-ATHZ-04"},
		{"PII dossier export / sensitive data", "T1530", "WSTG-INFO-05"},
		{"Cross-Site Request Forgery (CSRF) deletesnippet accepts a plain GET with no token", "T1185", "WSTG-SESS-05"},
		{"Cross-Site Scripting (Stored) via img onerror", "T1059.007", "WSTG-INPV-02"},
		{"Session Management / Cookie Flags HttpOnly SameSite", "T1185", "WSTG-SESS-02"},
	}
	for _, tc := range cases {
		m, _ := mitreFor(tc.class)
		w, _ := wstgFor(tc.class)
		if m != tc.wantMitre {
			t.Errorf("%q mitre=%s want %s", tc.class, m, tc.wantMitre)
		}
		if w != tc.wantWSTG {
			t.Errorf("%q wstg=%s want %s", tc.class, w, tc.wantWSTG)
		}
	}
}
