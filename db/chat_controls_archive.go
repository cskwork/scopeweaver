package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"github.com/Autumn-27/artex/chatstate"
)

var chatArchiveTables = []string{"main_sessions", "chat_control_gates", "chat_control_sessions", "chat_control_inputs", "chat_control_runs", "chat_transcript_revisions", "chat_history_changes", "chat_context_snapshots", "chat_context_changes"}

func SnapshotChatControls(ctx context.Context, tx *sql.Tx, taskID int64) (map[string]json.RawMessage, error) {
	out := map[string]json.RawMessage{}
	for _, table := range chatArchiveTables {
		query := `SELECT t.* FROM ` + table + ` t JOIN chat_control_sessions s ON s.session_key=t.session_key WHERE s.task_id=$1`
		switch table {
		case "main_sessions":
			query = `SELECT m.* FROM main_sessions m JOIN tasks t ON t.exploration_id=m.exploration_id WHERE t.id=$1 ORDER BY m.seq`
		case "chat_control_gates", "chat_control_sessions":
			query = `SELECT * FROM ` + table + ` WHERE task_id=$1`
		}
		raw, _, err := queryArchiveRows(tx, query, taskID)
		if err != nil {
			return nil, fmt.Errorf("snapshot %s: %w", table, err)
		}
		out[table] = raw
	}
	return out, nil
}
func RestoreChatControls(ctx context.Context, tx *sql.Tx, tables map[string]json.RawMessage) error {
	// Break circular pointers for insertion; restore them only after all manifests
	// and attempts exist. Every restored gate starts paused with no active claim.
	type link struct {
		table, key, column string
		value              json.RawMessage
	}
	var links []link
	for _, table := range chatArchiveTables {
		raw := tables[table]
		if rawRowCount(raw) == 0 {
			continue
		}
		var rows []map[string]json.RawMessage
		if err := json.Unmarshal(raw, &rows); err != nil {
			return err
		}
		for _, row := range rows {
			resetLink := func(column string) {
				value := row[column]
				if len(value) > 0 && string(value) != "null" {
					keyColumn := "id"
					if table == "chat_control_sessions" {
						keyColumn = "session_key"
					}
					var key string
					_ = json.Unmarshal(row[keyColumn], &key)
					links = append(links, link{table, key, column, value})
					row[column] = json.RawMessage(`null`)
				}
			}
			switch table {
			case "chat_control_gates":
				row["paused"] = json.RawMessage(`true`)
				row["closed"] = json.RawMessage(`false`)
				row["active_run_id"] = json.RawMessage(`null`)
				row["operation_id"] = json.RawMessage(`""`)
			case "chat_control_sessions":
				resetLink("active_revision_id")
				resetLink("parent_session_key")
			case "chat_control_inputs":
				resetLink("supersedes_run_id")
				if string(row["status"]) == `"claimed"` {
					row["status"] = json.RawMessage(`"unknown"`)
				}
			case "chat_control_runs":
				resetLink("supersedes_run_id")
				if string(row["status"]) == `"running"` {
					row["status"] = json.RawMessage(`"unknown"`)
				}
			case "chat_context_changes":
				if string(row["status"]) == `"preparing"` {
					row["status"] = json.RawMessage(`"aborted"`)
				}
			case "chat_history_changes":
				if string(row["status"]) == `"preparing"` {
					row["status"] = json.RawMessage(`"failed"`)
				}
			}
		}
		encoded, err := json.Marshal(rows)
		if err != nil {
			return err
		}
		if _, err = tx.ExecContext(ctx, `INSERT INTO `+table+` SELECT * FROM json_populate_recordset(NULL::`+table+`,$1::json)`, encoded); err != nil {
			return fmt.Errorf("restore %s: %w", table, err)
		}
	}
	for _, l := range links {
		key := "id"
		if l.table == "chat_control_sessions" {
			key = "session_key"
		}
		var value string
		if err := json.Unmarshal(l.value, &value); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE `+l.table+` SET `+l.column+`=$2 WHERE `+key+`=$1`, l.key, value); err != nil {
			return err
		}
	}
	return nil
}

func (d *DB) TaskChatTranscriptRevisions(ctx context.Context, taskID int64) ([]chatstate.TranscriptRevision, error) {
	rows, err := d.QueryContext(ctx, `SELECT r.id,r.session_key,r.transcript_id,r.history_revision,r.end_record_uuid,r.sha256,r.byte_length FROM chat_transcript_revisions r JOIN chat_control_sessions s USING(session_key) WHERE s.task_id=$1 ORDER BY r.id`, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []chatstate.TranscriptRevision
	for rows.Next() {
		var r chatstate.TranscriptRevision
		if err = rows.Scan(&r.ID, &r.SessionKey, &r.TranscriptID, &r.HistoryRevision, &r.EndRecordUUID, &r.SHA256, &r.ByteLength); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}
func ChatArchiveTranscriptRevisions(tables map[string]json.RawMessage) ([]chatstate.TranscriptRevision, error) {
	var rows []struct {
		ID         string `json:"id"`
		Session    string `json:"session_key"`
		Transcript string `json:"transcript_id"`
		History    int64  `json:"history_revision"`
		End        string `json:"end_record_uuid"`
		SHA        string `json:"sha256"`
		Length     int64  `json:"byte_length"`
	}
	raw := tables["chat_transcript_revisions"]
	if len(raw) == 0 {
		return nil, nil
	}
	if err := json.Unmarshal(raw, &rows); err != nil {
		return nil, err
	}
	out := make([]chatstate.TranscriptRevision, 0, len(rows))
	for _, r := range rows {
		out = append(out, chatstate.TranscriptRevision{ID: r.ID, SessionKey: r.Session, TranscriptID: r.Transcript, HistoryRevision: r.History, EndRecordUUID: r.End, SHA256: r.SHA, ByteLength: r.Length})
	}
	return out, nil
}
