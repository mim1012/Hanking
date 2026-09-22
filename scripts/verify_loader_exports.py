#!/usr/bin/env python3
"""Verify PE export table contains EntryPoint (WMI/MSI rundll32 contract)."""
from __future__ import annotations

import struct
import sys
from pathlib import Path


def pe_exports(path: Path) -> list[str]:
    data = path.read_bytes()
    if data[:2] != b"MZ":
        raise SystemExit(f"{path}: not MZ")
    e = struct.unpack_from("<I", data, 0x3C)[0]
    if data[e : e + 4] != b"PE\0\0":
        raise SystemExit(f"{path}: bad PE")
    magic = struct.unpack_from("<H", data, e + 24)[0]
    num_sec = struct.unpack_from("<H", data, e + 6)[0]
    opt_size = struct.unpack_from("<H", data, e + 20)[0]
    if magic == 0x20B:
        export_rva = struct.unpack_from("<I", data, e + 24 + 112)[0]
    else:
        export_rva = struct.unpack_from("<I", data, e + 24 + 96)[0]
    sec_off = e + 24 + opt_size
    secs = []
    for i in range(num_sec):
        off = sec_off + i * 40
        name = data[off : off + 8].split(b"\0")[0].decode("ascii", "replace")
        vsize, va, rawsize, rawptr = struct.unpack_from("<IIII", data, off + 8)
        secs.append((name, va, vsize, rawptr, rawsize))

    def rva_to_off(rva: int) -> int | None:
        for _n, va, vsize, rawptr, rawsize in secs:
            if va <= rva < va + max(vsize, rawsize):
                return rawptr + (rva - va)
        return None

    if not export_rva:
        return []
    off = rva_to_off(export_rva)
    if off is None:
        return []
    vals = struct.unpack_from("<IIHHIIIIIII", data, off)
    nnames = vals[7]
    aon = vals[9]
    names = []
    for i in range(nnames):
        no = rva_to_off(aon + i * 4)
        if no is None:
            continue
        name_rva = struct.unpack_from("<I", data, no)[0]
        no2 = rva_to_off(name_rva)
        if no2 is None:
            continue
        end = data.index(b"\0", no2)
        names.append(data[no2:end].decode())
    return names


def main() -> int:
    root = Path(__file__).resolve().parents[1]
    dll = Path(sys.argv[1]) if len(sys.argv) > 1 else root / "bin" / "loader.dll"
    # Avoid direct .exe path guards in some runners — copy-friendly
    if not dll.exists():
        print(f"MISSING {dll}")
        return 2
    names = pe_exports(dll)
    print(f"{dll}: exports={names}")
    need = {"EntryPoint"}
    ok = need.issubset(set(names))
    if ok:
        print("OK EntryPoint export present")
        return 0
    print("FAIL missing EntryPoint — WMI/MSI rundll32 path will break")
    return 1


if __name__ == "__main__":
    raise SystemExit(main())
