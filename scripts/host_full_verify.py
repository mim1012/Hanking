#!/usr/bin/env python3
"""Host file-transfer + exec + screenshot + stream verification."""
import base64, hashlib, json, os, ssl, sys, time, urllib.request, urllib.error

AGENT = "agent-447313a4ee79d586"
TOKEN = "lab-admin-token-change-me"
CTX = ssl.create_default_context(); CTX.check_hostname = False; CTX.verify_mode = ssl.CERT_NONE

def api(method, path, body=None, retries=6):
    last = None
    for i in range(retries):
        try:
            req = urllib.request.Request("https://127.0.0.1:8443" + path, method=method)
            req.add_header("X-Auth-Token", TOKEN)
            req.add_header("Content-Type", "application/json")
            data = json.dumps(body).encode() if body is not None else None
            with urllib.request.urlopen(req, data=data, context=CTX, timeout=20) as r:
                return json.loads(r.read().decode())
        except Exception as e:
            last = e; time.sleep(1.2 * (i + 1))
    raise last

def send_cmd(cmd_type, payload="", timeout=60):
    r = api("POST", "/admin/cmd", {"agent_id": AGENT, "cmd_type": cmd_type, "payload": payload, "force": True})
    cid = r.get("command_id") or r.get("id")
    print(f"  cmd={cmd_type} id={cid}")
    deadline = time.time() + timeout
    while time.time() < deadline:
        time.sleep(3)
        d = api("GET", f"/admin/jobs?agent_id={AGENT}&limit=10")
        jobs = d.get("jobs", d) if isinstance(d, dict) else d
        if isinstance(jobs, dict): jobs = list(jobs.values())
        for j in jobs or []:
            if j.get("id") == cid and j.get("status") in ("done", "failed", "error", "cancelled"):
                return j
    return {"status": "TIMEOUT", "id": cid}

def main():
    phase = sys.argv[1] if len(sys.argv) > 1 else "all"
    print(f"=== target {AGENT} ===")

    if phase in ("all", "upload"):
        print("[1] UPLOAD host_probe.bat -> C:\\Users\\Public\\host_probe.bat")
        raw = open("/tmp/opencode/host_probe.bat", "rb").read()
        sha = hashlib.sha256(raw).hexdigest()
        print(f"  local size={len(raw)} sha256={sha}")
        r = api("POST", "/admin/upload", {"agent_id": AGENT, "path": r"C:\Users\Public\host_probe.bat", "data_b64": base64.b64encode(raw).decode()})
        print("  server:", json.dumps(r, ensure_ascii=False)[:400])
        tid = r.get("transfer_id")
        deadline = time.time() + 60
        while time.time() < deadline:
            time.sleep(3)
            d = api("GET", f"/admin/jobs?agent_id={AGENT}&limit=30")
            jobs = d.get("jobs", d) if isinstance(d, dict) else d
            if isinstance(jobs, dict): jobs = list(jobs.values())
            states = [j.get("status") for j in (jobs or []) if tid in j.get("payload", "")]
            if states:
                print(f"  chunk states: {states}")
                if all(s in ("done", "failed", "error") for s in states):
                    break
        print("  upload done")

    if phase in ("all", "exec"):
        print("[2] EXEC uploaded file via cmd /c")
        j = send_cmd("exec", json.dumps({"path": r"C:\Windows\System32\cmd.exe", "args": ["/c", r"C:\Users\Public\host_probe.bat"]}), timeout=60)
        print("  status:", j.get("status"), "exit:", j.get("exit_code"))
        print("  result:", j.get("result"))

    if phase in ("all", "hash"):
        print("[3] VERIFY remote file hash + presence")
        j = send_cmd("exec", json.dumps({"path": r"C:\Windows\System32\WindowsPowerShell\v1.0\powershell.exe", "args": ["-NoProfile", "-Command", "(Get-FileHash C:\\Users\\Public\\host_probe.bat -Algorithm SHA256).Hash; (Get-Item C:\\Users\\Public\\host_probe.bat).Length"]}), timeout=60)
        print("  status:", j.get("status"), "exit:", j.get("exit_code"))
        print("  result:", j.get("result"))

    if phase in ("all", "screenshot"):
        print("[4] SCREENSHOT")
        j = send_cmd("screenshot", timeout=90)
        print("  status:", j.get("status"))
        result = j.get("result") or j.get("data") or ""
        if isinstance(result, str) and result:
            try:
                meta = json.loads(result)
                raw = base64.b64decode(meta["data_b64"])
                out = f"/tmp/opencode/host_screen.png"
                open(out, "wb").write(raw)
                print(f"  screenshot saved: {out} size={len(raw)} sha256={meta.get('sha256')} filetype={meta.get('type')}")
            except Exception as e:
                print("  parse failed:", e, "raw:", result[:200])

    if phase in ("all", "stream"):
        print("[5] STREAM start + frame")
        j = send_cmd("stream_start", json.dumps({"fps": 1.0}), timeout=30)
        print("  start:", j.get("status"), j.get("result"))
        jf = send_cmd("stream_frame", timeout=60)
        print("  frame cmd:", jf.get("status"), (jf.get("result") or "")[:150])
        # pull latest cached frame from server
        try:
            f = api("GET", f"/admin/stream?agent_id={AGENT}&wait_sec=5")
            print("  admin/stream frame keys:", list(f.keys()) if isinstance(f, dict) else f)
            if isinstance(f, dict) and f.get("data"):
                meta = f["data"]
                if isinstance(meta, str):
                    meta = json.loads(meta)
                raw = base64.b64decode(meta["data_b64"])
                out = f"/tmp/opencode/host_stream_frame.png"
                open(out, "wb").write(raw)
                print(f"  stream frame saved: {out} size={len(raw)} sha256={meta.get('sha256')}")
        except Exception as e:
            print("  stream poll failed:", e)
        send_cmd("stream_stop", timeout=30)

if __name__ == "__main__":
    main()
