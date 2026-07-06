# -*- coding: utf-8 -*-
"""_wiki lint: dead [[links]], orphan pages, stale file:line backlinks, index counts.

Usage: python -X utf8 D:\\Project\\_wiki\\scripts\\lint.py
Implements the Lint cycle from _wiki/AGENTS.md (CLAUDE.md is an @AGENTS.md pointer).
Read-only; prints a report.
"""
import os
import re
import sys

WIKI_ROOT = r"D:\Project\_wiki"
PROJECT_ROOT = r"D:\Project"

pages = {}  # basename(no ext) -> relpath
for dirpath, dirnames, filenames in os.walk(WIKI_ROOT):
    if ".obsidian" in dirpath or "scripts" in dirpath:
        continue
    for fn in filenames:
        if fn.endswith(".md"):
            rel = os.path.relpath(os.path.join(dirpath, fn), WIKI_ROOT)
            pages[fn[:-3]] = rel

link_re = re.compile(r"\[\[([^\]|#]+?)(?:[|#][^\]]*)?\]\]")
# backtick refs like `massage/src/lib/foo.ts:204` or `PJT v4.7 1023 new (2)/scripts/foo.py:12`
# (path segments may contain spaces, parens, Korean — real folder names under D:\Project)
ref_re = re.compile(r"`([A-Za-z0-9_.\-() 가-힣]+(?:/[A-Za-z0-9_.\-\[\]() 가-힣]+)+\.[A-Za-z]{1,4}):(\d+)`")

PLACEHOLDERS = {"개념", "다른-개념", "링크", "프로젝트"}  # schema/README template examples

dead_links = []
inbound = {name: 0 for name in pages}
stale_refs = []
checked_refs = 0

for name, rel in pages.items():
    text = open(os.path.join(WIKI_ROOT, rel), encoding="utf-8").read()
    for m in link_re.finditer(text):
        target = m.group(1).strip()
        if target in PLACEHOLDERS:
            continue
        if target in pages:
            if target != name:
                inbound[target] += 1
        else:
            dead_links.append((rel, target))
    for m in ref_re.finditer(text):
        path, line = m.group(1), int(m.group(2))
        checked_refs += 1
        # resolve: try D:\Project\<path>, then the page's own project prefix
        candidates = [os.path.join(PROJECT_ROOT, *path.split("/"))]
        if name.startswith("project-"):
            proj = name[len("project-"):]
            candidates.append(os.path.join(PROJECT_ROOT, proj, *path.split("/")))
        found = None
        for c in candidates:
            if os.path.isfile(c):
                found = c
                break
        if not found:
            stale_refs.append((rel, f"{path}:{line}", "file missing"))
            continue
        try:
            n_lines = sum(1 for _ in open(found, encoding="utf-8", errors="ignore"))
            if line > n_lines:
                stale_refs.append((rel, f"{path}:{line}", f"only {n_lines} lines"))
        except OSError:
            stale_refs.append((rel, f"{path}:{line}", "unreadable"))

orphans = [
    (n, rel) for n, rel in pages.items()
    if inbound[n] == 0
    and rel.startswith("wiki" + os.sep)
    and os.sep + "도메인" + os.sep not in rel  # maps are entry points
]

n_concepts = sum(1 for r in pages.values() if r.startswith(os.path.join("wiki", "concepts")))
n_projects = sum(1 for r in pages.values() if r.startswith(os.path.join("wiki", "projects")))

print(f"pages: {len(pages)} (concepts {n_concepts}, projects {n_projects})")
print(f"\nDEAD LINKS: {len(dead_links)}")
for rel, t in dead_links:
    print(f"  {rel} -> [[{t}]]")
print(f"\nORPHANS (no inbound links): {len(orphans)}")
for n, rel in orphans:
    print(f"  {rel}")
print(f"\nFILE:LINE refs checked: {checked_refs}, stale: {len(stale_refs)}")
for rel, ref, why in stale_refs:
    print(f"  {rel} -> {ref} ({why})")

# log.md: every entry line must match `- YYYY-MM-DD | 사이클 | 요약`, dates non-decreasing
log_issues = []
log_path = os.path.join(WIKI_ROOT, "log.md")
if os.path.isfile(log_path):
    entry_re = re.compile(r"^- (\d{4}-\d{2}-\d{2}) \| [^|]+ \| .+$")
    prev_date = None
    for i, raw in enumerate(open(log_path, encoding="utf-8"), 1):
        s = raw.rstrip("\n")
        if not s.startswith("- "):
            continue
        m = entry_re.match(s)
        if not m:
            log_issues.append(f"line {i}: 형식 불일치 (`- YYYY-MM-DD | 사이클 | 요약`)")
            continue
        if prev_date and m.group(1) < prev_date:
            log_issues.append(f"line {i}: 날짜 역순 ({m.group(1)} < {prev_date})")
        prev_date = m.group(1)
print(f"\nlog.md issues: {len(log_issues)}")
for issue in log_issues:
    print(f"  {issue}")

readme = open(os.path.join(WIKI_ROOT, "README.md"), encoding="utf-8").read()
m = re.search(r"개념 (\d+) · 프로젝트 (\d+)", readme)
if m:
    rc, rp = int(m.group(1)), int(m.group(2))
    ok = "OK" if (rc == n_concepts and rp == n_projects) else "MISMATCH"
    print(f"\nREADME counts: 개념 {rc}/{n_concepts}, 프로젝트 {rp}/{n_projects} -> {ok}")

sys.exit(0)
