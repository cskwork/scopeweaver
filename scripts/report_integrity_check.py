#!/usr/bin/env python3
# Vendored from the pentest-assessment-playbook skill
# (skills/pentest-assessment-playbook/scripts/report_integrity_check.py),
# MIT license. Kept stdlib-only so the gate needs no dependencies.
"""Mechanical integrity checks for a findings report before it is rendered to PDF.

Catches the defects that survive proofreading:
  1. claimed total != number of distinct finding ids (merged/placeholder ids excluded)
  2. a finding listed in a table but with no detail section (or vice versa)
  3. a stale figure left behind after a scope change ("6 tenant" while the body says 8)
  4. a newly minted id that duplicates an existing one (same asset, same class)
  5. appendix drift: an id in the findings list with no CWE/CVSS row (builders append findings to the
     card list long after the appendix JSON was generated), and a vector that does not compute to its
     stated score ("7.5" next to a C:H/I:H/A:H vector = 9.8, which a reviewer checks in one click)

Usage:
  python3 report_integrity_check.py REPORT.md
  python3 report_integrity_check.py REPORT.md --prefix KDT --stale "enam tenant" "6 tenant" "keenam"
  python3 report_integrity_check.py REPORT.md --merged KDT-08      # ids that are aliases, not findings
  python3 report_integrity_check.py REPORT.md --appendix findings_cwe.json   # CWE/vector table drift

Exit code 1 when any check fails, so it can gate a build script. Duplicate-asset hits are
warnings: they need a human decision (merge vs keep both), not an automatic failure.
"""
import argparse
import collections
import json
import math
import re
import sys

MERGED_RE = re.compile(r"digabung|disamakan|duplikat|duplicate of|merged into", re.I)
ASSET_RE = re.compile(r"\b(?:[a-z0-9][a-z0-9-]{0,40}\.)+(?:id|co\.id|com|net|org)\b")
PATH_RE = re.compile(r"/(?:apps|api|admin|login|config|database)[\w/.-]*")

AVW = {"N": 0.85, "A": 0.62, "L": 0.55, "P": 0.2}
ACW = {"L": 0.77, "H": 0.44}
UIW = {"N": 0.85, "R": 0.62}
CIAW = {"N": 0.0, "L": 0.22, "H": 0.56}


def cvss_base(vector):
    """CVSS 3.1 base score from a vector string, rounded up to one decimal."""
    if not vector or not vector.startswith("CVSS:3"):
        return None
    m = dict(p.split(":", 1) for p in vector.split("/")[1:] if ":" in p)
    try:
        scope = m["S"]
        prw = {"N": 0.85, "L": 0.62 if scope == "U" else 0.68,
               "H": 0.27 if scope == "U" else 0.5}[m["PR"]]
        iss = 1 - ((1 - CIAW[m["C"]]) * (1 - CIAW[m["I"]]) * (1 - CIAW[m["A"]]))
        impact = (6.42 * iss) if scope == "U" else (7.52 * (iss - 0.029) - 3.25 * (iss - 0.02) ** 15)
        exploit = 8.22 * AVW[m["AV"]] * ACW[m["AC"]] * prw * UIW[m["UI"]]
    except KeyError:
        return None
    if impact <= 0:
        return 0.0
    total = (impact + exploit) if scope == "U" else 1.08 * (impact + exploit)
    return math.ceil(round(min(total, 10), 6) * 10) / 10


def sections(text, prefix):
    """Map id -> section body (## / ### heading up to the next heading)."""
    out = {}
    parts = re.split(rf"^(#+\s*{prefix}-\d+.*)$", text, flags=re.M)
    for i in range(1, len(parts) - 1, 2):
        head, body = parts[i], parts[i + 1]
        m = re.search(rf"{prefix}-(\d+)", head)
        if m:
            out.setdefault(int(m.group(1)), head + body)
    return out


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("path")
    ap.add_argument("--prefix", default="KDT", help="finding id prefix, e.g. KDT")
    ap.add_argument("--stale", nargs="*", default=[],
                    help="phrases that must not appear (old counts after a scope change)")
    ap.add_argument("--merged", nargs="*", default=[],
                    help="ids that are aliases of another finding, e.g. KDT-08")
    ap.add_argument("--appendix", help="JSON list of {id, cvss, vector} for the CWE/CVSS appendix table")
    a = ap.parse_args()

    text = open(a.path, encoding="utf8", errors="replace").read()
    ids = {int(x) for x in re.findall(rf"{a.prefix}-(\d+)", text)}
    secs = sections(text, a.prefix)
    problems, warnings = [], []

    # 0. merged placeholders, passed explicitly plus auto-detected by phrasing
    merged = set()
    for token in a.merged:
        m = re.search(r"(\d+)", token)
        if m:
            merged.add(int(m.group(1)))
    for fid, body in secs.items():
        if MERGED_RE.search(body[:400]):
            merged.add(fid)
    if merged:
        print(f"note aliases not counted as findings: {sorted(a.prefix + '-' + str(i) for i in merged)}")

    effective = ids - merged

    # 1. claimed total
    claimed = re.findall(r"\*\*(\d+)\s+(?:temuan|findings?)", text)
    if claimed:
        total = int(claimed[0])
        if total != len(effective):
            problems.append(
                f"claimed total {total} != {len(effective)} effective ids "
                f"({len(ids)} ids minus {len(merged)} aliases)")
        else:
            print(f"ok  total: {total} findings == {len(effective)} effective ids")
    else:
        print("warn no 'N temuan/N findings' line found; skipping total check")

    # 2. id -> detail section coverage (alias ids may legitimately have no section)
    missing = sorted(ids - set(secs) - merged)
    orphan = sorted(set(secs) - ids)
    if missing:
        problems.append(f"ids without a detail section: {missing}")
    else:
        print(f"ok  every effective id has a detail section ({len(secs)} sections)")
    if orphan:
        problems.append(f"detail sections with no id reference: {orphan}")

    # 3. stale scope figures
    for phrase in a.stale:
        n = text.count(phrase)
        if n:
            problems.append(f"stale phrase x{n}: {phrase!r}")
    if a.stale:
        print(f"ok  no stale phrases ({len(a.stale)} checked)" if not any(
            text.count(p) for p in a.stale) else "fail stale phrases present")

    # 4. duplicate coverage: two ids describing the same asset and class
    assets = collections.defaultdict(set)
    for fid, body in secs.items():
        if fid in merged or len(body) < 200:
            continue
        for host in ASSET_RE.findall(body):
            if host.count(".") >= 1 and not host.endswith((".php", ".sql")):
                assets[fid].add(host.lower())
        for path in PATH_RE.findall(body):
            assets[fid].add(path.lower())
    pairs = set()
    for fid, mine in assets.items():
        for other, theirs in assets.items():
            if fid >= other:
                continue
            shared = mine & theirs
            if shared:
                pairs.add((fid, other, tuple(sorted(shared))[:3]))
    if pairs:
        for fid, other, shared in sorted(pairs):
            warnings.append(
                f"possible duplicate coverage: {a.prefix}-{fid} and {a.prefix}-{other} both name {shared} "
                "- upgrade the older row and delete the new one, or state the different security property")

    # 5. appendix drift: every finding must have a CWE/vector row, and the vector must compute to the stated score
    if a.appendix:
        try:
            rows = json.load(open(a.appendix, encoding="utf8"))
        except Exception as exc:                                  # noqa: BLE001
            problems.append(f"appendix unreadable: {a.appendix}: {exc}")
            rows = []
        have = {int(m.group(1)) for r in rows
                for m in [re.search(rf"{a.prefix}-(\d+)", str(r.get("id", "")))] if m}
        gap = sorted(effective - have)
        if gap:
            problems.append(f"ids missing from appendix: {sorted(a.prefix + '-' + str(i) for i in gap)}")
        else:
            print(f"ok  appendix covers every effective id ({len(have)} rows)")
        for r in rows:
            stated, vec = r.get("cvss"), r.get("vector", "")
            got = cvss_base(vec)
            if stated in (None, "") or got is None:
                continue
            if abs(float(stated) - got) > 0.05:
                problems.append(
                    f"{r.get('id')}: stated CVSS {stated} but {vec} computes {got} "
                    "- derive the vector FROM the score (enumerate C/I/A x S/AC/PR/UI, keep the one that lands on it)")
        if not any("appendix" in p or "stated CVSS" in p for p in problems):
            print("ok  every appendix vector computes to its stated score")

    if warnings:
        print("\nWARN (needs a human decision)")
        for w in warnings:
            print("  -", w)

    if problems:
        print("\nFAIL")
        for p in problems:
            print("  -", p)
        return 1
    print("\nPASS")
    return 0


if __name__ == "__main__":
    sys.exit(main())
