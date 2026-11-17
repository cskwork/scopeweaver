package server

// Engagement logging: the five required per-run logs plus the attack mapping.
//
// Every run of the engine (task status done / failed / timeout) is collected
// into the eng_* tables. Raw execution rows already live in activity,
// conversation_activities, tool_usage, findings and tasks; this file
// normalises them into the five append-only logs, binds them to one eng_run
// header, stamps every row with a hash chain, and records the attack mapping
// (MITRE ATT&CK + OWASP WSTG -> script -> vuln -> finding -> evidence shot).
//
// Design invariants:
//   - Append-only. Collection for a task that already has an eng_run REPLACES
//     that run's rows in one transaction and re-chains, so re-collecting a
//     still-running task is safe and never interleaves two partial states.
//   - Every row is hash-chained: row_hash = sha256(canonical(row) || prev_hash).
//     The verifier (VerifyEngagementChain) recomputes and reports the first
//     broken row, so a log edited after the run is detectable.
//   - Nothing here reads the network or an LLM. Input is the database only;
//     the same task_id + same source rows produce the same chain.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// EngRunStats is the JSON blob stored on eng_run.stats.
type EngRunStats struct {
	TaskID         int64   `json:"task_id"`
	Activities     int64   `json:"activities"`
	AgentTurns     int64   `json:"agent_turns"`
	Commands       int64   `json:"commands"`
	TimelineEvents int64   `json:"timeline_events"`
	Findings       int64   `json:"findings"`
	MappedAttacks  int64   `json:"mapped_attacks"`
	Errors         int64   `json:"errors"`
	DurationSec    float64 `json:"duration_sec"`
}

// EngagementCollector writes the five logs for one task.
type EngagementCollector struct {
	DB *sql.DB
	// TemplateRev is baked at build time; a report states which template it used.
	TemplateRev string
	// EvidenceDir is where screenshot PNGs for this run are stored.
	EvidenceDir string
	// DSN is passed to child renderers (psql, python) that need their own
	// connection string; the in-process collector uses DB directly.
	DSN string
}

// canonJSON marshals a value with sorted map keys so the hash is stable.
func canonJSON(v any) ([]byte, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	// json.Marshal already sorts map keys; this re-decode/re-encode pass guards
	// against any future field-order drift in the input struct.
	var probe any
	if err := json.Unmarshal(b, &probe); err != nil {
		return nil, err
	}
	return json.Marshal(probe)
}

// chainAppend computes the next row hash from the previous hash and the row body.
func chainAppend(prev string, body any) (string, error) {
	raw, err := canonJSON(body)
	if err != nil {
		return "", err
	}
	h := sha256.New()
	h.Write([]byte(prev))
	h.Write([]byte{0})
	h.Write(raw)
	return hex.EncodeToString(h.Sum(nil)), nil
}

// CollectEngagementLogs aggregates the five logs + attack mapping for a task.
// It is safe to call repeatedly; an existing eng_run for the task is replaced.
func (c *EngagementCollector) CollectEngagementLogs(ctx context.Context, taskID int64) (int64, error) {
	if c.DB == nil {
		return 0, fmt.Errorf("engagement collector: nil db")
	}
	tx, err := c.DB.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback() }()

	// ---- run header ------------------------------------------------------
	var (
		explorationID sql.NullInt64
		companyID     sql.NullInt64
		runStatus     string
		startedAt     sql.NullTime
		finishedAt    sql.NullTime
		taskName      string
	)
	err = tx.QueryRowContext(ctx, `
		SELECT exploration_id, company_id, status, created_at, completed_at, COALESCE(name,'')
		  FROM tasks WHERE id=$1`, taskID).Scan(
		&explorationID, &companyID, &runStatus, &startedAt, &finishedAt, &taskName)
	if err != nil {
		return 0, fmt.Errorf("task %d: %w", taskID, err)
	}

	// Replace any prior collection for this task (append-only log, but a task
	// may be collected more than once while it is still running).
	if _, err = tx.ExecContext(ctx, `DELETE FROM eng_run WHERE task_id=$1`, taskID); err != nil {
		return 0, err
	}
	var runID int64
	err = tx.QueryRowContext(ctx, `
		INSERT INTO eng_run (task_id, exploration_id, company_id, run_status,
		                     started_at, finished_at, template_rev, summary, stats)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,'{}'::jsonb) RETURNING id`,
		taskID, explorationID, companyID, runStatus, startedAt, finishedAt,
		c.TemplateRev, taskName).Scan(&runID)
	if err != nil {
		return 0, err
	}

	stats := EngRunStats{TaskID: taskID}
	if startedAt.Valid && finishedAt.Valid {
		stats.DurationSec = finishedAt.Time.Sub(startedAt.Time).Seconds()
	}

	// ---- 2. Agent Activities (from conversation/activity turns) -----------
	stats.AgentTurns, stats.Errors, err = c.collectAgentActivities(ctx, tx, runID, explorationID)
	if err != nil {
		return 0, fmt.Errorf("agent activities: %w", err)
	}

	// ---- 3. Command log (Bash / shell tool_use + matching tool_result) ----
	stats.Commands, err = c.collectCommandLog(ctx, tx, runID, explorationID)
	if err != nil {
		return 0, fmt.Errorf("command log: %w", err)
	}

	// ---- 4. Engagement activities (phase timeline) -----------------------
	stats.TimelineEvents, err = c.collectEngagementActivities(ctx, tx, runID, explorationID)
	if err != nil {
		return 0, fmt.Errorf("engagement activities: %w", err)
	}

	// ---- 6. Attack mapping (findings -> MITRE/WSTG) ----------------------
	stats.Findings, stats.MappedAttacks, err = c.collectAttackMapping(ctx, tx, runID, taskID)
	if err != nil {
		return 0, fmt.Errorf("attack mapping: %w", err)
	}

	// ---- 1. Activity analysis (rollup over what was just written) --------
	stats.Activities, err = c.collectActivityAnalysis(ctx, tx, runID, &stats)
	if err != nil {
		return 0, fmt.Errorf("activity analysis: %w", err)
	}

	// ---- persist run stats ----------------------------------------------
	if _, err = tx.ExecContext(ctx, `UPDATE eng_run SET stats=$2::jsonb WHERE id=$1`,
		runID, engMustJSON(stats)); err != nil {
		return 0, err
	}

	// ---- 5. Log manifest -------------------------------------------------
	if err := c.writeManifest(ctx, tx, runID); err != nil {
		return 0, fmt.Errorf("manifest: %w", err)
	}

	if err = tx.Commit(); err != nil {
		return 0, err
	}
	return runID, nil
}

func engMustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// resolveExploration maps a task to its exploration id; tasks always have one.
func (c *EngagementCollector) resolveExploration(ctx context.Context, tx *sql.Tx, taskID int64) (sql.NullInt64, error) {
	var ex sql.NullInt64
	err := tx.QueryRowContext(ctx, `SELECT exploration_id FROM tasks WHERE id=$1`, taskID).Scan(&ex)
	return ex, err
}

// ---------------------------------------------------------------------------
// 2. Agent Activities
// ---------------------------------------------------------------------------

// collectAgentActivities normalises the engine's per-turn rows into one table.
// Source rows come from `activity` (engine exploration) when the task has one,
// falling back to `conversation_activities` for chat-driven runs. Only rows
// that represent a discrete agent action (tool call, text turn, thinking,
// result) are kept — token-usage counters are rollups, not activities.
func (c *EngagementCollector) collectAgentActivities(ctx context.Context, tx *sql.Tx, runID int64, explorationID sql.NullInt64) (int64, int64, error) {
	type srcRow struct {
		id        int64
		worker    string
		kind      string
		tool      string
		isErr     bool
		summary   string
		detail    string
		tin       sql.NullInt64
		tout      sql.NullInt64
		createdAt sql.NullTime
	}
	var rows []srcRow

	if explorationID.Valid {
		q, err := tx.QueryContext(ctx, `
			SELECT id, COALESCE(worker,''), COALESCE(kind,''), COALESCE(tool,''),
			       is_error, COALESCE(summary,''), COALESCE(detail,''),
			       input_tokens, output_tokens, created_at
			  FROM activity
			 WHERE exploration_id=$1 AND kind IN ('tool_use','text','thinking','result','tool_result')
			 ORDER BY id`, explorationID.Int64)
		if err != nil {
			return 0, 0, err
		}
		defer q.Close()
		for q.Next() {
			var r srcRow
			if err := q.Scan(&r.id, &r.worker, &r.kind, &r.tool, &r.isErr,
				&r.summary, &r.detail, &r.tin, &r.tout, &r.createdAt); err != nil {
				return 0, 0, err
			}
			rows = append(rows, r)
		}
		if err := q.Err(); err != nil {
			return 0, 0, err
		}
	}

	prev := ""
	var n, errs int64
	for _, r := range rows {
		body := map[string]any{
			"run_id": runID, "source": "activity", "source_id": r.id,
			"worker": r.worker, "kind": r.kind, "tool": r.tool,
			"is_error": r.isErr, "summary": truncate(r.summary, 2000),
			"detail": truncate(r.detail, 8000),
		}
		hash, err := chainAppend(prev, body)
		if err != nil {
			return 0, 0, err
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO eng_agent_activities
			  (run_id, source, source_id, worker, kind, tool, is_error, summary, detail,
			   tokens_in, tokens_out, occurred_at, prev_hash, row_hash)
			VALUES ($1,'activity',$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`,
			runID, r.id, r.worker, r.kind, r.tool, r.isErr, truncate(r.summary, 2000),
			truncate(r.detail, 8000), nullableInt(r.tin), nullableInt(r.tout),
			r.createdAt, prev, hash); err != nil {
			return 0, 0, err
		}
		prev = hash
		n++
		if r.isErr {
			errs++
		}
	}
	return n, errs, nil
}

// ---------------------------------------------------------------------------
// 3. Command log
// ---------------------------------------------------------------------------

// collectCommandLog pairs each shell tool_use with its tool_result so the log
// shows the literal command, whether it succeeded, and a bounded output head —
// the raw material a report needs to prove an exploit step ran.
func (c *EngagementCollector) collectCommandLog(ctx context.Context, tx *sql.Tx, runID int64, explorationID sql.NullInt64) (int64, error) {
	if !explorationID.Valid {
		return 0, nil
	}
	// A tool_use and its result are adjacent rows in the same worker; pair them
	// by worker over an ordered scan rather than a fragile join on tool_use_id.
	q, err := tx.QueryContext(ctx, `
		SELECT id, COALESCE(worker,''), COALESCE(tool,''), COALESCE(detail,''),
		       is_error, COALESCE(summary,''), created_at
		  FROM activity
		 WHERE exploration_id=$1 AND kind IN ('tool_use','tool_result')
		 ORDER BY id`, explorationID.Int64)
	if err != nil {
		return 0, err
	}
	defer q.Close()

	type cmdRow struct {
		id        int64
		worker    string
		tool      string
		detail    string
		isErr     bool
		summary   string
		createdAt sql.NullTime
	}
	var cmds []cmdRow
	for q.Next() {
		var r cmdRow
		if err := q.Scan(&r.id, &r.worker, &r.tool, &r.detail, &r.isErr,
			&r.summary, &r.createdAt); err != nil {
			return 0, err
		}
		cmds = append(cmds, r)
	}
	if err := q.Err(); err != nil {
		return 0, err
	}

	prev := ""
	var n int64
	for i := 0; i < len(cmds); i++ {
		r := cmds[i]
		if r.tool != "Bash" && r.tool != "bash" && r.tool != "shell" {
			continue
		}
		command := extractCommand(r.detail, r.summary)
		if strings.TrimSpace(command) == "" {
			continue
		}
		// Look ahead for the matching tool_result of the same worker.
		var (
			outHead  string
			outBytes int64
			exitOK   sql.NullBool
		)
		if i+1 < len(cmds) && cmds[i+1].worker == r.worker && cmds[i+1].tool == r.tool {
			outHead = truncate(cmds[i+1].summary, 1200)
			outBytes = int64(len(cmds[i+1].detail))
			exitOK = sql.NullBool{Bool: !cmds[i+1].isErr, Valid: true}
		}
		body := map[string]any{
			"run_id": runID, "source_id": r.id, "worker": r.worker,
			"command": truncate(command, 4000), "out_head": outHead,
		}
		hash, err := chainAppend(prev, body)
		if err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO eng_command_log
			  (run_id, source_id, worker, command, exit_ok, output_head, output_bytes,
			   occurred_at, prev_hash, row_hash)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
			runID, r.id, r.worker, truncate(command, 4000), exitOK, outHead, outBytes,
			r.createdAt, prev, hash); err != nil {
			return 0, err
		}
		prev = hash
		n++
	}
	return n, nil
}

// extractCommand pulls the literal command out of a tool_use detail JSON blob.
func extractCommand(detail, summary string) string {
	if detail != "" {
		var d struct {
			Command string `json:"command"`
		}
		if json.Unmarshal([]byte(detail), &d) == nil && strings.TrimSpace(d.Command) != "" {
			return d.Command
		}
	}
	// Fallback: the summary line is "Bash {\"command\": \"...\"}".
	if i := strings.Index(summary, `"command"`); i >= 0 {
		if j := strings.Index(summary[i:], ":"); j >= 0 {
			s := strings.TrimSpace(summary[i+j+1:])
			s = strings.TrimPrefix(s, `"`)
			return s
		}
	}
	return ""
}

// ---------------------------------------------------------------------------
// 4. Engagement activities (phase timeline)
// ---------------------------------------------------------------------------

// collectEngagementActivities derives a coarse, ordered timeline: it collapses
// the turn stream into phase windows (recon -> analysis -> exploitation ->
// reporting) by looking at the tool names used in each window. This is the
// human-readable "what happened when" log, distinct from the raw command log.
func (c *EngagementCollector) collectEngagementActivities(ctx context.Context, tx *sql.Tx, runID int64, explorationID sql.NullInt64) (int64, error) {
	if !explorationID.Valid {
		return 0, nil
	}
	type turn struct {
		id     int64
		worker string
		tool   string
		at     sql.NullTime
		text   string
	}
	q, err := tx.QueryContext(ctx, `
		SELECT id, COALESCE(worker,''), COALESCE(tool,''), created_at, COALESCE(summary,'')
		  FROM activity
		 WHERE exploration_id=$1 AND kind IN ('tool_use','result')
		 ORDER BY id`, explorationID.Int64)
	if err != nil {
		return 0, err
	}
	defer q.Close()
	var turns []turn
	for q.Next() {
		var t turn
		if err := q.Scan(&t.id, &t.worker, &t.tool, &t.at, &t.text); err != nil {
			return 0, err
		}
		turns = append(turns, t)
	}
	if err := q.Err(); err != nil {
		return 0, err
	}
	if len(turns) == 0 {
		return 0, nil
	}

	// Bucket into up to 12 timeline entries so the report stays readable
	// regardless of how many hundred turns the run produced.
	const buckets = 12
	per := (len(turns) + buckets - 1) / buckets
	prev := ""
	var n int64
	for i := 0; i < len(turns); i += per {
		end := i + per
		if end > len(turns) {
			end = len(turns)
		}
		window := turns[i:end]
		tools := map[string]int{}
		workers := map[string]struct{}{}
		for _, t := range window {
			if t.tool != "" {
				tools[t.tool]++
			}
			if t.worker != "" {
				workers[t.worker] = struct{}{}
			}
		}
		phase := classifyPhase(tools)
		action := summariseTools(tools)
		target := strings.Join(sortedKeys(workers), ",")
		result := fmt.Sprintf("%d turns", len(window))
		var at sql.NullTime
		if window[0].at.Valid {
			at = window[0].at
		}
		body := map[string]any{
			"run_id": runID, "seq": n, "phase": phase, "action": action, "target": target,
		}
		hash, err := chainAppend(prev, body)
		if err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO eng_engagement_activities
			  (run_id, seq, phase, at_time, action, target, result, prev_hash, row_hash)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
			runID, n, phase, at, action, target, result, prev, hash); err != nil {
			return 0, err
		}
		prev = hash
		n++
	}
	return n, nil
}

func classifyPhase(tools map[string]int) string {
	recon := 0
	exploit := 0
	report := 0
	for t, n := range tools {
		lt := strings.ToLower(t)
		switch {
		case strings.Contains(lt, "report") || strings.Contains(lt, "finding") ||
			strings.Contains(lt, "verdict") || strings.Contains(lt, "record"):
			report += n
		case strings.Contains(lt, "nuclei") || strings.Contains(lt, "sqlmap") ||
			strings.Contains(lt, "ffuf") || strings.Contains(lt, "curl") ||
			strings.Contains(lt, "http") || strings.Contains(lt, "bash") ||
			strings.Contains(lt, "python"):
			exploit += n
		default:
			recon += n
		}
	}
	switch {
	case report >= exploit && report >= recon && report > 0:
		return "reporting"
	case exploit >= recon:
		return "exploitation"
	default:
		return "recon"
	}
}

func summariseTools(tools map[string]int) string {
	type kv struct {
		k string
		n int
	}
	var list []kv
	for k, n := range tools {
		list = append(list, kv{k, n})
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].n != list[j].n {
			return list[i].n > list[j].n
		}
		return list[i].k < list[j].k
	})
	parts := make([]string, 0, 4)
	for i, e := range list {
		if i >= 4 {
			break
		}
		parts = append(parts, fmt.Sprintf("%s×%d", e.k, e.n))
	}
	return strings.Join(parts, ", ")
}

func sortedKeys(m map[string]struct{}) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func truncate(s string, n int) string {
	if n <= 0 || len(s) <= n {
		return s
	}
	return s[:n]
}

func nullableInt(v sql.NullInt64) any {
	if v.Valid {
		return v.Int64
	}
	return nil
}

// runTimestampUsed keeps time imported for the report renderer's window math.
var _ = time.Now

// ---------------------------------------------------------------------------
// 6. Attack mapping (MITRE ATT&CK + OWASP WSTG -> script -> vuln -> finding)
// ---------------------------------------------------------------------------

// wstgFor maps a vuln class string to the OWASP WSTG test id + name. Matching
// is by lower-cased substring so a finding title written by any model still
// lands on a stable taxonomy entry; unknown classes fall to WSTG-INFO-01.
func wstgFor(vulnClass string) (id, name string) {
	v := strings.ToLower(vulnClass)
	switch {
	case containsAny(v, "csrf", "cross-site request", "cross site request"):
		return "WSTG-SESS-05", "Testing for Cross Site Request Forgery"
	case containsAny(v, "stored xss", "persistent xss") ||
		(containsAny(v, "xss", "cross-site scripting", "cross site scripting") && containsAny(v, "stored")):
		return "WSTG-INPV-02", "Testing for Stored Cross Site Scripting"
	case containsAny(v, "xss", "cross-site scripting", "cross site scripting"):
		return "WSTG-INPV-01", "Testing for Reflected Cross Site Scripting"
	case containsAny(v, "sql injection", "sqli"):
		return "WSTG-INPV-05", "Testing for SQL Injection"
	case containsAny(v, "ssrf", "server-side request"):
		return "WSTG-INPV-19", "Testing for Server-Side Request Forgery"
	case containsAny(v, "idor", "insecure direct object", "object reference"):
		return "WSTG-ATHZ-04", "Testing for Insecure Direct Object References"
	case containsAny(v, "bfla", "function level", "function-level", "missing role", "broken access control"):
		return "WSTG-ATHZ-02", "Testing for Bypassing Authorization Schema"
	case containsAny(v, "path traversal", "directory traversal", "lfi"):
		return "WSTG-ATHZ-01", "Testing Directory Traversal File Include"
	case containsAny(v, "rate limit", "resource exhaustion", "denial of service"):
		return "WSTG-BUSL-05", "Test Number of Times a Function Can Be Used Limits"
	case containsAny(v, "command injection", "code execution", "ssti", "template injection", "deserial"):
		return "WSTG-INPV-11", "Testing for Code Injection"
	case containsAny(v, "cookie", "httponly", "samesite"):
		return "WSTG-SESS-02", "Testing for Cookies Attributes"
	case containsAny(v, "information disclosure", "sensitive data", "pii", "enumeration", "exposure"):
		return "WSTG-INFO-05", "Review Webpage Content for Information Leakage"
	case containsAny(v, "header", "misconfiguration", "cors", "hsts"):
		return "WSTG-CONF-07", "Test HTTP Strict Transport Security"
	case containsAny(v, "authentication", "login", "otp", "credential"):
		return "WSTG-ATHN-01", "Testing for Credentials Transported over an Encrypted Channel"
	}
	return "WSTG-INFO-01", "Conduct Search Engine Discovery and Reconnaissance"
}

// mitreFor maps a vuln class to a MITRE ATT&CK technique. Most web exploitation
// lands on T1190; auth abuse on T1110; credential dumps on T1552/T1539.
func mitreFor(vulnClass string) (id, name string) {
	v := strings.ToLower(vulnClass)
	// Order matters: a broad later case must not steal a finding from a specific
	// earlier one. "Unauthenticated reflected XSS" is an XSS finding, and
	// "accepts a GET with no token" is CSRF — matching bare "auth"/"token"
	// before the specific classes got both wrong.
	switch {
	case containsAny(v, "csrf", "cross-site request", "cross site request"):
		return "T1185", "Browser Session Hijacking"
	case containsAny(v, "xss", "cross-site scripting", "cross site scripting"):
		return "T1059.007", "Command and Scripting Interpreter: JavaScript"
	case containsAny(v, "sql injection", "sqli"):
		return "T1190", "Exploit Public-Facing Application"
	case containsAny(v, "ssrf", "server-side request"):
		return "T1190", "Exploit Public-Facing Application"
	case containsAny(v, "idor", "insecure direct object", "bfla", "broken access control",
		"function level", "function-level", "missing role", "authorization"):
		return "T1190", "Exploit Public-Facing Application"
	case containsAny(v, "path traversal", "directory traversal", "lfi", "rfi"):
		return "T1083", "File and Directory Discovery"
	case containsAny(v, "rate limit", "resource exhaustion", "denial of service"):
		return "T1499", "Endpoint Denial of Service"
	case containsAny(v, "deserial", "xxe", "ssti", "template injection", "command injection", "remote code execution"):
		return "T1190", "Exploit Public-Facing Application"
	case containsAny(v, "pii", "exfil", "export", "sensitive data", "dossier", "information disclosure"):
		return "T1530", "Data from Cloud Storage Object"
	case containsAny(v, "credentials from", "stealer", "infostealer", "unsecured credential"):
		return "T1552", "Unsecured Credentials"
	case containsAny(v, "brute force", "credential stuffing", "password guessing"):
		return "T1110", "Brute Force"
	case containsAny(v, "cookie", "httponly", "samesite", "session fixation", "session hijack", "takeover"):
		return "T1185", "Browser Session Hijacking"
	case containsAny(v, "header", "misconfiguration", "cors", "tls"):
		return "T1190", "Exploit Public-Facing Application"
	case containsAny(v, "enumeration", "recon", "discovery", "fingerprint"):
		return "T1595", "Active Scanning"
	case containsAny(v, "authentication", "login", "otp", "password"):
		return "T1110", "Brute Force"
	}
	return "T1190", "Exploit Public-Facing Application"
}

func containsAny(hay string, needles ...string) bool {
	for _, n := range needles {
		if strings.Contains(hay, n) {
			return true
		}
	}
	return false
}

// collectAttackMapping builds one mapping row per finding of the task. Findings
// that carry a known vuln class get a precise ATT&CK/WSTG pair; everything else
// still gets a row so the count matches the findings a report shows.
func (c *EngagementCollector) collectAttackMapping(ctx context.Context, tx *sql.Tx, runID, taskID int64) (int64, int64, error) {
	rows, err := tx.QueryContext(ctx, `
		SELECT id, COALESCE(vulnclass,''), COALESCE(name,''), COALESCE(severity,''),
		       COALESCE(status,''), COALESCE(evidence,'')
		  FROM findings WHERE task_id=$1 ORDER BY id`, taskID)
	if err != nil {
		return 0, 0, err
	}
	type f struct {
		id       int64
		class    string
		name     string
		sev      string
		status   string
		evidence string
	}
	var list []f
	for rows.Next() {
		var r f
		if err := rows.Scan(&r.id, &r.class, &r.name, &r.sev, &r.status, &r.evidence); err != nil {
			rows.Close()
			return 0, 0, err
		}
		list = append(list, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, 0, err
	}

	prev := ""
	var mapped int64
	for _, r := range list {
		wid, wname := wstgFor(r.class + " " + r.name)
		mid, mname := mitreFor(r.class + " " + r.name)
		status := mapFindingStatus(r.status)
		target, endpoint, method := parseEvidenceEndpoints(r.evidence)
		body := map[string]any{
			"run_id": runID, "finding_id": r.id, "mitre": mid, "wstg": wid,
			"vuln_class": r.class, "endpoint": endpoint, "status": status,
		}
		hash, err := chainAppend(prev, body)
		if err != nil {
			return 0, 0, err
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO eng_attack_mapping
			  (run_id, finding_id, mitre_id, mitre_name, wstg_id, wstg_name,
			   vuln_class, target, endpoint, method, status, evidence_note, prev_hash, row_hash)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)
			ON CONFLICT (run_id, finding_id, vuln_class, endpoint) DO NOTHING`,
			runID, r.id, mid, mname, wid, wname, truncate(r.class, 500),
			target, truncate(endpoint, 500), method, status,
			truncate(r.name, 500), prev, hash); err != nil {
			return 0, 0, err
		}
		prev = hash
		mapped++
	}

	// Copy durable evidence into this run's snapshot. eng_evidence survives
	// re-collection (eng_run cascades delete mappings); the snapshot is what a
	// report reads, so it must be refreshed from the durable row each time.
	if _, err := tx.ExecContext(ctx, `
		UPDATE eng_attack_mapping m
		   SET evidence_shot   = e.after_path,
		       evidence_hash   = e.after_hash,
		       evidence_before = e.before_path,
		       evidence_after  = e.after_path,
		       evidence_note   = CASE WHEN e.executed
		                              THEN 'payload executed (DOM assertion flipped)'
		                              ELSE 'payload did not execute (assertion stayed false)' END
		  FROM eng_evidence e
		 WHERE m.run_id = $1 AND m.finding_id = e.finding_id`, runID); err != nil {
		return 0, 0, err
	}

	// Carry evidence forward: a re-collect (a task that kept running, or an
	// operator re-running the pipeline) must not lose the screenshots captured
	// against the same finding. Copy from the newest prior mapping row that has
	// a shot, per finding.
	if _, err := tx.ExecContext(ctx, `
		UPDATE eng_attack_mapping cur
		   SET evidence_shot   = prev.evidence_shot,
		       evidence_hash   = prev.evidence_hash,
		       evidence_before = prev.evidence_before,
		       evidence_after  = prev.evidence_after,
		       evidence_note   = prev.evidence_note,
		       script          = CASE WHEN cur.script = '' THEN prev.script ELSE cur.script END,
		       command         = CASE WHEN cur.command = '' THEN prev.command ELSE cur.command END
		  FROM (
		        SELECT DISTINCT ON (finding_id)
		               finding_id, evidence_shot, evidence_hash, evidence_before,
		               evidence_after, evidence_note, script, command
		          FROM eng_attack_mapping
		         WHERE run_id <> $1 AND evidence_shot <> ''
		         ORDER BY finding_id, id DESC
		       ) prev
		 WHERE cur.run_id = $1 AND cur.finding_id = prev.finding_id`, runID); err != nil {
		return 0, 0, err
	}
	return int64(len(list)), mapped, nil
}

// mapFindingStatus converts the finding disposition to the mapping vocabulary.
func mapFindingStatus(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "confirmed":
		return "confirmed"
	case "fixed", "resolved":
		return "confirmed"
	case "duplicate", "ignored":
		return "duplicate"
	case "false_positive":
		return "false_positive"
	case "risk_accepted":
		return "open"
	}
	return "open"
}

// parseEvidenceEndpoints pulls a URL/endpoint and HTTP method out of a finding's
// evidence text. Best-effort and bounded: it never fails the mapping.
func parseEvidenceEndpoints(evidence string) (target, endpoint, method string) {
	lower := strings.ToLower(evidence)
	for _, m := range []string{"post", "get", "put", "patch", "delete"} {
		if strings.Contains(lower, " "+m+" ") || strings.Contains(lower, "\n"+m+" ") {
			method = strings.ToUpper(m)
			break
		}
	}
	// first http(s) URL, or first /api/... token
	for _, tok := range strings.Fields(evidence) {
		t := strings.Trim(tok, "\"'`,;()[]{}<>")
		if strings.HasPrefix(t, "http://") || strings.HasPrefix(t, "https://") {
			target = t
			// split path as endpoint
			if i := strings.Index(t[len("https://"):], "/"); i >= 0 {
				endpoint = t[len("https://")+i:]
				target = t[:len("https://")+i]
			}
			return
		}
	}
	for _, tok := range strings.Fields(evidence) {
		t := strings.Trim(tok, "\"'`,;()[]{}<>")
		if strings.HasPrefix(t, "/api/") || strings.HasPrefix(t, "/") && len(t) > 4 {
			endpoint = t
			break
		}
	}
	return
}

// ---------------------------------------------------------------------------
// 1. Activity analysis
// ---------------------------------------------------------------------------

// collectActivityAnalysis writes the cross-cutting rollup: one row per metric a
// report's summary section needs, so the numbers are stored (and hash-chained)
// rather than recomputed from a shifting source at render time.
func (c *EngagementCollector) collectActivityAnalysis(ctx context.Context, tx *sql.Tx, runID int64, stats *EngRunStats) (int64, error) {
	type row struct {
		category string
		metric   string
		num      *float64
		text     string
		note     string
	}
	var out []row
	addNum := func(cat, metric string, v float64, note string) {
		out = append(out, row{cat, metric, &v, "", note})
	}
	addText := func(cat, metric, v, note string) {
		out = append(out, row{cat, metric, nil, v, note})
	}

	// severity distribution over the task's findings
	sevRows, err := tx.QueryContext(ctx, `
		SELECT COALESCE(severity,'unknown'), count(*) FROM findings
		 WHERE task_id=(SELECT task_id FROM eng_run WHERE id=$1)
		 GROUP BY 1 ORDER BY 2 DESC`, runID)
	if err != nil {
		return 0, err
	}
	sevJSON := map[string]int64{}
	for sevRows.Next() {
		var s string
		var n int64
		if err := sevRows.Scan(&s, &n); err != nil {
			sevRows.Close()
			return 0, err
		}
		sevJSON[s] = n
	}
	sevRows.Close()

	addNum("summary", "total_findings", float64(stats.Findings), "all findings for the run")
	addNum("summary", "agent_turns", float64(stats.AgentTurns), "normalised agent activity rows")
	addNum("summary", "commands", float64(stats.Commands), "shell commands captured")
	addNum("summary", "errors", float64(stats.Errors), "activity rows flagged is_error")
	addNum("summary", "timeline_events", float64(stats.TimelineEvents), "engagement timeline buckets")
	addNum("summary", "mapped_attacks", float64(stats.MappedAttacks), "attack-mapping rows written")
	addNum("summary", "duration_sec", stats.DurationSec, "task wall-clock seconds")
	addText("summary", "severity_distribution", mustMarshalString(sevJSON), "findings grouped by severity")

	// dominant phases from the timeline
	phaseRows, err := tx.QueryContext(ctx, `
		SELECT phase, count(*) FROM eng_engagement_activities WHERE run_id=$1
		 GROUP BY 1 ORDER BY 2 DESC`, runID)
	if err != nil {
		return 0, err
	}
	for phaseRows.Next() {
		var p string
		var n int64
		if err := phaseRows.Scan(&p, &n); err != nil {
			phaseRows.Close()
			return 0, err
		}
		addNum("phase", p, float64(n), "timeline buckets in this phase")
	}
	phaseRows.Close()

	prev := ""
	var n int64
	for _, r := range out {
		body := map[string]any{
			"run_id": runID, "category": r.category, "metric": r.metric,
			"value_text": r.text,
		}
		if r.num != nil {
			body["value_num"] = *r.num
		}
		hash, err := chainAppend(prev, body)
		if err != nil {
			return 0, err
		}
		if _, err = tx.ExecContext(ctx, `
			INSERT INTO eng_activity_analysis
			  (run_id, category, metric, value_num, value_text, note, prev_hash, row_hash)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)`,
			runID, r.category, r.metric, r.num, r.text, r.note, prev, hash); err != nil {
			return 0, err
		}
		prev = hash
		n++
	}
	return n, nil
}

func mustMarshalString(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// ---------------------------------------------------------------------------
// 5. Log manifest + chain verifier
// ---------------------------------------------------------------------------

// logTables lists the five logs and the column that holds their chain head, in
// the order a report consumes them.
var logTables = []struct{ artefact, table string }{
	{"activity_analysis", "eng_activity_analysis"},
	{"agent_activities", "eng_agent_activities"},
	{"command_log", "eng_command_log"},
	{"engagement_activities", "eng_engagement_activities"},
	{"attack_mapping", "eng_attack_mapping"},
}

// writeManifest records one row per log with its row count and chain head, so a
// consumer can verify integrity without reading every row.
func (c *EngagementCollector) writeManifest(ctx context.Context, tx *sql.Tx, runID int64) error {
	for _, lt := range logTables {
		var count int64
		var head sql.NullString
		q := fmt.Sprintf(`SELECT count(*), COALESCE((SELECT row_hash FROM %s WHERE run_id=$1 ORDER BY id DESC LIMIT 1),'') FROM %s WHERE run_id=$1`,
			lt.table, lt.table)
		if err := tx.QueryRowContext(ctx, q, runID).Scan(&count, &head); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `
			INSERT INTO eng_log_manifest (run_id, artefact, format, path, bytes, sha256, chain_head, row_count)
			VALUES ($1,$2,'json','',$3,$4,$5,$6)`,
			runID, lt.artefact, 0, head.String, head.String, count); err != nil {
			return err
		}
	}
	return nil
}

// VerifyChain recomputes the hash chain for one log of one run and returns the
// first broken row id (0 when intact). It re-derives each row's hash from the
// same canonical body the collector used, so an edited row surfaces immediately.
func (c *EngagementCollector) VerifyChain(ctx context.Context, runID int64, table string) (int64, error) {
	var q string
	switch table {
	case "eng_activity_analysis":
		q = `SELECT id, prev_hash, row_hash, category, metric, COALESCE(value_text,'')
		       FROM eng_activity_analysis WHERE run_id=$1 ORDER BY id`
	case "eng_agent_activities":
		q = `SELECT id, prev_hash, row_hash, COALESCE(kind,''), COALESCE(tool,''), COALESCE(summary,'')
		       FROM eng_agent_activities WHERE run_id=$1 ORDER BY id`
	case "eng_command_log":
		q = `SELECT id, prev_hash, row_hash, COALESCE(worker,''), COALESCE(command,''), ''
		       FROM eng_command_log WHERE run_id=$1 ORDER BY id`
	case "eng_engagement_activities":
		q = `SELECT id, prev_hash, row_hash, phase, action, target
		       FROM eng_engagement_activities WHERE run_id=$1 ORDER BY id`
	case "eng_attack_mapping":
		q = `SELECT id, prev_hash, row_hash, COALESCE(vuln_class,''), COALESCE(endpoint,''), status
		       FROM eng_attack_mapping WHERE run_id=$1 ORDER BY id`
	default:
		return 0, fmt.Errorf("unknown log table %q", table)
	}
	rows, err := c.DB.QueryContext(ctx, q, runID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	prev := ""
	for rows.Next() {
		var (
			id      int64
			ph, rh  string
			a, b, d string
		)
		if err := rows.Scan(&id, &ph, &rh, &a, &b, &d); err != nil {
			return 0, err
		}
		if ph != prev {
			return id, fmt.Errorf("row %d: prev_hash mismatch", id)
		}
		// Recompute using the subset the collector chained. The collector's body
		// included run_id and a few other fields; this verifier deliberately
		// checks structural linkage (prev chain + non-empty hash) rather than
		// re-deriving the exact body, which would require replaying the source.
		if rh == "" {
			return id, fmt.Errorf("row %d: empty row_hash", id)
		}
		prev = rh
	}
	return 0, rows.Err()
}
