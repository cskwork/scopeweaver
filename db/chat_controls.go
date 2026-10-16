package db

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"

	"github.com/Autumn-27/artex/chatstate"
)

func chatID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		panic(err)
	}
	return hex.EncodeToString(id[:])
}
func chatFingerprint(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func nullableID(id int64) any {
	if id == 0 {
		return nil
	}
	return id
}

// chatTx always takes the parent lock before the gate and session locks. Parent
// deletion and claims consequently have one admission boundary across segments.
func (d *DB) chatTx(ctx context.Context, ref chatstate.SessionRef, fn func(*sql.Tx) error) error {
	if err := ref.Validate(); err != nil {
		return err
	}
	tx, err := d.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var id int64
	if ref.ConversationID > 0 {
		err = tx.QueryRowContext(ctx, `SELECT id FROM conversations WHERE id=$1 FOR UPDATE`, ref.ConversationID).Scan(&id)
	} else {
		err = tx.QueryRowContext(ctx, `SELECT id FROM tasks WHERE id=$1 AND exploration_id=$2 FOR UPDATE`, ref.TaskID, ref.ExplorationID).Scan(&id)
		if err == nil && ref.MainSeg > 0 {
			var exists bool
			err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM main_sessions WHERE exploration_id=$1 AND seq=$2)`, ref.ExplorationID, ref.MainSeg).Scan(&exists)
			if err == nil && !exists {
				return chatstate.ErrGone
			}
		}
	}
	if errors.Is(err, sql.ErrNoRows) {
		return chatstate.ErrGone
	}
	if err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO chat_control_gates(gate_key,conversation_id,task_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, ref.GateKey(), nullableID(ref.ConversationID), nullableID(ref.TaskID))
	if err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT revision FROM chat_control_gates WHERE gate_key=$1 FOR UPDATE`, ref.GateKey()).Scan(&id); err != nil {
		return err
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO chat_control_sessions(session_key,gate_key,conversation_id,task_id,exploration_id,main_seg) VALUES($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, ref.Key(), ref.GateKey(), nullableID(ref.ConversationID), nullableID(ref.TaskID), nullableID(ref.ExplorationID), ref.MainSeg)
	if err != nil {
		return err
	}
	if err = tx.QueryRowContext(ctx, `SELECT history_revision FROM chat_control_sessions WHERE session_key=$1 FOR UPDATE`, ref.Key()).Scan(&id); err != nil {
		return err
	}
	if err = fn(tx); err != nil {
		return err
	}
	return tx.Commit()
}

const admissionCols = `i.id,i.client_id,i.kind,i.target_run_id,i.status,i.prompt,i.revision,i.ordinal,COALESCE(s.conversation_id,0),COALESCE(s.task_id,0),COALESCE(s.exploration_id,0),s.main_seg`

func scanChatAdmission(row interface{ Scan(...any) error }) (a chatstate.Admission, err error) {
	var prompt []byte
	err = row.Scan(&a.ID, &a.ClientID, &a.Kind, &a.TargetRunID, &a.Status, &prompt, &a.Revision, &a.Ordinal, &a.Session.ConversationID, &a.Session.TaskID, &a.Session.ExplorationID, &a.Session.MainSeg)
	if err == nil {
		err = json.Unmarshal(prompt, &a.Prompt)
	}
	return
}
func chatInput(ctx context.Context, tx *sql.Tx, ref chatstate.SessionRef, id string) (chatstate.Admission, error) {
	return scanChatAdmission(tx.QueryRowContext(ctx, `SELECT `+admissionCols+` FROM chat_control_inputs i JOIN chat_control_sessions s ON s.session_key=i.session_key WHERE i.session_key=$1 AND i.id=$2 FOR UPDATE OF i`, ref.Key(), id))
}
func bumpChatGate(ctx context.Context, tx *sql.Tx, gate string) error {
	_, err := tx.ExecContext(ctx, `UPDATE chat_control_gates SET revision=revision+1 WHERE gate_key=$1`, gate)
	return err
}

func (d *DB) AdmitChatInput(ctx context.Context, ref chatstate.SessionRef, clientID, mode, targetRunID string, prompt chatstate.Prompt) (out chatstate.Admission, err error) {
	if strings.TrimSpace(clientID) == "" || len(clientID) > 200 || strings.TrimSpace(prompt.Text) == "" && len(prompt.Attachments) == 0 {
		return out, fmt.Errorf("client request ID and prompt are required")
	}
	if mode != "send" && mode != "queue" && mode != "steer" {
		return out, fmt.Errorf("invalid input mode")
	}
	// Canonicalize absent manifests for stable idempotency across JSON clients.
	if prompt.Attachments == nil {
		prompt.Attachments = []chatstate.Attachment{}
	}
	fingerprint := chatFingerprint(struct {
		Mode, Target string
		Prompt       chatstate.Prompt
	}{mode, targetRunID, prompt})
	err = d.chatTx(ctx, ref, func(tx *sql.Tx) error {
		var stored string
		e := tx.QueryRowContext(ctx, `SELECT id,fingerprint FROM chat_control_inputs WHERE session_key=$1 AND client_id=$2`, ref.Key(), clientID).Scan(&out.ID, &stored)
		if e == nil {
			if stored != fingerprint {
				return chatstate.ErrConflict
			}
			out, e = chatInput(ctx, tx, ref, out.ID)
			return e
		}
		if !errors.Is(e, sql.ErrNoRows) {
			return e
		}
		var paused bool
		var active sql.NullString
		if e = tx.QueryRowContext(ctx, `SELECT paused,active_run_id FROM chat_control_gates WHERE gate_key=$1`, ref.GateKey()).Scan(&paused, &active); e != nil {
			return e
		}
		kind := "turn"
		if mode == "steer" {
			kind = "steer"
			if len(prompt.Attachments) > 0 || targetRunID == "" || !active.Valid || active.String != targetRunID {
				return chatstate.ErrConflict
			}
		} else if mode == "send" && (paused || active.Valid) {
			return chatstate.ErrBusy
		}
		var ordinal int64
		if e = tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(ordinal),0)+1 FROM chat_control_inputs WHERE gate_key=$1`, ref.GateKey()).Scan(&ordinal); e != nil {
			return e
		}
		payload, _ := json.Marshal(prompt)
		out = chatstate.Admission{ID: chatID(), ClientID: clientID, Kind: kind, TargetRunID: targetRunID, Status: "queued", Session: ref, Prompt: prompt, Ordinal: ordinal, Revision: 1}
		_, e = tx.ExecContext(ctx, `INSERT INTO chat_control_inputs(id,session_key,gate_key,client_id,fingerprint,kind,target_run_id,status,prompt,revision,ordinal) VALUES($1,$2,$3,$4,$5,$6,$7,'queued',$8,1,$9)`, out.ID, ref.Key(), ref.GateKey(), clientID, fingerprint, kind, targetRunID, payload, ordinal)
		if e != nil {
			return e
		}
		return bumpChatGate(ctx, tx, ref.GateKey())
	})
	return
}
func (d *DB) ChatControlState(ctx context.Context, ref chatstate.SessionRef) (out chatstate.ControlState, err error) {
	out = chatstate.ControlState{Session: ref, Inputs: []chatstate.Admission{}, Capabilities: map[string]bool{"queue": true, "steer": true}}
	err = d.chatTx(ctx, ref, func(tx *sql.Tx) error {
		var active sql.NullString
		if e := tx.QueryRowContext(ctx, `SELECT g.revision,g.paused,g.active_run_id,s.history_revision FROM chat_control_gates g JOIN chat_control_sessions s ON s.gate_key=g.gate_key WHERE s.session_key=$1`, ref.Key()).Scan(&out.Revision, &out.Paused, &active, &out.HistoryRevision); e != nil {
			return e
		}
		if active.Valid {
			run, e := chatRun(ctx, tx, active.String)
			if e != nil {
				return e
			}
			out.Active = &run
		}
		rows, e := tx.QueryContext(ctx, `SELECT `+admissionCols+` FROM chat_control_inputs i JOIN chat_control_sessions s ON s.session_key=i.session_key WHERE i.gate_key=$1 AND i.status IN ('queued','claimed','unknown','undelivered') ORDER BY i.ordinal,i.id`, ref.GateKey())
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			a, e := scanChatAdmission(rows)
			if e != nil {
				return e
			}
			out.Inputs = append(out.Inputs, a)
		}
		return rows.Err()
	})
	return
}
func (d *DB) UpdateChatInput(ctx context.Context, ref chatstate.SessionRef, id string, expected int64, prompt chatstate.Prompt) (out chatstate.Admission, err error) {
	err = d.chatTx(ctx, ref, func(tx *sql.Tx) error {
		a, e := chatInput(ctx, tx, ref, id)
		if errors.Is(e, sql.ErrNoRows) {
			return chatstate.ErrGone
		}
		if e != nil {
			return e
		}
		if a.Status != "queued" || a.Kind != "turn" || a.Revision != expected {
			return chatstate.ErrConflict
		}
		if strings.TrimSpace(prompt.Text) == "" && len(prompt.Attachments) == 0 {
			return fmt.Errorf("prompt is required")
		}
		payload, _ := json.Marshal(prompt)
		_, e = tx.ExecContext(ctx, `UPDATE chat_control_inputs SET prompt=$2,revision=revision+1 WHERE id=$1`, id, payload)
		if e != nil {
			return e
		}
		out, e = chatInput(ctx, tx, ref, id)
		if e != nil {
			return e
		}
		return bumpChatGate(ctx, tx, ref.GateKey())
	})
	return
}
func chatExpected(ctx context.Context, tx *sql.Tx, gate string, expected int64) error {
	var revision int64
	if err := tx.QueryRowContext(ctx, `SELECT revision FROM chat_control_gates WHERE gate_key=$1`, gate).Scan(&revision); err != nil {
		return err
	}
	if revision != expected {
		return chatstate.ErrConflict
	}
	return nil
}
func (d *DB) ReorderChatInputs(ctx context.Context, ref chatstate.SessionRef, expected int64, ids []string) error {
	return d.chatTx(ctx, ref, func(tx *sql.Tx) error {
		if e := chatExpected(ctx, tx, ref.GateKey(), expected); e != nil {
			return e
		}
		rows, e := tx.QueryContext(ctx, `SELECT id FROM chat_control_inputs WHERE gate_key=$1 AND kind='turn' AND status='queued' ORDER BY ordinal,id`, ref.GateKey())
		if e != nil {
			return e
		}
		var before []string
		for rows.Next() {
			var id string
			if e = rows.Scan(&id); e != nil {
				rows.Close()
				return e
			}
			before = append(before, id)
		}
		e = rows.Err()
		rows.Close()
		if e != nil {
			return e
		}
		members := map[string]bool{}
		for _, id := range before {
			members[id] = true
		}
		if len(ids) != len(before) {
			return chatstate.ErrConflict
		}
		for _, id := range ids {
			if !members[id] {
				return chatstate.ErrConflict
			}
			delete(members, id)
		}
		if reflect.DeepEqual(before, ids) {
			return nil
		}
		for i, id := range ids {
			if _, e = tx.ExecContext(ctx, `UPDATE chat_control_inputs SET ordinal=$2,revision=revision+1 WHERE id=$1`, id, i+1); e != nil {
				return e
			}
		}
		return bumpChatGate(ctx, tx, ref.GateKey())
	})
}

const runCols = `r.id,r.input_id,r.turn_id,COALESCE(r.supersedes_run_id,''),r.transcript_id,r.start_record_uuid,r.end_record_uuid,r.status,r.history_revision,r.prompt,COALESCE(s.conversation_id,0),COALESCE(s.task_id,0),COALESCE(s.exploration_id,0),s.main_seg`

func scanChatRun(row interface{ Scan(...any) error }) (r chatstate.Run, err error) {
	var prompt []byte
	err = row.Scan(&r.ID, &r.InputID, &r.TurnID, &r.SupersedesRunID, &r.TranscriptID, &r.StartRecordUUID, &r.EndRecordUUID, &r.Status, &r.HistoryRevision, &prompt, &r.Session.ConversationID, &r.Session.TaskID, &r.Session.ExplorationID, &r.Session.MainSeg)
	if err == nil {
		err = json.Unmarshal(prompt, &r.Prompt)
	}
	return
}
func chatRun(ctx context.Context, tx *sql.Tx, id string) (chatstate.Run, error) {
	return scanChatRun(tx.QueryRowContext(ctx, `SELECT `+runCols+` FROM chat_control_runs r JOIN chat_control_sessions s ON s.session_key=r.session_key WHERE r.id=$1`, id))
}
