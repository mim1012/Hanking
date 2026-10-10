#!/bin/bash
# quick /health check against local c2server
cd "$(dirname "$0")" || exit 1
TOKEN=*** -n 's/^export ADMIN_TOKEN="\([^"]*\)".*/\1/p' config.lab.env)
echo "token_len=${#TOKEN}"
curl -sk -H "X-Auth-Token: $TOKEN" https://127.0.0.1:8443/health
echo
