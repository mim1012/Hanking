#!/usr/bin/env python3
import re, subprocess, urllib.request, ssl
env = {}
for line in open('/Users/jhkim/c2-project/config.lab.env'):
    m = re.match(r'^export\s+([A-Z_]+)=["\']?([^"\']*)["\']?\s*$', line)
    if m:
        env[m.group(1)] = m.group(2)
tok = env.get('ADMIN_TOKEN', '')
print('token_len=%d' % len(tok))
ctx = ssl.create_default_context()
ctx.check_hostname = False
ctx.verify_mode = ssl.CERT_NONE
req = urllib.request.Request('https://127.0.0.1:8443/health', headers={'X-Auth-Token': tok})
try:
    r = urllib.request.urlopen(req, context=ctx, timeout=10)
    print('HTTP', r.status)
    print(r.read().decode()[:400])
except urllib.error.HTTPError as e:
    print('HTTPError', e.code, e.read().decode()[:200])
except Exception as e:
    print('ERR', type(e).__name__, e)
