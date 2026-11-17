package server

import (
	"strings"
	"testing"
)

func TestCVSSVectorKnown(t *testing.T) {
	// The classic CVSS v3.1 example: AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H = 9.8
	s := cvssScore("CVSS:3.1/AV:N/AC:L/PR:N/UI:N/S:U/C:H/I:H/A:H")
	if s < 9.7 || s > 9.9 {
		t.Fatalf("expected ~9.8 got %.2f", s)
	}
	// XSS reflected: AV:N/AC:L/PR:N/UI:R/S:C/C:L/I:L/A:N = 5.8 (CVSS v3.1 spec)
	s = cvssScore("CVSS:3.1/AV:N/AC:L/PR:N/UI:R/S:C/C:L/I:L/A:N")
	if s < 5.7 || s > 5.9 {
		t.Fatalf("XSS reflected expected 5.8 got %.2f", s)
	}
	// Deterministic: same input, same output
	a, _ := cvssForClass("Cross-Site Scripting (Reflected)", "high")
	b, _ := cvssForClass("Cross-Site Scripting (Reflected)", "high")
	if a != b || a == "" {
		t.Fatalf("non-deterministic or empty: %q vs %q", a, b)
	}
	// Unknown class + known severity falls back to a banded score with caveat
	s2, v2 := cvssForClass("some unclassified weirdness", "medium")
	if s2 == "" || v2 == "" {
		t.Fatalf("expected severity-band fallback, got %q/%q", s2, v2)
	}
	// No mapping at all → empty, template renders "not scored"
	s3, v3 := cvssForClass("", "")
	if s3 != "" || v3 != "" {
		t.Fatalf("expected empty for no class/severity, got %q/%q", s3, v3)
	}
}

func TestRemediationDeterministic(t *testing.T) {
	a := remediationFor("Cross-Site Request Forgery (csrf)", "")
	b := remediationFor("Cross-Site Request Forgery (csrf)", "")
	if a == "" || a != b {
		t.Fatalf("remediation empty or non-deterministic")
	}
	if !strings.Contains(a, "anti-CSRF token") {
		t.Fatalf("CSRF remediation should name a token, got: %s", a)
	}
	// Agent-emitted class variants must still reach the right control, not the
	// generic severity fallback. These are real class strings from a run.
	variants := map[string]string{
		"Session Management / Cookie Flags":            "HttpOnly",
		"Security Misconfiguration / Missing Headers":  "CSP",
		"Cross-Site Request Forgery (CSRF)":            "anti-CSRF token",
		"Cross-Site Scripting (Reflected)":             "HTML-encode",
		"Cross-Site Scripting (Stored)":                "Encode on output",
	}
	for class, expect := range variants {
		got := remediationFor(class, "high")
		if !strings.Contains(got, expect) {
			t.Errorf("class %q should reach %q control, got: %.80s", class, expect, got)
		}
	}
	// Unknown class returns severity-band guidance, never a blank string
	if r := remediationFor("totally novel bug", "high"); r == "" {
		t.Fatalf("expected fallback guidance for unknown class")
	}
}
