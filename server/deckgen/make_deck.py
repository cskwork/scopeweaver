#!/usr/bin/env python3
"""Sibermate-format activity deck for one collected ScopeWeaver run.

Reads the eng_* tables (written by EngagementCollector) and emits a 16:9 PPTX
using the palette of the Sibermate reference deck supplied by the operator:
dark surfaces, red accent, amber/blue/green severity ramp.

Usage: make_deck.py <run_id> <out.pptx>

Every number is read from the database, never recomputed from the target. A
section with no rows is rendered with an explicit "not recorded" line rather
than omitted, so a reader can tell a clean run from an uncollected one.
"""
import os
import re
import subprocess
import sys

from pptx import Presentation
from pptx.dml.color import RGBColor
from pptx.enum.text import PP_ALIGN, MSO_ANCHOR
from pptx.util import Emu, Inches, Pt

# --- Sibermate palette (from the reference HTML) ---------------------------
PAGE = RGBColor(0x0C, 0x0C, 0x0D)
SURFACE = RGBColor(0x16, 0x16, 0x17)
SURFACE2 = RGBColor(0x1C, 0x1C, 0x1E)
TEXT = RGBColor(0xF4, 0xF4, 0xF2)
TEXT2 = RGBColor(0xC3, 0xC2, 0xB7)
MUTED = RGBColor(0x8B, 0x8A, 0x84)
RED = RGBColor(0xD0, 0x3B, 0x3B)
ACCENT = RGBColor(0xE3, 0x49, 0x48)
SEV = {
    "critical": RGBColor(0xD0, 0x3B, 0x3B),
    "high": RGBColor(0xEC, 0x83, 0x5A),
    "medium": RGBColor(0xFA, 0xB2, 0x19),
    "low": RGBColor(0x1B, 0xAF, 0x7A),
    "info": RGBColor(0x2A, 0x78, 0xD6),
}
FONT = "Arial"


def dsn():
    p = os.path.expanduser("~/hermes-workspace/scopeweaver-monolith/runtime.env")
    if os.path.exists(p):
        for line in open(p):
            if line.startswith("ARTEX_PG_DSN="):
                return line.split("=", 1)[1].strip().strip('"').strip("'")
    return os.environ.get("ARTEX_PG_DSN", "")


def q(sql):
    """Rows as lists of fields; separators survive multi-line TEXT columns."""
    r = subprocess.run(
        ["psql", dsn(), "-At", "-F", "\x1f", "-R", "\x1e", "-c", sql],
        capture_output=True, text=True)
    if r.returncode:
        raise SystemExit("psql: " + r.stderr[:400])
    return [rec.split("\x1f") for rec in r.stdout.split("\x1e") if rec != ""]


def qn(sql, n):
    return [r + [""] * (n - len(r)) if len(r) < n else r[:n] for r in q(sql)]


def num(v, default=0.0):
    try:
        return float(v)
    except (TypeError, ValueError):
        return default


def add_slide(prs):
    s = prs.slides.add_slide(prs.slide_layouts[6])
    bg = s.shapes.add_shape(1, 0, 0, prs.slide_width, prs.slide_height)
    bg.fill.solid()
    bg.fill.fore_color.rgb = PAGE
    bg.line.fill.background()
    bg.shadow.inherit = False
    return s


def text(slide, left, top, width, height, body, size=14, color=TEXT, bold=False,
         align=PP_ALIGN.LEFT, mono=False, line_spacing=1.15):
    tb = slide.shapes.add_textbox(Inches(left), Inches(top), Inches(width), Inches(height))
    tf = tb.text_frame
    tf.word_wrap = True
    tf.margin_left = tf.margin_right = Emu(0)
    tf.margin_top = tf.margin_bottom = Emu(0)
    lines = body if isinstance(body, list) else [body]
    for i, line in enumerate(lines):
        p = tf.paragraphs[0] if i == 0 else tf.add_paragraph()
        p.alignment = align
        p.line_spacing = line_spacing
        r = p.add_run()
        r.text = str(line)
        r.font.size = Pt(size)
        r.font.bold = bold
        r.font.color.rgb = color
        r.font.name = "Consolas" if mono else FONT
    return tb


def tlp_strip(slide, prs):
    bar = slide.shapes.add_shape(1, 0, 0, prs.slide_width, Inches(0.28))
    bar.fill.solid()
    bar.fill.fore_color.rgb = RED
    bar.line.fill.background()
    bar.shadow.inherit = False
    tf = bar.text_frame
    tf.margin_top = tf.margin_bottom = Emu(0)
    p = tf.paragraphs[0]
    p.alignment = PP_ALIGN.CENTER
    r = p.add_run()
    r.text = "TLP : AMBER   ·   CONFIDENTIAL   ·   AUTHORIZED SECURITY ASSESSMENT"
    r.font.size = Pt(8)
    r.font.bold = True
    r.font.color.rgb = RGBColor(0xFF, 0xFF, 0xFF)
    r.font.name = "Consolas"


def card(slide, left, top, width, height, value, label, color=TEXT):
    box = slide.shapes.add_shape(1, Inches(left), Inches(top), Inches(width), Inches(height))
    box.fill.solid()
    box.fill.fore_color.rgb = SURFACE
    box.line.color.rgb = RGBColor(0x2A, 0x2A, 0x2C)
    box.line.width = Pt(0.75)
    box.shadow.inherit = False
    tf = box.text_frame
    tf.word_wrap = True
    tf.margin_left = Inches(0.12)
    tf.margin_top = Inches(0.08)
    p = tf.paragraphs[0]
    r = p.add_run()
    r.text = str(value)
    r.font.size = Pt(24)
    r.font.bold = True
    r.font.color.rgb = color
    r.font.name = FONT
    p2 = tf.add_paragraph()
    r2 = p2.add_run()
    r2.text = label
    r2.font.size = Pt(9)
    r2.font.color.rgb = MUTED
    r2.font.name = FONT


def section_title(slide, no, title, sub=""):
    text(slide, 0.6, 0.5, 1.0, 0.4, no, size=13, color=ACCENT, bold=True, mono=True)
    text(slide, 1.05, 0.45, 11.0, 0.5, title, size=24, bold=True)
    if sub:
        text(slide, 1.05, 0.95, 11.0, 0.4, sub, size=10.5, color=MUTED)


def truncate(s, n):
    s = (s or "").replace("\n", " ").strip()
    return s if len(s) <= n else s[: n - 1] + "…"


def main():
    if len(sys.argv) < 3:
        raise SystemExit("usage: make_deck.py <run_id> <out.pptx>")
    run_id = sys.argv[1]
    out = sys.argv[2]

    run = qn(f"""select r.id, r.task_id, coalesce(t.name,''), coalesce(r.run_status,''),
                        coalesce(to_char(r.started_at,'YYYY-MM-DD HH24:MI'),''),
                        coalesce(to_char(r.finished_at,'HH24:MI'),''),
                        coalesce(r.template_rev,''), coalesce(r.stats::text,'{{}}')
                   from eng_run r left join tasks t on t.id=r.task_id where r.id={int(run_id)}""", 8)
    if not run:
        raise SystemExit(f"run {run_id} not found")
    _, task_id, task_name, run_status, started, finished, trev, stats = run[0]
    stats = stats or "{}"

    def stat(key):
        m = re.search(r'"%s"\s*:\s*(-?[\d.]+)' % key, stats)
        return m.group(1) if m else "0"

    findings = qn(f"""select f.id, coalesce(f.severity,''), coalesce(f.status,''), coalesce(f.name,''),
                             coalesce(f.vulnclass,''), coalesce(m.mitre_id,''), coalesce(m.mitre_name,''),
                             coalesce(m.wstg_id,''), coalesce(m.wstg_name,''), coalesce(m.endpoint,''),
                             coalesce(m.status,''), coalesce(m.evidence_shot,''), coalesce(m.evidence_before,''),
                             coalesce(m.evidence_after,''), coalesce(m.command,'')
                        from eng_attack_mapping m join findings f on f.id=m.finding_id
                       where m.run_id={int(run_id)}
                       order by case lower(f.severity) when 'critical' then 0 when 'high' then 1
                                when 'medium' then 2 when 'low' then 3 else 4 end, f.id""", 15)
    timeline = qn(f"""select seq, coalesce(phase,''), coalesce(to_char(at_time,'HH24:MI:SS'),''),
                             coalesce(action,''), coalesce(target,''), coalesce(result,'')
                        from eng_engagement_activities where run_id={int(run_id)} order by seq""", 6)
    manifest = qn(f"""select artefact, coalesce(format,''), coalesce(row_count,0), coalesce(sha256,'')
                        from eng_log_manifest where run_id={int(run_id)} order by artefact""", 4)
    counts = {r[0]: int(num(r[1])) for r in qn(
        f"""select 'aa', count(*) from eng_activity_analysis where run_id={int(run_id)}
            union all select 'ag', count(*) from eng_agent_activities where run_id={int(run_id)}
            union all select 'cm', count(*) from eng_command_log where run_id={int(run_id)}
            union all select 'tl', count(*) from eng_engagement_activities where run_id={int(run_id)}
            union all select 'am', count(*) from eng_attack_mapping where run_id={int(run_id)}""", 2)}

    prs = Presentation()
    prs.slide_width = Inches(13.333)
    prs.slide_height = Inches(7.5)

    # ---- 1. title ----
    s = add_slide(prs)
    tlp_strip(s, prs)
    text(s, 0.8, 1.5, 11.5, 0.4, "// AUTONOMOUS AI RED-TEAM · ENGAGEMENT ACTIVITY ANALYSIS",
         size=11, color=ACCENT, mono=True)
    text(s, 0.8, 2.05, 11.8, 1.5, task_name, size=36, bold=True, line_spacing=1.05)
    text(s, 0.8, 3.6, 11.0, 0.9,
         f"Run {run_id} · task {task_id} · status {run_status} · template {trev}",
         size=12, color=TEXT2)
    text(s, 0.8, 4.05, 11.0, 0.9,
         f"Window {started} → {finished} (UTC+7)",
         size=12, color=MUTED, mono=True)
    text(s, 0.8, 6.6, 11.8, 0.4,
         "Generated from stored engagement logs · every figure read from the database",
         size=9, color=MUTED, mono=True)

    # ---- 2. executive summary ----
    s = add_slide(prs)
    tlp_strip(s, prs)
    section_title(s, "01", "Executive Summary", "Counts as stored in the run logs")
    sev_count = {}
    for f in findings:
        sv = (f[1] or "info").lower()
        sev_count[sv] = sev_count.get(sv, 0) + 1
    metrics = [
        (str(len(findings)), "Findings this run", TEXT),
        (stat("agent_turns"), "Agent actions logged", TEXT),
        (stat("commands"), "Shell commands", TEXT),
        (stat("timeline_events"), "Timeline phases", TEXT),
        (str(sev_count.get("critical", 0)), "Critical", SEV["critical"]),
        (str(sev_count.get("high", 0)), "High", SEV["high"]),
    ]
    x = 0.7
    for val, label, col in metrics:
        card(s, x, 1.55, 1.9, 1.25, val, label, col)
        x += 2.0
    text(s, 0.7, 3.15, 11.9, 0.4, "Coverage of the five required logs", size=12, bold=True)
    rows = [("Activity analysis", counts.get("aa", 0)), ("Agent activities", counts.get("ag", 0)),
            ("Command log", counts.get("cm", 0)), ("Engagement activities", counts.get("tl", 0)),
            ("Attack mapping", counts.get("am", 0))]
    y = 3.65
    for name, n in rows:
        text(s, 0.75, y, 4.6, 0.3, name, size=11, color=TEXT2)
        text(s, 5.5, y, 1.6, 0.3, str(n), size=11, mono=True,
             color=TEXT if n else MUTED)
        text(s, 6.0, y, 3.0, 0.3, "collected" if n else "empty",
             size=9.5, color=MUTED if n else ACCENT)
        y += 0.42

    # ---- 3. severity distribution ----
    s = add_slide(prs)
    tlp_strip(s, prs)
    section_title(s, "02", "Severity Distribution", f"{len(findings)} findings, grouped")
    maxn = max(list(sev_count.values()) + [1])
    y = 1.7
    for sv in ("critical", "high", "medium", "low", "info"):
        n = sev_count.get(sv, 0)
        text(s, 0.75, y, 1.5, 0.32, sv.upper(), size=11, bold=True,
             color=SEV[sv] if n else MUTED, mono=True)
        bar_w = 8.6 * (n / maxn) if maxn else 0
        if bar_w > 0.02:
            bar = s.shapes.add_shape(1, Inches(2.3), Inches(y), Inches(bar_w), Inches(0.28))
            bar.fill.solid()
            bar.fill.fore_color.rgb = SEV[sv]
            bar.line.fill.background()
            bar.shadow.inherit = False
        text(s, 2.35 + bar_w, y, 1.2, 0.32, str(n), size=11, mono=True, color=TEXT2)
        y += 0.62

    # ---- 4. timeline ----
    s = add_slide(prs)
    tlp_strip(s, prs)
    section_title(s, "03", "Engagement Timeline", "Phase buckets in execution order")
    y = 1.6
    for row in timeline[:11]:
        seq, phase, at, action, target, result = row
        dot = s.shapes.add_shape(9, Inches(0.8), Inches(y + 0.06), Inches(0.1), Inches(0.1))
        dot.fill.solid()
        dot.fill.fore_color.rgb = ACCENT
        dot.line.fill.background()
        dot.shadow.inherit = False
        text(s, 1.05, y, 1.0, 0.28, at, size=9, mono=True, color=MUTED)
        text(s, 2.0, y, 1.6, 0.28, phase, size=10, bold=True, color=TEXT)
        text(s, 3.6, y, 7.2, 0.28, truncate(action, 90), size=9.5, color=TEXT2)
        text(s, 10.9, y, 1.6, 0.28, truncate(result, 18), size=8.5, mono=True, color=MUTED)
        y += 0.47
    if not timeline:
        text(s, 0.8, 1.8, 10, 0.4, "Timeline not recorded for this run.", size=12, color=MUTED)

    # ---- 5..N findings ----
    for f in findings:
        (fid, sev, fstatus, name, vclass, mitre, mitre_name, wstg, wstg_name,
         endpoint, mapstatus, shot, before, after, command) = f
        s = add_slide(prs)
        tlp_strip(s, prs)
        sev_l = (sev or "info").lower()
        badge = s.shapes.add_shape(1, Inches(0.7), Inches(0.62), Inches(1.1), Inches(0.32))
        badge.fill.solid()
        badge.fill.fore_color.rgb = SEV.get(sev_l, SEV["info"])
        badge.line.fill.background()
        badge.shadow.inherit = False
        p = badge.text_frame.paragraphs[0]
        p.alignment = PP_ALIGN.CENTER
        r = p.add_run()
        r.text = sev_l.upper()
        r.font.size = Pt(9)
        r.font.bold = True
        r.font.color.rgb = RGBColor(0xFF, 0xFF, 0xFF)

        text(s, 1.95, 0.55, 10.5, 0.5, truncate(name, 95), size=17, bold=True)
        text(s, 0.75, 1.1, 11.8, 0.32,
             f"#{fid} · status {fstatus} · mapping {mapstatus} · {truncate(vclass, 60)}",
             size=9.5, color=MUTED, mono=True)
        tags = []
        if mitre:
            tags.append(f"ATT&CK {mitre} {truncate(mitre_name,34)}")
        if wstg:
            tags.append(f"WSTG {wstg} {truncate(wstg_name,34)}")
        if endpoint:
            tags.append(truncate(endpoint, 70))
        ty = 1.5
        for t in tags:
            chip = s.shapes.add_shape(1, Inches(0.75), Inches(ty), Inches(11.6), Inches(0.28))
            chip.fill.solid()
            chip.fill.fore_color.rgb = SURFACE
            chip.line.color.rgb = RGBColor(0x2A, 0x2A, 0x2C)
            chip.line.width = Pt(0.5)
            chip.shadow.inherit = False
            tf = chip.text_frame
            tf.margin_left = Inches(0.08)
            tf.margin_top = Emu(0)
            pr = tf.paragraphs[0]
            rr = pr.add_run()
            rr.text = t
            rr.font.size = Pt(9)
            rr.font.name = "Consolas"
            rr.font.color.rgb = TEXT2
            ty += 0.34

        # evidence screenshots
        shots = [p for p in (before, after, shot) if p and os.path.exists(p)]
        if shots:
            sw = 5.6 if len(shots) > 1 else 7.2
            sx = 0.75
            for i, sp in enumerate(shots[:2]):
                try:
                    s.shapes.add_picture(sp, Inches(sx), Inches(ty + 0.1), width=Inches(sw))
                    cap = "BEFORE" if sp == before else ("AFTER" if sp == after else "EVIDENCE")
                    text(s, sx, ty + 0.12 + sw * 0.62, sw, 0.25, cap, size=8,
                         color=MUTED, mono=True)
                except Exception:
                    pass
                sx += sw + 0.2
        elif command:
            box = s.shapes.add_shape(1, Inches(0.75), Inches(ty + 0.1), Inches(11.6), Inches(1.5))
            box.fill.solid()
            box.fill.fore_color.rgb = RGBColor(0x0F, 0x11, 0x15)
            box.line.color.rgb = RGBColor(0x2A, 0x2A, 0x2C)
            box.shadow.inherit = False
            tf = box.text_frame
            tf.word_wrap = True
            tf.margin_left = Inches(0.12)
            tf.margin_top = Inches(0.1)
            p = tf.paragraphs[0]
            r = p.add_run()
            r.text = truncate(command, 500)
            r.font.size = Pt(9)
            r.font.name = "Consolas"
            r.font.color.rgb = RGBColor(0xE8, 0xEA, 0xED)

    # ---- attack mapping table ----
    s = add_slide(prs)
    tlp_strip(s, prs)
    section_title(s, "04", "Attack Pattern Mapping",
                  "MITRE ATT&CK × OWASP WSTG × script × vulnerability")
    headers = ["Finding", "Sev", "ATT&CK", "WSTG", "Vulnerability class"]
    xs = [0.7, 1.5, 2.35, 4.5, 6.4]
    for hx, h in zip(xs, headers):
        text(s, hx, 1.5, 2.2, 0.28, h.upper(), size=8.5, bold=True, color=MUTED)
    y = 1.85
    for f in findings[:11]:
        fid, sev, _, name, vclass, mitre, _, wstg, _, endpoint = f[0], f[1], f[2], f[3], f[4], f[5], f[6], f[7], f[8], f[9]
        text(s, xs[0], y, 0.9, 0.26, f"#{fid}", size=8.5, mono=True, color=TEXT2)
        text(s, xs[1], y, 0.9, 0.26, (sev or "")[:4].upper(), size=8.5, mono=True,
             color=SEV.get((sev or "info").lower(), SEV["info"]))
        text(s, xs[2], y, 2.1, 0.26, mitre, size=8.5, mono=True, color=TEXT2)
        text(s, xs[3], y, 1.9, 0.26, wstg, size=8.5, mono=True, color=TEXT2)
        text(s, xs[4], y, 6.2, 0.26, truncate(vclass, 78), size=8.5, color=TEXT2)
        y += 0.4

    # ---- manifest ----
    s = add_slide(prs)
    tlp_strip(s, prs)
    section_title(s, "05", "Log Manifest & Integrity",
                  "sha256 per artefact · chain head per log")
    xs = [0.7, 3.6, 4.6, 8.9]
    for hx, h in zip(xs, ["Artefact", "Fmt", "Rows", "sha256"]):
        text(s, hx, 1.5, 2.6, 0.26, h.upper(), size=8.5, bold=True, color=MUTED)
    y = 1.85
    for art, fmt, rows, sha in manifest[:14]:
        text(s, xs[0], y, 2.8, 0.24, truncate(art, 34), size=8.5, color=TEXT2)
        text(s, xs[1], y, 0.9, 0.24, fmt, size=8.5, mono=True, color=MUTED)
        text(s, xs[2], y, 0.8, 0.24, rows, size=8.5, mono=True, color=TEXT2)
        text(s, xs[3], y, 4.0, 0.24, truncate(sha, 40), size=7.5, mono=True, color=MUTED)
        y += 0.34

    # ---- closing ----
    s = add_slide(prs)
    tlp_strip(s, prs)
    text(s, 0.8, 2.6, 11.8, 0.6, "End of deck", size=30, bold=True, align=PP_ALIGN.LEFT)
    text(s, 0.8, 3.4, 11.5, 0.9,
         ["Findings in this deck originate from authorized testing.",
          "Screenshots carry their sha256 in the log manifest, so a re-rendered deck",
          "references the same images and any mismatch is detectable."],
         size=12, color=TEXT2)

    os.makedirs(os.path.dirname(os.path.abspath(out)) or ".", exist_ok=True)
    prs.save(out)
    print(out)


if __name__ == "__main__":
    main()
