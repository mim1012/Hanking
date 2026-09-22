# Smoke log — v1.3.0-lab-bundle (2026-07-31)

Host: Kali lab (Linux). Windows `agent.exe` built but not executed here.

## Results

```
health: server_ver=1.3.0-lab-bundle transports=[wss,http] persist=true
WSS agent: agent-58e61c1cbc1bd4ef online transport=wss
shell uname -s → done result="Linux\n" exit_code=0
upload → path=./data/smoke_agent/got_upload.txt content=hello-upload-bundle-v13 sha ok
download → command done ready saved_path=data/files/cmd-..._to_download.txt size=16
AGENT_TOKEN on /admin → 401
HTTP beacon: agent-http-beacon-smoke transport=http
beacon shell echo beacon-ok → done result="beacon-ok\n"
agent.exe PE header OK (MZ + PE)
```

## Commands used

See README quick start + admin upload/download examples.
