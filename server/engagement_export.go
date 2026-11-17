package server

// File export of the five engagement logs.
//
// The database holds the canonical rows; these files are the artefact a human
// (or an auditor) actually opens. Both exist on every run: a log that only lives
// in a table is not a deliverable, and a file with no table behind it cannot be
// verified. Every file written here is recorded in eng_log_manifest with its
// sha256 and row count.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// EngLogFiles maps artefact name -> written path.
type EngLogFiles map[string]string

// ExportLogs writes the five logs for a run into dir.
func (c *EngagementCollector) ExportLogs(ctx context.Context, runID int64, dir string) (EngLogFiles, error) {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	out := EngLogFiles{}
	write := func(name, ext string, rows []map[string]any) error {
		path := filepath.Join(dir, fmt.Sprintf("%s-run-%d.%s", name, runID, ext))
		f, err := os.Create(path)
		if err != nil {
			return err
		}
		defer f.Close()
		enc := json.NewEncoder(f)
		enc.SetEscapeHTML(false)
		for _, r := range rows {
			if ext == "jsonl" {
				if err := enc.Encode(r); err != nil {
					return err
				}
				continue
			}
		}
		if ext == "json" {
			pretty, _ := json.MarshalIndent(rows, "", "  ")
			if _, err := f.Write(pretty); err != nil {
				return err
			}
		}
		out[name] = path
		hash, _ := fileHash(path)
		var size int64
		if st, err := os.Stat(path); err == nil {
			size = st.Size()
		}
		_, _ = c.DB.ExecContext(ctx, `
			INSERT INTO eng_log_manifest (run_id, artefact, format, path, bytes, sha256, chain_head, row_count)
			VALUES ($1,$2,$3,$4,$5,$6,'',$7)`,
			runID, name, ext, path, size, hash, len(rows))
		return nil
	}

	// 1. activity analysis
	rows, err := c.rows(ctx, `SELECT category, metric, COALESCE(value_num::text,''),
		COALESCE(value_text,''), COALESCE(note,'')
		FROM eng_activity_analysis WHERE run_id=$1 ORDER BY id`, runID,
		[]string{"category", "metric", "value_num", "value_text", "note"})
	if err != nil {
		return nil, err
	}
	if err := write("activity_analysis", "json", rows); err != nil {
		return nil, err
	}

	// 2. agent activities
	rows, err = c.rows(ctx, `SELECT COALESCE(to_char(occurred_at,'YYYY-MM-DD"T"HH24:MI:SSOF'),''),
		COALESCE(worker,''), COALESCE(kind,''), COALESCE(tool,''), is_error,
		COALESCE(summary,''), COALESCE(detail,'')
		FROM eng_agent_activities WHERE run_id=$1 ORDER BY id`, runID,
		[]string{"ts", "worker", "kind", "tool", "is_error", "summary", "detail"})
	if err != nil {
		return nil, err
	}
	if err := write("agent_activities", "jsonl", rows); err != nil {
		return nil, err
	}

	// 3. command log
	rows, err = c.rows(ctx, `SELECT COALESCE(to_char(occurred_at,'YYYY-MM-DD"T"HH24:MI:SSOF'),''),
		COALESCE(worker,''), COALESCE(command,''),
		CASE WHEN exit_ok IS NULL THEN '' WHEN exit_ok THEN 'ok' ELSE 'error' END,
		COALESCE(output_head,''), COALESCE(output_bytes,0)
		FROM eng_command_log WHERE run_id=$1 ORDER BY id`, runID,
		[]string{"ts", "worker", "command", "exit", "output_head", "output_bytes"})
	if err != nil {
		return nil, err
	}
	if err := write("command_log", "jsonl", rows); err != nil {
		return nil, err
	}

	// 4. engagement activities
	rows, err = c.rows(ctx, `SELECT seq, COALESCE(phase,''),
		COALESCE(to_char(at_time,'YYYY-MM-DD"T"HH24:MI:SSOF'),''),
		COALESCE(action,''), COALESCE(target,''), COALESCE(result,'')
		FROM eng_engagement_activities WHERE run_id=$1 ORDER BY seq`, runID,
		[]string{"seq", "phase", "ts", "action", "target", "result"})
	if err != nil {
		return nil, err
	}
	if err := write("engagement_activities", "json", rows); err != nil {
		return nil, err
	}

	// 6. attack mapping
	rows, err = c.rows(ctx, `SELECT m.finding_id, COALESCE(f.severity,''), COALESCE(f.name,''),
		COALESCE(m.vuln_class,''), COALESCE(m.mitre_id,''), COALESCE(m.mitre_name,''),
		COALESCE(m.wstg_id,''), COALESCE(m.wstg_name,''), COALESCE(m.method,''),
		COALESCE(m.endpoint,''), COALESCE(m.status,''), COALESCE(m.script,''),
		COALESCE(m.command,''), COALESCE(m.evidence_shot,''), COALESCE(m.evidence_hash,'')
		FROM eng_attack_mapping m LEFT JOIN findings f ON f.id=m.finding_id
		WHERE m.run_id=$1 ORDER BY m.id`, runID,
		[]string{"finding_id", "severity", "title", "vuln_class", "mitre_id", "mitre_name",
			"wstg_id", "wstg_name", "method", "endpoint", "status", "script", "command",
			"evidence_shot", "evidence_hash"})
	if err != nil {
		return nil, err
	}
	if err := write("attack_mapping", "json", rows); err != nil {
		return nil, err
	}

	// 5. manifest (written last, so it can include the four files above)
	man, err := c.rows(ctx, `SELECT artefact, COALESCE(format,''), COALESCE(path,''),
		COALESCE(bytes,0), COALESCE(sha256,''), COALESCE(row_count,0)
		FROM eng_log_manifest WHERE run_id=$1 ORDER BY id`, runID,
		[]string{"artefact", "format", "path", "bytes", "sha256", "row_count"})
	if err != nil {
		return nil, err
	}
	body := map[string]any{
		"run_id":      runID,
		"generated_at": time.Now().Format(time.RFC3339),
		"log_files":   out,
		"entries":     man,
	}
	path := filepath.Join(dir, fmt.Sprintf("log_manifest-run-%d.json", runID))
	b, _ := json.MarshalIndent(body, "", "  ")
	if err := os.WriteFile(path, b, 0o644); err != nil {
		return nil, err
	}
	out["log_manifest"] = path
	return out, nil
}

// rows runs a query and maps each row to a map keyed by the supplied names.
func (c *EngagementCollector) rows(ctx context.Context, q string, runID int64, names []string) ([]map[string]any, error) {
	rs, err := c.DB.QueryContext(ctx, q, runID)
	if err != nil {
		return nil, err
	}
	defer rs.Close()
	cols, err := rs.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rs.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rs.Scan(ptrs...); err != nil {
			return nil, err
		}
		row := map[string]any{}
		for i, name := range names {
			if i >= len(vals) {
				break
			}
			v := vals[i]
			if b, ok := v.([]byte); ok {
				v = string(b)
			}
			row[name] = v
		}
		out = append(out, row)
	}
	return out, rs.Err()
}

var _ = sql.ErrNoRows
