#!/bin/zsh
# Lab C2 server launcher. Sources config.lab.env (C2_CRYPTO=0 is required for
# the session-1 v1.4.4 agent to register) and starts c2server in the background.
set -e
cd "$(dirname "$0")"

if ! [ -f bin/c2server ]; then
  echo "missing bin/c2server (build first: cd server && go build -o ../bin/c2server .)" >&2
  exit 1
fi

if pgrep -x c2server >/dev/null; then
  echo "c2server already running (pid $(pgrep -x c2server))" >&2
  exit 1
fi

source config.lab.env
setsid nohup ./bin/c2server > data/c2server_run.log 2>&1 < /dev/null &
disown
sleep 2

if ! pgrep -x c2server >/dev/null; then
  echo "start failed — see data/c2server_run.log" >&2
  exit 1
fi

echo "c2server up (pid $(pgrep -x c2server))"
# Remove old certs so new ones get generated with listen IP in SANs
rm -f ./certs/server.crt ./certs/server.key
curl -sk -H "X-Auth-Token: $ADMIN_TOKEN" "https://127.0.0.1:8443/health" | python3 -c \
  "import sys,json;d=json.load(sys.stdin);print('status',d.get('status'),'| crypto',d.get('crypto'))" 2>/dev/null || echo "health check failed (wait a moment)"
