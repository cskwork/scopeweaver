# Chat Controls and Context Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add Hermes-style queue, steer, retry, edit, branch, and context controls to standalone and task MainAgent chat with complete UI access.

**Architecture:** A shared conversation control service persists admission and history revisions in PostgreSQL and drives the existing ChatAgent/MainAgent runners. Norma transcripts remain the full model record; the existing activity streams remain the display record. History changes prepare a durable transcript revision before publishing it, and every operation shares the existing conversation or task MainAgent admission boundary.

**Tech Stack:** Existing Go 1.26.3, Norma v0.4.3, PostgreSQL/pgx, Next.js/React/TypeScript, Node tests, and Playwright. Keep production dependencies unchanged.

**Spec:** [Approved design](../specs/2026-10-16-chat-controls-design.md). Read its pinned Hermes references before the task that ports that behavior. [Document paste](2026-10-16-chat-document-paste.md) is an independently deliverable companion plan; UI integration below consumes its uploaded manifests and submission gate.

## Global Constraints

- Every supported operation has a visible UI control; slash commands provide optional shortcuts.
- Use Hermes Agent and Hermes Desktop as the behavioral and implementation references, while retaining Scopeweaver's Go, Norma, PostgreSQL, and Next.js architecture and current visual style.
- A task retains its existing limit of one active MainAgent run across all its segments.
- MainAgent queue pause and Resume apply across the task's segments, and the UI identifies this task-wide effect.
- Steering is text only and targets a specific active run ID.
- Branch creation is idempotent and idle-only.
- Manual compression is serialized with session admission and is available only while idle.
- Keep archived display history intact.
- No active pointer may reference an incomplete transcript.
- All new UI text follows the existing localization system.

## Review Focus

1. Restart between input acceptance and tool completion: retain input as unknown, pause the gate, and require acknowledgement without replay. Tasks 2 and 11.
2. Stop or task deletion races a queue claim from another MainAgent segment: the task gate prevents another run. Tasks 2 and 10.
3. Branch point predates compression: no summary containing later facts enters the child. Task 6.
4. A late snapshot arrives after retry or segment switch: reject its old history revision and keep `/btw` scoped to its actual segment. Task 9.
5. Profile switch reduces the context window, or summary persistence fails: recompute capacity and retain the old active transcript on failure. Tasks 8 and 9.

## File and contract map

| Area | New files | Existing integration points |
| --- | --- | --- |
| Protocol and state | `chatstate/types.go` | `server/chatupload.go`, `web/src/lib/types.ts` |
| Durable control | `db/chat_controls.go`, `server/chat_dispatch.go`, `server/chat_controls.go` | `db/schema.sql`, `server/conversations.go`, task chat handlers in `server/server.go` |
| Live correction | `agent/chat_steering.go` | `agent/chat.go`, `agent/mainagent.go`, existing `server/engine.go` guard behavior |
| History revisions | `chatstate/transcripts.go`, `server/chat_history.go`, `db/chat_history.go` | `agent/capture.go`, activity readers, `db/exploration.go` |
| Context | `agent/chat_context.go`, `agent/chat_compact.go`, `server/chat_context.go`, `db/chat_context.go` | provider binding, runtime policy, Noa, side-question capture |
| UI | `web/src/hooks/use-chat-controls.ts`, `web/src/components/chat-queue.tsx`, `chat-message-actions.tsx`, `chat-context-inspector.tsx` | standalone chat, task sessions, Transcript, API/types/localization |
| Lifecycle | `db/chat_controls_archive.go` | task archive/delete/restore and conversation deletion |

`chatstate` contains shared types and filesystem transcript helpers, imports no `db` or `server`, and has no provider credentials. Use string UUIDs for input/run/revision IDs. Serialize all int64 identifiers, ordinals, and revision counters in the new control DTOs as decimal strings using `json:",string"`; TypeScript represents them as strings. Token counts, MainSeg, and array indices remain numbers. Retain existing numeric conversation IDs at compatibility endpoints.

Define these Go types in Task 1; all exported fields receive matching snake_case JSON tags:

```go
type SessionRef struct {
    ConversationID, TaskID, ExplorationID int64
    MainSeg int
}
type Attachment struct { Name, Path string; Size int64; Abs string }
type Prompt struct { Text string; Attachments []Attachment }
type Admission struct {
    ID, ClientID, Kind, TargetRunID, Status string
    Session SessionRef
    Prompt Prompt
    Revision, Ordinal int64
}
type Run struct {
    ID, InputID, TurnID, SupersedesRunID, TranscriptID, StartRecordUUID string
    Session SessionRef
    HistoryRevision int64
}
type ControlState struct {
    Session SessionRef
    Revision, HistoryRevision int64
    Paused bool
    Active *Run
    Inputs []Admission
    Capabilities map[string]bool
}
type ContextCategory struct { Name string; Tokens int; Estimated bool }
type ContextView struct {
    HistoryRevision, Sequence int64
    ProfileID int64
    Model, Compactor, Source, Summary string
    UpdatedAt time.Time
    Window, Used, OutputReserve, Headroom, CompressionCount int
    Estimated bool
    Categories []ContextCategory
}
type CompactOptions struct { Focus string; KeepRecent int }
type HistoryChange struct {
    ClientID, Kind, TurnID, Title string
    ExpectedRevision int64
    Replacement *Prompt
}
```

Input kinds are `turn` and `steer`. Input states are `queued`, `claimed`, `delivered`, `completed`, `failed`, `cancelled`, `undelivered`, and `unknown`; `delivered` applies to a steer, while a claimed turn creates a Run. Run states are `running`, `completed`, `failed`, `stopped`, and `unknown`; a stopped run cancels its claimed turn input, while pending turns remain queued behind the paused gate. Session keys are `conv-<id>` or `task-<id>-exp-<id>-main-s<seg>`; `GateKey()` is `conv-<id>` or `task-<id>-main`. Preserve legacy physical transcript names until a published revision exists. Errors shared across layers are `ErrConflict`, `ErrBusy`, `ErrGone`, `ErrUnknownRun`, and `ErrAmbiguousHistory` in `chatstate`.

## HTTP contract

Standalone base is `/api/conversations/{id}/controls`. Task base is `/api/tasks/{id}/chat/controls?seg=N`; append the path before the query string. Resolve the parent from the authenticated route and confirm segment ownership. New clients always send `seg`; old task clients retain current-segment behavior at their existing endpoint.

| Method and suffix | Input and output |
| --- | --- |
| GET base | `ControlState`; safe during an active run. |
| POST `/inputs` | `{client_request_id, mode: "send"|"queue"|"steer", message, attachments, target_run_id?}`; return accepted `Admission`. Send on a busy gate returns 409; the UI explicitly chooses queue. |
| PATCH `/inputs/{inputID}` | `{expected_revision, message, attachments}`; return updated queued input. |
| DELETE `/inputs/{inputID}` | `expected_revision` query; cancel only an unclaimed input. |
| POST `/queue/order` | `{expected_revision, input_ids}`; same queued membership, new order. |
| POST `/pause`, `/resume` | `{expected_revision}`; return `ControlState`. Resume refuses unacknowledged unknown runs. |
| POST `/inputs/{inputID}/discard` | `{expected_revision}`; acknowledge an unknown run and keep the queue paused. |
| POST `/retry`, `/edit`, `/branch` | `HistoryChange`; return `{session, history_revision, run_id?}`. Edit includes Replacement, branch includes completed TurnID, retry resolves the latest real turn. |
| GET `/attempts?turn_id=ID` | Attempts belonging to that session/turn: `{run_id, status, active, created_at}`. |
| GET `/attempts/{runID}/activities` | Paginated existing Activity DTO shape restricted to that attempt, including inactive audit history and source links. |
| GET `/context` | `ContextView`, excluding credentials and proxy configuration. |
| POST `/compact` | `{client_request_id, expected_revision, focus, keep_recent}`; default retention 2, minimum 1; return published `ContextView`. |

Reuse current Stop endpoints, routing them through the shared service. All new mutations are idempotent by session and client ID where supplied, and revision-checked. Same idempotency key with a different payload returns 409. Return 404 for unavailable parents; return 409 for stale revisions, active history mutation, unknown runs, ambiguous legacy boundaries, and unavailable side snapshots. Preserve existing localized error responses. Streaming/polling transports stay unchanged; the control hook polls state while pending work exists and reloads on explicit mutations and session focus.

## Task 1 Session identities and durable admission

**Files:** Create `chatstate/types.go`, `chatstate/types_test.go`, `db/chat_controls.go`, `db/chat_controls_test.go`; modify `db/schema.sql`, `db/exploration.go`, `server/dto.go`, and alias the existing `server.chatAttachment` to `chatstate.Attachment` in `server/chatupload.go`.

**Interfaces:** Implement `SessionRef.Validate() error`, `Key() string`, `GateKey() string`; DB methods `AdmitChatInput(ctx context.Context, ref chatstate.SessionRef, clientID, mode, targetRunID string, prompt chatstate.Prompt) (chatstate.Admission, error)`, `ChatControlState(ctx context.Context, ref chatstate.SessionRef) (chatstate.ControlState, error)`, `UpdateChatInput(ctx context.Context, ref chatstate.SessionRef, id string, expected int64, prompt chatstate.Prompt) (chatstate.Admission, error)`, and `ReorderChatInputs(ctx context.Context, ref chatstate.SessionRef, expected int64, ids []string) error`. Extend Activity and its DTO with optional `turn_id`, `run_id`, `active_attempt`, and `history_mutable`; absent legacy metadata does not imply mutation is safe.

- [ ] **Step 1 Write failing tests** `TestChatSessionKeys` and `TestChatAdmission`. Assert `segment0.Key() != segment1.Key()`, `segment0.GateKey() == segment1.GateKey()`, `first.ID == duplicate.ID`, and `errors.Is(err, chatstate.ErrConflict)` for changed-payload duplicates and stale edits. Invalid mixed-parent refs fail validation. Verify a reopened DB returns the same queue and attachment manifests, including `state.Inputs[0].Prompt.Attachments[0].Path == "uploads/report.pdf"`.
- [ ] **Step 2 Run** `go test ./chatstate -run '^TestChatSessionKeys$' -count=1` and `./scripts/test-db.sh -run '^TestChatAdmission$' -v`. Expect missing-type/method failures. Never count skipped DB tests as evidence.
- [ ] **Step 3 Implement types and idempotent schema additions**: `chat_control_gates` owns pause/active-run state; `chat_control_sessions` owns parent IDs, segment, lineage, revision/published transcript pointer; `chat_control_inputs` owns durable payload/order/revision; `chat_control_runs` owns turn/attempt IDs, original prompt, transcript boundaries, model identity, status, and usage. Parent-specific foreign keys cascade; child lineage references use SET NULL so parent deletion cannot delete children. Add optional turn/run/active-attempt fields to activity tables without changing old rows. Lock parent, gate, session, then input in that order in every transaction. Use the existing schema migration lock, not a new migration framework.
- [ ] **Step 4 Rerun both commands** and assert actual named DB tests execute and pass. Reapply schema twice in `TestChatAdmission` to establish upgrade idempotency.
- [ ] **Step 5 Commit** the listed files with `feat(chat): persist session identities and input admission`.

## Task 2 Dispatcher HTTP controls and recovery

**Files:** Create `server/chat_dispatch.go`, `server/chat_controls.go`, `server/chat_dispatch_test.go`; extend `db/chat_controls.go`. Modify `server/conversations.go`, `server/server.go`, and startup/shutdown wiring.

**Interfaces:** Add DB methods `ClaimNextChatInput(ctx context.Context, gate string) (*chatstate.Run, error)`, `FinishChatRun(ctx context.Context, runID, status string) error`, `PauseChatGate(ctx context.Context, ref chatstate.SessionRef, expected int64) error`, `ResumeChatGate(ctx context.Context, ref chatstate.SessionRef, expected int64) error`, `DiscardUnknownChatRun(ctx context.Context, ref chatstate.SessionRef, inputID string, expected int64) error`, `CancelChatInput(ctx context.Context, ref chatstate.SessionRef, id string, expected int64) error`, and `RecoverChatRuns(ctx context.Context) error`. In server, `executeChatRun(ctx context.Context, run chatstate.Run) error` adapts the existing runners; `dispatchChatGate(gate string)` claims durable work and never holds a DB transaction during inference.

- [ ] **Step 1 Write failing tests** `TestChatQueueClaimOnce`, `TestChatStopPausesAllSegments`, `TestChatRestartUnknownBarrier`, and `TestChatQueueFailurePauses`. Assert `successfulClaims == 1`; after Stop, `otherSegmentProviderCalls == 0`; reopening produces `recovered.Status == "unknown"`; Resume gives `errors.Is(err, chatstate.ErrUnknownRun)` until Discard; Discard leaves `state.Paused == true`; explicit Resume executes the remaining prompt once; failed runs pause the gate. Inject barriers before claim, after durable claim, and before first model call.
- [ ] **Step 2 Run** `./scripts/test-db.sh -race -run '^TestChat(QueueClaimOnce|StopPausesAllSegments|RestartUnknownBarrier|QueueFailurePauses)$' -v`. Expect missing service behavior.
- [ ] **Step 3 Implement the dispatcher and queue endpoints from the HTTP contract**. Validate uploaded-file ownership and mention references at admission and claim. Preserve busy compatibility flags and register cancellation before returning 202. Every user path, including old send handlers, takes the durable gate; generated trigger runs take an external gate reservation but remain ordered by their existing trigger pump. Stop/failure pauses user draining; successful completion wakes it. Recovery is explicit for claimed runs, leaves queued payloads intact, and does not reuse a lost owner. Provider cancellation, deletion, and unknown execution must not be reported as success.
- [ ] **Step 4 Rerun tests** and existing `TestConversationListRunningState`, worker-message, and trigger-merge regressions through the disposable DB script. Assert zero leaked gate reservations.
- [ ] **Step 5 Commit** with `feat(chat): dispatch durable follow-ups and recover lost runs`.

## Task 3 Run metadata and live steering

**Files:** Create `agent/chat_steering.go`, `agent/chat_steering_test.go`, `agent/chat_run_control.go`, `server/chat_steering_test.go`; modify `agent/chat.go`, `agent/mainagent.go`, `agent/capture.go`, `agent/runinfo.go`, and `db/chat_controls.go`.

**Interfaces:** `agent.WithChatRun(ctx context.Context, run chatstate.Run, control ChatRunControl) context.Context`; `ChatRunControl` has `TakeSteer(ctx context.Context, runID string) (*chatstate.Admission, error)`, `SteerDelivered(ctx context.Context, runID, inputID string) error`, and `RecordBoundary(ctx context.Context, runID, beforeUUID, afterUUID string) error`. Implement `wrapChatSteering(inner harness.HookRunner, run chatstate.Run, control ChatRunControl) harness.HookRunner` following the existing `steerHooks` safe boundary behavior. The server adapter implements this interface over the DB and activity writer.

- [ ] **Step 1 Write failing tests** `TestChatSteerTargetsRun`, `TestChatSteerPreservesGuard`, `TestChatSteerUndeliveredAfterFinish`, and `TestChatRunBoundaries`. Assert duplicate admission yields `len(deliveredCorrections) == 1`, a cancelled/later run cannot consume it, the proposed next tool is blocked for replanning, prior executed tools are not repeated, and `guardCalls == 1` for the subsequent action. Attachments or a blocking approval prevent steer admission. A text-only terminal response with no subsequent boundary leaves `input.Status == "undelivered"`. Verify both streaming and non-streaming runs link real transcript UUIDs to their display activities.
- [ ] **Step 2 Run** `./scripts/test-db.sh -race -run '^TestChat(Steer|RunBoundaries)' -v`; also run the pure hook tests with a fake guard/provider. Expect missing hooks and metadata.
- [ ] **Step 3 Implement hooks and run identity propagation** without replacing guard hooks or changing Worker behavior. Reserve steering delivery for the target run, write its correction through the existing denied-tool-result/replanning path, and mark delivered only after that correction is recorded. On ambiguous persistence failure, stop and mark the owning run unknown rather than consuming the steer twice. Mark remaining steering undelivered on terminal state. Acknowledge before/after UUIDs from the active transcript, excluding internal reminders and timeline markers as turn starts. Before marking completed, verify terminal messages were durably recorded; Norma's writer can retain append errors without returning them through Prompt, so an unpersisted terminal result must leave the run unknown rather than successful.
- [ ] **Step 4 Rerun the tests** plus `TestWorkerChatMarkerOnlyMatchesItsNormalUserTurn` and existing guard/worker intervention tests. Expect paired tool messages and one delivery per steer.
- [ ] **Step 5 Commit** with `feat(chat): steer active runs without bypassing guard hooks`.

## Task 4 Queue UI and command routing

**Files:** Create `web/src/hooks/use-chat-controls.ts`, `web/src/components/chat-queue.tsx`, `web/src/lib/chat-commands.ts`, `web/src/lib/chat-commands.test.mjs`. Modify `web/src/lib/types.ts`, `web/src/lib/api.ts`, standalone chat, task sessions, and the interface/zh-Hans message catalogs.

**Interfaces:** TypeScript mirrors shared DTOs using the names `ChatSessionRef`, `ChatPrompt`, `ChatAdmission`, `ChatControlState`, `ChatContextView`, and `ChatCompactOptions`. Export `chatControlsBase(ref: ChatSessionRef): string` and `parseChatCommand(text: string): ChatCommand | null`. `ChatCommand` discriminates queue/steer with `text: string`, retry/context/usage with no payload, branch with `title: string`, compact with `options: ChatCompactOptions`, and invalid with `message: string`; unsupported command text returns null. `useChatControls(ref: ChatSessionRef | null)` returns `{state: ChatControlState | null, reload(): Promise<void>, submit(mode: "send" | "queue" | "steer", prompt: ChatPrompt, targetRunID?: string): Promise<ChatAdmission>, updateInput(id: string, expected: string, prompt: ChatPrompt): Promise<ChatAdmission>, removeInput(id: string, expected: string): Promise<void>, reorder(expected: string, ids: string[]): Promise<void>, pause(): Promise<void>, resume(): Promise<void>, discardUnknown(id: string, expected: string): Promise<void>}`. Every submission retains its client request ID until a definitive response. Capability keys are `queue`, `steer`, `retry`, `edit`, `branch`, `context`, and `compact`; boundary-specific mutation eligibility also comes from activity metadata.

- [ ] **Step 1 Add failing parser tests** asserting `parseChatCommand("/btw question") === null`, `parseChatCommand("/queue /retry").text === "/retry"`, `parseChatCommand("/steer focus auth").kind === "steer"`, `parseChatCommand("/compress here 0").kind === "invalid"`, and plain text returns null. Add browser assertions for visible queue/edit/reorder/remove/Resume/Discard controls, a lost response retried with the same client ID, and a session switch that discards stale callbacks but restores each draft.
- [ ] **Step 2 Run** `cd web && node --test src/lib/chat-commands.test.mjs`. Expect missing parser. Add initial browser cases to the runner completed in Task 11.
- [ ] **Step 3 Implement the hook, API methods, parser, and queue panel** following the pinned Hermes panel/hook. Default busy Send to Queue, expose Steer only for eligible text, and keep Stop independent. Preserve the user's pre-edit draft on cancel; freeze only uploaded manifests from the companion paste plan. Show task-wide pause wording. Route read-only commands immediately; enable history/context commands only when the server capabilities for later tasks are present, never by sending their text to the model.
- [ ] **Step 4 Run** `cd web && npm test && npm run build:static`. Exercise the current queue cases at desktop/mobile widths against a disposable fixture backend; the final runner is delivered in Task 11.
- [ ] **Step 5 Commit** with `feat(chat): expose queue and steer controls in the composer`.

## Task 5 Durable transcript revisions

**Files:** Create `chatstate/transcripts.go`, `chatstate/transcripts_test.go`, `db/chat_history.go`, `db/chat_history_test.go`; modify `db/schema.sql`, `agent/chat.go`, `agent/mainagent.go`, and capture wiring.

**Interfaces:** Define `chatstate.TranscriptRevision` with `ID, SessionKey, TranscriptID, EndRecordUUID, SHA256 string` and `HistoryRevision int64`. In chatstate, `PrepareTranscript(ctx context.Context, store *transcript.Store, session SessionRef, records []transcript.Record) (TranscriptRevision, error)` writes and syncs a fresh JSONL file plus manifest; `ValidateTranscriptRevision(store *transcript.Store, revision TranscriptRevision) error`. DB methods are `PublishChatHistory(ctx context.Context, ref chatstate.SessionRef, expected int64, revision chatstate.TranscriptRevision) error` and `ActiveChatTranscript(ctx context.Context, ref chatstate.SessionRef) (*chatstate.TranscriptRevision, error)`. Resolve the logical session separately from physical TranscriptID, preserving existing workspace, cache headers, runinfo, and side-parent identities.

- [ ] **Step 1 Write failing tests** `TestChatTranscriptPublishCrashWindows`, `TestChatTranscriptResumeStructured`, and `TestChatTranscriptLegacyMapping`. Assert `after.TranscriptID == before.TranscriptID` following a partial write, checksum mismatch, or failed DB publish; an unreferenced prepared file is recoverable garbage. Compaction boundaries, tool IDs, reasoning metadata, and skill unlock calls survive resume; repeated identical prompt text is mapped by order/UUID, not a truncated-summary match. Assert `errors.Is(err, chatstate.ErrAmbiguousHistory)` for ambiguity while ordinary chat remains usable. With fixture run costs 10 and 20 input tokens, retry plus branch copying preserves `billedInputTokens == 30` before any new inference.
- [ ] **Step 2 Run** `go test ./chatstate -run '^TestChatTranscript' -count=1` and `./scripts/test-db.sh -run '^TestChatTranscript' -v`. Expect missing revision functions.
- [ ] **Step 3 Implement preparation, validation, publication, and revision selection**. Copy full records rather than display summaries; exclude superseded active attempts from model replay while retaining them in archived audit data. Normalize record SessionID/parent chains into the new physical file. Fsync file and parent directory where supported before CAS publication. Add `chat_transcript_revisions`; retain the old file on every failure. Use fresh physical transcript IDs with `Session.Resume` instead of changing Norma source, and propagate Resume errors instead of silently starting empty. Namespace Noa archive state by logical session and history revision so old state cannot reintroduce superseded messages. Norma Terminal.Usage is cumulative within one Prompt/query, not the entire resumed lifetime: charge that run-local value once, never subtract restored usage or charge copied transcript usage again.
- [ ] **Step 4 Rerun both commands** plus existing SDK-integration/session-header and skill-unlock regressions. Expect legacy filenames to continue loading until their first published mutation.
- [ ] **Step 5 Commit** with `feat(chat): publish recoverable transcript revisions`.

## Task 6 Retry edit and conversation branches

**Files:** Create `server/chat_history.go`, `server/chat_history_test.go`; extend `db/chat_history.go` and `chatstate/transcripts.go`. Modify `db/exploration.go`, activity history readers in `db/conversation.go`, and attachment validation in `server/chatupload.go`.

**Interfaces:** `Server.changeChatHistory(ctx context.Context, ref chatstate.SessionRef, change chatstate.HistoryChange) (ChatHistoryResult, error)`; `ChatHistoryResult` has `Session chatstate.SessionRef`, `HistoryRevision int64`, `RunID string`. `DB.CommitChatHistoryChange(ctx context.Context, sourceRef, targetRef chatstate.SessionRef, change chatstate.HistoryChange, revision chatstate.TranscriptRevision) (chatstate.ControlState, error)` publishes pointer, lineage, idempotency result, and any replacement input in one transaction. `DB.NewMainSessionBranch(ctx context.Context, ref chatstate.SessionRef, parentTurnID, clientID string) (chatstate.SessionRef, error)` allocates a unique preparing segment while holding the task gate. `CopyChatAttachments(ctx context.Context, sourceDir, targetDir string, prompt chatstate.Prompt) (chatstate.Prompt, error)` copies only validated manifests, atomically staging the destination.

- [ ] **Step 1 Write failing tests** `TestChatRetryLatestUserTurn`, `TestChatEditLatestAndEarlier`, `TestChatBranchBeforeCompaction`, `TestChatBranchAttachmentRetention`, and `TestChatBranchIdempotency`. Assert retry keeps the session identity, supersedes exactly one attempt, uses the selected current model, retains all prior billed usage, and does not replay old steering. Latest edit rewinds that session; older edit returns a child. Put a distinctive later fact into a parent compression summary and assert `!strings.Contains(childRequestText, laterFact)`. Delete the standalone parent and read the child's copied document bytes successfully. Concurrent duplicate branch requests produce one child; task segments are allocated uniquely, and no explicit Branch runs a model automatically.
- [ ] **Step 2 Run** `./scripts/test-db.sh -race -run '^TestChat(Retry|Edit|Branch)' -v`. Expect missing handlers/behavior.
- [ ] **Step 3 Implement history mutation and attempt-list/activity endpoints** using Task 5 revisions and the shared gate. Keep all mutation requests idempotent including retry/edit admission; use CommitChatHistoryChange to publish atomically with any follow-up input. Reconstruct branch prefixes from original chronological births through the source record boundary, dropping summaries/tail duplicates introduced after it. Re-summarize only that prefix if required by the new window before first execution. Rewrite attachment manifests, not arbitrary text substrings. Show unavailable generated artifacts, retain provenance, copy no running process, and clean failed child preparation. Preparing child/segment rows are omitted from ordinary lists until publication. Task children share task state and artifacts but get independent segment transcripts and lineage.
- [ ] **Step 4 Rerun tests** with a restarted server and both provider transport modes. Verify old attempt traces remain inspectable, active display/history pagination selects the current attempt, and cumulative usage is charged once per actual inference.
- [ ] **Step 5 Commit** with `feat(chat): retry edit and branch structured conversations`.

## Task 7 Message actions and branch navigation

**Files:** Create `web/src/components/chat-message-actions.tsx`, `web/src/lib/chat-history-actions.ts`, `web/src/lib/chat-history-actions.test.mjs`; modify `web/src/components/transcript.tsx`, standalone chat, task sessions, API/types, and localization catalogs.

**Interfaces:** Add hook methods `retry(clientID: string): Promise<ChatHistoryResult>`, `edit(change: ChatHistoryChange): Promise<ChatHistoryResult>`, and `branch(change: ChatHistoryChange): Promise<ChatHistoryResult>`; `ChatHistoryChange` mirrors Go HistoryChange. `ChatHistoryResult = {session: ChatSessionRef; history_revision: string; run_id?: string}`. `availableHistoryActions(activity: Activity, state: ChatControlState): {edit: boolean; retry: boolean; branch: boolean}` selects actions from server-supplied turn/run metadata, not message text. All promises use the existing API error convention.

- [ ] **Step 1 Add failing tests** for latest versus earlier actions, unavailable legacy mapping, busy mutation disablement, and a completed tool-only response. Browser cases assert edit restores text/attachments, cancelling preserves the draft, retry updates the active attempt without adding a conversation rail entry, branching opens a child with a parent link, and task branching selects the new segment with shared-task wording.
- [ ] **Step 2 Run** `cd web && node --test src/lib/chat-history-actions.test.mjs`. Expect missing action selection.
- [ ] **Step 3 Implement actions and navigation** with keyboard-accessible buttons and localized retry side-effect copy. Add a scrollable read-only disclosure for superseded attempts using lazy activity detail. Preserve source approval links and activity pagination. Parent links may show unavailable after parent deletion without breaking the child.
- [ ] **Step 4 Run** `cd web && npm test && npm run build:static`; run the new message-action browser checks through the Task 11 runner. Expect both `/chat` and task session selectors to reflect the server result.
- [ ] **Step 5 Commit** with `feat(chat): add message actions and branch navigation`.

## Task 8 Context telemetry and capacity

**Files:** Create `agent/chat_context.go`, `agent/chat_context_test.go`, `server/chat_context.go`, `db/chat_context.go`; modify `agent/assembly.go`, `agent/deferred.go`, `agent/local_runtime.go`, `agent/noa.go`, `server/assembly.go`, `server/side_questions.go`, `sidequestion/capture.go`, and `db/schema.sql` for provenance and actual-provider capture integration.

**Interfaces:** `MeasureChatContext(req llm.CompletionRequest, profileID int64, model string, window, reserve int, provenance ContextProvenance) chatstate.ContextView`. Define `ContextProvenance` in agent with `SystemSegments map[int]string`, `ToolCategories map[string]string`, and `MessageBlocks map[string]string`; message-block keys are `<message-index>:<block-index>`, values identify the sole owning category. Capture provenance while assembling skills/MCP and map it to the final request; unmatched content is `other`. `ObserveChatContext(ctx context.Context, view chatstate.ContextView) error` fills the context-bound run's HistoryRevision and increasing Sequence before saving. DB methods `SaveChatContext(ctx context.Context, ref chatstate.SessionRef, runID string, view chatstate.ContextView) error` and `ChatContext(ctx context.Context, ref chatstate.SessionRef) (chatstate.ContextView, error)` reject old history/run/sequence and feed GET context.

- [ ] **Step 1 Write failing tests** `TestChatContextPartition`, `TestChatContextMeasuredAnchor`, `TestChatContextModelSwitch`, and `TestChatContextCompactorPolicy`. Assert `categoryTotal == estimatedRequestTokens` for complete single attribution, sparse providers give `view.Estimated == true`, and `view.Used != cumulativeBilledTokens` in a fixture where those values deliberately differ. A lower-window profile changes Headroom before a new call, and stale profile identity does not reuse the old anchor. Test native 32K/64K policy remains at 72%, with current output limits and Noa fallback unchanged.
- [ ] **Step 2 Run** `./scripts/test-db.sh -run '^TestChatContext' -v`. Expect missing telemetry.
- [ ] **Step 3 Implement telemetry** at the actual bound provider request, composing the existing side-question capture rather than making competing observers. Store last request source/time and a provider input-token anchor; categories always retain Estimated true when locally calculated. Use the selected profile's effective output reservation, mark unknown provider defaults as estimates, and calculate Headroom from window minus used minus reservation. Wrap the current compactor to observe successful automatic boundaries, without changing its trigger policy or counting summary model requests as main context. Add `chat_context_snapshots` and GET context capability.
- [ ] **Step 4 Rerun tests** plus native runtime, Noa, priority, and provider binding regressions. Assert telemetry failures cannot abort a valid main run or leak credentials into ContextView.
- [ ] **Step 5 Commit** with `feat(chat): expose measured and estimated context capacity`.

## Task 9 Manual compression and segment scoped side questions

**Files:** Create `agent/chat_compact.go`, `agent/chat_compact_test.go`, `server/chat_compact_test.go`; modify `server/chat_context.go`, `agent/side_questions.go`, `sidequestion/capture.go`, `db/side_questions.go`, `server/side_questions.go`, schema, and `web/src/lib/side-questions.ts`.

**Interfaces:** `PrepareChatCompaction(ctx context.Context, records []transcript.Record, req llm.CompletionRequest, opts chatstate.CompactOptions, summarize compaction.Summarizer, inputBudget int) ([]transcript.Record, chatstate.ContextView, error)` preserves the full original record archive and appends a validated boundary/summary/tail working set using the current compactor's serialization conventions. `DB.InvalidateSideSnapshot(ctx context.Context, parent sidequestion.Parent, historyRevision int64) error`; add `MainSeg *int` to Parent and `HistoryRevision int64` to Snapshot. Add `HistoryRevision int64` and `SourceRunID string` to sidequestion.Exchange and persisted request rows, allowing side replay to exclude superseded attempts while retaining their visible history. Omitted/zero segment preserves the old main key; positive segments use distinct keys. Existing Worker keys stay unchanged.

- [ ] **Step 1 Write failing tests** `TestChatManualCompactAtomic`, `TestChatManualCompactRetainedTail`, `TestChatSideSnapshotHistoryRevision`, and `TestChatSideSnapshotSegmentIsolation`. Assert `len(retainedExchanges) == 2` by default and rejection of KeepRecent 0; focus reaches the summarizer; summary/disk-write/CAS failure leaves `after.TranscriptID == before.TranscriptID`; a complete protected tail that cannot fit returns an error. Deliver an old snapshot after publication and assert it is rejected; existing in-flight side answers complete against the original snapshot, later admission waits for a new valid snapshot, and two MainAgent segments have distinct side history. Put a sentinel fact in a side answer based on a superseded attempt; assert `!strings.Contains(nextSideRequestText, sentinelFact)` while the historical answer stays visible. Compaction with unchanged active run lineage preserves eligible side follow-ups.
- [ ] **Step 2 Run** `./scripts/test-db.sh -race -run '^TestChat(ManualCompact|SideSnapshot)' -v` and `go test ./sidequestion -count=1`. Expect missing invalidation/segment behavior.
- [ ] **Step 3 Implement POST compact** using the same gate and revision publication as history mutations. Decode retention as an optional integer: omitted becomes 2, explicit values below 1 are rejected before preparation. Preserve structured tool/reasoning pairs; use actual profile input/output budgets. Reuse existing summaries/adapters, adding a host manual preparation adapter where Norma lacks a public force-compact call. Publish only a validated, durable candidate; retain the active automatic compactor and its policy for later runs. Refresh or invalidate side snapshots on every history mutation, check revision on delayed saves, filter side replay to source runs retained in the active history, and thread `seg` through side API URL construction without breaking existing query strings. Persist segment keys in side-session rows and prevent snapshot recreation after parent deletion.
- [ ] **Step 4 Rerun commands** plus existing side-question capture, clear/cancel, archive, and overflow tests. Assert compression is idempotent by client ID and consumes no second summary call after a lost HTTP response.
- [ ] **Step 5 Commit** with `feat(chat): compact context safely and isolate side snapshots`.

## Task 10 Context inspector and lifecycle integration

**Files:** Create `web/src/components/chat-context-inspector.tsx`, `web/src/lib/chat-context-view.ts`, `web/src/lib/chat-context-view.test.mjs`, and `db/chat_controls_archive.go`. Modify control hook/API, both chat views, localization, `db/task_archives.go`, `db/task_archives_restore.go`, `server/task_archives.go`, `server/task_archive_package.go`, and parent deletion paths.

**Interfaces:** Hook methods `context(): Promise<ChatContextView>` and `compact(options: ChatCompactOptions): Promise<ChatContextView>`. `formatChatContext(view: ChatContextView): {usage: string; headroom: string; categories: {name: string; label: string}[]}` returns localized rows with explicit unknown/estimated states. Archive helper `SnapshotChatControls(ctx context.Context, tx *sql.Tx, taskID int64) (map[string]json.RawMessage, error)` and `RestoreChatControls(ctx context.Context, tx *sql.Tx, tables map[string]json.RawMessage) error` serialize the new task-bound tables; existing archive file staging carries all referenced transcript revisions.

- [ ] **Step 1 Add failing tests** for estimated/measured footer text, stale model capacity, default retention 2, and idle-only Compact now. Assert `formatted.usage.includes("estimated")` for the English estimated fixture and the corresponding catalog wording for Korean/Chinese; `compactButton.disabled === true` while busy. Add `TestChatControlsArchiveRoundTrip` and `TestChatDeleteClaimBarrier`: archive and restore task branches, transcript manifests, pending inputs, and side histories; assert `restoredState.Paused == true`; v1/v2/v3 archive restore still works; deletion during claim gives `providerCalls == 0` and rejects late snapshot saves.
- [ ] **Step 2 Run** `cd web && node --test src/lib/chat-context-view.test.mjs` and `./scripts/test-db.sh -race -run '^TestChat(ControlsArchive|DeleteClaim)' -v`. Expect missing rendering/lifecycle support.
- [ ] **Step 3 Implement inspector and lifecycle changes**. Footer meter opens a Popover on desktop and Drawer on mobile. Include category table, model, timestamps, compactor, count, summary availability, focus, retention, and busy/failure/success feedback. Route `/context` and `/usage` to local UI and `/compress` through the same endpoint. Upgrade task archive format from 3 to 4, retain v1-v3 readers, stage revision files before database removal, and restore pointers only after file validation. Stop/cancel queues and join active operations before deletion/archive. Never restore work as runnable automatically or revive a deleted parent from a delayed callback. Standalone child files remain independent.
- [ ] **Step 4 Run** `cd web && npm test && npm run build:static` and targeted archive/deletion/side tests via the disposable DB script. Verify all new labels in English, Korean, and Simplified Chinese.
- [ ] **Step 5 Commit** with `feat(chat): add context inspector and preserve controls across archives`.

## Task 11 Browser regression and final integration gate

**Files:** Create `e2e/chat-controls.mjs`, `server/chat_controls_e2e_test.go`; extend `e2e/smoke.mjs` only for cases that fit its existing fixture flow. Modify `.github/workflows/ci.yml` to run the new opt-in browser gate with existing Playwright lockfile dependencies; update `docs/VERIFICATION.md` with fresh receipts.

**Interfaces:** `TestChatControlsE2E` follows the existing `TestAgentAdapterE2E` pattern: explicit `SCOPEWEAVER_CHAT_E2E=1`, `dbtest.DSN(t)`, fresh temporary data directories, fake loopback model, and a child Node browser process. Serve `web/out` plus the real Handler from a test listener. The Go harness owns restart, failure, and fixture synchronization endpoints for that test process only. `runChatControlsChecks({base, token, restart, fixture})` runs the browser scenarios; the disposable DB script supplies the DSN. No paid model calls or real task targets.

- [ ] **Step 1 Complete failing browser scenarios** from Tasks 4, 7, and 10: queue three prompts and reorder/edit/remove before execution; pause/Resume; two windows see the same queue; steer a running fake tool; restart after claim shows lost-run Discard and no replay; retry creates another attempt in the same rail entry; edit an older message opens a branch; delete parent and open child document; task segments share the execution gate; context meter updates after a model switch and manual compaction. Run desktop and 390-pixel viewport checks for all three locales. Use barrier-controlled fixtures, not arbitrary delays.
- [ ] **Step 2 Run** `SCOPEWEAVER_CHAT_E2E=1 ./scripts/test-db.sh -run '^TestChatControlsE2E$' -v` after the frontend static build. Require a real feature assertion failure when proving each new scenario, rather than missing browser/server setup. The harness verifies prerequisites before changing test state.
- [ ] **Step 3 Finish integration defects within their owning modules** and add the opt-in CI job using disposable PostgreSQL and the existing `e2e/package-lock.json`. Include the companion paste suite after its deliverable is available. Preserve existing CI checks.
- [ ] **Step 4 Run final verification**: `./scripts/test-db.sh -race -v`; `go test ./... -count=1` for remaining packages with the DB gate separately evidenced; `cd web && npm test && npm run build:static`; `SCOPEWEAVER_CHAT_E2E=1 ./scripts/test-db.sh -run '^TestChatControlsE2E$' -v`; and the existing smoke run against a disposable `scopeweaver_test_*` backend. Run existing required CI gates, including frontend audit, unchanged. Require named DB tests with PASS rather than SKIP, successful browser exits, and `git diff --check` clean. Report pre-existing failures distinctly and resolve any failure caused by this work before delivery.
- [ ] **Step 5 Commit** tested integration and receipts with `test(chat): verify controls history and context recovery`.

## Execution order and completion

Tasks 1 through 11 are ordered by their shared interfaces. The document-paste plan is independently executable and must complete before claiming the combined composer experience. Keep the new control capabilities disabled for release until history, context, side snapshots, archive/deletion, and browser recovery checks are complete. Each task's commit should retain working legacy chat.

A final review must trace the approved design's acceptance bullets to the tests above and inspect the pinned Hermes references against the implemented behavior. Review especially the intentional adaptations: Queue as the busy default, structured Norma branch history, same-task segment branches, file-backed preparation before PostgreSQL publication, and browser-based document paste.
