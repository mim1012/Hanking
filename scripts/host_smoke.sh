#!/usr/bin/env bash
# host_smoke.sh — Kali host: C2 + Linux agent live smoke
# checks: health / agents / shell(always-cmd) / screenshot(always-allowed) / priv
# API: POST /admin/cmd {agent_id, cmd_type, payload} / GET /admin/jobs?agent_id=
set -u
cd /home/kali/Projects/c2-project
source config.lab.env
ADMIN="Authorization: Bearer $ADMIN_TOKEN"
BASE="https://127.0.0.1:8443"
TS=$(date +%s)
OUT=data/host_smoke_${TS}
mkdir -p "$OUT"

echo "[*] cleanup previous instances"
pkill -f 'bin/agent_linux' 2>/dev/null
pkill -f 'bin/c2server' 2>/dev/null
sleep 1

echo "[*] start c2server"
./bin/c2server > "$OUT/server.log" 2>&1 &
SRV=$!
sleep 2
if ! kill -0 $SRV 2>/dev/null; then echo "FAIL: server died"; cat "$OUT/server.log"; exit 1; fi

echo "[*] start agent (wss)"
C2_TRANSPORT=wss ./bin/agent_linux > "$OUT/agent.log" 2>&1 &
AGT=$!
sleep 4
if ! kill -0 $AGT 2>/dev/null; then echo "FAIL: agent died"; cat "$OUT/agent.log"; kill $SRV 2>/dev/null; exit 1; fi

echo "== [1] health =="
curl -sk -H "$ADMIN" "$BASE/health" | tee "$OUT/health.json"; echo

echo "== [2] agents =="
curl -sk -H "$ADMIN" "$BASE/admin/agents" | tee "$OUT/agents.json"; echo
AGENT_ID=$(python3 -c "import json;d=json.load(open('$OUT/agents.json'));a=[x for x in d['agents'] if x.get('online')];print(a[0]['id'] if a else '')")
echo "AGENT_ID=$AGENT_ID (online)"
if [ -z "$AGENT_ID" ]; then echo "FAIL: no online agent"; kill $AGT $SRV 2>/dev/null; exit 1; fi

cmd() { # $1=type $2=payload
  curl -sk -X POST -H "$ADMIN" -H "Content-Type: application/json" \
    -d "{\"agent_id\":\"$AGENT_ID\",\"cmd_type\":\"$1\",\"payload\":\"$2\"}" \
    "$BASE/admin/cmd"
}

echo "== [3] shell (always-cmd, game_active=false) =="
MARK="HOST_SMOKE_OK_${TS}"
cmd shell "echo $MARK; id" | tee "$OUT/cmd_shell.json"; echo
sleep 3
curl -sk -H "$ADMIN" "$BASE/admin/jobs?agent_id=$AGENT_ID" | tee "$OUT/jobs.json" > /dev/null
python3 - "$OUT/jobs.json" "$MARK" <<'PY'
import json,sys
d=json.load(open(sys.argv[1])); mark=sys.argv[2]
for j in d.get('jobs',[]):
    if mark in (j.get('result') or ''):
        print("SHELL_PASS status=%s exit=%s" % (j.get('status'), j.get('exit_code')))
        print("  result:", (j.get('result') or '').strip().split('\n')[0])
        sys.exit(0)
print("SHELL_FAIL")
PY

echo "== [4] screenshot (always-allowed; gate removed) =="
cmd screenshot "" -w "\n" > /dev/null 2>&1
curl -sk -X POST -H "$ADMIN" -H "Content-Type: application/json" \
  -d "{\"agent_id\":\"$AGENT_ID\",\"cmd_type\":\"screenshot\"}" \
  "$BASE/admin/cmd" -w "\nHTTP:%{http_code}\n" | tee "$OUT/cmd_shot.txt"

echo "== [5] priv =="
cmd priv "" > /dev/null
sleep 2
curl -sk -H "$ADMIN" "$BASE/admin/jobs?agent_id=$AGENT_ID" | tee "$OUT/jobs2.json" > /dev/null
python3 - "$OUT/jobs2.json" <<'PY'
import json,sys
d=json.load(open(sys.argv[1]))
privs=[j for j in d.get('jobs',[]) if j.get('cmd_type')=='priv' and j.get('result')]
if privs:
    r=json.loads(privs[-1]['result'])
    print("PRIV_PASS is_admin=%s elev_method=%s uid=%s" % (r.get('is_admin'), r.get('elev_method'), r.get('uid')))
else:
    print("PRIV_FAIL")
PY

echo "== [6] logs tail =="
tail -3 "$OUT/server.log"
tail -3 "$OUT/agent.log"

echo "[*] stop"
kill $AGT $SRV 2>/dev/null
sleep 1
echo "DONE outdir=$OUT"
