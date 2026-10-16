package db

import (
	"github.com/Autumn-27/artex/chatstate"
	"github.com/Autumn-27/norma/transcript"
	"testing"
)

func TestChatControlsArchiveRoundTrip(t *testing.T) {
	d, _ := controlFixture(t)
	if err := d.EnsureLLMUsageTable(); err != nil {
		t.Fatal(err)
	}
	if err := d.EnsureLLMRecordsTable(); err != nil {
		t.Fatal(err)
	}
	task, err := d.CreateTaskWithOptions("archive chat", "idle", TaskCreateOptions{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = d.Exec(`DELETE FROM task_archives WHERE task_id=$1`, task.ID)
		_ = d.DeleteTask(task.ID)
	}()
	ref := chatstate.SessionRef{TaskID: task.ID, ExplorationID: task.ExplorationID}
	child, err := d.NewMainSessionBranch(t.Context(), ref, "parent-turn", "child")
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := chatstate.PrepareTranscript(t.Context(), transcript.NewStore(t.TempDir()), child, nil)
	if err != nil {
		t.Fatal(err)
	}
	state, _ := d.ChatControlState(t.Context(), child)
	if err = d.PublishChatHistory(t.Context(), child, state.Revision, prepared); err != nil {
		t.Fatal(err)
	}
	if _, err = d.Exec(`UPDATE main_sessions SET preparing=false WHERE exploration_id=$1 AND seq=$2`, child.ExplorationID, child.MainSeg); err != nil {
		t.Fatal(err)
	}
	queueInput(t, d, child, "completed before archive")
	run, e := d.ClaimNextChatInput(t.Context(), child.GateKey())
	if e != nil || run == nil {
		t.Fatal(run, e)
	}
	if e = d.FinishChatRun(t.Context(), run.ID, "completed"); e != nil {
		t.Fatal(e)
	}
	if e = d.SaveChatContext(t.Context(), child, run.ID, chatstate.ContextView{HistoryRevision: 1, Sequence: 3, Model: "fixture", Used: 100, Window: 32000, Estimated: true}); e != nil {
		t.Fatal(e)
	}
	parked := queueInput(t, d, child, "parked after restore")
	if err := d.SetPaused(task.ID, true); err != nil {
		t.Fatal(err)
	}
	archive, err := d.QueueTaskArchive(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := d.ClaimTaskArchiveJob(t.Context())
	if err != nil || claimed == nil || claimed.ID != archive.ID || claimed.State != Archiving {
		t.Fatalf("archive claim: %+v, %v", claimed, err)
	}
	snapshot, err := d.SnapshotTaskArchive(task.ID)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.FormatVersion != 4 || rawRowCount(snapshot.Tables["chat_transcript_revisions"]) == 0 {
		t.Fatal("v4 archive missing transcript revisions")
	}
	if err := d.CompleteTaskArchive(archive.ID, snapshot, "/offline/chat-v4.tar.zst", "fixture", 100, 50); err != nil {
		t.Fatal(err)
	}
	var hotGates int
	if err := d.QueryRow(`SELECT count(*) FROM chat_control_gates WHERE task_id=$1`, task.ID).Scan(&hotGates); err != nil || hotGates != 0 {
		t.Fatalf("archive retained hot control gates: %d, %v", hotGates, err)
	}
	if _, err := d.QueueTaskArchiveRestore(archive.ID); err != nil {
		t.Fatal(err)
	}
	claimed, err = d.ClaimTaskArchiveJob(t.Context())
	if err != nil || claimed == nil || claimed.ID != archive.ID || claimed.State != Restoring {
		t.Fatalf("restore claim: %+v, %v", claimed, err)
	}
	if _, err := d.RestoreTaskArchive(archive.ID, snapshot, 0); err != nil {
		t.Fatal(err)
	}
	if err := d.CompleteTaskArchiveRestore(archive.ID); err != nil {
		t.Fatal(err)
	}
	restored, err := d.ChatControlState(t.Context(), child)
	if err != nil || !restored.Paused || len(restored.Inputs) != 1 {
		t.Fatal(restored, err)
	}
	if restored.Inputs[0].ID != parked.ID || restored.Inputs[0].Prompt.Text != parked.Prompt.Text || restored.Inputs[0].Status != "queued" || restored.Inputs[0].Session != child {
		t.Fatal(restored, err)
	}
	revision, err := d.ActiveChatTranscript(t.Context(), child)
	if err != nil || revision.TranscriptID != prepared.TranscriptID {
		t.Fatal(revision, err)
	}
	contextView, e := d.ChatContext(t.Context(), child)
	if e != nil || contextView.Used != 100 {
		t.Fatal("context missing", contextView, e)
	}
	restoredRun, e := d.ChatRun(t.Context(), run.ID)
	if e != nil || restoredRun.Session != child || restoredRun.HistoryRevision != 1 {
		t.Fatal("run identity changed", restoredRun, e)
	}
	if run, err := d.ClaimNextChatInput(t.Context(), ref.GateKey()); err != nil || run != nil {
		t.Fatal("restore dispatched pending work", run, err)
	}
	if TaskArchiveFormatVersion != 4 {
		t.Fatal("archive format not upgraded")
	}
	for _, v := range []int{1, 2, 3, 4} {
		if !IsTaskArchiveFormatSupported(v) {
			t.Fatal(v)
		}
	}
}
