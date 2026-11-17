package server

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"
)

// TestEvidenceEngineXSSProof proves the capture pipeline end-to-end against a
// deliberately vulnerable local handler: the page reflects ?q= into HTML without
// escaping, the payload sets a JS marker, and the engine must report
// Executed=true with two distinct screenshots whose hashes differ.
//
// This is the contract that makes a report's screenshot trustworthy: a status
// code alone would not distinguish the reflected payload from a benign echo.
func TestEvidenceEngineXSSProof(t *testing.T) {
	if _, err := os.Stat("/usr/bin/chromium-browser"); err != nil {
		if _, err2 := os.Stat("/home/ubuntu/bin/chromium"); err2 != nil {
			t.Skip("no chromium on host")
		}
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "text/html")
		// Deliberately unescaped reflection — the vulnerability under test.
		fmt.Fprintf(w, `<html><head><title>search</title></head>
<body><h1>Results for: %s</h1><div id="out">no result</div></body></html>`, q)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	out := t.TempDir()
	eng := &EvidenceEngine{OutputDir: out, Timeout: 40 * time.Second}

	payload := `<script>window.__xss_proof=true;document.title="XSS-PROOF"</script>`
	before := srv.URL + "/?q=hello"
	// The payload must be URL-encoded or Chrome truncates the query at the first
	// raw quote/angle bracket. This mirrors how a real XSS proof is delivered.
	after := srv.URL + "/?q=" + url.QueryEscape(payload)

	res, err := eng.CaptureXSSProof(context.Background(), "test-xss", before, after, payload,
		`window.__xss_proof === true`)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	t.Logf("before=%s after=%s executed=%v", res.BeforeValue, res.AfterValue, res.Executed)
	t.Logf("shots: %s (%s) / %s (%s)", res.BeforePath, res.BeforeHash[:8], res.AfterPath, res.AfterHash[:8])

	if !res.Executed {
		t.Fatalf("payload did not execute: before=%q after=%q err=%q", res.BeforeValue, res.AfterValue, res.Error)
	}
	if res.BeforePath == "" || res.AfterPath == "" {
		t.Fatal("screenshots missing")
	}
	if res.BeforeHash == res.AfterHash {
		t.Error("before/after screenshots are byte-identical — the capture did not observe a change")
	}
	for _, p := range []string{res.BeforePath, res.AfterPath} {
		st, err := os.Stat(p)
		if err != nil || st.Size() < 1000 {
			t.Errorf("screenshot %s missing or too small: %v", p, err)
		}
	}
}

// TestEvidenceEngineNoFalsePositive is the negative control: a page that merely
// ECHOES the payload as text (escaped) must not report Executed=true.
func TestEvidenceEngineNoFalsePositive(t *testing.T) {
	if _, err := os.Stat("/usr/bin/chromium-browser"); err != nil {
		if _, err2 := os.Stat("/home/ubuntu/bin/chromium"); err2 != nil {
			t.Skip("no chromium on host")
		}
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		q := r.URL.Query().Get("q")
		w.Header().Set("Content-Type", "text/html")
		// Escaped reflection — NOT a vulnerability.
		safe := strings.NewReplacer("<", "&lt;", ">", "&gt;").Replace(q)
		fmt.Fprintf(w, `<html><body><h1>Results for: %s</h1></body></html>`, safe)
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	eng := &EvidenceEngine{OutputDir: t.TempDir(), Timeout: 40 * time.Second}
	payload := `<script>window.__xss_proof=true</script>`
	res, err := eng.CaptureXSSProof(context.Background(), "neg", srv.URL+"/?q=hi",
		srv.URL+"/?q="+payload, payload, `window.__xss_proof === true`)
	if err != nil {
		t.Fatalf("capture: %v", err)
	}
	if res.Executed {
		t.Fatalf("FALSE POSITIVE: escaped reflection reported as executed (after=%q)", res.AfterValue)
	}
	t.Logf("correctly not executed: after=%q", res.AfterValue)
}

// TestEvidenceProxyArgs pins the Chromium egress contract that keeps a proof
// screenshot from silently going blank.
//
// The service process inherits HTTP_PROXY/HTTPS_PROXY (runtime.env). If the
// capture browser inherits those too, it cannot load the page and stores a
// blank frame, which the pipeline reads as "payload did not execute" — a silent
// false negative on every finding. The contract is two-sided:
//
//   - Proxy set  -> route deliberately via --proxy-server=<Proxy>
//   - Proxy empty-> pin direct egress via --no-proxy-server
func TestEvidenceProxyArgs(t *testing.T) {
	deliberate := (&EvidenceEngine{Proxy: "http://127.0.0.1:18080"}).proxyArgs()
	if len(deliberate) != 1 || !strings.HasPrefix(deliberate[0], "--proxy-server=") {
		t.Fatalf("deliberate proxy must be passed through, got %v", deliberate)
	}
	if !strings.Contains(deliberate[0], "http://127.0.0.1:18080") {
		t.Errorf("proxy URL dropped: %v", deliberate)
	}

	direct := (&EvidenceEngine{}).proxyArgs()
	if len(direct) != 1 || direct[0] != "--no-proxy-server" {
		t.Fatalf("empty Proxy must pin direct egress, got %v", direct)
	}
}

// TestEvidenceStripProxyEnv proves the capture environment never carries an
// inherited proxy. Chrome honours HTTP_PROXY/HTTPS_PROXY over --no-proxy-server
// in some builds, so the flag alone would not save a screenshot. The parent
// service sets these, so an unstripped env is the blank-frame bug.
//
// NO_PROXY is deliberately kept: it does not set a proxy, it excludes hosts
// from one — dropping it would push more of the browser's traffic through a
// deliberate --proxy-server route.
func TestEvidenceStripProxyEnv(t *testing.T) {
	in := []string{
		"PATH=/usr/bin",
		"HTTP_PROXY=http://agent-proxy:3128",
		"HTTPS_PROXY=http://agent-proxy:3128",
		"ALL_PROXY=socks5://agent-socks:1080",
		"http_proxy=http://agent-proxy:3128", // lowercase form must match too
		"https_proxy=http://agent-proxy:3128",
		"NO_PROXY=localhost,127.0.0.1",
		"HOME=/home/ubuntu",
	}
	out := stripProxyEnv(in)

	for _, keep := range []string{"PATH=/usr/bin", "HOME=/home/ubuntu", "NO_PROXY=localhost,127.0.0.1"} {
		if !containsEnv(out, keep) {
			t.Errorf("stripper dropped %q (must be kept): %v", keep, out)
		}
	}
	for _, drop := range []string{
		"HTTP_PROXY=", "HTTPS_PROXY=", "ALL_PROXY=",
		"http_proxy=", "https_proxy=",
	} {
		if containsEnv(out, drop) {
			t.Errorf("stripper kept %q (must be dropped): %v", drop, out)
		}
	}
}

// containsEnv reports whether kv is present in the environment list exactly.
func containsEnv(env []string, kv string) bool {
	for _, e := range env {
		if e == kv {
			return true
		}
	}
	return false
}
