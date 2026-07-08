# project-kmong

> 본인 **크몽(Kmong) 셀러 계정 운영 자동화 툴킷** — 자기 포트폴리오/서비스(gig)를 kmong.com에 등록·수정·검증하고, 상세페이지 이미지를 대량 생성하며, 나라장터(G2B) IT용역 입찰업체 등록을 준비. (크몽 스크래퍼도, 클론 마켓도, 납품물도 아님 — 자기 계정 운영 자동화.) | 도메인: 브라우저 자동화(셀러 운영) | 스택: Python3.12, patchright(봇탐지 회피 Playwright 포크)+표준 playwright(CDP attach), Pillow. DB 없음(산출물은 `.omc/**` JSON/스크린샷). git 없음 | 상태: 동작하는 활성 프로토타입(2026-06-08 실행·검증 기록). 폴더 `D:\Project\kmong`.

> 8700개 파일 대부분은 Chrome CDP 프로필 덤프(세션 아티팩트) — 실소스는 `src/`·`scripts/`(34개)·`procurement/`.

## 핵심 자산 (다른 곳에서 주워올 것)
- **patchright + 영속 Chrome 프로필 실행** — `scripts/kmong_probe_with_profile.py:23` `launch_persistent_context(user_data_dir=…)` — undetected Playwright 포크로 실제 로그인된 Chrome 유저프로필을 그대로 구동, 봇방어 사이트를 쿠키 재입력 없이 조작. → [[봇탐지-우회-스택]] · [[브라우저-세션쿠키-영속화]]
- **실행 중인 Chrome에 CDP attach** — `scripts/kmong_finalize_proof_service_cdp.py:108` `connect_over_cdp(CDP_URL)`(127.0.0.1:9222) — 사람이 이미 로그인해 둔 실 Chrome 탭(`my-gigs/edit/779390`)에 붙어 그 세션으로 자동화. 2FA/캡차 우회 실전 방식(`KMONG_CDP_URL` env 파라미터화). → [[브라우저-세션쿠키-영속화]]
- **React controlled input 강제 주입** — `scripts/kmong_finalize_proof_service_cdp.py:56` — `.fill()`이 안 먹는 React 제어 input을 native value setter(`Object.getOwnPropertyDescriptor(...).set.call(el, v)`) + `input`/`change` 이벤트 dispatch로 뚫음. 프런트 폼 자동화의 반복 함정 해법. → [[사람행동-시뮬레이션]](인접)
- **전체 페이지/프레임 DOM 스냅샷 추출기** — `scripts/cdp_capture_current_page.py:23` `frame_snapshot` — 텍스트·링크·이미지·폼컨트롤·OG메타를 한 번에 뽑아 summary/frames/visible_text로 저장(iframe 포함 최대텍스트 프레임 자동선택 `:135`). 임의 사이트 구조 파악용.
- **Pillow 다중 규격 캔버스 피팅** — `src/kmong_auto/assets.py:15` `build_main_variants` — 소스 1장 → 5개 규격(652x488/1200x900/900x900 등) 여백 배경 위 중앙정렬 리사이즈(`@dataclass(frozen=True, slots=True) ImageVariant`). 썸네일/상세페이지 대량 생성.
- **JSON 템플릿 기반 체크리스트/누락검사 리포터** — `scripts/g2b_prepare_it_vendor_pack.py:25` — JSON 템플릿을 `@dataclass Check(label, ready, note)`로 매핑 → 준비완료 카운트·마크다운 체크박스 리포트. "구조화 입력 → 누락필드 게이트 리포트" 로직은 도메인 무관 재사용.

## 주의점 / 함정
- CDP 프로필 덤프(`.omc/kmong/chrome-cdp-profile/**`, `C/Users/.../Chrome/User Data/**`)가 트리에 커밋되면 안 되는 실세션 아티팩트로 다수 존재.
- `tests/`는 설정만 있고 실제 테스트 파일 없음.

## 의존 개념: [[봇탐지-우회-스택]] · [[브라우저-세션쿠키-영속화]] · [[사람행동-시뮬레이션]]
