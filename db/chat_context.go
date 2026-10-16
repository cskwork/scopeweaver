package db

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/Autumn-27/artex/chatstate"
)

func (d *DB) SaveChatContext(ctx context.Context, ref chatstate.SessionRef, runID string, view chatstate.ContextView) error {
	return d.chatTx(ctx, ref, func(tx *sql.Tx) error {
		var closed bool
		if err := tx.QueryRowContext(ctx, `SELECT closed FROM chat_control_gates WHERE gate_key=$1`, ref.GateKey()).Scan(&closed); err != nil {
			return err
		}
		if closed {
			return chatstate.ErrGone
		}
		var history int64
		var latest string
		if err := tx.QueryRowContext(ctx, `SELECT history_revision FROM chat_control_sessions WHERE session_key=$1`, ref.Key()).Scan(&history); err != nil {
			return err
		}
		if history != view.HistoryRevision {
			return chatstate.ErrConflict
		}
		if err := tx.QueryRowContext(ctx, `SELECT id FROM chat_control_runs WHERE session_key=$1 AND active ORDER BY turn_ordinal DESC,created_at DESC LIMIT 1`, ref.Key()).Scan(&latest); err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if runID != "" && latest != runID {
			return chatstate.ErrConflict
		}
		var oldRun string
		var seq int64
		err := tx.QueryRowContext(ctx, `SELECT run_id,sequence FROM chat_context_snapshots WHERE session_key=$1`, ref.Key()).Scan(&oldRun, &seq)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if err == nil && oldRun == runID && seq >= view.Sequence {
			return chatstate.ErrConflict
		}
		view.RunID = runID
		body, err := json.Marshal(view)
		if err != nil {
			return err
		}
		request, err := json.Marshal(view.Request)
		if err != nil {
			return err
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO chat_context_snapshots(session_key,run_id,history_revision,sequence,view,request,profile_identity) VALUES($1,$2,$3,$4,$5,$6,$7) ON CONFLICT(session_key) DO UPDATE SET run_id=EXCLUDED.run_id,history_revision=EXCLUDED.history_revision,sequence=EXCLUDED.sequence,view=EXCLUDED.view,request=EXCLUDED.request,profile_identity=EXCLUDED.profile_identity`, ref.Key(), runID, view.HistoryRevision, view.Sequence, body, request, view.ProfileIdentity)
		return err
	})
}
func (d *DB) ChatContext(ctx context.Context, ref chatstate.SessionRef) (view chatstate.ContextView, err error) {
	err = d.chatTx(ctx, ref, func(tx *sql.Tx) error {
		var body, request []byte
		var identity string
		err := tx.QueryRowContext(ctx, `SELECT c.view,c.request,c.profile_identity FROM chat_context_snapshots c JOIN chat_control_sessions s USING(session_key) WHERE c.session_key=$1 AND c.history_revision=s.history_revision`, ref.Key()).Scan(&body, &request, &identity)
		if errors.Is(err, sql.ErrNoRows) {
			view.Source = "unobserved"
			view.Estimated = true
			return nil
		}
		if err != nil {
			return err
		}
		if err = json.Unmarshal(body, &view); err != nil {
			return err
		}
		view.ProfileIdentity = identity
		return json.Unmarshal(request, &view.Request)
	})
	return
}
