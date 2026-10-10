#!/usr/bin/env python3
"""Inventory string literals in agent/ and loader/ Go sources.

Classifies literals that are candidates for runtime XOR obfuscation so no
C2-identifying plaintext remains in the compiled binary.

Usage: python3 scripts/audit_literals.py [root]
"""
import re
import sys
from collections import defaultdict
from pathlib import Path

ROOT = Path(sys.argv[1]) if len(sys.argv) > 1 else Path(".")

# scan targets
DIRS = ["agent", "loader"]

# regex for string literals: double-quoted (with escapes) or backtick raw
DQ = re.compile(r'"((?:[^"\\]|\\.)*)"')
BQ = re.compile(r"`([^`]*)`")

ENV_RE = re.compile(r"^[A-Z][A-Z0-9]*(?:_[A-Z0-9]+)+$")

# known Win32 / ntdll / amsi API names
API_NAMES = {
    "CreateToolhelp32Snapshot", "Module32FirstW", "Module32NextW",
    "Process32FirstW", "Process32NextW", "CloseHandle", "VirtualProtect",
    "VirtualAlloc", "VirtualAllocEx", "VirtualProtectEx", "VirtualFree",
    "WriteProcessMemory", "ReadProcessMemory", "CreateRemoteThread",
    "OpenProcess", "GetModuleHandleW", "GetModuleHandleExW", "LoadLibraryW",
    "GetProcAddress", "CreateProcessW", "CreateProcessA", "GetThreadContext",
    "SetThreadContext", "ResumeThread", "WaitForSingleObject", "CreateFileW",
    "WinExec", "NtUnmapViewOfSection", "ZwUnmapViewOfSection",
    "AmsiInitialize", "AmsiScanString", "AmsiScanBuffer", "AmsiUninitialize",
    "RtlInitAnsiString", "NtOpenProcess", "NtQuerySystemInformation",
    "QueryPerformanceCounter", "GetCurrentProcessId",
}

DLL_RE = re.compile(r"^[A-Za-z0-9_\-]+\.(dll|exe)$", re.I)
PATH_RE = re.compile(r"^[A-Za-z]:\\")
SID_RE = re.compile(r"^s-1-(5|16)-[0-9]+$", re.I)
UA_RE = re.compile(r"^(Mozilla|Microsoft|Opera)")
CMD_NAMES = {
    "net", "whoami", "bash", "sudo", "schtasks", "tasklist", "powershell",
    "cmd", "reg", "svchost.exe", "agent.exe", "payload.bin", "rundll32.exe",
    "regsvr32.exe", "notepad.exe", "powershell.exe", "wmic",
}

def scan_file(p: Path):
    txt = p.read_text(errors="replace")
    out = []
    # remove comments naively enough
    lines = txt.split("\n")
    buf = []
    in_block = False
    for ln in lines:
        if in_block:
            if "*/" in ln:
                in_block = False
                ln = ln[ln.index("*/") + 2:]
            else:
                continue
        if "/*" in ln:
            i = ln.index("/*")
            if "*/" in ln[i:]:
                ln = ln[:i] + ln[i + 4:ln.index("*/", i) + 2]
            else:
                buf.append(ln[:i])
                in_block = True
                continue
        buf.append(ln)
    src = "\n".join(buf)
    for m in DQ.finditer(src):
        raw = m.group(0)[1:-1]
        try:
            val = raw.encode("utf-8").decode("unicode_escape").encode("latin-1").decode("utf-8", "replace")
        except Exception:
            val = raw
        out.append(("dq", val, raw))
    for m in BQ.finditer(src):
        out.append(("bq", m.group(1), m.group(1)))
    return out

def classify(val: str):
    v = val.strip()
    if not v:
        return None
    if ENV_RE.match(v) and len(v) >= 4:
        return "env"
    if v in API_NAMES:
        return "api"
    if DLL_RE.match(v) and not v.startswith("{{"):
        return "dll"
    if SID_RE.match(v):
        return "sid"
    if UA_RE.match(v):
        return "ua"
    if PATH_RE.match(v):
        return "path"
    if v in CMD_NAMES:
        return "cmd"
    if v in {"X-Auth-Token", "User-Agent", "Content-Type", "Accept",
             "application/json", "aes-256-gcm", "c2-lab-v1|",
             "C2Lab_", "C2_EVASION", "X-Auth-Token", "X-Agent-Id"}:
        return "marker"
    if re.match(r"^/(?:FO|CSV|NH|create|tn|tr|f|sc|rl|ru|query|delete|ct)\b", v) or v.startswith("/"):
        return "flag"
    return None

def main():
    groups = defaultdict(set)
    counts = defaultdict(int)
    for d in DIRS:
        for p in sorted((ROOT / d).rglob("*.go")):
            if any(part in p.parts for part in (".build", "main.go.bak")):
                continue
            if p.name.endswith((".bak",)) or ".bak" in p.name:
                continue
            for kind, val, raw in scan_file(p):
                if "\\x" in raw and len(raw) > 120:
                    # already-encoded table
                    continue
                if "{{" in val:
                    continue
                c = classify(val)
                if c:
                    groups[c].add((val, str(p.relative_to(ROOT))))
                    counts[c] += 1
    for c in sorted(groups):
        print(f"\n### {c} ({counts[c]})")
        for val, where in sorted(groups[c]):
            print(f"  {val!r:60} <- {where}")

if __name__ == "__main__":
    main()
