# Chat Document Paste Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Paste copied documents and long text into standalone and task chat using the existing attachment pipeline.

**Architecture:** Share clipboard classification and draft attachment state between the two upload-capable composers. Keep short text on the browser's normal paste path, and route files and long text through `api.chatUpload`. The conversation controls companion plan consumes the resulting uploaded manifests.

**Tech Stack:** Existing Next.js, React, TypeScript, browser ClipboardEvent/File APIs, Node tests, Playwright, and Go upload endpoint. No new production dependency.

**Spec:** [Approved chat design](../specs/2026-10-16-chat-controls-design.md), especially Document paste and User interface.

## Global Constraints

- Every supported operation has a visible UI control; slash commands provide optional shortcuts.
- Use the existing upload endpoint and its 128 MiB total request limit.
- For plain text at or below 3,000 JavaScript string code units, keep normal insertion at the caret and normal undo/selection behavior.
- For longer plain text, create a UTF-8 `.txt` attachment with a unique name and a Pasted content chip.
- Preserve line endings and whitespace in the saved content.
- File and text attachment creation never submits a message automatically.
- All new UI text follows the existing localization system.

## Review Focus

1. Clipboard offers both a file and its filename as text: attach the bytes once without inserting that filename; preserve unrelated accompanying text. Task 1.
2. Two rapid pastes start before a new draft acquires a conversation: create one upload owner, preserving both batches and order. Task 2.
3. Upload resolves after switching sessions: update only the originating draft. Task 2.
4. Long paste followed by edits and Insert as text: restore complete text at a valid location without overwriting the later draft. Tasks 1 and 2.
5. IME, mentions, and keyboard send preferences: file handling must not turn paste or composition completion into Send. Task 3.

## File responsibilities

| File | Responsibility |
| --- | --- |
| Create `web/src/lib/chat-paste.ts` | Pure classification, text attachment naming, and safe text restoration. |
| Create `web/src/hooks/use-chat-attachments.ts` | Draft-scoped attachment queue, owner acquisition, uploads, retries, preview sources. |
| Create `web/src/components/chat-attachment-chips.tsx` | Shared chip, preview, failure/retry, removal, and Insert as text controls. |
| Modify standalone `chat/page.tsx` and task `sessions-tab.tsx` | Use shared attachment state and pass `onPaste` through MentionTextarea. |
| Create `e2e/chat-paste.mjs` | Real browser clipboard behavior, caret/undo, and composer integration. |

## Task 1 Clipboard classification and text restoration

**Files:** Create `web/src/lib/chat-paste.ts` and `web/src/lib/chat-paste.test.mjs`.

**Interfaces:** Export `LARGE_PASTE_THRESHOLD = 3000`; `ChatPaste = {kind: "inline"} | {kind: "attach"; files: File[]; text: string; pastedText?: string}`; `classifyChatPaste(files: File[], plainText: string): ChatPaste`; `makePastedTextFile(text: string, id: string): File`; `restorePastedText(draft: string, source: {draft: string; start: number; end: number}, pastedText: string): {text: string; caret: number}`. File names use `pasted_content_<id>.txt`. `text` is accompanying inline text for file paste. Long-text classification returns `files: []`, `text: ""`, and `pastedText` equal to the original text; Task 2 supplies the unique ID and constructs the File. Restore at the saved selection only if the draft is unchanged; otherwise insert at the current draft's end, preserving intervening edits.

- [ ] **Step 1 Write failing Node tests** using the existing `.test.mjs` imports of `.ts` modules. Assert the following expressions, with File/TextEncoder globals from the Node version already used by the project:

```js
assert.equal(classifyChatPaste([], "x".repeat(3000)).kind, "inline");
assert.equal(classifyChatPaste([], "x".repeat(3001)).kind, "attach");
assert.equal(classifyChatPaste([], "😀".repeat(1500)).kind, "inline");
assert.equal(await makePastedTextFile("  한글\r\n中文\n", "fixed").text(), "  한글\r\n中文\n");
assert.equal(classifyChatPaste([new File(["data"], "report.pdf")], "report.pdf").text, "");
assert.equal(classifyChatPaste([new File(["data"], "report.pdf")], "Please review").text, "Please review");
assert.equal(restorePastedText("later edits", {draft: "old", start: 0, end: 3}, "pasted").text, "later editspasted");
```

Also assert multiple original files retain their names/order, and a clipboard containing only a path stays inline. For generated paste content, assert MIME `text/plain;charset=utf-8` and bytes equal TextEncoder output. Tests provide fixed IDs; production generates IDs in Task 2.

- [ ] **Step 2 Run** `cd web && node --test src/lib/chat-paste.test.mjs`. Expect failure because the module does not exist.
- [ ] **Step 3 Implement the exported functions**. Suppress accompanying text only when it exactly equals the supplied filenames or their standard file-URL/path representation; do not broadly remove substrings. Read only event-supplied data. Never read an arbitrary clipboard path from the server.
- [ ] **Step 4 Rerun the command**. Expect every new test to pass.
- [ ] **Step 5 Stage the files above and commit** with `feat(chat): classify document and long text paste`.

## Task 2 Shared draft attachment state and composer controls

**Files:** Create `web/src/hooks/use-chat-attachments.ts`, `web/src/lib/chat-attachment-state.ts`, `web/src/lib/chat-attachment-state.test.mjs`, and `web/src/components/chat-attachment-chips.tsx`. Modify `web/src/app/(main)/chat/page.tsx`, `web/src/app/(main)/function/tasks/detail/_tabs/sessions-tab.tsx`, `web/src/components/mention-textarea.tsx`, `web/src/lib/api.ts`, and localized copy in `web/src/i18n/messages/interface.ts` and `zh-Hans.ts`.

**Interfaces:** Consume Task 1. Export `UploadOwner = {scope: "task" | "session"; id: string}`, `ChatDraft = {text: string; start: number; end: number}`, and `DraftAttachment = {id: string; ownerKey: string; file?: File; status: "pending" | "uploading" | "ready" | "failed"; uploaded?: ChatAttachment; error?: string; pastedText?: string; source?: ChatDraft}`. Ready items require `uploaded`; locally pending/uploading items require `file`. `useChatAttachments` accepts `{draftKey: string; ensureOwner: () => Promise<UploadOwner>; readDraft: (key: string) => ChatDraft; writeDraft: (key: string, next: {text: string; caret: number}) => void}` and returns `{items: DraftAttachment[], uploading: boolean, canSubmit: boolean, uploaded: ChatAttachment[], addFiles(files: File[]): void, loadUploaded(manifests: ChatAttachment[]): void, onPaste(event: React.ClipboardEvent<HTMLTextAreaElement>): void, insertAsText(id: string): void, retry(id: string): void, remove(id: string): void, clear(): void}`. `loadUploaded` restores ready manifests for queued/history editing without fabricating browser File bytes. `uploaded` snapshots ready manifests; `canSubmit` is false if any pending, uploading, or failed item remains. A provider above the session view owns the draft-keyed map and read/write callbacks so switching/unmounting views cannot lose pending work or target the wrong text.

- [ ] **Step 1 Add failing state and browser assertions**. Pure reducer tests assert `readyForSubmit(state) === false` for pending/failed items; retry preserves File identity; completion tagged draft A changes A only; deleting an uploading entry prevents its late result from reappearing. In `e2e/chat-paste.mjs`, prepare named checks `rapid paste uses one owner`, `failed upload keeps draft`, and `preview restores long text after later edits`; the browser entry point is completed in Task 3.
- [ ] **Step 2 Run** `cd web && node --test src/lib/chat-attachment-state.test.mjs`. Expect failure for missing state functions. Export `reduceAttachments(state, event)` and `readyForSubmit(state)` from the new state module; event and state unions are defined beside those functions.
- [ ] **Step 3 Implement the hook and chips**. Snapshot `clipboardData.items` synchronously, prevent default only when consuming files or large text, and reuse `api.chatUpload`. Serialize owner creation with one promise per draft, then batch files under that owner without overlapping mutable FileLists. Keep each File for retry. Key late callbacks by draft and item ID. Generate unique IDs using the existing non-secure-origin fallback pattern. Show full text in a scrollable preview; Insert as text removes that generated attachment and uses Task 1 restoration behavior. Forward external `onPaste` via MentionTextarea's existing textarea props without replacing its mention or composition handlers. Keep the file picker on the same ingestion path. Preserve current task/session upload ownership and limits.
- [ ] **Step 4 Run the Node tests and** `cd web && npm test && npm run build:static`. Expect all tests and the static build to succeed; no new message is submitted by any attachment action.
- [ ] **Step 5 Stage the listed files and commit** with `feat(chat): attach clipboard documents in both chat composers`.

## Task 3 Browser verification and delivery

**Files:** Complete `e2e/chat-paste.mjs`; modify `e2e/smoke.mjs` to call exported `runChatPasteChecks({base, token, step, check, openPage})`; modify `docs/VERIFICATION.md` only with fresh execution receipts.

**Interfaces:** Use existing smoke-runner authentication and disposable database guard. Browser tests can route upload requests for deterministic failure/delay; real successful uploads still call the existing endpoint. Dispatch a ClipboardEvent with DataTransfer files for file-path coverage, and use real browser copy/paste keyboard actions for native text insertion and undo. Never use a synthetic event to claim native undo was verified.

- [ ] **Step 1 Add failing browser checks** for both composers: two copied documents create two chips; 3,000 code units stay inline; 3,001 become one text chip with exact preview; empty-draft concurrent pastes acquire one conversation; file+independent-text preserves text; a delayed upload returns to its originating draft after session switch; failure/retry retains file and text; long text restoration preserves later edits; paste triggers zero message POSTs. Add a Chinese IME/mention interaction and both configured send shortcuts. Run at desktop and 390-pixel width, using English, Korean, and Simplified Chinese labels.
- [ ] **Step 2 Run the smoke suite before wiring the new hook** if Task 2 is being implemented in one working cycle, or temporarily omit only that wiring to demonstrate the tests fail on the original paste behavior. Restore wiring immediately. Capture an actual missing-chip/assertion failure, not a broken server or missing dependency.
- [ ] **Step 3 Complete the smoke integration and correct composer integration defects** within the files owned by Tasks 1 and 2. Use existing Playwright; no new UI test framework.
- [ ] **Step 4 Verify** `cd web && npm test && npm run build:static`; run `node e2e/smoke.mjs` against a locally started test backend with `SCOPEWEAVER_URL` and `ARTEX_PG_DSN` explicitly selecting a fresh `scopeweaver_test_*` database, following the existing smoke header. Install existing lockfile dependencies only when executing this plan. Expect new paste checks to print PASS and the smoke process to exit 0. A missing disposable backend is an unmet gate, never a passing result.
- [ ] **Step 5 Stage changed files and commit** with `test(chat): verify clipboard documents and draft retention`.

## Handoff to conversation controls

The companion [conversation controls plan](2026-10-16-chat-controls-and-context.md) consumes only completed `ChatAttachment[]` manifests plus the `canSubmit` gate. It must not serialize browser File objects into queue payloads. This paste deliverable can ship independently after its checks pass.
