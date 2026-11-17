package server

// The report gate enforces one rule: a finished engagement report is a claim of
// completeness, so it is not rendered until the underlying analysis is complete.
// An incomplete run still writes its five raw logs (those are data, and data is
// never withheld), but it refuses to emit the polished HTML/PDF/DOCX/PPTX that a
// client would read as "this is the whole picture".
//
// This exists because a report that looks finished but silently drops vectors is
// worse than no report: the reader cannot see the gap. Refusing to render is the
// honest signal.

import (
	"context"
	"fmt"
	"strings"
)

// GateVerdict is the outcome of assessing one run against the completeness bar.
type GateVerdict struct {
	// Complete is true only when every blocking requirement is met. When false,
	// the report formats are withheld and Missing names each unmet requirement.
	Complete bool
	// Missing lists each requirement that is not satisfied, in plain language a
	// reader can act on.
	Missing []string
	// Warnings lists conditions that do not block rendering but should be stated
	// in the report itself (a failed or blocked run, non-browser vectors without
	// screenshot evidence).
	Warnings []string
	// Reason is the one-line explanation returned to a caller that asked for a
	// report and was refused.
	Reason string
}

// assessReportGate decides whether a run has earned a report. The bar:
//
//   - every finding carries a CVSS base score (a finding without a score is an
//     un-prioritised finding, and a client cannot act on priority ordering that
//     skips one)
//   - every finding carries remediation (a finding without a fix is a finding
//     half-delivered)
//   - every browser-demonstrable finding carries screenshot evidence
//     (confirmed ≠ proven — a response code is not a PoC-of-impact)
//
// A run with zero findings passes trivially: there is nothing to under-deliver on,
// and "no findings" is a legitimate result.
func (c *EngagementCollector) assessReportGate(ctx context.Context, runID int64) (GateVerdict, error) {
	v := GateVerdict{Complete: true}

	// Findings that reach the report = de-duplicated, not duplicate/false_positive.
	rows, err := c.DB.QueryContext(ctx, `
		SELECT m.finding_id, f.vulnclass, f.severity, f.name,
		       COALESCE(m.status,''), COALESCE(m.evidence_shot,'')
		  FROM eng_attack_mapping m
		  JOIN findings f ON f.id=m.finding_id
		 WHERE m.run_id=$1
		   AND COALESCE(f.status,'') NOT IN ('duplicate','false_positive')`, runID)
	if err != nil {
		return v, err
	}
	defer rows.Close()

	var total int
	var noCVSS, noRemed, noEvidence []string
	for rows.Next() {
		var id int64
		var class, sev, name, status, shot string
		if err := rows.Scan(&id, &class, &sev, &name, &status, &shot); err != nil {
			return v, err
		}
		total++
		label := fmt.Sprintf("#%d %s", id, truncate(name, 48))
		cvss, vector := cvssForClass(class, sev)
		if cvss == "" {
			noCVSS = append(noCVSS, label)
		}
		// A severity-band fallback is honest (its vector field says so) and is not
		// an unparseable vector — do not flag it. Only a real v3.1 vector that
		// fails to compute is a defect worth refusing over.
		if vector != "" && !strings.Contains(vector, "severity band") && cvssScore(vector) < 0 {
			noCVSS = append(noCVSS, label+" (unparseable vector)")
		}
		if remediationFor(class, sev) == "" {
			noRemed = append(noRemed, label)
		}
		// A browser-demonstrable finding owes a screenshot. Injection classes and
		// CSRF are the ones a reader expects to see rendered.
		if shot == "" && browserDemonstrable(class) && status == "confirmed" {
			noEvidence = append(noEvidence, label)
		}
	}
	if err := rows.Err(); err != nil {
		return v, err
	}

	if total == 0 {
		// Nothing to under-deliver on; the report states its own emptiness.
		return v, nil
	}

	if len(noCVSS) > 0 {
		v.Complete = false
		v.Missing = append(v.Missing, fmt.Sprintf(
			"%d of %d findings carry no CVSS v3.1 score: %v", len(noCVSS), total, noCVSS))
	}
	if len(noRemed) > 0 {
		v.Complete = false
		v.Missing = append(v.Missing, fmt.Sprintf(
			"%d of %d findings carry no remediation: %v", len(noRemed), total, noRemed))
	}
	if len(noEvidence) > 0 {
		v.Complete = false
		v.Missing = append(v.Missing, fmt.Sprintf(
			"%d of %d browser-demonstrable findings carry no screenshot evidence: %v",
			len(noEvidence), total, noEvidence))
	}
	// Non-blocking: a failed/blocked run is still reported, but the report must
	// say so rather than read as a clean bill of health.
	var runStatus string
	_ = c.DB.QueryRowContext(ctx, `SELECT COALESCE(status,'') FROM eng_run WHERE id=$1`, runID).Scan(&runStatus)
	if runStatus == "failed" || runStatus == "timeout" || runStatus == "blocked" {
		v.Warnings = append(v.Warnings,
			"run did not complete normally ("+runStatus+"): coverage may be partial")
	}
	if len(noEvidence) == 0 && total > 0 {
		// nothing
	}
	v.Reason = fmt.Sprintf("report withheld: %s", joinSentences(v.Missing))
	return v, nil
}

// browserDemonstrable reports whether a vulnerability class is the kind a reader
// expects to see executed in a browser, and therefore owes screenshot evidence.
func browserDemonstrable(class string) bool {
	c := strings.ToLower(class)
	for _, kw := range []string{"xss", "cross-site scripting", "csrf", "cross-site request forgery",
		"open redirect", "clickjacking"} {
		if strings.Contains(c, kw) {
			return true
		}
	}
	return false
}

func joinSentences(parts []string) string {
	return strings.Join(parts, "; ")
}

// ReportGateError is returned when a report was requested for a run that has not
// earned one. The caller (HTTP handler) turns it into a clear 409 with the list
// of what is missing, so the operator sees the gap instead of a half-report.
type ReportGateError struct {
	Verdict GateVerdict
}

func (e *ReportGateError) Error() string { return e.Verdict.Reason }
