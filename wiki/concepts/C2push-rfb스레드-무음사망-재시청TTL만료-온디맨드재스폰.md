# C2push rfb 스레드 무음사망 → 재시청 TTL 만료 → stream_start 온디맨드 재스폰

## 증상

뷰어에서 좌석(PC-229) 클릭 → 재생 아이콘만 떠 있고 화면 없음. WS 푸시 keepalive(`stream_start`)가 정상 도착했는데 c2push 로그에 `rfb 연결 OK`/`connect EXC` 등 rfb 라이프사이클 라인이 **전혀 없음** → ~90초 후 `stream TTL 만료 → 인코더 정지`로 끝남.

## 근본원인

c2push 좌석 스레드가 생성(RFB 수신 `rfb_loop` 스레드 1개 포함)하는 구조인데, 좌석 주소 재구성(refresh_seats, 60초 주기) 중 `_one()`이 예외로 무음 실패(`except: return`)하면 SEATS에서 해당 좌석이 잠깐 빠짐. 그 순간 rfb_loop의 `if name not in SEATS: return` / `if s_ is None: break` 경로가 **로그 없이 스레드를 영구 사망**시킴. 이후 좌석이 SEATS에 돌아와도 **재스폰 수단이 없음** — 좌석 스레드는 살아있으니(pong 계속) 송신은 멀쩡한데 수신(프레임 공급)만 죽은 채로 stream_start 명령을 받아버림.

## 해결 (2026-10-10 패치)

`stream_start` 핸들러에서 rfb 스레드 핸들(`stt.rfb_thread`)의 `is_alive()` 판정 → 죽었으면 그 자리에서 재스폰. 무조건 스폰(좌석 시작 시점)은 제거하고 온디맨드 스폰으로 전환. 무음 사망 경로 전부에 로그(`rfb thread exit (좌석 무기록|치워짐)`) 부여. 재스폰은 살아있는 스레드엔 발동 안 함(이중 연결 방지) — 실측으로 00:28 스폰 스레드가 00:36 stream_start에서 재스폰 라인 없이 `rfb 연결 OK` 복귀를 확인.

## 재발 방지 교훈

- **사용자 화면이 재생 아이콘이면 곧바로 "게이트 차단(온라인+한게임)"만 의심하지 말 것** — 게이트는 통과했는데 수신 스레드가 죽은 상태일 수 있다. 판별법: c2push 로그에서 stream_start 도착 직후 `rfb 연결 OK` 라인 없으면 rfb 스레드 사망.
- 백엔드 WS 조회 포맷: `payload`는 JSON **문자열**(`JSON.stringify` 결과, app.js format이 신뢰 소스).
- 스레드 무음 사망 방어는 "stream_start 수신 시점에 생존 판정 + 온디맨드 스폰"이 가장 안전 — refresh 주기로 복구를 기다리지 않는다.

## 2차 결함: refresh 부분 실패 시 좌석 조용한 증발 (2026-10-10)

위 온디맨드 재스폰만으로는 부족했다. refresh_seats가 "이번 주기 해석 성공분만"으로 SEATS를 전면 교체(clear+update)하기 때문에, 좌석 주소 해석(remote-url)이 일부 실패하면 그 좌석이 조용히 SEATS에서 증발 → 존재 중이던 `rfb thread exit (좌석 무기록)` 로 이어짐. 뷰어 목록(resource=winhips 로스터)은 남아 초록 표시라 클릭→프레임 0 재생아이콘이 됨.

수정: **보류-재사용(carry-over)** — 이번 주기 direct 목록에 있으나 해석 실패한 좌석은 이전 주소값을 그대로 SEATS에 유지(`carried`), 진짜 roster 이탈만 프룬. 해석 실패는 이제 print로 소리 나게 로깅. seats.json도 SEATS(fresh_all)와 동기화.

정본 구현: `dev-cache/Temp/opencode/c2push_v4.py` `refresh_seats()` — carried/fresh_all 커밋 블록 및 `_one()` 실패 로그 (배포본 `/root/c2/c2push.py` 2026-10-10, E2E T_FIRST_DATA=2.37s/5.5MB로 프레임 흐름 복구 실측).

## 배제된 접근 (dead-end)

- **"성공분만 전면 교체" refresh 구성** — 부분 실패가 곧 전체 좌석 증발로 번짐. 관측 기반: 패치 전 refresh 로그 좌석수 15~18대 진동 + rfb thread exit 반복, 패치 후 18→21 단조 증가·안정.
- **rfb 스레드를 refresh 주기에 맞춰 무조건 스폰(좌석 시작 시)** — stream_start 전엔 쓸모없는 대기 스레드만 남고, 사망 후 부활 수단도 없어 무음 정체로 빠짐. stream_start 시점 온디맨드 스폰으로 교체.

## 3차 확장: c2server 롱폴 이벤트 대기 + 204 백오프 (v19, 2026-10-10)

프레임 생산(c2push rfb)은 정상인데 뷰어 재생이 뚝뚝 끊긴 두 번째 원인은 **서버 롱폴 대기 구조**였다: `/admin/stream` raw 모드가 init 존재 시 `len(out)==0`(신규 세그 없음)이어도 즉시 빈 200을 반환 → 브라우저가 0ms 갭으로 재요청 → 요청 폭풍+세그 사이 공백. v19: ①빈 백로그 시 wake 채널 이벤트 대기(300ms 백오프, 무부하) — `streamNotify`가 init/seg 수신부 4곳에서 브로드캐스트 ②창 끝 눕히면 **204 No Content** 반환 ③브라우저 pollMSE는 204 받으면 150ms 백오프 재폴, octet-stream 성공은 즉시(0ms) 순차 롱폴 유지.

정본 구현: `c2-project/server/main.go` `handleAdminStream` rawFmt len(out)==0 분기 + `wakeChan`/`streamNotify`(wakeMu 별도 락 — streamMu 홀드 중 notify 교착 방지), `c2-project/server/ui/static/app.js` `pollMSE` 204 분기.

실측(vultr, poll serve 로그): waited_ms 119~183ms(폴 요청이 세그 도착까지 홀드됨 — 이전 즉시 반환 구조 대비), 204 발화 0건(세그가 항상 1초 창 내 도착). 브라우저 E2E(OX-045, 폴레그): 첫프레임 1.06s, 스톨 0건, lag avg 0.883s/max 1.175s, 폴 갭 median 466ms, c2server CPU 5.1→5.4%(무부하 대기로 증가 없음).

## 관련: [[좌석동기화-전면재구성-와-보류-재사용]]
