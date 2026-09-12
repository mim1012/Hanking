# _wiki — D:\Project 크로스프로젝트 지식 베이스

> Karpathy LLM Wiki 패턴 구현. "이 기능 어디서 주워와?"에 즉시 답하는 재사용 색인.
> Obsidian으로 이 폴더(`D:\Project\_wiki`)를 볼트로 열면 그래프 뷰 + `[[링크]]` 그대로 동작.
> 현재: 개념 73 · 프로젝트 44 (죽은 링크 0).

## 🗺️ 도메인별 지도 (여기서 시작)
- [[네이버-트래픽-커머스]] · [[트레이딩]] · [[앱-자동화-리버싱]] · [[웹앱-SaaS]] · [[소셜-수집]] · [[에이전트-인프라]]
- 전체 자동 목록(Dataview): [[MOC-개념]] · [[MOC-프로젝트]]

## 사용법 (4사이클)
- **Query** — "네이버 순위 어디 있어?" → [[네이버-순위-계산]] → canonical `파일:줄`
- **검증 완료 게이트** — 순위·자동화는 모바일/웹 모두 정본 history 적재 후 read-back 대조가 끝나야 완료로 판정 → [[골든플로우-실구동-게이트]]
- **Ingest** — 새 프로젝트 스캔: Claude에게 "`/ingest <프로젝트명>`" (스키마는 [AGENTS.md](AGENTS.md) — CLAUDE.md는 @포인터)
- **Compile** — 스캔 결과를 concept/project 페이지로 생성
- **Lint** — 죽은 링크·오래된 `파일:줄` 점검: `python -X utf8 scripts/lint.py` (2026-07-05 첫 실행: 87/87 유효)
- **Log** — 모든 사이클 실행 후 [log.md](log.md)에 한 줄 append (append-only 이력)

## 개념 (재사용 기법) — concepts/
### 트래픽/수집 자동화
- [[봇탐지-우회-스택]] — "다 바꿔봤는데 똑같이 막힌다"(CDP 부착=tell) → 네이버 쇼핑엔 안 통함, WebView는 X-Requested-With로 정체 노출
- [[사람행동-시뮬레이션]] — 압력 흔들기·균일분포 딜레이가 오히려 부자연스러움 → 기기 입력축 실측, 목표도달 폴링으로 조기중단
- [[IP-로테이션]] — 성공률 급락은 IP 평판 아니라 IP당 동시요청 밀도 문제 → 병사 분산으로 IP당 밀도 낮추기
- [[캡차-차단-대응]] — 영수증 캡차 풀어도 곧바로 다음 캡차(체이닝) → 최대 N라운드 재시도, 418은 URL 직행이 원인
- [[TLS-임퍼소네이션-WAF우회]] — Cloudflare/Akamai/DataDome WAF 차단 → curl_cffi로 JA3/JA4 지문 복제, 429는 백오프 재시도
- [[네이버-순위-계산]] — slot_sequence 조인 6% 불일치 → slot_status_id 기준 조인, 미발견 재시도가 신규슬롯 굶김 주의
- [[순위체크-대시보드-배포-게이트]] — "적재했다"만으론 미완 → history read-back·인증 401 확인까지 끝나야 완료
- [[네이버-공식API-서명]] — 검색광고 API HMAC 서명 포맷 삽질 → canonical HMAC-SHA256 구현 그대로 이식
- [[브라우저-세션쿠키-영속화]] — 구글SSO·네이버 로그인처럼 자동화가 막히는 인증 → 사람 1회 로그인 후 세션/프로필 재주입
- [[앱-자동업데이트-채널]] — INSTALL_FAILED_UPDATE_INCOMPATIBLE(서명 불일치) → 릴리스 keystore 고정+백업, OTA는 서비스루프 폴링
- [[유입수-집계되는-진입]] — webView.loadUrl 진입은 유입수 0으로 집계 → 실제 탭 제스처만 유입으로 잡힘
- [[CDP없는-OS레벨-브라우저자동화]] — 뭘 바꿔도 똑같이 차단(CDP 부착 자체가 tell) → OS레벨 UIA 실클릭으로 전환
- [[RDP없이-원격-headed-브라우저-관찰]] — RDP 세션 뺏기면 100% MID_NOT_FOUND → tscon 콘솔복귀 + SSH 스크린샷으로 관찰
- [[키워드-파이프라인-러너호환-MID검증]] — SERP positive인데 러너는 MID_NOT_FOUND → (slot,MID,query) 실기검증 후에만 active 승격
- [[검증리그-데이터형태-재현]] — 로컬 18/18 통과가 프로덕션 75% → 표본 수 대신 데이터 형태(빈값·경계값) 재현
- [[브라우저-셋틀-임계값-함정]] — 빈 페이지도 "안정"으로 오판 → 브라우저 UI 텍스트 길이 넘게 임계값 상향
- [[SSH-공개키인증-Win32버그-우회]] — 키 인식 후 "No more authentication methods to try" → 비밀번호+plink로 우회
- [[Windows예약작업-IT트리거-로그오프함정]] — RDP 로그오프 후 예약작업이 조용히 안 돔 → `/IT` 대신 `/ru`+`/rp`로 등록
- [[Supabase-무오프셋-타임스탬프-로컬시간오독]] — 온라인인데 전부 오프라인으로 뜸(9시간 밀림) → UTC `Z` 보존해 재직렬화
- [[Postgres-함수내-정규화비교-불일치]] — 에러 없이 매칭 행수만 조용히 줄어듦 → 조인마다 정규화 여부 개별 확인
- [[크롬창-오클루전-렌더차단]] — HTML은 오는데 화면만 "로드 중"에서 멈춤 → occlusion 비활성화+강제 최전면화
- [[소프트웨어버전-실물검증-API불신]] — 버전 API는 "최신"인데 실제 배포 채널은 구버전 → 설치파일 메타데이터로 직접 확정
- [[plpgsql-선읽기-최적화]] — 슬롯 늘수록 호출이 수백ms씩 느려짐 → WHERE절 서브쿼리를 DECLARE로 1회 선읽기
- [[실험-그래프-오케스트레이션]] — 텔레메트리 0행인데 "진행중"만 반복 → 시간기반 NO_TRAFFIC 판정을 별도로 둠
- [[전략인지-큐-노드-capability-게이트]] — strategy null 폴백이 DB 기본값 채워지는 순간 죽은 코드 → 노드측 명시 옵트인(allowlist)
- [[크롬프로필-DPAPI-로그인-이식불가]] — 프로필 복사하면 로그아웃 상태(DPAPI 키 불일치) → 그 PC 콘솔에서 직접 로그인
- [[러너-인스턴스락-PID재사용]] — 락 PID가 살아있어도 실은 재사용된 다른 프로세스 → 커맨드라인까지 대조해 유효성 판단
- [[UA-ClientHints-정합-CDP모바일위장]] — 모바일 UA + 데스크톱 userAgentData 불일치 = 봇 tell → Client Hints까지 정합

### 앱 자동화/리버싱
- [[Shizuku-입력주입]] — 접근성 gesture가 탐지돼 자동클릭 차단 → Shizuku 셸 권한으로 input tap 위장
- [[디컴파일-ViewId-자동화]] — jadx 힙 OOM으로 디컴파일 실패 → 여유메모리 확보 후 재시도, ViewId를 파싱 상수로
- [[State-Machine-접근성서비스]] — OTA(pm install -r) 후 접근성 바인딩이 풀림 → 바인딩 확인·복구를 작업시작 게이트로 묶기
- [[크롬-CDP-온디바이스-브리지]] — CDP Input(dispatchTouchEvent)이 Android 크롬에서 no-op → 실기기 su input tap으로 클릭
- [[행정동-기반-지역필터]] — 반경/좌표 필터 UI가 실제 매칭과 분리된 죽은 기능 → 행정구역 사전 Set-containment로 매칭
- [[카카오T-예약콜-실시간감지]] — count API는 "맞춤" 개수만 반환(전체 아님) → 직접 폴링 대신 이미 파싱된 화면만 읽기
- [[R8-Gson-DTO-keep-함정]] — 릴리스에서만 전필드 null(예외 없음, try-catch도 안 탐) → DTO keep 규칙 등록 필수
- [[ARM64네이티브-동적후킹-Frida에뮬레이터]] — control-flow-flattening으로 정적분석 막힌 .so 로직 → x86_64 에뮬레이터+Frida 런타임 후킹
- [[Flutter-DartAOT-Blutter정적복원]] — jadx로 libapp.so는 리소스뿐(Dart 로직 안 보임) → Blutter로 의사-Dart 정적복원

### 트레이딩
- [[KIS-API-계좌포맷]] — 모의투자 플래그가 실주문을 못 막음(계좌번호로만 구분) → mutation gate로 별도 차단
- [[포트-어댑터-거래소추상화]] — 새 거래소 붙일 때마다 전략 재작성 → 헥사고날로 전략/브로커 분리해 스왑 가능하게
- [[리스크-관리-상태기계]] — 킬스위치·포지션사이징을 매 프로젝트 재구현 → 상태기계+순수함수 지표로 백테스트/실거래 공용
- [[조회전용-분석수집기-주문격리]] — 조회화면이 run_once 재사용시 실제 취소주문이 나감 → narrow Protocol+래퍼로 주문경로 차단
- [[원자적-설정파일-쓰기-fail-closed]] — GUI 저장이 미편집 섹션을 통째로 날림 → 기존값 먼저 읽고 병합, 손상파일은 예외로 승격

### 데스크톱 GUI/테스트
- [[Tk-Canvas-도넛-트리맵-시각화]] — Tk create_arc 각도 방향이 반대로 돎 → `_to_tk_angles` 변환, 360도는 `create_oval` 분기
- [[Tk-테스트-안정화-및-창캡처]] — `master` 생략 시 간헐적 TclError → `master` 명시, 안 보이는 창은 PrintWindow로 캡처

### 웹앱/SaaS · 라이브 커머스
- [[웹앱-인증-세션-계층]] — 새 웹앱마다 인증 재구현 반복 → scrypt+HttpOnly+JWT 블랙리스트 세션 계층 재사용
- [[카카오-네이버-소셜로그인-AuthJS]] — 탈퇴·정지 회원이 소셜 재로그인으로 유효세션 재발급 → signIn 콜백에 status 게이트
- [[멀티테넌트-요청파이프라인]] — 테넌트 간 데이터 누출 위험 → 모든 쿼리에 organizationId 필터 강제
- [[정산-포인트-원장]] — 잔액·거래내역이 뒤섞여 정산이 안 맞음 → 이중부기식 원장(Settlement/PointTransaction)
- [[원자적-조건부-감소]] — 동시요청으로 오버셀·더블스펜드 → updateMany gte+count 검증으로 락 없이 원자 차감
- [[실시간-스트리밍-소켓아키텍처]] — 소켓 인증 정책이 fix마다 왕복 반전 → 진입점 1곳+불변식 문서화+3종 스모크
- [[반응형-오프캔버스-모바일]] — 폰에서 입력창 포커스시 화면이 확대되며 튐 → input font-size 16px, 사이드바 오프캔버스
- [[서버리스-공유-레이트리밋]] — 인스턴스별 인메모리 리미터는 서버리스에서 무용지물 → Upstash Redis 공유 카운터+폴백 로깅
- [[요청-코얼레싱-인플라이트]] — 동시요청마다 DB 쿼리가 중복 실행 → 동일키 요청을 Promise 하나로 합쳐 1회만
- [[Vercel-Production-비용-가드]] — 테스트 명령이 실수로 Production 과금 요청으로 둔갑 → 로컬 기본값, 원격은 항상 명시
- [[레거시-코드체계-호환-필터]] — exact match 필터가 구코드 데이터를 조용히 누락(환불사고) → variant IN-list 조회
- [[실시간-토큰리프레시-락-먼저-비교]] — 동시 리프레시로 대량 로그아웃 캐스케이드 → 락 먼저 잡고 비교, 결과는 코얼레싱
- [[PostgREST-1000행캡-회피]] — 1000행 넘으면 에러 없이 조용히 잘림 → DISTINCT ON RPC 또는 키셋(seek) 순회
- [[서버리스-응답후작업-after]] — 응답 후 fire-and-forget 작업이 조용히 유실(로컬엔 안 드러남) → `after()`로 완료 보장
- [[시각-회귀-베이스라인-게이트]] — 병합 중 디자인 표현이 조용히 유실 → CI 픽셀 diff로 드리프트 자동 차단
- [[목데이터-수명-단일정본]] — mock가 "복구용"으로 살아남아 정본이 세 곳으로 분산 → DB 시드 순간 mock는 죽은 것
- [[관리자-즉시저장-패턴]] — "삭제 안 됨"·"수정 유실" 사용자 리포트 → 클릭 즉시 DB반영 + 동시요청 busy 가드
- [[골든플로우-실구동-게이트]] — 문자열assertion 테스트 209개 통과했는데 실버그 배포 → 실브라우저 클릭까지 하는 프리뷰 게이트
- [[Supabase-마이그레이션-코드배포-분리]] — "SQL 실행했다"인데 응답이 그대로 → git push 코드배포 여부를 별도로 확인
- [[동시-클레임-랜덤화-떼몰림-방지]] — 여러 워커가 매 라운드 같은 후보 1개로 몰림 → 상위 K개 잠금 후 무작위 1개 선택
- [[Supabase-RPC-대량prepare-timeout-완화]] — 대량 pending 후보 prepare RPC가 statement timeout으로 중단 → seed 인덱스·필터·소batch로 완화

### 에이전트 인프라
- [[윈도우서버-git-자동배포]] — SFTP 수동배포가 서버마다 드리프트 → push하면 5분 내 reset --hard로 반영
- [[멀티에이전트-역할오케스트레이션]] — 병렬 포크가 같은 파일 수정시 충돌 → worktree로 격리 + 자가승인 금지
- [[파일기반-지속메모리]] — recall 제외창 단위 불일치로 긴 대화 중 한 턴이 조용히 소실 → 제외창을 verbatim 창에 맞춤
- [[AI-구독-사용량-조회]] — 계정 재로그인시 이전 계정 refresh_token이 무효화 → CODEX_HOME을 계정별로 완전분리
- [[하네스-플러그인-흡수-vs-설치]] — 플러그인 설치해도 Windows에선 훅이 조용히 무효(fail-open) → 설치 대신 규율만 SSOT로 흡수
- [[CLAUDE.md-로드아웃-패턴]] — 손으로 붙이면 마커 없어 멱등교체·배타검사 무력화 → store.py 경유 마커기반 append
- [[git훅-PATH잘림-클린PATH커밋]] — `'"node"' is not recognized` (cmd 자식이 PATH 못 찾음) → PATH를 클린 세트로 교체
- [[워크트리-격리-PR-staging-검증]] — junction node_modules로 next build가 즉시 실패 → 워크트리에 `npm ci` 실설치

## 프로젝트 — projects/
### 네이버/커머스 트래픽·순위
- [[project-adpang-production]] · [[project-electrone_navershopping]] · [[project-sellermate_naver_place_all]]
- [[project-turafic]] · [[project-naver-traffic-android-samsung]] · [[project-naver_turafic_samsung]] · [[project-turafic_update]]
### 커머스/수집/소셜
- [[project-dorami]] · [[project-band-sourcing]] · [[project-insta]] · [[project-kmong]]
### 트레이딩
- [[project-SOLX]] · [[project-StockDante]] · [[project-mt5]] · [[project-PJT-v4.7-KIS자동매매]] · [[project-Crypto]] · [[project-PJT-V5.0]]
### 앱 자동화/리버싱
- [[project-Vortex]] · [[project-UberAutoAccept]] · [[project-guardian-decompile]] · [[project-Media_enhanced-v1.0]] · [[project-turbo]] · [[project-touchassist]]
### 웹앱/SaaS
- [[project-massage]] · [[project-medical-ERP]] · [[project-wishcat]] · [[project-BAGJO1]] · [[project-tableflow]] · [[project-delivery-platform]]
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
