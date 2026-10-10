#!/usr/bin/env python3
# Minimal operator helper for the lab C2 admin API.
# Usage:
#   c2_admin.py shell '<cmd>' [agent_id]
#   c2_admin.py priv [agent_id]
#   c2_admin.py jobs [agent_id]
#   c2_admin.py agents
import sys, json, subprocess, urllib.request, ssl

HOST = "https://127.0.0.1:8443"
TOKEN = "lab-admin-token-change-me"
CTX = ssl.create_default_context()
CTX.check_hostname = False
CTX.verify_mode = ssl.CERT_NONE

def api(method, path, body=None, token=TOKEN):
    req = urllib.request.Request(HOST + path, method=method)
    req.add_header("X-Auth-Token", token)
    req.add_header("Content-Type", "application/json")
    data = json.dumps(body).encode() if body is not None else None
    with urllib.request.urlopen(req, data=data, context=CTX, timeout=20) as r:
        return json.loads(r.read().decode())

def main():
    mode = sys.argv[1] if len(sys.argv) > 1 else "shell"
    agent = sys.argv[3] if len(sys.argv) > 3 else "agent-447313a4ee79d586"
    if mode == "agents":
        d = api("GET", "/admin/agents")
        for a in d.get("agents", []):
            print(json.dumps(a, ensure_ascii=False))
        return
    if mode == "jobs":
        d = api("GET", "/admin/jobs")
        jobs = d.get("jobs", d) if isinstance(d, dict) else d
        if isinstance(jobs, dict):
            jobs = list(jobs.values())
        for j in (jobs or [])[-10:]:
            print(json.dumps(j, ensure_ascii=False)[:700])
        return
    payload = sys.argv[2] if len(sys.argv) > 2 else ""
    body = {"agent_id": agent, "cmd_type": mode, "payload": payload}
    try:
        resp = api("POST", "/admin/cmd", body)
        print("SENT:", json.dumps(resp, ensure_ascii=False))
    except urllib.error.HTTPError as e:
        print("HTTP", e.code, e.read().decode()[:500])
        return
    import time
    jid = resp.get("job_id") or resp.get("command_id") or resp.get("id")
    for _ in range(40):
        time.sleep(1)
        d = api("GET", "/admin/jobs")
        jobs = d.get("jobs", d) if isinstance(d, dict) else d
        if isinstance(jobs, dict):
            jobs = list(jobs.values())
        for j in (jobs or []):
            if (jid and str(j.get("id")) == str(jid)) or (jid and str(j.get("job_id")) == str(jid)):
                st = j.get("status")
                print(f"POLL: status={st}")
                if st in ("done", "failed", "error", "cancelled"):
                    print(json.dumps(j, ensure_ascii=False)[:2000])
                    return
    print("TIMEOUT waiting for job result")

if __name__ == "__main__":
    main()
