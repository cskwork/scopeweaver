package server

// Report generation: the tech report (HTML -> PDF/DOCX) and the Sibermate deck.
//
// Everything a report shows is read from the eng_* tables written by
// EngagementCollector plus the findings those rows point at. Nothing is
// re-derived from live target state at render time, so a report generated
// today for a run from last week renders the same numbers it did then.
//
// Artefacts produced per run:
//   report-<slug>.html   self-contained tech report (inline CSS, local images)
//   report-<slug>.pdf    same document printed through headless Chromium
//   report-<slug>.docx   same document converted by pandoc
//   deck-<slug>.pptx     Sibermate-format activity deck
//
// Every artefact is recorded in eng_log_manifest with its sha256 so the
// integrity of a delivered report is checkable later.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// ReportFinding is one finding with its attack mapping and screenshot evidence.
type ReportFinding struct {
	ID         int64
	Severity   string
	Status     string
	Title      string
	VulnClass  string
	Summary    string
	Evidence   string
	MITRE      string
	MITREName  string
	WSTG       string
	WSTGName   string
	Endpoint   string
	Method     string
	Script     string
	Command    string
	MapStatus  string
	ShotPath   string
	ShotHash   string
	ShotBefore string
	ShotAfter  string
	Note       string
}

// ReportTimeline is one engagement-activity bucket.
type ReportTimeline struct {
	Seq    int
	Phase  string
	At     string
	Action string
	Target string
	Result string
}

// ReportCommand is one entry of the command log.
type ReportCommand struct {
	Worker  string
	Command string
	ExitOK  string
	Output  string
	At      string
}

// ReportData is the whole template input.
type ReportData struct {
	RunID       int64
	TaskID      int64
	TaskName    string
	RunStatus   string
	StartedAt   string
	FinishedAt  string
	Duration    string
	TemplateRev string
	GeneratedAt string
	Client      string
	Target      string

	Stats    map[string]string
	Severity []struct {
		Name  string
		Count int
		Pct   float64
	}
	TotalFindings int
	UniqueCount   int
	// Duplicates counts findings an independent pass recorded that repeat one
	// already listed. They are excluded from the finding list and reported as a
	// number, so the reader sees the dedupe instead of an inflated total.
	Duplicates int

	Findings []ReportFinding
	Timeline []ReportTimeline
	Commands []ReportCommand
	Manifest []struct {
		Artefact string
		Format   string
		Rows     int64
		SHA256   string
	}

	// LogsAvailable reports which of the five logs carried rows, so the report
	// states coverage instead of pretending an empty section is a clean one.
	LogsAvailable map[string]int64
}

// BuildReportHTML renders the tech report for one collected run.
func (c *EngagementCollector) BuildReportHTML(ctx context.Context, runID int64, outDir string) (string, error) {
	data, err := c.loadReportData(ctx, runID, outDir)
	if err != nil {
		return "", err
	}
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	slug := slugify(fmt.Sprintf("task-%d-run-%d", data.TaskID, data.RunID))
	path := filepath.Join(outDir, "report-"+slug+".html")

	f, err := os.Create(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	if err := techReportTmpl.Execute(f, data); err != nil {
		return "", err
	}
	return path, nil
}

func slugify(s string) string {
	var b strings.Builder
	prevDash := false
	for _, r := range strings.ToLower(s) {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') {
			b.WriteRune(r)
			prevDash = false
		} else if !prevDash {
			b.WriteByte('-')
			prevDash = true
		}
	}
	return strings.Trim(b.String(), "-")
}

func (c *EngagementCollector) loadReportData(ctx context.Context, runID int64, outDir string) (*ReportData, error) {
	d := &ReportData{
		RunID:         runID,
		GeneratedAt:   time.Now().Format("2006-01-02 15:04:05 MST"),
		Stats:         map[string]string{},
		LogsAvailable: map[string]int64{},
	}
	var (
		startedAt, finishedAt sql.NullTime
		statsJSON             string
	)
	err := c.DB.QueryRowContext(ctx, `
		SELECT r.task_id, COALESCE(t.name,''), r.run_status, r.started_at, r.finished_at,
		       COALESCE(r.template_rev,''), COALESCE(r.stats::text,'{}')
		  FROM eng_run r LEFT JOIN tasks t ON t.id=r.task_id WHERE r.id=$1`, runID).Scan(
		&d.TaskID, &d.TaskName, &d.RunStatus, &startedAt, &finishedAt, &d.TemplateRev, &statsJSON)
	if err != nil {
		return nil, fmt.Errorf("run %d: %w", runID, err)
	}
	if startedAt.Valid {
		d.StartedAt = startedAt.Time.Format("2006-01-02 15:04 MST")
	}
	if finishedAt.Valid {
		d.FinishedAt = finishedAt.Time.Format("2006-01-02 15:04 MST")
	}
	// duration + headline stats
	var durSec float64
	for _, key := range []string{
		"activities", "agent_turns", "commands", "timeline_events",
		"findings", "mapped_attacks", "errors",
	} {
		var v float64
		if scanJSONNumber(statsJSON, key, &v) {
			d.Stats[key] = fmt.Sprintf("%.0f", v)
			if key == "findings" {
				d.TotalFindings = int(v)
			}
		}
	}
	if scanJSONNumber(statsJSON, "duration_sec", &durSec) {
		d.Duration = humanDuration(durSec)
	}

	// per-log row counts (coverage statement)
	for _, lt := range logTables {
		var n int64
		q := "SELECT count(*) FROM " + lt.table + " WHERE run_id=$1"
		if err := c.DB.QueryRowContext(ctx, q, runID).Scan(&n); err == nil {
			d.LogsAvailable[lt.artefact] = n
		}
	}

	// findings + mapping + evidence
	rows, err := c.DB.QueryContext(ctx, `
		SELECT f.id, COALESCE(f.severity,''), COALESCE(f.status,''), COALESCE(f.name,''),
		       COALESCE(f.vulnclass,''), COALESCE(f.summary,''), COALESCE(f.evidence,''),
		       COALESCE(m.mitre_id,''), COALESCE(m.mitre_name,''),
		       COALESCE(m.wstg_id,''), COALESCE(m.wstg_name,''),
		       COALESCE(m.endpoint,''), COALESCE(m.method,''), COALESCE(m.script,''),
		       COALESCE(m.command,''), COALESCE(m.status,''),
		       COALESCE(m.evidence_shot,''), COALESCE(m.evidence_hash,''),
		       COALESCE(m.evidence_before,''), COALESCE(m.evidence_after,''),
		       COALESCE(m.evidence_note,'')
		  FROM eng_attack_mapping m
		  JOIN findings f ON f.id=m.finding_id
		 WHERE m.run_id=$1
		   AND COALESCE(f.status,'') NOT IN ('duplicate','false_positive')
		 ORDER BY CASE lower(f.severity)
		            WHEN 'critical' THEN 0 WHEN 'high' THEN 1 WHEN 'medium' THEN 2
		            WHEN 'low' THEN 3 ELSE 4 END, f.id`, runID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	sevCount := map[string]int{}
	for rows.Next() {
		var f ReportFinding
		if err := rows.Scan(&f.ID, &f.Severity, &f.Status, &f.Title, &f.VulnClass,
			&f.Summary, &f.Evidence, &f.MITRE, &f.MITREName, &f.WSTG, &f.WSTGName,
			&f.Endpoint, &f.Method, &f.Script, &f.Command, &f.MapStatus,
			&f.ShotPath, &f.ShotHash, &f.ShotBefore, &f.ShotAfter, &f.Note); err != nil {
			return nil, err
		}
		// copy screenshot next to the report so the html/docx are relocatable
		if f.ShotPath != "" {
			if rel, err := copyIntoReport(f.ShotPath, outDir); err == nil {
				f.ShotPath = rel
			}
		}
		d.Findings = append(d.Findings, f)
		sevCount[strings.ToLower(strings.TrimSpace(f.Severity))]++
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	d.UniqueCount = len(d.Findings)
	// Headline count is the de-duplicated figure; the raw mapping count is shown
	// separately as "duplicates excluded". Reporting the raw number as "findings"
	// overstates the run (an independent pass that repeats an issue is not a new
	// issue).
	d.TotalFindings = len(d.Findings)
	_ = c.DB.QueryRowContext(ctx, `
		SELECT count(*) FROM eng_attack_mapping m
		  JOIN findings f ON f.id=m.finding_id
		 WHERE m.run_id=$1 AND COALESCE(f.status,'') IN ('duplicate','false_positive')`, runID).
		Scan(&d.Duplicates)

	order := []string{"critical", "high", "medium", "low", "info"}
	total := 0
	for _, s := range order {
		total += sevCount[s]
	}
	if total == 0 {
		total = 1
	}
	for _, s := range order {
		if sevCount[s] == 0 {
			continue
		}
		d.Severity = append(d.Severity, struct {
			Name  string
			Count int
			Pct   float64
		}{strings.Title(s), sevCount[s], float64(sevCount[s]) / float64(total) * 100})
	}

	// timeline
	trows, err := c.DB.QueryContext(ctx, `
		SELECT seq, phase, COALESCE(to_char(at_time,'HH24:MI:SS'),''), action, target, result
		  FROM eng_engagement_activities WHERE run_id=$1 ORDER BY seq`, runID)
	if err == nil {
		defer trows.Close()
		for trows.Next() {
			var t ReportTimeline
			if err := trows.Scan(&t.Seq, &t.Phase, &t.At, &t.Action, &t.Target, &t.Result); err == nil {
				d.Timeline = append(d.Timeline, t)
			}
		}
	}

	// commands (bounded: the report shows the first 200, the log keeps all)
	crows, err := c.DB.QueryContext(ctx, `
		SELECT COALESCE(worker,''), COALESCE(command,''),
		       CASE WHEN exit_ok IS NULL THEN '—' WHEN exit_ok THEN 'ok' ELSE 'error' END,
		       COALESCE(output_head,''), COALESCE(to_char(occurred_at,'HH24:MI:SS'),'')
		  FROM eng_command_log WHERE run_id=$1 ORDER BY id LIMIT 200`, runID)
	if err == nil {
		defer crows.Close()
		for crows.Next() {
			var r ReportCommand
			if err := crows.Scan(&r.Worker, &r.Command, &r.ExitOK, &r.Output, &r.At); err == nil {
				d.Commands = append(d.Commands, r)
			}
		}
	}

	// manifest
	mrows, err := c.DB.QueryContext(ctx, `
		SELECT artefact, COALESCE(format,''), COALESCE(row_count,0), COALESCE(sha256,'')
		  FROM eng_log_manifest WHERE run_id=$1 ORDER BY artefact`, runID)
	if err == nil {
		defer mrows.Close()
		for mrows.Next() {
			var m struct {
				Artefact string
				Format   string
				Rows     int64
				SHA256   string
			}
			if err := mrows.Scan(&m.Artefact, &m.Format, &m.Rows, &m.SHA256); err == nil {
				d.Manifest = append(d.Manifest, m)
			}
		}
	}
	return d, nil
}

// copyIntoReport copies an evidence PNG into the report dir and returns its
// basename, so the HTML can reference it relatively.
func copyIntoReport(src, dir string) (string, error) {
	if src == "" {
		return "", nil
	}
	st, err := os.Stat(src)
	if err != nil || st.IsDir() {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	dst := filepath.Join(dir, filepath.Base(src))
	if same, err := sameFile(src, dst); err == nil && same {
		return filepath.Base(dst), nil
	}
	in, err := os.ReadFile(src)
	if err != nil {
		return "", err
	}
	if err := os.WriteFile(dst, in, 0o644); err != nil {
		return "", err
	}
	return filepath.Base(dst), nil
}

func sameFile(a, b string) (bool, error) {
	ha, err := fileHash(a)
	if err != nil {
		return false, err
	}
	hb, err := fileHash(b)
	if err != nil {
		return false, err
	}
	return ha == hb, nil
}

func fileHash(p string) (string, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return "", err
	}
	s := sha256.Sum256(b)
	return hex.EncodeToString(s[:]), nil
}

func humanDuration(sec float64) string {
	s := int(sec)
	h := s / 3600
	m := (s % 3600) / 60
	s = s % 60
	if h > 0 {
		return fmt.Sprintf("%dh %02dm %02ds", h, m, s)
	}
	return fmt.Sprintf("%dm %02ds", m, s)
}

// scanJSONNumber pulls one numeric key out of a flat JSON object without
// pulling in a decoder for the whole struct.
func scanJSONNumber(blob, key string, out *float64) bool {
	needle := `"` + key + `":`
	i := strings.Index(blob, needle)
	if i < 0 {
		return false
	}
	rest := strings.TrimSpace(blob[i+len(needle):])
	end := 0
	for end < len(rest) && (rest[end] == '-' || rest[end] == '.' || (rest[end] >= '0' && rest[end] <= '9') || rest[end] == 'e' || rest[end] == 'E' || rest[end] == '+') {
		end++
	}
	if end == 0 {
		return false
	}
	var v float64
	if _, err := fmt.Sscanf(rest[:end], "%g", &v); err != nil {
		return false
	}
	*out = v
	return true
}

// ---------------------------------------------------------------------------
// Conversion: HTML -> PDF (Chromium) and HTML -> DOCX (pandoc)
// ---------------------------------------------------------------------------

// HTMLToPDF prints a local HTML file to PDF through headless Chromium.
func (e *EvidenceEngine) HTMLToPDF(ctx context.Context, htmlPath, pdfPath string) error {
	cctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()

	pageWS, cleanup, err := e.launch(cctx)
	if err != nil {
		return err
	}
	defer cleanup()
	conn, err := dialCDP(cctx, pageWS)
	if err != nil {
		return err
	}
	defer conn.ws.Close()
	if _, err := conn.send(cctx, "Page.enable", map[string]any{}); err != nil {
		return err
	}
	abs, err := filepath.Abs(htmlPath)
	if err != nil {
		return err
	}
	if _, err := conn.send(cctx, "Page.navigate", map[string]any{"url": "file://" + abs}); err != nil {
		return err
	}
	settle(cctx, 2500*time.Millisecond)
	raw, err := conn.send(cctx, "Page.printToPDF", map[string]any{
		"printBackground": true,
		"paperWidth":      8.27, // A4
		"paperHeight":     11.69,
		"marginTop":       0.4, "marginBottom": 0.4,
		"marginLeft": 0.35, "marginRight": 0.35,
	})
	if err != nil {
		return err
	}
	var out struct {
		Data string `json:"data"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return err
	}
	pdf, err := base64.StdEncoding.DecodeString(out.Data)
	if err != nil {
		return err
	}
	return os.WriteFile(pdfPath, pdf, 0o644)
}

// HTMLToDOCX converts with pandoc when present. Pandoc keeps tables, headings
// and embedded images, which is what a tech report needs.
func HTMLToDOCX(ctx context.Context, htmlPath, docxPath string) error {
	bin, err := exec.LookPath("pandoc")
	if err != nil {
		return fmt.Errorf("pandoc not installed: %w", err)
	}
	cmd := exec.CommandContext(ctx, bin,
		htmlPath,
		"-f", "html",
		"-t", "docx",
		"-o", docxPath,
		"--resource-path", filepath.Dir(htmlPath),
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("pandoc: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// GenerateAll renders HTML then derives PDF and DOCX, recording each artefact
// in the manifest. A converter that is missing is reported, not silently
// skipped: the caller sees exactly which formats exist.
func (c *EngagementCollector) GenerateAll(ctx context.Context, runID int64, outDir string) (map[string]string, []string, error) {
	// The five logs are always written as files alongside the report: the
	// operator's requirement is that every run leaves logs on disk, not only
	// rows in a table.
	logFiles, err := c.ExportLogs(ctx, runID, outDir)
	if err != nil {
		return nil, nil, err
	}
	htmlPath, err := c.BuildReportHTML(ctx, runID, outDir)
	if err != nil {
		return nil, nil, err
	}
	produced := map[string]string{"html": htmlPath}
	for name, path := range logFiles {
		produced["log_"+name] = path
	}
	var warnings []string

	eng := &EvidenceEngine{OutputDir: filepath.Dir(htmlPath), Timeout: 90 * time.Second}
	pdfPath := strings.TrimSuffix(htmlPath, ".html") + ".pdf"
	if err := eng.HTMLToPDF(ctx, htmlPath, pdfPath); err != nil {
		warnings = append(warnings, "pdf: "+err.Error())
	} else {
		produced["pdf"] = pdfPath
	}

	docxPath := strings.TrimSuffix(htmlPath, ".html") + ".docx"
	if err := HTMLToDOCX(ctx, htmlPath, docxPath); err != nil {
		warnings = append(warnings, "docx: "+err.Error())
	} else {
		produced["docx"] = docxPath
	}

	// record artefacts
	for format, path := range produced {
		hash, _ := fileHash(path)
		var bytes int64
		if st, err := os.Stat(path); err == nil {
			bytes = st.Size()
		}
		_, _ = c.DB.ExecContext(ctx, `
			INSERT INTO eng_log_manifest (run_id, artefact, format, path, bytes, sha256, chain_head, row_count)
			VALUES ($1,$2,$3,$4,$5,$6,'',0)`,
			runID, "report_"+format, format, path, bytes, hash)
	}
	return produced, warnings, nil
}