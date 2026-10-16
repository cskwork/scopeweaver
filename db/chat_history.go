package db

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/Autumn-27/artex/chatstate"
	"github.com/Autumn-27/norma/transcript"
)

func validatePreparedChatRevision(ref chatstate.SessionRef, revision chatstate.TranscriptRevision) error {
	if revision.SessionKey != ref.Key() || revision.Directory == "" {
		return fmt.Errorf("invalid prepared transcript owner")
	}
	return chatstate.ValidateTranscriptRevision(transcript.NewStore(revision.Directory), revision)
}
func publishChatRevision(ctx context.Context, tx *sql.Tx, ref chatstate.SessionRef, revision chatstate.TranscriptRevision) error {
	var history int64
	if err := tx.QueryRowContext(ctx, `SELECT history_revision FROM chat_control_sessions WHERE session_key=$1`, ref.Key()).Scan(&history); err != nil {
		return err
	}
	history++
	if _, err := tx.ExecContext(ctx, `INSERT INTO chat_transcript_revisions(id,session_key,transcript_id,history_revision,end_record_uuid,sha256,byte_length) VALUES($1,$2,$3,$4,$5,$6,$7)`, revision.ID, ref.Key(), revision.TranscriptID, history, revision.EndRecordUUID, revision.SHA256, revision.ByteLength); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `UPDATE chat_control_sessions SET transcript_id=$2,history_revision=$3,active_revision_id=$4 WHERE session_key=$1`, ref.Key(), revision.TranscriptID, history, revision.ID)
	return err
}
func (d *DB) PublishChatHistory(ctx context.Context, ref chatstate.SessionRef, expected int64, revision chatstate.TranscriptRevision) error {
	if err := validatePreparedChatRevision(ref, revision); err != nil {
		return err
	}
	return d.chatTx(ctx, ref, func(tx *sql.Tx) error {
		if err := chatExpected(ctx, tx, ref.GateKey(), expected); err != nil {
			return err
		}
		var active sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT active_run_id FROM chat_control_gates WHERE gate_key=$1`, ref.GateKey()).Scan(&active); err != nil {
			return err
		}
		if active.Valid {
			return chatstate.ErrBusy
		}
		if err := publishChatRevision(ctx, tx, ref, revision); err != nil {
			return err
		}
		return bumpChatGate(ctx, tx, ref.GateKey())
	})
}
func (d *DB) ActiveChatTranscript(ctx context.Context, ref chatstate.SessionRef) (out *chatstate.TranscriptRevision, err error) {
	err = d.chatTx(ctx, ref, func(tx *sql.Tx) error {
		r := chatstate.TranscriptRevision{SessionKey: ref.Key(), TranscriptID: ref.LegacyTranscriptID()}
		var active sql.NullString
		if e := tx.QueryRowContext(ctx, `SELECT active_revision_id,history_revision FROM chat_control_sessions WHERE session_key=$1`, ref.Key()).Scan(&active, &r.HistoryRevision); e != nil {
			return e
		}
		if active.Valid {
			if e := tx.QueryRowContext(ctx, `SELECT id,session_key,transcript_id,history_revision,end_record_uuid,sha256,byte_length FROM chat_transcript_revisions WHERE id=$1 AND session_key=$2`, active.String, ref.Key()).Scan(&r.ID, &r.SessionKey, &r.TranscriptID, &r.HistoryRevision, &r.EndRecordUUID, &r.SHA256, &r.ByteLength); e != nil {
				return e
			}
		}
		out = &r
		return nil
	})
	return
}
