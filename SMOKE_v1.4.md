# Smoke log — v1.4.0-lab-malleable (2026-07-31)

> **Note (2026-07-31)**: 현재 config 정본 PSK는 `lab-psk-change-me` 이다. 아래 v1.4 스모크 당시 값은 기록 보존용.

Host: Kali lab (Linux). Windows agent built (`bin/agent_win_amd64`, MZ/PE OK) — not executed here.

## Config under test

- `BEACON_URIS=/jquery.min.js,/cdn/analytics.js,/api/v2/telemetry`
- `AGENT_WS_PATH=/socket/v1`
- `RESPONSE_HEADERS=Server: cloudflare; X-Content-Type-Options: nosniff; Cache-Control: no-store`
- `C2_PSK=lab-psk-v14-change-me`, `C2_CRYPTO=1`

## Results

```
health: server_ver=1.4.0-lab-malleable malleable=true crypto=true
        beacon_uris=[/jquery.min.js /cdn/analytics.js /api/v2/telemetry]
response headers: server=cloudflare, cache-control=no-store, x-content-type-options=nosniff

HTTP beacon + AEAD:
  agent-http-malleable-smoke online transport=http
  shell echo malleable-crypto-ok → done result="malleable-crypto-ok\n" exit_code=0

WSS + AEAD:
  agent-wss-malleable-smoke online transport=wss
  shell uname -s → done result="Linux\n" exit_code=0

URI rotation (encrypted POST):
  /jquery.min.js → 200 envelope
  /cdn/analytics.js → 200 envelope
  /api/v2/telemetry → 200 envelope

Negative:
  plaintext register with crypto on → 400 crypto required
  wrong PSK → 400 message authentication failed

Windows agent PE: MZ OK (bin/agent_win_amd64)
```

## Verdict

B-H7 (URI/UA/headers/response headers/host knobs) + B-H8 (AES-256-GCM envelope) **smoke PASS** on lab loopback.


## v1.4.4 always-cmd
See `data/linux_always_cmd_smoke.md` and `docs/always-cmd-elevated.md`.
