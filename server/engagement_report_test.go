package server

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// TestGenerateReportRealRun renders the full artefact set for a task that has
// findings, and asserts each requested format exists and is non-trivial.
//
// This is the contract the operator asked for: every run produces a report, in
// HTML (tech report), PDF and DOCX, from stored logs only.
func TestGenerateReportRealRun(t *testing.T) {
	db := testDB(t)
	defer db.Close()

	// pick a task with the most findings, so the report has substance
	var taskID int64
	err := db.QueryRow(`
		SELECT task_id FROM findings
		 WHERE task_id IS NOT NULL
		 GROUP BY task_id ORDER BY count(*) DESC LIMIT 1`).Scan(&taskID)
	if err != nil {
		t.Skipf("no task with findings: %v", err)
	}
	t.Logf("task with most findings: %d", taskID)

	out := t.TempDir()
	c := &EngagementCollector{DB: db, TemplateRev: "test-rev", EvidenceDir: out}
	ctx := context.Background()

	runID, err := c.CollectEngagementLogs(ctx, taskID)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	produced, warnings, err := c.GenerateAll(ctx, runID, out)
	if err != nil {
		t.Fatalf("generate: %v", err)
	}
	for _, w := range warnings {
		t.Logf("warning: %s", w)
	}
	for format, path := range produced {
		st, err := os.Stat(path)
		if err != nil {
			t.Errorf("%s missing: %v", format, err)
			continue
		}
		// Log exports are legitimately small (an activity-analysis rollup is a
		// dozen rows); only the rendered report formats need a floor. A broken
		// renderer produces a zero-byte file either way.
		floor := int64(2000)
		if strings.HasPrefix(format, "log_") {
			floor = 100
		}
		if st.Size() < floor {
			t.Errorf("%s suspiciously small: %d bytes", format, st.Size())
		}
		t.Logf("%-5s %8d bytes  %s", format, st.Size(), filepath.Base(path))
	}
	if _, ok := produced["html"]; !ok {
		t.Fatal("html not produced")
	}
	if _, ok := produced["pdf"]; !ok {
		t.Error("pdf not produced")
	}
	if _, ok := produced["docx"]; !ok {
		t.Error("docx not produced")
	}

	// the HTML must actually contain finding content and the taxonomy tags
	html, err := os.ReadFile(produced["html"])
	if err != nil {
		t.Fatal(err)
	}
	s := string(html)
	if !strings.Contains(s, "ATT&amp;CK") {
		t.Error("html lacks ATT&CK tags — attack mapping not rendered")
	}
	if !strings.Contains(s, "WSTG") {
		t.Error("html lacks WSTG tags")
	}

	// manifest must record the report artefacts
	var n int64
	if err := db.QueryRow(`SELECT count(*) FROM eng_log_manifest WHERE run_id=$1 AND artefact LIKE 'report_%'`, runID).Scan(&n); err != nil {
		t.Fatal(err)
	}
	if n < 2 {
		t.Errorf("manifest report rows=%d, want >=2", n)
	}
	t.Logf("manifest report artefacts: %d", n)
}

// TestReportEmptyRunStillRenders covers the "no findings" path: the report must
// still generate rather than fail, and must state the gap instead of implying
// a clean run.
func TestReportEmptyRunStillRenders(t *testing.T) {
	db := testDB(t)
	defer db.Close()
	var taskID int64
	err := db.QueryRow(`
		SELECT t.id FROM tasks t
		 WHERE NOT EXISTS (SELECT 1 FROM findings f WHERE f.task_id=t.id)
		   AND EXISTS (SELECT 1 FROM activity a WHERE a.exploration_id=t.exploration_id)
		 LIMIT 1`).Scan(&taskID)
	if err != nil {
		t.Skipf("no finding-free task with activity: %v", err)
	}
	out := t.TempDir()
	c := &EngagementCollector{DB: db, TemplateRev: "test-rev", EvidenceDir: out}
	runID, err := c.CollectEngagementLogs(context.Background(), taskID)
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	htmlPath, err := c.BuildReportHTML(context.Background(), runID, out)
	if err != nil {
		t.Fatalf("render: %v", err)
	}
	b, _ := os.ReadFile(htmlPath)
	if !strings.Contains(string(b), "No findings are attached to this run") {
		t.Error("empty run must state the absence of findings explicitly")
	}
	_ = sql.ErrNoRows
}
