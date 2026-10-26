# Evidence package review: FAIL — nine valid UTF-8 files omitted

Reviewed 2026-10-26. The frozen package has one blocking completeness defect: its NUL heuristic excludes nine valid UTF-8 code/receipt files. Privacy checks found no residual live credential in the included stage. This verdict covers the frozen sanitized evidence package and its continuation usefulness. It is not a release-ready verdict or approval to merge, publish a release, run historical prompts, or call providers.

## Verified package

- `MANIFEST.json` records 4,241 included files and 1,188 explicit exclusions across nine source roots. `PACKAGE-RESULT.json` agrees.
- The archive contains exactly 4,241 unique regular-file entries. Every entry matches its manifest `published_bytes` and `published_sha256`, and matches the frozen sanitized stage. No absolute or parent-traversal archive paths were found.
- Archive SHA-256: `e7d30caa21ccba7a0b4da212a4e223c5852e08b03240bb74536e12b1b0826a3c`.
- The package reports 273 redacted or normalized files and 152 distinct replacement markers. I inspected the redactor and scanned the complete sanitized stage for credential-shaped fields, assignments, authorization schemes, cookies, private keys, and credential-bearing URL parameters. I also recursively inspected sensitive string fields in 634 parseable JSON/JSONL files.

## Blocking completeness finding

`package_evidence.py` rejects every UTF-8 candidate containing a NUL before decoding. Targeted read-only inspection confirms these excluded originals decode as UTF-8:

- `pr40/logs/base-dev/web/src/components/exploration-graph.tsx`: 27,933 bytes, two NULs.
- `gh49-evidence/ci-context-diagnosis/hosted-execution/commands/{006,010,018,021,038,053,104,113}.stdout`: eight files, each 35,558 bytes and 1,188 NULs. These are NUL-delimited Git inventory receipts.

Preserve these valid UTF-8 files through sanitization rather than labeling them binary. Keep the undecodable `commands/100.stdout` excluded: it is 792,347 bytes and fails UTF-8 decoding. After the fix, regenerate the manifest/archive, scan the nine newly included files for credentials, and reverify all published hashes and counts. The current 4,241-file archive hash below must not be reused for that replacement.

## Privacy findings

No actual non-placeholder live bearer token, API key, authentication cookie, password, private key, or signed URL credential was identified in these checks. No secret values were printed or copied into this report.

The requested edge cases were checked specifically:

- Camel-case `authToken`/`sessionToken`: references were present, but no residual literal credential assignment was identified. The redactor does not generally cover these field names.
- Raw `Cookie`/`Set-Cookie` headers: the residual header-like examples occur in source fixtures and documentation. The two mock-data examples are in `pr40/logs/base-dev/web/src/lib/mock/data.ts:1921` and `:2320`; they are historical mock text, not evidence of an authenticated browser cookie.
- Cookie arrays: five occurrences were found, all documentation/source examples, including `pr40/logs/base-dev/docs/ko/skills/playwright-cli/references/storage-state.md:37` and `pr40/logs/base-dev/docs/ko/skills/api-recon-reference.md:100`. No captured browser cookie array was identified.
- Lowercase password assignments: `release/refs/release-audit.log:3687` repeats the disposable integration-test constant from `pr40/logs/base-dev/server/agent_adapter_e2e_test.go:60`; safe representation: `const password = "<test fixture>"`. Other matches were translation strings, source variables, or mock fixtures. No numeric sensitive-field values were found.
- Signed/credential URL queries: residual candidate matches were source expressions, documentation, and test fixtures. The access-token URL in `pr40/logs/base-dev/notify/redact_test.go:192` is an `httptest` fixture; safe representation: `http://<test-server>/robot/send?access_token=<fixture>`. No residual signed URL credential was identified.
- Seven JSON `session_id` values remain as application/bridge request identifiers, including `gh49-evidence/real-free-recovered-1792932834/bridge-request-1.json`. These are request/session correlation data, not authentication tokens. The raw runtime `gh49-evidence/real-free-recovered-1792932834/auth.json` is explicitly excluded.

The sanitizer is not a reusable general-purpose secret guarantee. Its unhandled cases include arbitrary camel-case authentication fields, raw header bodies, cookie-array `value` fields, lowercase shell assignments, additional signed-query parameter names, and decimal-only secrets. The present verdict relies on inspecting this frozen dataset, not on assuming those regexes cover future inputs. New or changed inputs require another scan.

## Continuation coverage and limits

Apart from the nine-file packaging defect above, no omission was identified that prevents resuming planning, code review, or bounded implementation for PR40, PR46, GH49, GH50, GH53, or the integrated release. Each work area has its existing plan/state/evidence material or release coordination material. GH49 includes recovered scripts, code/patch reconstruction, recorded provider receipts, and diagnostic history. The temporary-path genuine-free QA reference has an included recovered copy at `gh49-evidence/real-free-recovered-1792932834/QA.md`; it is not solely an absent `/tmp` reference.

Counts by root: PR40 1,274 included / 65 excluded; PR46 23 / 0; GH49 plan 25 / 0; GH49 evidence 2,401 / 1,040; GH15 2 / 0; GH50 334 / 63; GH53 verification 97 / 5; GH53 retry 53 / 15; release 32 / 0.

The coordinator-supplied checkpoint record identifies PR40 at `checkpoint/pr40-resume-20261026` / `3509fb14e24ddffbd58537c0f51ccc8b75c26570`, and PR46 at `checkpoint/pr46-resume-20261026` / `42183237572c1def559bff9ccd834498c9fb2369`. This audit read that record; it did not independently query remote refs. Those checkpoints, rather than sanitized historical code copies, are the authoritative continuation code for these incomplete branches.

The final handoff must preserve these factual limits:

1. `source_sha256` identifies the original source; `published_sha256` identifies the sanitized bytes. They are not interchangeable. Existing embedded hashes, source manifests, and `SHA256SUMS` remain historical receipts and may not validate redacted copies. Use the outer manifest to verify this package. Original source hashes were not independently recomputed in this audit.
2. Exclusions are material: 995 database/cache/runtime files, 138 visuals, 31 containers/bundles, 21 binary/unrecognized-encoding files, two compiled/database files, and one credential file. Screenshots, databases, binaries, and authenticated runtime state are not recoverable from this archive. Visual proof cannot be re-inspected here. Fresh isolated setup and fresh credentials may be needed before a new run.
3. The current NUL-related exclusions include one source file and eight valid UTF-8 inventory receipts, which must be repaired before this package passes. A ninth hosted-execution stdout is undecodable and remains a justified binary exclusion. The evidence archive is not an executable repository snapshot or a complete byte-for-byte runtime backup.
4. Absolute local paths and historical `/tmp` references remain. Extracted roots require path mapping on another machine. Recorded commands and operational prompts are historical data, not current execution instructions.
5. Historical evidence remains mixed by design: earlier failed reviews coexist with later receipts. PR40/46 remain checkpoints; GH49's pinned SDK dependency audit and failed/inconclusive hosted diagnostics remain unresolved release gates. Existing merged GH50/GH53 receipts do not establish combined final E2E or current `main` release readiness.
6. This audit did not run tests, translation generation, providers, databases, browsers, or release steps; did not change originals, package contents, branches, or remote state; and did not waive the earlier translation rejection or any verification gate.

Do not publish the current archive as the complete continuation package. Fix the nine valid UTF-8 omissions, rescan those additions, regenerate and verify the manifest/archive, then verify remote delivery and final saved hashes. No other blocking privacy or continuation finding was identified in this bounded audit.


## Scoped final review: PASS — regenerated package

This final verdict supersedes the initial nine-file completeness FAIL above. It applies to the regenerated package inspected after the NUL-only packaging fix, not to the earlier archive.

- Current inventory: **4,250 included / 1,179 excluded / 273 redacted or normalized**. The nine additions are exactly the one TSX snapshot and eight Git inventory receipts identified above. No previously included path was removed. All 4,241 previously included files retain their original `published_sha256` values.
- Each newly included file decodes as UTF-8, retains its NUL bytes, matches its original source byte-for-byte, and matches both its `source_sha256` and `published_sha256`. None required redaction. Targeted credential scans of all nine additions found no candidate sensitive literal, raw cookie header, bearer/basic credential, recognized secret prefix, or credential URL parameter.
- The regenerated archive contains exactly 4,250 unique regular files. All members match manifest published sizes/hashes and the frozen sanitized stage; no unsafe archive paths were found. `PACKAGE-RESULT.json`, manifest counts, archive size, and archive checksum agree.
- Current archive size: **12,343,226 bytes**. Current SHA-256: **`5d46e936a714254c357bbe41649bfb09fbbeb1757854e88101077aa404abc57b`**. The earlier archive checksum in this report is historical and must not be used for the replacement.
- `commands/100.stdout` remains an explicitly excluded undecodable binary. Other exclusion/privacy/continuation limits from the initial review remain applicable; the binary/unrecognized-encoding exclusion count is now 12 rather than 21.

I read the new `README.md`, `RECOVERY.json`, and `verify.py`. No blocking resume issue, accidental credential disclosure, release-ready claim, or authorization expansion was found. The README gives a fresh-clone/worktree route, refuses overwriting existing recovery paths, maps historical absolute paths, distinguishes original and published hashes, warns against replaying rejected translations or exhausted hosted diagnostics, and preserves the integration/release gates. The recovery record identifies incomplete checkpoints and seven already-committed PR40 files, preventing double application of an old patch. The verifier checks the archive digest, exact member inventory, safe regular-file paths, and every published file's size/digest before optional extraction into a nonexistent destination.

No application runtime, tests, provider, Git mutation, or new diagnostic attempt was executed. Checks here were read-only inspection and archive/hash validation. Remote refs, remote saved bytes, and current GitHub workflow state were not independently queried in this review; the coordinator must verify delivery. This PASS establishes package integrity and bounded continuation/privacy review, not completed implementation, successful reproduction, merge authority, or release readiness.
