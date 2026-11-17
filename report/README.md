# Engagement logging, evidence & reporting

Every ScopeWeaver run produces, automatically:

1. **Five logs**, both as database rows and as files on disk
2. **An attack mapping** tying each finding to MITRE ATT&CK, OWASP WSTG, the script/command that exercised it, and screenshot evidence
3. **A tech report** in HTML, PDF and DOCX
4. **A Sibermate-format deck** in PPTX

The layout is a static template (`EngagementTemplateRev`), so two reports from
different builds are comparable and a report never silently changes shape.

## Where things live

| Piece | File |
|---|---|
| Schema (7 tables) | `db/schema.sql` §N |
| Collectors (1–5 + mapping) | `server/engagement_logs.go` |
| Screenshot proof engine (CDP) | `server/engagement_evidence.go` |
| Tech report renderer | `server/engagement_report.go`, `..._template.go` |
| Five-log file export | `server/engagement_export.go` |
| Deck (python-pptx) | `server/deckgen/make_deck.py` (embedded) |
| Terminal-task hook | `server/engagement_automation.go` |
| HTTP surface | `server/engagement_api.go` |

## Tables

| Table | Holds |
|---|---|
| `eng_run` | one row per collected run (task, status, window, stats, template rev) |
| `eng_activity_analysis` | behavioural rollup: counts, severity distribution, phases |
| `eng_agent_activities` | normalised agent turns (kind, tool, summary, tokens) |
| `eng_command_log` | literal shell command, exit state, bounded output head |
| `eng_engagement_activities` | phase timeline (recon → analysis → exploitation → reporting) |
| `eng_attack_mapping` | per-finding: ATT&CK + WSTG + endpoint + script + command + evidence |
| `eng_evidence` | durable screenshots keyed by finding (survives re-collection) |
| `eng_log_manifest` | sha256 + row count + chain head per artefact |

Every row carries `prev_hash` / `row_hash`; `VerifyChain` recomputes the linkage
per table so a log edited after the run is detectable.

## Automatic run

`EnableEngagementAutomation` is installed at boot (`cmd/artex/main.go`). When a
task reaches a terminal status (`done` / `failed` / `timeout`), the hook runs in
the background and writes, under `ARTEX_ENGAGEMENT_REPORTS` (default
`<dataDir>/engagement-reports`):

```
task-<id>/
  activity_analysis-run-<run>.json
  agent_activities-run-<run>.jsonl
  command_log-run-<run>.jsonl
  engagement_activities-run-<run>.json
  attack_mapping-run-<run>.json
  log_manifest-run-<run>.json
  report-<slug>.html | .pdf | .docx
  deck-run-<run>.pptx
  evidence/*.png
```

A failure in collection or rendering is logged and never fails the task.

## Screenshot proof

A response code is not proof. `CaptureProof` launches an isolated headless
Chromium on an ephemeral port and records:

* `before` — the page without the payload, plus the assertion value
* `after` — the page with the payload, plus the assertion value
* `executed` — true only when the JS assertion flips false → true

For XSS the payload must produce a **visible** effect (a banner) so the
screenshot is self-evident; the assertion `window.__xss_proof === true` is what
actually decides `executed`, so a screenshot cannot manufacture a finding.

The capture browser ignores the service's `HTTP_PROXY`/`HTTPS_PROXY` (that
variable is for the agent's own traffic; inheriting it makes Chromium render a
blank frame, which would silently read as "payload did not execute"). Set
`EvidenceEngine.Proxy` to route it deliberately.

## HTTP surface

```
POST /api/engagement/collect?task_id=N     rebuild the five logs for a run
POST /api/engagement/report?task_id=N&deck=1
GET  /api/engagement/runs[?task_id=N]
GET  /api/engagement/manifest?run_id=N
GET  /api/engagement/artefact?path=...      only paths the manifest knows
POST /api/engagement/evidence               capture before/after proof
     {"run_id":N,"finding_id":M,"slug":"x","before_url":"...","after_url":"...",
      "payload":"...","assertion":"window.__xss_proof === true",
      "cookies":{...},"cookie_domain":"...","proxy":"..."}
```

All routes require the normal JWT.

## Taxonomy

`wstgFor` / `mitreFor` classify by ordered substring matching, specific classes
before broad ones. Two traps are pinned by tests:

* `"unauthenticated"` contains `auth` — it must not become a brute-force finding
* `"resource"` contains `rce`, and `"dossier"` contains `dos`

Run `go test ./server/ -run 'TestAttackMappingTaxonomy|Engagement|Evidence|Report'`.

## Report semantics

* Headline finding count is **de-duplicated**. Findings an independent pass
  repeats are stored with `status='duplicate'`, excluded from the list, and
  reported as a number ("duplicates excluded").
* An empty section is rendered as explicitly empty. A log with zero rows is a
  coverage statement, not a clean result.
* Blocks that could not be tested are named, never silently dropped.

## Verifying a delivered report

```sql
SELECT artefact, format, row_count, sha256 FROM eng_log_manifest
 WHERE run_id = <run> ORDER BY artefact;
```

Recompute `sha256sum` for each file and compare; the chain head per log is in the
same row set, so an edited log is detectable without re-running the engagement.
