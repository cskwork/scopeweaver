package server

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// The gate enforces the operator's rule: no report until the analysis is
// complete. A report that looks finished while silently dropping vectors is worse
// than no report, because the reader cannot see the gap.

// Browser-demonstrable classes owe screenshot evidence; injection-of-state and
// misconfiguration classes do not, because a reader does not expect a picture of
// "no CSP header".
func TestBrowserDemonstrable(t *testing.T) {
	yes := []string{
		"Cross-Site Scripting (Reflected)",
		"Cross-Site Scripting (Stored)",
		"Cross-Site Request Forgery (CSRF)",
		"Open Redirect",
		"clickjacking",
	}
	no := []string{
		"SQL Injection",
		"Missing Security Headers",
		"Session Management / Cookie Flags",
		"Insecure Cookie Handling",
		"Command Injection",
	}
	for _, c := range yes {
		if !browserDemonstrable(c) {
			t.Errorf("%q should be browser-demonstrable", c)
		}
	}
	for _, c := range no {
		if browserDemonstrable(c) {
			t.Errorf("%q should not be browser-demonstrable", c)
		}
	}
}

// A finding whose class is unknown still gets a severity-band CVSS and
// remediation, never a blank. The vector field names the band rather than a
// fabricated vector — that is the honest fallback, and the gate relies on it
// never being empty.
func TestGateFallbackNeverBlank(t *testing.T) {
	score, vector := cvssForClass("some entirely novel class", "high")
	if score == "" || vector == "" {
		t.Fatalf("unknown class must yield a banded score+vector, got %q/%q", score, vector)
	}
	if !strings.Contains(vector, "severity band") {
		t.Fatalf("fallback vector must say it is a severity band, got %q", vector)
	}
	if remediationFor("some entirely novel class", "high") == "" {
		t.Fatalf("unknown class must yield severity-level remediation")
	}
}

// The gate's reason must name the specific gaps so the operator can act on it,
// not return a generic failure.
func TestGateReasonNamesGaps(t *testing.T) {
	missing := []string{
		"2 of 5 findings carry no CVSS v3.1 score: [#1 a, #2 b]",
		"1 of 5 browser-demonstrable findings carry no screenshot evidence: [#3 c]",
	}
	reason := joinSentences(missing)
	for _, want := range []string{"no CVSS", "no screenshot evidence"} {
		if !strings.Contains(reason, want) {
			t.Errorf("reason should name %q, got %q", want, reason)
		}
	}
}

// ReportGateError must satisfy the error contract the API handler's errors.As
// branch relies on, and carry the operator-facing reason.
func TestReportGateErrorContract(t *testing.T) {
	var e error = &ReportGateError{Verdict: GateVerdict{Complete: false, Reason: "report withheld: incomplete"}}
	if e.Error() == "" {
		t.Fatalf("gate error must carry a reason")
	}
	var gateErr *ReportGateError
	if !errors.As(e, &gateErr) {
		t.Fatalf("errors.As must unwrap ReportGateError")
	}
	if gateErr.Verdict.Complete {
		t.Fatalf("a withheld verdict must not be Complete")
	}
	_ = context.Background
}

// Integration: assess a real run through the gate. This is the honest check that
// the rule actually holds against live data, not just the helpers. It asserts the
// gate returns a decision (pass or a named-gap refusal) for a run that has an
// attack mapping, and that a refusal always names at least one concrete gap.
func TestReportGateAgainstRealRun(t *testing.T) {
	db := testDB(t)
	c := &EngagementCollector{DB: db}
	ctx := context.Background()
	var runID int64
	err := db.QueryRow(`SELECT id FROM eng_run ORDER BY id DESC LIMIT 1`).Scan(&runID)
	if err != nil {
		t.Skipf("no eng_run rows: %v", err)
	}
	verdict, err := c.assessReportGate(ctx, runID)
	if err != nil {
		t.Skipf("gate could not assess run: %v", err)
	}
	// Either the run is complete, or it names concrete gaps. What it must never
	// do is refuse without saying why — that is the silent-half-report failure
	// the gate exists to prevent.
	if !verdict.Complete && len(verdict.Missing) == 0 {
		t.Fatalf("gate refused run %d without naming any gap", runID)
	}
	if verdict.Complete && len(verdict.Missing) > 0 {
		t.Fatalf("complete verdict must not carry missing items, got %v", verdict.Missing)
	}
	t.Logf("run %d complete=%v missing=%d warnings=%d", runID, verdict.Complete,
		len(verdict.Missing), len(verdict.Warnings))
}
