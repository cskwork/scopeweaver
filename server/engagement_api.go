package server

// HTTP surface for the engagement pipeline. The automatic hook covers "every
// run"; these endpoints exist so an operator can re-collect a past task, force a
// render, and fetch the artefacts without waiting for a new run.

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// registerEngagementRoutes wires /api/engagement/* onto the server mux.
func (s *Server) registerEngagementRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/engagement/collect", s.engagementCollect)
	mux.HandleFunc("POST /api/engagement/report", s.engagementReport)
	mux.HandleFunc("GET /api/engagement/runs", s.engagementRuns)
	mux.HandleFunc("GET /api/engagement/manifest", s.engagementManifest)
	mux.HandleFunc("GET /api/engagement/artefact", s.engagementArtefact)
	mux.HandleFunc("POST /api/engagement/evidence", s.engagementEvidence)
}

// engagementCollectorFor builds a collector rooted at the task's output dir.
func (s *Server) engagementCollectorFor(taskID int64) (*EngagementCollector, string) {
	root := os.Getenv("ARTEX_ENGAGEMENT_REPORTS")
	if root == "" {
		root = filepath.Join(s.m.dir, "engagement-reports")
	}
	out := filepath.Join(root, "task-"+strconv.FormatInt(taskID, 10))
	return &EngagementCollector{
		DB:          s.m.pg.DB,
		TemplateRev: EngagementTemplateRev,
		EvidenceDir: filepath.Join(out, "evidence"),
		DSN:         os.Getenv("ARTEX_PG_DSN"),
	}, out
}

func (s *Server) engagementCollect(w http.ResponseWriter, r *http.Request) {
	taskID, err := strconv.ParseInt(r.URL.Query().Get("task_id"), 10, 64)
	if err != nil || taskID <= 0 {
		writeErr(w, http.StatusBadRequest, "task_id required")
		return
	}
	c, _ := s.engagementCollectorFor(taskID)
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Minute)
	defer cancel()
	runID, err := c.CollectEngagementLogs(ctx, taskID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "task_id": taskID, "run_id": runID})
}

func (s *Server) engagementReport(w http.ResponseWriter, r *http.Request) {
	taskID, err := strconv.ParseInt(r.URL.Query().Get("task_id"), 10, 64)
	if err != nil || taskID <= 0 {
		writeErr(w, http.StatusBadRequest, "task_id required")
		return
	}
	c, out := s.engagementCollectorFor(taskID)
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	runID, err := c.CollectEngagementLogs(ctx, taskID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	produced, warnings, err := c.GenerateAll(ctx, runID, out)
	if err != nil {
		// A withheld report is not a server error: the run simply has not earned a
		// report yet. Return 409 with the specific gaps so the operator sees what
		// to close, and still hand back the raw logs that were written.
		var gateErr *ReportGateError
		if errors.As(err, &gateErr) {
			writeJSON(w, http.StatusConflict, map[string]any{
				"ok":           false,
				"run_id":       runID,
				"error":        "report withheld: analysis not complete",
				"missing":      gateErr.Verdict.Missing,
				"warnings":     gateErr.Verdict.Warnings,
				"files":        produced,
				"logs_written": true,
			})
			return
		}
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	if strings.Contains(r.URL.Query().Get("deck"), "1") || r.URL.Query().Get("deck") == "true" {
		if deck, derr := c.GenerateDeck(ctx, runID, out); derr != nil {
			warnings = append(warnings, "deck: "+derr.Error())
		} else {
			produced["pptx"] = deck
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "run_id": runID, "files": produced, "warnings": warnings})
}

func (s *Server) engagementRuns(w http.ResponseWriter, r *http.Request) {
	taskID := r.URL.Query().Get("task_id")
	q := `SELECT r.id, r.task_id, COALESCE(r.run_status,''), r.created_at,
	             (SELECT count(*) FROM eng_attack_mapping m WHERE m.run_id=r.id)
	        FROM eng_run r`
	args := []any{}
	if taskID != "" {
		q += ` WHERE r.task_id=$1`
		args = append(args, taskID)
	}
	q += ` ORDER BY r.id DESC LIMIT 100`
	rows, err := s.m.pg.QueryContext(r.Context(), q, args...)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var id, tid, attacks int64
		var status string
		var created time.Time
		if err := rows.Scan(&id, &tid, &status, &created, &attacks); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"run_id": id, "task_id": tid, "status": status,
			"created_at": created, "attack_mappings": attacks,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": out})
}

func (s *Server) engagementManifest(w http.ResponseWriter, r *http.Request) {
	runID, err := strconv.ParseInt(r.URL.Query().Get("run_id"), 10, 64)
	if err != nil || runID <= 0 {
		writeErr(w, http.StatusBadRequest, "run_id required")
		return
	}
	rows, err := s.m.pg.QueryContext(r.Context(), `
		SELECT artefact, COALESCE(format,''), COALESCE(path,''), COALESCE(bytes,0),
		       COALESCE(sha256,''), COALESCE(chain_head,''), COALESCE(row_count,0)
		  FROM eng_log_manifest WHERE run_id=$1 ORDER BY artefact`, runID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	out := []map[string]any{}
	for rows.Next() {
		var artefact, format, path, sha, head string
		var bytes, n int64
		if err := rows.Scan(&artefact, &format, &path, &bytes, &sha, &head, &n); err != nil {
			continue
		}
		out = append(out, map[string]any{
			"artefact": artefact, "format": format, "path": path, "bytes": bytes,
			"sha256": sha, "chain_head": head, "row_count": n,
		})
	}
	writeJSON(w, http.StatusOK, map[string]any{"run_id": runID, "artefacts": out})
}

// engagementArtefact streams a stored artefact by manifest id or path.
func (s *Server) engagementArtefact(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Query().Get("path")
	if p == "" {
		writeErr(w, http.StatusBadRequest, "path required")
		return
	}
	// Only serve files the manifest knows about — never an arbitrary path.
	var known int64
	_ = s.m.pg.QueryRowContext(r.Context(),
		`SELECT count(*) FROM eng_log_manifest WHERE path=$1`, p).Scan(&known)
	if known == 0 {
		writeErr(w, http.StatusNotFound, "artefact not in manifest")
		return
	}
	if _, err := os.Stat(p); err != nil {
		writeErr(w, http.StatusNotFound, "file missing on disk")
		return
	}
	http.ServeFile(w, r, p)
}

// engagementEvidence captures a before/after screenshot pair and stores it on
// the mapping row for a finding, so a report has visual proof for a vector.
type evidenceReq struct {
	RunID     int64  `json:"run_id"`
	FindingID int64  `json:"finding_id"`
	Slug      string `json:"slug"`
	BeforeURL string `json:"before_url"`
	AfterURL  string `json:"after_url"`
	Payload   string `json:"payload"`
	Assertion string `json:"assertion"`
	// Cookies lets a capture prove a vector that needs a session.
	Cookies      map[string]string `json:"cookies,omitempty"`
	CookieDomain string            `json:"cookie_domain,omitempty"`
	// Proxy optionally routes the capture browser (empty = direct).
	Proxy string `json:"proxy,omitempty"`
}

func (s *Server) engagementEvidence(w http.ResponseWriter, r *http.Request) {
	var req evidenceReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json: "+err.Error())
		return
	}
	if req.RunID == 0 || req.AfterURL == "" {
		writeErr(w, http.StatusBadRequest, "run_id and after_url required")
		return
	}
	// Resolve the owning task from the run so evidence lands in that task's
	// directory and a report for the run can find it by relative path.
	var taskForRun int64
	_ = s.m.pg.QueryRowContext(r.Context(),
		`SELECT task_id FROM eng_run WHERE id=$1`, req.RunID).Scan(&taskForRun)
	if taskForRun == 0 && req.FindingID > 0 {
		// A stale run id (the run was re-collected) must not lose the evidence:
		// fall back to the task the finding belongs to.
		_ = s.m.pg.QueryRowContext(r.Context(),
			`SELECT COALESCE(task_id,0) FROM findings WHERE id=$1`, req.FindingID).Scan(&taskForRun)
	}
	c, out := s.engagementCollectorFor(taskForRun)
	eng := &EvidenceEngine{
		OutputDir: filepath.Join(out, "evidence"), Timeout: 60 * time.Second,
		Cookies: req.Cookies, CookieDomain: req.CookieDomain, Proxy: req.Proxy,
	}
	slug := req.Slug
	if slug == "" {
		slug = "finding-" + strconv.FormatInt(req.FindingID, 10)
	}
	ctx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	res, err := eng.CaptureXSSProof(ctx, slug, req.BeforeURL, req.AfterURL, req.Payload, req.Assertion)
	if err != nil && res == nil {
		writeErr(w, http.StatusInternalServerError, err.Error())
		return
	}
	// Persist durably (survives re-collection) and onto this run's snapshot.
	if req.FindingID > 0 {
		note := "payload did not execute (assertion stayed false)"
		if res.Executed {
			note = "payload executed (DOM assertion flipped)"
		}
		_, _ = c.DB.ExecContext(ctx, `
			INSERT INTO eng_evidence (task_id, finding_id, slug, payload, assertion,
			                          before_path, after_path, before_hash, after_hash,
			                          before_value, after_value, executed, note)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
			ON CONFLICT (finding_id) DO UPDATE
			   SET slug=EXCLUDED.slug, payload=EXCLUDED.payload, assertion=EXCLUDED.assertion,
			       before_path=EXCLUDED.before_path, after_path=EXCLUDED.after_path,
			       before_hash=EXCLUDED.before_hash, after_hash=EXCLUDED.after_hash,
			       before_value=EXCLUDED.before_value, after_value=EXCLUDED.after_value,
			       executed=EXCLUDED.executed, note=EXCLUDED.note, created_at=now()`,
			taskForRun, req.FindingID, slug, req.Payload, req.Assertion,
			res.BeforePath, res.AfterPath, res.BeforeHash, res.AfterHash,
			res.BeforeValue, res.AfterValue, res.Executed, note)
		_, _ = c.DB.ExecContext(ctx, `
			UPDATE eng_attack_mapping
			   SET evidence_shot=$3, evidence_hash=$4, evidence_before=$5, evidence_after=$6,
			       script=COALESCE(NULLIF(script,''),$7), evidence_note=$8
			 WHERE run_id=$1 AND finding_id=$2`,
			req.RunID, req.FindingID, res.AfterPath, res.AfterHash,
			res.BeforePath, res.AfterPath, req.Payload, note)
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": res.Executed, "evidence": res})
}
