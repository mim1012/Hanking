# 한게임 클라이언트·런처 프로세스 조사

> 작성: 2026-07-31  
> 목적: 에이전트가 **특정 한게임 프로세스 실행 중일 때만** 화면 송출 + 대시보드 초록불  
> 범위: 조사 + **§3-1 구현 완료** (2026-07-31). Win VM tasklist 실측 교차검증은 후속.
>
> **구현 상태**: agent/server `1.4.2-lab-hangame31` — 상세 `docs/hangame-3-1-verify.md`

---

## 1. 요구 정리

| 항목 | 내용 |
|------|------|
| 트리거 | 한게임 **게임 클라이언트** 프로세스가 살아 있을 때 |
| UI | 대시보드 해당 좌석/에이전트 **초록불** |
| 미디어 | 그때만 **화면 송출** (그 외 idle / 송출 off) |
| 예 | 바둑이, 포커(7포커·라스·듀얼 등) |

**핵심 구분**

| 계층 | 역할 | 대시보드 초록불? | 화면 송출? |
|------|------|------------------|-----------|
| **한스타터(런처)** | 설치·패치·실행 중계 | ❌ (기본) | ❌ |
| **게임 클라이언트 .exe** | 실제 플레이 창 | ✅ | ✅ |
| 웹 로비만 (브라우저) | 미설치/미실행 | ❌ | ❌ |

런처만 떠 있는 PC방 좌석은 “한게임 유저”가 아님. **클라이언트 exe 매칭**이 정답.

---

## 2. 출처 (신뢰도)

| ID | 출처 | 내용 | 신뢰 |
|----|------|------|------|
| S1 | `outputs/WormGPT_한게임유저감지_분석보고서.md` (2026-07-23) | PC방 관리(게토) `WmTGN.ini` 의 `gprov=NHN` 프로세스 목록 | **높음** — 현장 감지 설정 추출 |
| S2 | 안랩 ASEC / 블로그 (Aduska 부트킷, 게임 계정 탈취) | `POKER7`, `LASPOKER`, `BADUKI`, `HIGHLOW2`, `DUALPOCKER` 등 | **높음** — 악성코드 타깃 목록으로 교차 확인 |
| S3 | 안랩/보안 블로그 wowtcex 계열 | `baduki, poker7, duelpoker, hoola3, laspoker, highlow2` 실행 시 캡처 | **중~높** |
| S4 | 한게임 CS `hanstarter.do` | **한스타터 / 한스타터2 / 한스타터 클리어** (포커·신맞Go 는 한스타터2) | **높음** — 공식 런처 계층 |
| S5 | 설치 가이드 블로그 | `HangameStarterSetup.exe` 설치 파일명 | **중** — 인스톨러명, 상주 프로세스명과 다를 수 있음 |

> 이 Kali/헤드리스 환경에서는 한게임 클라이언트를 직접 실행해 `tasklist` 실측하지 못함.  
> **2026-08-04 Win VM 실측 완료** → §3.1에 `hangamepoker.exe` 확정 반영, allowlist 교차검증 done.

---

## 3. 프로세스 카탈로그 (감지 후보)

### 3.1 우선 타깃 (요청 예: 바둑이·포커)

| 표시명 | process (정규화) | 비고 | 출처 |
|--------|------------------|------|------|
| 한게임 포커 (통합 클라이언트) | `hangamepoker.exe` | **2026-08-04 Win VM 실측** — 실제 설치물은 `C:\Hangame\HangamePoker\HangamePoker.exe`. 런처(한스타터2)의 자식이며 GameGuard(`GameMon.des`/`GameMon64.des`)를 자식으로 가짐. 단일 통합 클라이언트 | **실측** |
| 한게임 바둑이 | `Baduki.exe` / `baduki.exe` | 대소문자 혼용 관측 → **case-insensitive** | S1, S2, S3 |
| 한게임 바둑이 (x64 항목) | `baduki.exe` | WmTGN TG#121 별도 행, 동일 stem | S1 |
| 한게임 7포커 | `poker7.exe` | | S1, S2, S3 |
| 한게임 라스베가스 포커 | `laspoker.exe` | | S1, S2, S3 |
| 한게임 듀얼포커 | `DuelPoker.exe` | 악성코드 표기는 `DUALPOCKER` 변형 있음 | S1, S2, S3 |
| 한게임 하이로우 | `highlow2.exe` | 포커 계열 인접 | S1, S2, S3 |

### 3.2 고스톱·맞고 계열 (확장)

| 표시명 | process | 출처 |
|--------|---------|------|
| 한게임 맞고 | `ssduelgo.exe` | S1 |
| 한게임 맞고 (다른 빌드) | `ssgostop.exe` | S1 |
| 한게임 신맞고 | `gmahjong.exe` | S1 |
| 한게임 고스톱 | `duelgo.exe` | S1 |
| 한게임 더블맞고 | `doublego.exe` | S1 |

### 3.3 기타 보드·카드 (확장)

| 표시명 | process | 출처 |
|--------|---------|------|
| 한게임 바둑 | `bzbaduk.exe` | S1 — **바둑이(`Baduki`)와 별개** |
| 한게임 장기 | `hanjanggi.exe` | S1 |
| 한게임 신윷놀이 | `SinYutnori.exe` | S1 |
| 한게임 섯다 | `gssudda.exe` | S1 |
| 한게임 훌라 | `HOOLA3.exe` | S1, S3 |
| 한게임 한강 | `hangang.exe` | S1 |
| 한게임 오목 | `omok.exe` | S1 |
| 한게임 카드놀이 | `SolCard.exe` | S1 |
| 한게임 해상스홀덤 | *(WmTGN에 proc 미지정)* | S1 — **실측 필요** |

### 3.4 런처·인스톨러 (초록불/송출 트리거로 쓰지 말 것)

| 이름 | 알려진 파일/제품명 | 용도 | 권장 |
|------|-------------------|------|------|
| 한스타터 | 제품명 HanStarter / 한스타터 | 일반 게임 설치·실행 (포커·신맞Go 제외) | allowlist **제외** 또는 `tier=launcher` |
| 한스타터2 | HanStarter2 | **포커·신맞Go** 전용 스타터 (공식 CS) | 同上 |
| 한스타터 클리어 | HanStarter Clear | ActiveX 잔여 정리 | 제외 |
| 설치 패키지 | `HangameStarterSetup.exe` | 최초 설치 | 제외 (일시) |

정확한 **상주 프로세스 이미지 이름**(`HanStarter.exe` 등)은 공식 페이지에 이미지명으로 고정 공개되지 않음 → **VM 설치 후 `Get-Process` 1회 확정** 필요.

### 3.5 채널링·타사 (NHN 표기이나 한게임 본판 아님 — 기본 제외)

WmTGN `gprov=NHN` 에 섞여 있던 것: `kritika_client.exe`, `archeage.exe`, `R2client.exe`, `trgame.exe`, `sungame.exe`, `Goonzu.exe`  
→ 한게임 보드/포커 대시보드 목적과 무관하면 **별 프로파일**로 분리.

---

## 4. 감지 설계 권장 (구현 가이드, 코드 아님)

### 4.1 매칭 규칙

```
normalize(name) = lower(basename(image_name))  # ".exe" 포함 비교 또는 stem 비교
match if normalize(p) in GAME_SET
```

- **Case-insensitive** 필수 (`Baduki.exe` vs `baduki.exe`)
- 경로 무시, **이미지 이름만**
- 선택: 창 제목(`MainWindowTitle`)에 `한게임` / 게임명 포함 시 secondary score (cap_type 아이디어 — S1)

### 4.2 상태 머신 (에이전트 → 서버)

| agent 필드 | 값 | 대시보드 |
|------------|-----|----------|
| `game_active` | bool | 초록불 on/off |
| `game_id` | `baduki` / `poker7` / … | 라벨 |
| `game_pid` | int | 디버그 |
| `game_since` | unix | 지속 시간 |
| `stream_enable` | = `game_active` | 송출 게이트 |

폴링: **3~5초** (WmSvr 30~60초보다 짧게 — UX용).  
프로세스 종료 후 **1~2 틱 debounce** 후 불 끄기 (재시작 flicker 방지).

### 4.3 프로파일 JSON 스케치

```json
{
  "profile": "hangame-board-poker-v1",
  "match": "image_name_ci",
  "games": [
    {"id": "baduki",   "title": "한게임 바둑이",       "images": ["baduki.exe"]},
    {"id": "poker7",   "title": "한게임 7포커",       "images": ["poker7.exe"]},
    {"id": "laspoker", "title": "한게임 라스베가스 포커", "images": ["laspoker.exe"]},
    {"id": "duelpoker","title": "한게임 듀얼포커",     "images": ["duelpoker.exe", "dualpocker.exe"]},
    {"id": "highlow2", "title": "한게임 하이로우",     "images": ["highlow2.exe"]},
    {"id": "bzbaduk",  "title": "한게임 바둑",         "images": ["bzbaduk.exe"]}
  ],
  "launchers_ignore": ["hangamestartersetup.exe"],
  "stream": {"mode": "on_game_active_only"},
  "dashboard": {"led": "green_on_game_active"}
}
```

다중 게임 동시 실행 시: **우선순위 리스트 첫 매치** 또는 `games[]` 전부 보고.

### 4.4 화면 송출 게이트

```
if game_active:
    start_or_keep_stream()    # 기존 캡처 파이프라인
else:
    stop_stream()             # 대역·노출 최소화
    led = gray/red
```

전체 화면 vs 게임 창 HWND 캡처:  
1차 = **전체 화면**(단순), 2차 = 매칭 PID의 top-level window (바둑이/포커 창만).

### 4.5 서버·대시보드

- agent heartbeat / 별도 `POST /agent/presence` 에 `game_*` 필드 추가
- 대시보드 카드:  
  - 회색: 에이전트 온라인, 게임 없음  
  - **초록: `game_active`** + 게임 타이틀 뱃지  
  - 송출 썸네일은 `game_active` 일 때만 subscribe

---

## 5. 기존 생태계와의 관계

| 시스템 | 방식 | 우리와의 차이 |
|--------|------|----------------|
| 게토 WmSvr + `WmTGN.ini` | 호스트에서 proc 스캔 → 로컬 DB | **동일 아이디어** (프로세스 allowlist). 우리는 C2 agent 텔레메트리로 올림 |
| 과거 악성코드 | 동일 exe 이름에 훅/캡처 | 이름 목록 교차 검증용으로만 사용 |
| 한스타터 | 실행 중계 | 트리거에서 제외 |

게토가 쓰던 이름이 현장 표준에 가깝다 → **S1 목록을 1차 allowlist로 채택**하는 것이 합리적.

---

## 6. 미확정·리스크

| 항목 | 상태 | 대응 |
|------|------|------|
| 한스타터 상주 이미지 정확한 .exe 명 | 미실측 | VM 설치 후 확정, ignore 리스트에 추가 |
| 해상스홀덤 proc | WmTGN 미기재 | 실측 전 감지 제외 |
| 클라이언트 리네임/업데이트 | 가능 | 프로파일 핫리로드; 창 제목 fallback |
| `DUALPOCKER` vs `DuelPoker` | 표기 흔들림 | 이미지 별칭 배열 |
| 바둑(`bzbaduk`) vs 바둑이(`baduki`) | 다른 게임 | 대시보드 라벨 분리 |
| 관리자 권한 없이 타 세션 프로세스 | PC방 표준계정 | 동일 세션  agnet면 EnumProcesses 로 충분 |
| 개인정보·도박 화면 | 운영 리스크 | 송출 범위를 랩/승인 환경으로 제한 |

---

## 7. VM 실측 체크리스트 (다음 조사 액션)

관리자 PowerShell (한게임 클라 실행 전/후):

```powershell
# 1) 실행 전 베이스라인
Get-Process | Select-Object Name, Id | Sort-Object Name

# 2) 바둑이/포커 실행 후
Get-Process | Where-Object {
  $_.Name -match 'baduk|poker|duel|laspoker|highlow|hoola|gostop|duelgo|hangame|starter|hanstarter'
} | Format-Table Name, Id, Path -AutoSize

# 3) 결과 클립 → 이 리포트 §3 에 "실측 확인" 마크
```

확인할 것:
1. 실제 `Name` 이 S1 과 일치하는가  
2. 런처 프로세스 정확한 이름  
3. 게임 종료 후 잔존 프로세스 있는지 (잔존 시 debounce/화이트리스트 조정)

---

## 8. 구현 우선순위 (조사 후 추천)

| 순위 | 작업 | 의존 |
|------|------|------|
| P0 | `profiles/hangame_games.json` allowlist (§3.1+§3.2) | 본 조사 |
| P0 | agent: 3s 프로세스 스캔 → heartbeat 에 `game_active/game_id` | Win agent |
| P0 | server/dashboard: 초록불 바인딩 | heartbeat 필드 |
| P1 | `stream_enable` 게이트 (게임 중만 송출) | 기존 캡처 파이프 |
| P1 | VM 실측으로 목록 freeze + 한스타터 ignore | VM |
| P2 | 창 HWND 캡처 / 다중 게임 | P0 안정 후 |

---

## 9. 결론

1. **초록불·송출 트리거 = 게임 클라이언트 exe**, 한스타터(런처) 아님.  
2. 바둑이·포커 1차 세트:  
   `baduki.exe`, `poker7.exe`, `laspoker.exe`, `duelpoker.exe` (+별칭 `dualpocker.exe`), `highlow2.exe`.  
3. 목록의 **가장 단단한 근거**는 게토 `WmTGN.ini` (S1) + 안랩 타깃 목록 (S2/S3) 교차.  
4. 한스타터/한스타터2 는 공식 런처 계층(S4) — 감지 ignore.  
5. **Win VM `Get-Process` 1회**로 이미지명 freeze 후 구현 착수하는 것이 안전.

다음 단계 옵션:  
(A) `hangame_games.json` + agent 스캔 스켈레톤 구현  
(B) VM 실측 스크립트만 먼저 (`scripts/hangame_proc_probe.ps1`)  
(C) 대시보드 LED 프로토콜 필드만 server 쪽 먼저


## 실측 절차 (Win VM) — v1.4.3

```powershell
# baseline
.\scripts\hangame_proc_probe.ps1

# start 바둑이/7포커 client, then re-run
.\scripts\hangame_proc_probe.ps1

# paste data/hangame_proc_probe_last.md under 실측 확인
```

Allowlist remains research-derived until probe confirms image names on a real install.
