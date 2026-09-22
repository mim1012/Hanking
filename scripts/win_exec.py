#!/usr/bin/env python3
"""Reliable exec to the Windows agent through the flaky lab C2 channel.
If a command stays in 'sent' without a result for a few polls, resend it
(idempotent commands only). Retries API transport errors.
Usage: win_exec.py '<spec-file-or-json>' [timeout_sec]
"""
import json, sys, time, ssl, urllib.request, urllib.error

AGENT = "agent-447313a4ee79d586"
TOKEN = "lab-admin-token-change-me"
CTX = ssl.create_default_context(); CTX.check_hostname = False; CTX.verify_mode = ssl.CERT_NONE

def api(method, path, body=None, retries=5):
    last = None
    for i in range(retries):
        try:
            req = urllib.request.Request("https://127.0.0.1:8443" + path, method=method)
            req.add_header("X-Auth-Token", TOKEN)
            req.add_header("Content-Type", "application/json")
            data = json.dumps(body).encode() if body is not None else None
            with urllib.request.urlopen(req, data=data, context=CTX, timeout=15) as r:
                return json.loads(r.read().decode())
        except Exception as e:
            last = e
            time.sleep(1.5 * (i + 1))
    raise last

def get_jobs():
    d = api("GET", "/admin/jobs?agent_id=" + AGENT + "&limit=25")
    jobs = d.get("jobs", d) if isinstance(d, dict) else d
    if isinstance(jobs, dict): jobs = list(jobs.values())
    return jobs or []

def main():
    arg = sys.argv[1]
    spec = json.loads(arg) if arg.startswith("{") else json.load(open(arg))
    payload_str = json.dumps(spec)
    deadline = time.time() + int(sys.argv[2]) if len(sys.argv) > 2 else time.time() + 180
    cid = None
    attempts = 0
    last_status_at = time.time()
    while time.time() < deadline:
        if cid is None:
            r = api("POST", "/admin/cmd", {"agent_id": AGENT, "cmd_type": "exec", "payload": payload_str})
            cid = r.get("command_id") or r.get("id")
            attempts += 1
            last_status_at = time.time()
            print(f"[send#{attempts}] {cid}")
        time.sleep(3)
        found = False
        for j in get_jobs():
            if j.get("id") == cid:
                found = True
                st = j.get("status")
                if st in ("done", "failed", "error", "cancelled"):
                    print("STATUS:", st)
                    print("RESULT:", j.get("result"))
                    print("EXIT:", j.get("exit_code"))
                    return
                if st == "sent" and time.time() - last_status_at > 8:
                    print("[resend] still 'sent' after 8s -> resend (idempotent)")
                    cid = None
                break
        if not found:
            print("[resend] job id vanished -> resend")
            cid = None
    print("TIMEOUT")
    sys.exit(1)

if __name__ == "__main__":
    main()
