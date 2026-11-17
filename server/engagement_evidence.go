package server

// Evidence engine: proves an exploit actually executed, with a screenshot.
//
// The rule this file enforces: a response code is not proof. For an in-page
// payload (XSS and anything else that changes the DOM) the only acceptable
// evidence is (a) the page state BEFORE, (b) the page state AFTER, (c) a
// boolean assertion evaluated inside the live document, and (d) a PNG of each
// state, hashed so a re-rendered report can prove it is the same image.
//
// The engine drives a private headless Chromium over the DevTools Protocol on
// an ephemeral port. It never touches the operator's interactive browser: a
// fresh temp profile per capture, killed on return.

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// EvidenceEngine launches isolated headless Chromium instances for proof capture.
type EvidenceEngine struct {
	// ChromePath is the chromium binary; empty resolves a sane default.
	ChromePath string
	// OutputDir receives the PNGs for one run.
	OutputDir string
	// Timeout bounds a single navigation+assert cycle.
	Timeout time.Duration
	// Cookies are set on CookieDomain before navigation so a capture can prove a
	// vector behind a session (stored XSS viewed as the victim, authenticated
	// IDOR, and so on). Keys are cookie names, values are the raw values.
	Cookies      map[string]string
	CookieDomain string
	// Proxy routes the capture browser's traffic (http:// or socks5://). Empty
	// means direct: the service environment sets HTTP_PROXY/HTTPS_PROXY for the
	// agent's own traffic, and inheriting those makes Chromium fail to load the
	// page and capture a blank frame — which reads as "payload did not execute".
	Proxy string
}

// EvidenceResult is what a capture returns; it is persisted verbatim into
// eng_attack_mapping so a report never has to re-run the exploit.
type EvidenceResult struct {
	TargetURL   string `json:"target_url"`
	Payload     string `json:"payload"`
	Assertion   string `json:"assertion"`
	BeforePath  string `json:"before_path"`
	AfterPath   string `json:"after_path"`
	BeforeHash  string `json:"before_hash"`
	AfterHash   string `json:"after_hash"`
	BeforeValue string `json:"before_value"`
	AfterValue  string `json:"after_value"`
	Executed    bool   `json:"executed"`
	Error       string `json:"error,omitempty"`
}

func (e *EvidenceEngine) chrome() string {
	if e.ChromePath != "" {
		return e.ChromePath
	}
	for _, p := range []string{
		"/home/ubuntu/bin/chromium", "/usr/bin/chromium-browser",
		"/usr/bin/chromium", "/usr/bin/google-chrome",
	} {
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return "chromium"
}

func (e *EvidenceEngine) timeout() time.Duration {
	if e.Timeout > 0 {
		return e.Timeout
	}
	return 45 * time.Second
}

// cdpConn is a minimal DevTools Protocol client over one page target.
type cdpConn struct {
	ws      *websocket.Conn
	mu      sync.Mutex
	id      int64
	waiters map[int64]chan json.RawMessage
	closed  bool
}

func dialCDP(ctx context.Context, wsURL string) (*cdpConn, error) {
	d := websocket.Dialer{HandshakeTimeout: 10 * time.Second}
	ws, _, err := d.DialContext(ctx, wsURL, nil)
	if err != nil {
		return nil, err
	}
	c := &cdpConn{ws: ws, waiters: map[int64]chan json.RawMessage{}}
	go c.readLoop()
	return c, nil
}

func (c *cdpConn) readLoop() {
	for {
		_, data, err := c.ws.ReadMessage()
		if err != nil {
			c.mu.Lock()
			c.closed = true
			for _, ch := range c.waiters {
				close(ch)
			}
			c.waiters = map[int64]chan json.RawMessage{}
			c.mu.Unlock()
			return
		}
		var env struct {
			ID     int64           `json:"id"`
			Result json.RawMessage `json:"result"`
			Error  *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal(data, &env) != nil {
			continue
		}
		if env.ID == 0 {
			continue // event, not a response
		}
		c.mu.Lock()
		ch := c.waiters[env.ID]
		delete(c.waiters, env.ID)
		c.mu.Unlock()
		if ch != nil {
			if env.Error != nil {
				ch <- json.RawMessage(fmt.Sprintf(`{"__error":%q}`, env.Error.Message))
			} else {
				ch <- env.Result
			}
			close(ch)
		}
	}
}

func (c *cdpConn) send(ctx context.Context, method string, params any) (json.RawMessage, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("cdp closed")
	}
	c.id++
	id := c.id
	ch := make(chan json.RawMessage, 1)
	c.waiters[id] = ch
	c.mu.Unlock()

	payload, _ := json.Marshal(map[string]any{"id": id, "method": method, "params": params})
	if err := c.ws.WriteMessage(websocket.TextMessage, payload); err != nil {
		return nil, err
	}
	select {
	case res, ok := <-ch:
		if !ok {
			return nil, fmt.Errorf("cdp connection lost during %s", method)
		}
		if len(res) > 12 && strings.HasPrefix(string(res), `{"__error":`) {
			var e struct {
				Msg string `json:"__error"`
			}
			_ = json.Unmarshal(res, &e)
			return nil, fmt.Errorf("%s: %s", method, e.Msg)
		}
		return res, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

// launch starts an isolated Chromium and returns its page websocket URL plus a
// cleanup function. Port 0 asks the kernel for a free port so concurrent
// captures never collide on a fixed number.
func (e *EvidenceEngine) launch(ctx context.Context) (string, func(), error) {
	profile, err := os.MkdirTemp("", "eng-evidence-*")
	if err != nil {
		return "", nil, err
	}
	args := []string{
		"--headless=new",
		"--remote-debugging-port=0",
		"--user-data-dir=" + profile,
		"--no-first-run", "--no-default-browser-check",
		"--disable-gpu", "--disable-dev-shm-usage",
		"--no-sandbox",
		"--disable-background-networking", "--disable-sync",
		"--window-size=1280,900",
	}
	if e.Proxy != "" {
		args = append(args, "--proxy-server="+e.Proxy)
	} else {
		// Deterministic direct egress regardless of the parent environment.
		args = append(args, "--no-proxy-server")
	}
	args = append(args, "about:blank")
	cmd := exec.Command(e.chrome(), args...)
	// Drop inherited proxy variables: they are for the agent's traffic, not for
	// the proof browser, and Chrome honours them over --no-proxy-server in some
	// builds.
	env := make([]string, 0, len(os.Environ()))
	for _, kv := range os.Environ() {
		u := strings.ToUpper(kv)
		if strings.HasPrefix(u, "HTTP_PROXY=") || strings.HasPrefix(u, "HTTPS_PROXY=") ||
			strings.HasPrefix(u, "ALL_PROXY=") || strings.HasPrefix(u, "http_proxy=") ||
			strings.HasPrefix(u, "https_proxy=") || strings.HasPrefix(u, "all_proxy=") {
			continue
		}
		env = append(env, kv)
	}
	cmd.Env = env
	stderr, err := cmd.StderrPipe()
	if err != nil {
		os.RemoveAll(profile)
		return "", nil, err
	}
	if err := cmd.Start(); err != nil {
		os.RemoveAll(profile)
		return "", nil, err
	}
	cleanup := func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
			_, _ = cmd.Process.Wait()
		}
		os.RemoveAll(profile)
	}

	// "DevTools listening on ws://127.0.0.1:PORT/devtools/browser/<uuid>"
	re := regexp.MustCompile(`DevTools listening on (ws://[^\s]+)`)
	deadline := time.Now().Add(20 * time.Second)
	var browserWS string
	sc := bufio.NewScanner(stderr)
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for time.Now().Before(deadline) {
		if sc.Scan() {
			if m := re.FindStringSubmatch(sc.Text()); m != nil {
				browserWS = m[1]
				break
			}
		} else {
			break
		}
	}
	if browserWS == "" {
		cleanup()
		return "", nil, fmt.Errorf("chromium did not expose a devtools endpoint")
	}

	// Resolve the page target from the HTTP endpoint.
	base := strings.TrimSuffix(browserWS, "/devtools/browser/"+filepath.Base(browserWS))
	_ = base
	hostPort := regexp.MustCompile(`ws://([^/]+)/`).FindStringSubmatch(browserWS)
	if hostPort == nil {
		cleanup()
		return "", nil, fmt.Errorf("unparsable devtools url %q", browserWS)
	}
	pageWS, err := firstPageWS(ctx, "http://"+hostPort[1]+"/json/list", e.timeout())
	if err != nil {
		cleanup()
		return "", nil, err
	}
	return pageWS, cleanup, nil
}
// firstPageWS asks the Chrome HTTP endpoint for the page target's websocket URL.
func firstPageWS(ctx context.Context, listURL string, timeout time.Duration) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	if err != nil {
		return "", err
	}
	client := &http.Client{Timeout: timeout}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var targets []struct {
		Type                 string `json:"type"`
		WebSocketDebuggerURL string `json:"webSocketDebuggerUrl"`
	}
	if err := json.Unmarshal(body, &targets); err != nil {
		return "", fmt.Errorf("target list: %w", err)
	}
	for _, t := range targets {
		if t.Type == "page" && t.WebSocketDebuggerURL != "" {
			return t.WebSocketDebuggerURL, nil
		}
	}
	return "", fmt.Errorf("no page target in %s", listURL)
}

// CaptureProof is the generic capture: it loads `beforeURL`, screenshots it,
// evaluates `assertion` (a JS boolean expression) on it as the baseline, then
// loads `afterURL`, evaluates the same assertion, and screenshots again.
//
// A finding is `Executed` only when the assertion flips from false to true —
// i.e. the payload changed live page state, which a status-code check cannot
// show. The two PNGs are written to OutputDir and hashed.
func (e *EvidenceEngine) CaptureProof(ctx context.Context, slug, beforeURL, afterURL, assertion string) (*EvidenceResult, error) {
	if err := os.MkdirAll(e.OutputDir, 0o755); err != nil {
		return nil, err
	}
	cctx, cancel := context.WithTimeout(ctx, e.timeout())
	defer cancel()

	pageWS, cleanup, err := e.launch(cctx)
	if err != nil {
		return &EvidenceResult{TargetURL: afterURL, Assertion: assertion, Error: err.Error()}, err
	}
	defer cleanup()

	conn, err := dialCDP(cctx, pageWS)
	if err != nil {
		return &EvidenceResult{TargetURL: afterURL, Assertion: assertion, Error: err.Error()}, err
	}
	defer conn.ws.Close()

	if _, err := conn.send(cctx, "Page.enable", map[string]any{}); err != nil {
		return &EvidenceResult{TargetURL: afterURL, Assertion: assertion, Error: err.Error()}, err
	}
	if _, err := conn.send(cctx, "Runtime.enable", map[string]any{}); err != nil {
		return &EvidenceResult{TargetURL: afterURL, Assertion: assertion, Error: err.Error()}, err
	}
	if len(e.Cookies) > 0 {
		if _, err := conn.send(cctx, "Network.enable", map[string]any{}); err != nil {
			return &EvidenceResult{TargetURL: afterURL, Assertion: assertion, Error: err.Error()}, err
		}
		for name, value := range e.Cookies {
			params := map[string]any{
				"name": name, "value": value, "path": "/",
				"secure": false, "httpOnly": false,
			}
			if e.CookieDomain != "" {
				params["domain"] = e.CookieDomain
			}
			if _, err := conn.send(cctx, "Network.setCookie", params); err != nil {
				return &EvidenceResult{TargetURL: afterURL, Assertion: assertion,
					Error: "setCookie " + name + ": " + err.Error()}, err
			}
		}
	}

	res := &EvidenceResult{TargetURL: afterURL, Assertion: assertion}

	// ---- BEFORE ----
	if _, err := conn.send(cctx, "Page.navigate", map[string]any{"url": beforeURL}); err != nil {
		res.Error = "navigate(before): " + err.Error()
		return res, err
	}
	if !waitForLoad(cctx, conn, beforeURL, 20*time.Second) {
		settle(cctx, 800*time.Millisecond)
	}
	beforeVal, _ := evalBool(cctx, conn, assertion)
	res.BeforeValue = beforeVal
	bPath := filepath.Join(e.OutputDir, slug+"-before.png")
	bHash, bErr := captureScreenshot(cctx, conn, bPath)
	if bErr == nil {
		res.BeforePath, res.BeforeHash = bPath, bHash
	}

	// ---- AFTER ----
	if _, err := conn.send(cctx, "Page.navigate", map[string]any{"url": afterURL}); err != nil {
		res.Error = "navigate(after): " + err.Error()
		return res, err
	}
	if !waitForLoad(cctx, conn, afterURL, 25*time.Second) {
		settle(cctx, 1200*time.Millisecond)
	}
	afterVal, evalErr := evalBool(cctx, conn, assertion)
	if evalErr != nil {
		res.Error = "assert: " + evalErr.Error()
	}
	res.AfterValue = afterVal
	aPath := filepath.Join(e.OutputDir, slug+"-after.png")
	aHash, aErr := captureScreenshot(cctx, conn, aPath)
	if aErr == nil {
		res.AfterPath, res.AfterHash = aPath, aHash
	}

	// Executed means the assertion is true in the after state and was not true before.
	res.Executed = afterVal == "true" && beforeVal != "true"
	return res, nil
}

// waitForLoad polls until the document reports complete and its location matches
// want, or the context expires. A fixed sleep is not enough: a slow origin makes
// the assertion run against the previous document, which reads as "payload did
// not execute" and hides a real finding.
func waitForLoad(ctx context.Context, conn *cdpConn, want string, max time.Duration) bool {
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		js := `(() => { try { return document.readyState + "|" + location.href } catch(e){ return "err" } })()`
		raw, err := conn.send(ctx, "Runtime.evaluate", map[string]any{
			"expression": js, "returnByValue": true,
		})
		if err == nil {
			var out struct {
				Result struct {
					Value any `json:"value"`
				} `json:"result"`
			}
			if json.Unmarshal(raw, &out) == nil {
				got := fmt.Sprintf("%v", out.Result.Value)
				if strings.HasPrefix(got, "complete|") {
					// compare on the path+query, which is what we control
					if want == "" || strings.Contains(got, pathAndQuery(want)) {
						return true
					}
				}
			}
		}
		select {
		case <-time.After(150 * time.Millisecond):
		case <-ctx.Done():
			return false
		}
	}
	return false
}

// pathAndQuery strips scheme+host so the comparison survives URL normalisation.
func pathAndQuery(rawurl string) string {
	if i := strings.Index(rawurl, "://"); i >= 0 {
		rest := rawurl[i+3:]
		if j := strings.Index(rest, "/"); j >= 0 {
			return rest[j:]
		}
		return rest
	}
	return rawurl
}

// settle waits for navigation to paint without depending on lifecycle events,
// which vary between Chrome builds. Short and bounded on purpose.
func settle(ctx context.Context, d time.Duration) {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
	case <-ctx.Done():
	}
}

// evalBool runs a JS expression and returns "true"/"false" as a string.
func evalBool(ctx context.Context, conn *cdpConn, expr string) (string, error) {
	js := fmt.Sprintf(`(() => { try { return String(!!(%s)); } catch (e) { return "error:"+e.message; } })()`, expr)
	raw, err := conn.send(ctx, "Runtime.evaluate", map[string]any{
		"expression":    js,
		"returnByValue": true,
		"awaitPromise":  false,
	})
	if err != nil {
		return "", err
	}
	var out struct {
		Result struct {
			Type  string `json:"type"`
			Value any    `json:"value"`
		} `json:"result"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	return fmt.Sprintf("%v", out.Result.Value), nil
}

// captureScreenshot writes a PNG and returns its sha256.
func captureScreenshot(ctx context.Context, conn *cdpConn, path string) (string, error) {
	raw, err := conn.send(ctx, "Page.captureScreenshot", map[string]any{
		"format": "png", "captureBeyondViewport": true,
	})
	if err != nil {
		return "", err
	}
	var out struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", err
	}
	png, err := base64.StdEncoding.DecodeString(out.Data)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(path, png, 0o644); err != nil {
		return "", err
	}
	sum := sha256.Sum256(png)
	return hex.EncodeToString(sum[:]), nil
}

// CaptureXSSProof is the XSS-shaped wrapper. The caller supplies the payload and
// the JS marker the payload sets when it runs (e.g. `window.__xss === 1`).
// beforeURL is the same page without the payload so the baseline assertion is
// meaningful; when empty, the origin path of afterURL is used without query.
func (e *EvidenceEngine) CaptureXSSProof(ctx context.Context, slug, beforeURL, afterURL, payload, marker string) (*EvidenceResult, error) {
	if strings.TrimSpace(marker) == "" {
		marker = `window.__xss_proof === true`
	}
	res, err := e.CaptureProof(ctx, slug, beforeURL, afterURL, marker)
	if res != nil {
		res.Payload = payload
	}
	return res, err
}
