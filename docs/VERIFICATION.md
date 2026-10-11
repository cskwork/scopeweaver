# Verification and known limitations

[한국어](ko/VERIFICATION.md)

## v0.2.1 candidate — 2026-10-11

The Windows release candidate was built with the real static frontend and tested against disposable PostgreSQL 16.15 databases. These results supersede the historical dependency and Windows-runtime limitations below; the older record is retained for traceability.

| Check | Result |
| --- | --- |
| Frontend dependency audit | 0 vulnerabilities across all severities, including development dependencies |
| Frontend regression and localization tests | 26 passed; no skips |
| Production static export | TypeScript check passed; 31 pages generated |
| Native Windows Go suites, 18 packages | 1,016 tests/subtests passed; 0 failures; 13 existing platform/opt-in skips |
| Native Windows complete server suite | 311 tests/subtests passed; 3 existing opt-in/platform skips |
| Agent adapter | 7 unit tests and syntax checks passed; 3 MCP/Pi scenarios passed against the real authenticated backend and a fresh database |
| Local installer | All 4 isolated fixtures passed: fresh install, stale-file replacement, dependency failure and frontend-build failure |
| Backend translation catalog | 1,868 production keys passed English/Korean completeness and printf argument checks |
| Notification isolation | Deterministic stale-worker regression failed before repair and passed afterward; management/notification selection passed 5 consecutive runs |
| Windows evidence | Full PostgreSQL-backed suite passed, including concurrent writes, commit failures and long paths |
| Windows Python and skill files | Real Python execution and unusable-launcher fallback passed; nested Unicode paths passed through file listing and read handlers |
| Browser E2E on embedded Windows binary | 8 checks passed: setup outage protection in both languages, setup/login, same-origin SSE, usage API isolation, both localized usage cards, and no uncaught page errors |

Go counts include parent tests and subtests. The 13 native skips comprise ten Unix self-update fixtures, one unavailable symlink privilege, the opt-in adapter integration test, and the live-provider test. Adapter integration is checked separately, and Linux CI covers the Unix fixtures. No tests were disabled to produce these results.

Regressions reproduce the original Windows directory-sync failure, localization separator mismatch, unusable Python launcher, notification worker leakage, and profile-cleanup ordering. The E2E SQL helper now puts options before its connection URL; a direct outage/restore check confirmed that Windows psql executes the statements.

The release gate additionally requires the `frontend`, `windows`, `backend`, `agent-adapter`, and browser `smoke` jobs before and after merge. Publication results, platform archives, and checksums are available on the [v0.2.1 release](https://github.com/cskwork/scopeweaver/releases/tag/v0.2.1); this source record does not claim those later jobs or downloaded assets have already passed.

Existing configured Python interpreter paths are preserved. If a prior installation stored an unusable launcher alias, choose a runnable interpreter in Settings. Native Windows tests do not establish Windows self-update behavior or macOS runtime behavior. Live paid providers, external targets, and real notification recipients were not used.

## Historical verification — 2026-10-03

Validation recorded on **2026-10-03** for the English/Korean ScopeWeaver edition, based on upstream ARTEX commit `160fe13`. Tests use disposable PostgreSQL databases and local fixtures. This is source delivery; no binary release or container image has been published.

| Check | Result |
| --- | --- |
| Frontend regression and localization tests | 21 passed; TypeScript check passed |
| Production static export and embedded Go binary | Built successfully |
| Go regression suites, 18 packages | 986 tests/subtests passed; 1 inherited test failed; 1 external-provider test skipped |
| Complete server suite | 285 tests/subtests passed; only the opt-in live-provider test skipped |
| Backend translation catalog | 1,865 production keys passed English/Korean completeness and printf argument checks |
| Authored Chinese text audit | No untranslated authored source/docs or Go comments; deliberate compatibility/data exceptions retained |
| Desktop browser coverage | 24 routes in each language; no missing labels or page-level horizontal overflow |
| Mobile browser coverage | Dashboard, tasks, findings and settings in both languages; no page-level horizontal overflow at 390px |
| Real application screens | Login and notification forms checked in both languages; setup validation and initialization verified |
| Language switching | Navigation/status labels update; selection and unsent drafts are preserved |
| HTTP localization | Markdown/CSV export, language negotiation, saved preference and invalid-language rejection checked |
| GLM-5.3 provider contract | 21 focused tests/subtests passed with local HTTP/SSE fixtures; both endpoint paths, reasoning and tool-call replay covered |
| GLM template browser flow | Both templates checked in both languages; entered name/key/proxy preserved; no save, activation or provider request |
| Packaging and launch scripts | Shell syntax, bilingual packaging fixtures and 10 supervisor cases passed |
| Provenance and dependencies | Original license unchanged; dependency versions and Go module unchanged; documentation links and diff checks passed |

Counts include Go parent tests and their subtests. Package runs use separate databases initialized with the same supplementary metering tables as normal server startup. The original tests and checks remain enabled.

### Inherited issues at the time

`TestGraphOverviewExpandsAssociatedCompanyScope` failed with `task scope missing: <nil>` in both ScopeWeaver and untouched upstream source. The test expected detailed scope in the graph overview, which upstream commit `06a43f3` deliberately made compact. Commit `75a706d` replaced it with `TestGraphOverviewKeepsAssociatedCompanyScopeCompact`, which checks the compact overview and verifies the stored company scope directly. The complete backend CI has passed since then.

`TestTaskMetadataPatchReturnsRenameAndPin` previously produced an intermittent temporary-directory cleanup failure in upstream verification. It passed in the final ScopeWeaver server run; no unrelated lifecycle change was made to hide it.

The upstream mock preview has no notification metadata mock, so its Notifications page errors. The **real backend Notifications page and add-channel form pass** in both languages. No notification was sent during verification.

The v0.1.0 verification recorded **14 npm audit findings: 1 critical, 9 high and 4 moderate**. Framework/tooling updates and removal of the unused shadcn CLI dependency resolved the findings in the v0.2.1 candidate above. Run `cd web && npm audit` for the current registry report; audit results can change as new advisories are published.

### Historical scope and limits

- The optional `TestLiveContextReview` requires private external-provider configuration and was not run. No paid model calls, external target scans, or real notification deliveries were performed.
- GLM tests prove the local request/response contract, not account access or provider availability. Coding Plan is restricted to officially supported tools, and ScopeWeaver is not listed; see [provider setup](llm-providers.md).
- Docker image execution and Windows batch execution were not verified. Packaging fixtures do not claim a real Windows runtime test.
- Setup reached the authenticated route, but one browser wait for the window load event timed out; initialization and the new password were then confirmed through the real API.
- User-entered text, stored evidence, edited prompts, wire identifiers, legacy parsing markers, Unicode fixtures, and original upstream screenshots retain their original content. The English/Korean app does not translate arbitrary user data or third-party tool output.
- Historical validation documents under `sidequestion/` describe upstream work, not new ScopeWeaver runs.

Current remote checks are available in [GitHub Actions](https://github.com/cskwork/scopeweaver/actions/workflows/ci.yml).
