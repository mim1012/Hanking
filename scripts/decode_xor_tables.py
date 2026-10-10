#!/usr/bin/env python3
"""Decode all XOR tables in loader/xor.go for inspection."""
import re, sys

key = b"0p3nc0de-c2lab-x0r"
src = open(sys.argv[1] if len(sys.argv) > 1 else "loader/xor.go").read()
pat = re.compile(r"e(\w+)\s*=\s*\[\]byte\(\"((?:\\x[0-9a-f]{2})*)\"\)")
for m in pat.finditer(src):
    name, hexs = m.group(1), m.group(2)
    b = bytes(int(x, 16) for x in re.findall(r"\\x([0-9a-f]{2})", hexs))
    dec = bytes(c ^ key[i % len(key)] for i, c in enumerate(b))
    print(f"e{name} = {dec.decode(errors='replace')!r}")
