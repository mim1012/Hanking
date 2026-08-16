# _wiki — D:\Project 크로스프로젝트 지식 베이스

> Karpathy LLM Wiki 패턴 구현. "이 기능 어디서 주워와?"에 즉시 답하는 재사용 색인.
> Obsidian으로 이 폴더(`D:\Project\_wiki`)를 볼트로 열면 그래프 뷰 + `[[링크]]` 그대로 동작.
> 현재: 개념 48 · 프로젝트 42 (죽은 링크 0).

## 🗺️ 도메인별 지도 (여기서 시작)
- [[네이버-트래픽-커머스]] · [[트레이딩]] · [[앱-자동화-리버싱]] · [[웹앱-SaaS]] · [[소셜-수집]] · [[에이전트-인프라]]
- 전체 자동 목록(Dataview): [[MOC-개념]] · [[MOC-프로젝트]]

## 사용법 (4사이클)
- **Query** — "네이버 순위 어디 있어?" → [[네이버-순위-계산]] → canonical `파일:줄`
- **Ingest** — 새 프로젝트 스캔: Claude에게 "`/ingest <프로젝트명>`" (스키마는 [AGENTS.md](AGENTS.md) — CLAUDE.md는 @포인터)
- **Compile** — 스캔 결과를 concept/project 페이지로 생성
- **Lint** — 죽은 링크·오래된 `파일:줄` 점검: `python -X utf8 scripts/lint.py` (2026-07-05 첫 실행: 87/87 유효)
- **Log** — 모든 사이클 실행 후 [log.md](log.md)에 한 줄 append (append-only 이력)

## 개념 (재사용 기법) — concepts/
### 트래픽/수집 자동화
- [[봇탐지-우회-스택]] · [[사람행동-시뮬레이션]] · [[IP-로테이션]] · [[캡차-차단-대응]] · [[TLS-임퍼소네이션-WAF우회]]
- [[네이버-순위-계산]] · [[네이버-공식API-서명]] · [[브라우저-세션쿠키-영속화]] · [[앱-자동업데이트-채널]] · [[유입수-집계되는-진입]] · [[검증리그-데이터형태-재현]] · [[브라우저-셋틀-임계값-함정]] · [[SSH-공개키인증-Win32버그-우회]]

### 앱 자동화/리버싱
- [[Shizuku-입력주입]] · [[디컴파일-ViewId-자동화]] · [[State-Machine-접근성서비스]] · [[크롬-CDP-온디바이스-브리지]] · [[행정동-기반-지역필터]] · [[카카오T-예약콜-실시간감지]]

### 트레이딩
- [[KIS-API-계좌포맷]] · [[포트-어댑터-거래소추상화]] · [[리스크-관리-상태기계]]

### 웹앱/SaaS · 라이브 커머스
- [[웹앱-인증-세션-계층]] · [[카카오-네이버-소셜로그인-AuthJS]] · [[멀티테넌트-요청파이프라인]] · [[정산-포인트-원장]]
- [[원자적-조건부-감소]] · [[실시간-스트리밍-소켓아키텍처]] · [[반응형-오프캔버스-모바일]]
- [[서버리스-공유-레이트리밋]] · [[요청-코얼레싱-인플라이트]] · [[레거시-코드체계-호환-필터]] · [[실시간-토큰리프레시-락-먼저-비교]] · [[PostgREST-1000행캡-회피]] · [[서버리스-응답후작업-after]] · [[시각-회귀-베이스라인-게이트]] · [[목데이터-수명-단일정본]] · [[관리자-즉시저장-패턴]] · [[골든플로우-실구동-게이트]] · [[Supabase-마이그레이션-코드배포-분리]]

### 에이전트 인프라
- [[윈도우서버-git-자동배포]] · [[멀티에이전트-역할오케스트레이션]] · [[파일기반-지속메모리]] · [[AI-구독-사용량-조회]] · [[하네스-플러그인-흡수-vs-설치]] · [[CLAUDE.md-로드아웃-패턴]]

## 프로젝트 — projects/
### 네이버/커머스 트래픽·순위
- [[project-adpang-production]] · [[project-electrone_navershopping]] · [[project-sellermate_naver_place_all]]
- [[project-turafic]] · [[project-naver-traffic-android-samsung]] · [[project-naver_turafic_samsung]] · [[project-turafic_update]]
### 커머스/수집/소셜
- [[project-dorami]] · [[project-band-sourcing]] · [[project-insta]] · [[project-kmong]]
### 트레이딩
- [[project-SOLX]] · [[project-StockDante]] · [[project-mt5]] · [[project-PJT-v4.7-KIS자동매매]] · [[project-Crypto]]
### 앱 자동화/리버싱
- [[project-Vortex]] · [[project-UberAutoAccept]] · [[project-guardian-decompile]] · [[project-Media_enhanced-v1.0]] · [[project-turbo]] · [[project-touchassist]]
### 웹앱/SaaS
- [[project-massage]] · [[project-medical-ERP]] · [[project-wishcat]] · [[project-BAGJO1]] · [[project-tableflow]]
### 에이전트 인프라
- [[project-warm-agent]] · [[project-hermes-agent]] · [[project-oh-my-codex]] · [[project-Decepticon]] · [[project-claude-cockpit]] · [[project-usage-widget]]
### 게임 유틸/로컬 OCR
- [[project-starcraft-dashboard]]

## 통합 후보 (중복 지식 → 라이브러리화)
1. **IP 로테이션** — electrone/sellermate/android-samsung/turafic/insta 5곳 → 단일 라이브러리
2. **봇탐지 우회/세션쿠키 영속화** — electrone/somae/band/insta/wishcat/sinsang → 공유 stealth+세션 모듈
3. **KIS 계좌+토큰** — SOLX/StockDante/AutoTradeX/open-trading-api/PJT-v4.7 5곳 → 공통 KIS SDK
4. **웹앱 인증/세션** — massage/dorami/medical-ERP/adpang 4곳 → `@shared/auth` 패키지
5. **정산/포인트 원장** — dorami/medical-ERP/adpang 3곳 → 공유 원장 도메인
6. **멀티테넌트 파이프라인** — medical-ERP/adpang → 공통 테넌트 미들웨어
7. **파일 기반 메모리** — warm-agent/hermes/.omc/claude-forge/Decepticon → 공통 메모리 계층

## 빈/소스없음 (스캔 확인, 자산 없음)
- `PJT v4.6` 2폴더(nul만) · `YKP ERP System` · `ykp-dashboard` · `soomgo` · `n8n project`(문서만) · `korean-law`(법률문서) · `1.BlogCrawlTransfer2`(컴파일 바이너리) · `BAGJO`(`.git`만, 소스 없음) · `gajae-code-global`(+wsl: 설치된 gajae-code CLI 툴 홈, 원본소스 없음)

> 최근 스캔에서 **등록 프로젝트의 클론/워크트리로 확인돼 스킵**: `adpang-landing-page`·`adpang-ui-design-sandbox`·`adpang-production-security-quality`(→adpang), `massage-deploy`(→massage), `dorami-pr97-api-burst`·`dorami-pr98-clean`(→dorami), `vortex_rooting`(→Vortex 루팅 브랜치, project-Vortex 노트로 흡수). `MediaEnhanced`는 `Media_enhanced-v1.0`의 얕은 복제.

## raw (원본 소스)
- `raw/` — 프로젝트 폴더는 `D:\Project\*` 절대경로로 참조(복제 안 함)
- `raw/research/` — 외부 조사 원본. [[llm-wiki-x-사례조사]] · [[llm-wiki-에이전트-컨텍스트로드-사례조사]]
