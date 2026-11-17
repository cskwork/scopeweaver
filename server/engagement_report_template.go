package server

// Static tech-report template. The layout is fixed on purpose: a report issued
// today and one issued next month for the same run must look identical. No
// external CSS, no web fonts, no remote images — everything inline so the PDF
// and DOCX converters produce the same document offline.

import "html/template"

var techReportTmpl = template.Must(template.New("tech-report").Funcs(template.FuncMap{
	"sevClass": func(s string) string {
		switch s {
		case "Critical", "critical":
			return "sev-critical"
		case "High", "high":
			return "sev-high"
		case "Medium", "medium":
			return "sev-medium"
		case "Low", "low":
			return "sev-low"
		}
		return "sev-info"
	},
	"yesno": func(b bool) string {
		if b {
			return "yes"
		}
		return "no"
	},
}).Parse(`<!DOCTYPE html>
<html lang="en"><head><meta charset="utf-8">
<title>Security Assessment — {{.TaskName}}</title>
<style>
:root{--page:#ffffff;--ink:#14161a;--ink2:#4a4f57;--muted:#767b83;--line:#e3e5e9;
--surface:#f6f7f9;--critical:#c62828;--high:#e65100;--medium:#b8860b;--low:#2e7d32;--info:#546e7a;}
*{box-sizing:border-box}
body{margin:0;background:var(--page);color:var(--ink);font:14px/1.55 -apple-system,"Segoe UI",Roboto,Helvetica,Arial,sans-serif}
code,pre{font-family:ui-monospace,SFMono-Regular,Menlo,Consolas,monospace}
.tlp{background:#c62828;color:#fff;text-align:center;font-size:10px;letter-spacing:.08em;font-weight:700;padding:5px 8px;font-family:ui-monospace,monospace}
header.hero{padding:34px 32px 26px;border-bottom:1px solid var(--line);background:linear-gradient(180deg,#fbfbfc,#fff)}
.kicker{font-family:ui-monospace,monospace;font-size:11px;letter-spacing:.1em;color:var(--critical);margin-bottom:10px}
h1{margin:0 0 12px;font-size:27px;line-height:1.18;letter-spacing:-.01em}
.lede{color:var(--ink2);max-width:820px;margin:0 0 20px}
.meta{display:flex;flex-wrap:wrap;gap:26px;border-top:1px solid var(--line);padding-top:14px;font-size:12px}
.meta div{display:flex;flex-direction:column;gap:2px}
.meta .k{font-size:10px;text-transform:uppercase;letter-spacing:.07em;color:var(--muted)}
main{padding:0 32px 40px}
section{padding:26px 0;border-bottom:1px solid var(--line)}
section:last-child{border-bottom:none}
h2{font-size:18px;margin:0 0 16px;display:flex;align-items:baseline;gap:10px}
.secno{font-family:ui-monospace,monospace;font-size:12px;color:var(--critical);border:1px solid #f0c4c4;border-radius:4px;padding:1px 6px}
h3{font-size:14px;margin:20px 0 8px}
.statgrid{display:grid;grid-template-columns:repeat(auto-fit,minmax(130px,1fr));gap:10px;margin-bottom:16px}
.stat{background:var(--surface);border:1px solid var(--line);border-radius:7px;padding:12px 14px}
.stat .n{font-size:21px;font-weight:800}
.stat .l{font-size:11px;color:var(--muted);margin-top:3px}
table{width:100%;border-collapse:collapse;font-size:12.5px}
th{text-align:left;font-size:10.5px;text-transform:uppercase;letter-spacing:.05em;color:var(--muted);padding:7px 8px;border-bottom:1px solid var(--line)}
td{padding:9px 8px;border-bottom:1px solid var(--line);vertical-align:top}
.sev{display:inline-block;font-family:ui-monospace,monospace;font-size:10px;font-weight:800;padding:2px 7px;border-radius:4px;border:1px solid}
.sev-critical{color:#fff;background:var(--critical);border-color:var(--critical)}
.sev-high{color:#fff;background:var(--high);border-color:var(--high)}
.sev-medium{color:#fff;background:var(--medium);border-color:var(--medium)}
.sev-low{color:#fff;background:var(--low);border-color:var(--low)}
.sev-info{color:#fff;background:var(--info);border-color:var(--info)}
.finding{border:1px solid var(--line);border-radius:8px;padding:16px 18px;margin:14px 0;break-inside:avoid;page-break-inside:avoid}
.finding h4{margin:0 0 8px;font-size:15px}
.frow{display:flex;flex-wrap:wrap;gap:8px 18px;font-size:12px;color:var(--ink2);margin-bottom:10px}
.frow .tag{font-family:ui-monospace,monospace;font-size:11px;background:var(--surface);border:1px solid var(--line);border-radius:4px;padding:1px 6px}
pre{background:#0f1115;color:#e8eaed;padding:10px 12px;border-radius:6px;overflow-x:auto;font-size:11.5px;line-height:1.45;white-space:pre-wrap;word-break:break-word;margin:8px 0}
.shot{margin:10px 0 4px}
.shot img{max-width:100%;border:1px solid var(--line);border-radius:6px}
.shot .cap{font-size:11px;color:var(--muted);margin-top:4px;font-family:ui-monospace,monospace}
.two{display:grid;grid-template-columns:1fr 1fr;gap:10px}
@media (max-width:720px){.two{grid-template-columns:1fr}}
.callout{border-left:3px solid var(--critical);background:#fdf5f5;padding:10px 14px;margin:12px 0;font-size:12.5px;color:var(--ink2)}
.ok{border-left-color:var(--low);background:#f4f9f4}
.muted{color:var(--muted);font-size:12px}
footer{padding:20px 32px 40px;border-top:1px solid var(--line);color:var(--muted);font-size:11.5px}
</style></head><body>

<div class="tlp">TLP:AMBER · CONFIDENTIAL · AUTHORIZED SECURITY ASSESSMENT · NOT FOR DISTRIBUTION</div>

<header class="hero">
  <div class="kicker">// SECURITY ASSESSMENT REPORT · RUN {{.RunID}} · TASK {{.TaskID}}</div>
  <h1>{{.TaskName}}</h1>
  <p class="lede">Technical report generated from the engagement logs of run {{.RunID}}.
  Every finding in this document is backed by the attack-mapping row that produced it and, where the
  vector is demonstrable in a browser, by a before/after screenshot pair captured against the live page.
  Numbers are read from the stored logs, not recomputed at render time.</p>
  <div class="meta">
    <div><span class="k">Run status</span><span>{{.RunStatus}}</span></div>
    <div><span class="k">Started</span><span>{{.StartedAt}}</span></div>
    <div><span class="k">Finished</span><span>{{.FinishedAt}}</span></div>
    <div><span class="k">Duration</span><span>{{.Duration}}</span></div>
    <div><span class="k">Template rev</span><span>{{.TemplateRev}}</span></div>
    <div><span class="k">Generated</span><span>{{.GeneratedAt}}</span></div>
  </div>
</header>

<main>

<section>
  <h2><span class="secno">01</span> Executive Summary</h2>
  <div class="statgrid">
    <div class="stat"><div class="n">{{.TotalFindings}}</div><div class="l">Findings in this run</div></div>
    <div class="stat"><div class="n">{{.Stats.agent_turns}}</div><div class="l">Agent actions logged</div></div>
    <div class="stat"><div class="n">{{.Stats.commands}}</div><div class="l">Shell commands captured</div></div>
    <div class="stat"><div class="n">{{.Stats.timeline_events}}</div><div class="l">Timeline phases</div></div>
    <div class="stat"><div class="n">{{.Stats.mapped_attacks}}</div><div class="l">Attack mappings</div></div>
    <div class="stat"><div class="n">{{.Stats.errors}}</div><div class="l">Errored actions</div></div>
  </div>
  <table>
    <thead><tr><th>Severity</th><th>Count</th><th>Share</th></tr></thead>
    <tbody>
    {{range .Severity}}
      <tr><td><span class="sev {{sevClass .Name}}">{{.Name}}</span></td><td>{{.Count}}</td><td>{{printf "%.0f" .Pct}}%</td></tr>
    {{else}}
      <tr><td colspan="3" class="muted">No findings recorded for this run.</td></tr>
    {{end}}
    </tbody>
  </table>
</section>

<section>
  <h2><span class="secno">02</span> Coverage &amp; Log Integrity</h2>
  <p class="muted">Each of the five required logs is collected on every run. A zero here means the run produced
  nothing in that log, which the report states rather than hiding.</p>
  <table>
    <thead><tr><th>Log</th><th>Rows</th><th>Status</th></tr></thead>
    <tbody>
      <tr><td>1. Activity analysis</td><td>{{index .LogsAvailable "activity_analysis"}}</td><td>{{if index .LogsAvailable "activity_analysis"}}collected{{else}}empty{{end}}</td></tr>
      <tr><td>2. Agent activities</td><td>{{index .LogsAvailable "agent_activities"}}</td><td>{{if index .LogsAvailable "agent_activities"}}collected{{else}}empty{{end}}</td></tr>
      <tr><td>3. Command log</td><td>{{index .LogsAvailable "command_log"}}</td><td>{{if index .LogsAvailable "command_log"}}collected{{else}}empty{{end}}</td></tr>
      <tr><td>4. Engagement activities</td><td>{{index .LogsAvailable "engagement_activities"}}</td><td>{{if index .LogsAvailable "engagement_activities"}}collected{{else}}empty{{end}}</td></tr>
      <tr><td>5. Log manifest</td><td>{{len .Manifest}}</td><td>hash-chained</td></tr>
      <tr><td>6. Attack mapping</td><td>{{len .Findings}}</td><td>MITRE + WSTG</td></tr>
      <tr><td>&nbsp;&nbsp;↳ duplicates excluded</td><td>{{.Duplicates}}</td><td>independent pass repeated an existing finding</td></tr>
    </tbody>
  </table>
  <h3>Manifest (sha256 per artefact)</h3>
  <table>
    <thead><tr><th>Artefact</th><th>Format</th><th>Rows</th><th>sha256</th></tr></thead>
    <tbody>
    {{range .Manifest}}
      <tr><td>{{.Artefact}}</td><td>{{.Format}}</td><td>{{.Rows}}</td><td style="font-family:ui-monospace,monospace;font-size:10.5px">{{.SHA256}}</td></tr>
    {{else}}
      <tr><td colspan="4" class="muted">Manifest not written.</td></tr>
    {{end}}
    </tbody>
  </table>
</section>

<section>
  <h2><span class="secno">03</span> Findings, Attack Pattern &amp; Evidence</h2>
  <p class="muted">Each finding carries the pattern that classifies it (MITRE ATT&amp;CK technique and OWASP WSTG test),
  the endpoint, the script/command that exercised it, and the screenshot evidence when the vector is
  demonstrable in a browser. A vector that failed is still listed with its status — an absent finding is
  not the same as a tested-and-clean one.</p>

  {{range .Findings}}
  <div class="finding">
    <h4><span class="sev {{sevClass .Severity}}">{{.Severity}}</span> {{.Title}}</h4>
    <div class="frow">
      <span>Finding #{{.ID}}</span>
      <span>Status: <strong>{{.Status}}</strong></span>
      <span>Mapping: <strong>{{.MapStatus}}</strong></span>
      {{if .VulnClass}}<span>Class: {{.VulnClass}}</span>{{end}}
    </div>
    <div class="frow">
      {{if .MITRE}}<span class="tag">ATT&amp;CK {{.MITRE}} · {{.MITREName}}</span>{{end}}
      {{if .WSTG}}<span class="tag">WSTG {{.WSTG}} · {{.WSTGName}}</span>{{end}}
      {{if .Method}}<span class="tag">{{.Method}}</span>{{end}}
      {{if .Endpoint}}<span class="tag">{{.Endpoint}}</span>{{end}}
    </div>
    {{if .Summary}}<p>{{.Summary}}</p>{{end}}
    {{if .Command}}<h3>Reproduction command</h3><pre>{{.Command}}</pre>{{end}}
    {{if .ShotBefore}}
    <h3>Evidence — before / after</h3>
    <div class="two">
      <div class="shot">
        {{if .ShotBefore}}<img src="{{.ShotBefore}}" alt="before"><div class="cap">BEFORE · payload absent</div>{{end}}
      </div>
      <div class="shot">
        {{if .ShotAfter}}<img src="{{.ShotAfter}}" alt="after"><div class="cap">AFTER · payload executed</div>{{end}}
      </div>
    </div>
    {{else if .ShotPath}}
    <h3>Evidence</h3>
    <div class="shot"><img src="{{.ShotPath}}" alt="evidence"><div class="cap">{{.ShotHash}}</div></div>
    {{end}}
    {{if .Note}}<p class="muted">{{.Note}}</p>{{end}}
  </div>
  {{else}}
  <div class="callout">No findings are attached to this run. The attack mapping is empty because the run
  recorded no findings — this is a coverage statement, not a clean bill of health.</div>
  {{end}}
</section>

<section>
  <h2><span class="secno">04</span> Engagement Timeline</h2>
  <table>
    <thead><tr><th>#</th><th>Time</th><th>Phase</th><th>Action</th><th>Worker / target</th><th>Turns</th></tr></thead>
    <tbody>
    {{range .Timeline}}
      <tr><td>{{.Seq}}</td><td>{{.At}}</td><td>{{.Phase}}</td><td>{{.Action}}</td><td>{{.Target}}</td><td>{{.Result}}</td></tr>
    {{else}}
      <tr><td colspan="6" class="muted">Timeline empty for this run.</td></tr>
    {{end}}
    </tbody>
  </table>
</section>

<section>
  <h2><span class="secno">05</span> Command Log (first 200)</h2>
  <p class="muted">The full command log is stored in the database and exported alongside this report; this
  section shows the first 200 commands in execution order with their exit state.</p>
  {{range .Commands}}
  <div class="finding" style="padding:12px 14px">
    <div class="frow"><span class="tag">{{.At}}</span><span class="tag">{{.Worker}}</span><span class="tag">exit: {{.ExitOK}}</span></div>
    <pre>{{.Command}}</pre>
    {{if .Output}}<pre style="background:#f4f5f7;color:#3a3f46">{{.Output}}</pre>{{end}}
  </div>
  {{else}}
  <div class="callout">Command log empty for this run.</div>
  {{end}}
</section>

</main>
<footer>
  <p>Generated by ScopeWeaver engagement logging. Run {{.RunID}} · task {{.TaskID}} · template {{.TemplateRev}} · {{.GeneratedAt}}.</p>
  <p>All findings in this document originate from authorized testing. Screenshots are stored with their
  sha256 in the log manifest; a re-rendered report references the same image files, so a mismatch is
  detectable.</p>
</footer>
</body></html>`))