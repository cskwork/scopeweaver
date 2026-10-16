// Package chatstate holds shared chat protocol and transcript identities.
package chatstate

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrConflict         = errors.New("chat state changed; reload and try again")
	ErrBusy             = errors.New("chat is busy")
	ErrGone             = errors.New("chat is unavailable")
	ErrUnknownRun       = errors.New("a lost run requires acknowledgement")
	ErrAmbiguousHistory = errors.New("historical turn boundary is unavailable")
)

type SessionRef struct {
	ConversationID int64 `json:"conversation_id,string"`
	TaskID         int64 `json:"task_id,string"`
	ExplorationID  int64 `json:"exploration_id,string"`
	MainSeg        int   `json:"main_seg"`
}

func (r SessionRef) Validate() error {
	if r.ConversationID > 0 && r.TaskID == 0 && r.ExplorationID == 0 && r.MainSeg == 0 {
		return nil
	}
	if r.ConversationID == 0 && r.TaskID > 0 && r.ExplorationID > 0 && r.MainSeg >= 0 {
		return nil
	}
	return fmt.Errorf("invalid chat session")
}
func (r SessionRef) Key() string {
	if r.ConversationID > 0 {
		return fmt.Sprintf("conv-%d", r.ConversationID)
	}
	return fmt.Sprintf("task-%d-exp-%d-main-s%d", r.TaskID, r.ExplorationID, r.MainSeg)
}
func (r SessionRef) GateKey() string {
	if r.ConversationID > 0 {
		return r.Key()
	}
	return fmt.Sprintf("task-%d-main", r.TaskID)
}

type Attachment struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	Abs  string `json:"abs,omitempty"`
}
type Prompt struct {
	Text        string       `json:"text"`
	Attachments []Attachment `json:"attachments"`
}
type Admission struct {
	ID          string     `json:"id"`
	ClientID    string     `json:"client_request_id"`
	Kind        string     `json:"kind"`
	TargetRunID string     `json:"target_run_id,omitempty"`
	Status      string     `json:"status"`
	Session     SessionRef `json:"session"`
	Prompt      Prompt     `json:"prompt"`
	Revision    int64      `json:"revision,string"`
	Ordinal     int64      `json:"ordinal,string"`
}
type Run struct {
	ID              string     `json:"id"`
	InputID         string     `json:"input_id"`
	TurnID          string     `json:"turn_id"`
	SupersedesRunID string     `json:"supersedes_run_id,omitempty"`
	TranscriptID    string     `json:"transcript_id"`
	StartRecordUUID string     `json:"start_record_uuid"`
	EndRecordUUID   string     `json:"end_record_uuid,omitempty"`
	Status          string     `json:"status"`
	Session         SessionRef `json:"session"`
	HistoryRevision int64      `json:"history_revision,string"`
	Prompt          Prompt     `json:"prompt"`
}
type ControlState struct {
	Session         SessionRef      `json:"session"`
	Revision        int64           `json:"revision,string"`
	HistoryRevision int64           `json:"history_revision,string"`
	Paused          bool            `json:"paused"`
	Active          *Run            `json:"active"`
	Inputs          []Admission     `json:"inputs"`
	Capabilities    map[string]bool `json:"capabilities"`
}
type ContextCategory struct {
	Name      string `json:"name"`
	Tokens    int    `json:"tokens"`
	Estimated bool   `json:"estimated"`
}
type ContextView struct {
	HistoryRevision  int64             `json:"history_revision,string"`
	Sequence         int64             `json:"sequence,string"`
	ProfileID        int64             `json:"profile_id,string"`
	Model            string            `json:"model"`
	Compactor        string            `json:"compactor"`
	Source           string            `json:"source"`
	Summary          string            `json:"summary"`
	UpdatedAt        time.Time         `json:"updated_at"`
	Window           int               `json:"window"`
	Used             int               `json:"used"`
	OutputReserve    int               `json:"output_reserve"`
	Headroom         int               `json:"headroom"`
	CompressionCount int               `json:"compression_count"`
	Estimated        bool              `json:"estimated"`
	Categories       []ContextCategory `json:"categories"`
}
type CompactOptions struct {
	Focus      string `json:"focus"`
	KeepRecent int    `json:"keep_recent"`
}
type HistoryChange struct {
	ClientID         string  `json:"client_request_id"`
	Kind             string  `json:"kind"`
	TurnID           string  `json:"turn_id"`
	Title            string  `json:"title"`
	ExpectedRevision int64   `json:"expected_revision,string"`
	Replacement      *Prompt `json:"replacement,omitempty"`
}
