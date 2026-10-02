# Verification status

This is the first ScopeWeaver localization checkpoint, dated 2026-10-02, based on ARTEX commit `160fe13`. Final server comment/message coverage and the full regression audit are still in progress. This checkpoint is source code, not a binary release or published container image.

Verified locally so far:

- The frontend's 18 tests pass; English/Korean catalogs, language persistence, editable demo data, API transport, and legacy mention compatibility are covered.
- Production and demo static exports build successfully, and a binary with the production frontend embedded builds successfully.
- Desktop checks covered 24 routes in each language. The included demo has an inherited Notifications-page error; the real backend view is being checked separately.
- Mobile checks covered the dashboard, tasks, findings, and settings in both languages with no page-level horizontal overflow. Language changes preserve an unsent draft.
- Focused database, notification, report, archive, task, and server localization tests pass. Approval classification recognizes legacy Chinese, English, and Korean model markers without changing the stored decision type.
- Markdown/CSV exports, request-language negotiation, saved server language, and invalid-language rejection passed HTTP checks.
- The original AGPL-3.0 license is unchanged. Distribution scripts target this standalone repository, and docs distinguish planned releases from available source builds.

Known upstream limitations found during comparison with the untouched source:

- `TestGraphOverviewExpandsAssociatedCompanyScope` fails with a missing task-scope fixture.
- `TestTaskMetadataPatchReturnsRenameAndPin` can race with asynchronous temporary-directory cleanup.
- The standalone `llmrec` suite needs the metering tables created by normal application startup; an empty database without that initialization fails. The unchanged upstream and translated suites both pass with the normal initialization.
- The demo Notifications page lacks its metadata mock. This does not establish whether the real Notifications page works.
- The unchanged npm dependency set reports 14 audit findings: 1 critical, 9 high, and 4 moderate. Dependency upgrades are outside this translation change.

No external targets were scanned and no paid LLM calls were made. Docker image execution and Windows batch execution have not been verified. The validation documents under `sidequestion/` describe historical upstream work, not these checks.
