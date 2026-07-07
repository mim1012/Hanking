# log — append-only 이력 (Karpathy gist 표준)

> 규칙: Ingest/Compile/Lint/배선 변경마다 아래에 한 줄 append. 수정·삭제 금지(append-only).
> 형식: `- YYYY-MM-DD | 사이클 | 요약`

- 2026-07-05 | Compile | 위키 최초 생성 — raw/wiki/schema 3계층, 개념 19 · 프로젝트 23, X 사례조사 인제스트
- 2026-07-05 | Compile | massage 자산 회수 — [[서버리스-공유-레이트리밋]] · [[요청-코얼레싱-인플라이트]] · [[레거시-코드체계-호환-필터]] 신설, [[project-massage]] 상태 종료(환불)로 갱신
- 2026-07-05 | Ingest | [[llm-wiki-에이전트-컨텍스트로드-사례조사]] — 에이전트 컨텍스트 자동 로드 4계층(L1 룰/L2 훅/L3 핸드오프/L4 검색) 사례 정리
- 2026-07-05 | 배선 | L1: `D:\.claude\rules\reuse-wiki.md`(전 세션 조회·등록 의무) + Codex `AGENTS.md` REUSE WIKI 블록
- 2026-07-05 | Compile | 도메인 지도 6개(`wiki/도메인/`) + 프로젝트 페이지 확장(23→31), 개념 24
- 2026-07-05 | 배선 | L2: SessionStart 훅(`D:\.claude\hooks\inject-wiki-index.py`) — README 인덱스 자동 주입, 새 세션에서 end-to-end 검증 완료
- 2026-07-05 | 배선 | 모바일 열람용 Remotely Save v0.5.25 플러그인 설치·활성화(`.obsidian/plugins/remotely-save/`) — OneDrive 인증·폰 연결은 사용자 수동 단계
- 2026-07-05 | Lint | 첫 린트(`scripts/lint.py` 신설) — 죽은링크 오탐 5(스키마 플레이스홀더, allowlist 처리), stale `파일:줄` 14건 발견→전부 실경로로 교정(dorami/backend 접두사, band-sourcing-1, Vortex 전체경로, Decepticon packages 경로). 재실행 결과: 페이지 65, 죽은링크 0, 고아 0, `파일:줄` 87/87 유효, README 카운트 일치
- 2026-07-06 | 배선 | 모바일 동기화를 Syncthing으로 확정(사용자 선택, 클라우드 미경유) — v2.1.1 `D:\Tools\syncthing` 설치, `wiki` 폴더 공유 등록, `.stignore`(workspace/캐시/scripts 제외), 부팅 자동시작 등록. Remotely Save 플러그인은 비활성화. 폰 페어링은 사용자 수동 단계
- 2026-07-06 | 배선 | 승격 게이트 3가지 채택(kulaxyz/self-learning-skills, 설치 없이 규칙만 흡수) — CLAUDE.md 규칙 7 신설(검증된 성공·이름 붙인 실패 패턴·배제된 dead-end), concept 템플릿에 「배제된 접근 (dead-end)」 섹션 추가. 소급 수정 없음, 신규·갱신 시 적용
- 2026-07-06 | Compile | [[project-PJT-v4.7-KIS자동매매]] 자산 2건 등재 — US 데일리 시그널 파이프라인 P1(`stock_config_store` 원자적 config 주입, `watchlist_ingest` EOD→watch_list 자동 주입). 승격 게이트 충족: 검증된 성공(pytest 61/61)·실패 패턴(핀 심볼 증발/desync)·dead-end(별도 메타파일 split-brain 배제)
- 2026-07-06 | 배선 | 스키마 SSOT 통일 — 스키마 전문을 `AGENTS.md`로 이관(코덱스 네이티브 로드), `CLAUDE.md`는 `@AGENTS.md` 포인터 한 줄로 교체(클로드 임포트). 글로벌 참조 2곳(`D:\.claude\rules\reuse-wiki.md`, `D:\.codex\AGENTS.md` REUSE WIKI 블록)도 AGENTS.md로 갱신 — 두 에이전트가 같은 파일 하나를 읽어 스키마 드리프트 구조적 차단
- 2026-07-06 | Lint | lint 보강(log.md 형식·날짜순 검사, 공백·괄호 경로 정규식) + KIS자동매매 페이지 어긋난 참조 2건 실파일 검증 후 교정. 결과: 90/90 유효
- 2026-07-06 | 배선 | 코덱스 위키 연동 새 세션 2건 end-to-end 검증 — 글로벌 규칙으로 README→concept→원본 줄번호 대조까지 자율 수행, `_wiki` 내에서 AGENTS.md 스키마(승격 게이트·log 규칙) 네이티브 로드 확인
- 2026-07-06 | Compile | MOC 2종 신설([[MOC-개념]]·[[MOC-프로젝트]], Dataview 자동 목록+최근 7일 뷰) + Dataview v0.5.68 설치·활성화. README에 링크 및 AGENTS.md 스키마 포인터 반영
- 2026-07-06 | 배선 | 수주 킥오프 절차 신설(`contract-project-gates.md` 0절: AGENTS.md 골든플로우 스캐폴딩→위키 등록→스펙 스텁) — 코덱스 글로벌엔 포인터만
- 2026-07-06 | Ingest | massage 정본 스냅샷 → `raw/snapshots/massage/` (50파일 134KB, 해시 대조). 원본 폴더 삭제 대비
- 2026-07-06 | 배선 | git 백업 시작(초기 커밋 3392d06) + 스키마 규칙 8(사이클 후 커밋) + `.git`은 Syncthing 제외
- 2026-07-06 | Compile | massage 원본 폴더 삭제 대비 — 위키 참조 전부 스냅샷 경로(`_wiki/raw/snapshots/massage/`)로 재지향, 참조 파일 2건(communityStore·shop-store) 스냅샷 추가. GitHub `mim1012/massage`에 전 이력 푸시 확인
- 2026-07-06 | Compile | dorami 채팅 두더지잡기 포렌식(git 3,597커밋 + 코드) — 실패 패턴 5종 명명해 [[project-dorami]] 교훈·[[실시간-스트리밍-소켓아키텍처]] 함정/dead-end 등재. dorami AGENTS.md에 Chat Regression Contract(정책 불변식·공유 4파일 동반 스모크·스펙 공백) 배선
- 2026-07-06 | Compile | fablize 플러그인 평가→흡수 결정 등재 — 소스 3층 분해(텍스트/훅/엔진) 후 investigation-protocol의 "경쟁 가설 3개+·기각 보고"만 `.claude/rules/fable5-operating-mode.md` §6에 흡수(미설치). kulaxyz에 이은 2번째 동일 패턴이라 [[하네스-플러그인-흡수-vs-설치]] concept 신설(개념 24→25). 함정: 이중 지침 드리프트·Windows fail-open 훅 무효과. dead-end: setup.sh global 주입
- 2026-07-07 | 배선 | 모바일·노트북 동기화를 Obsidian Sync(E2E 암호화, 기결제 구독)로 전환 — PC 꺼짐 상태에서도 동기화되는 점이 결정 요인. Syncthing은 자동시작 해제·프로세스 종료(설정 보존, 바로가기는 `D:\Tools\syncthing\Syncthing.lnk.autostart-disabled`로 이동)
- 2026-07-07 | Compile | fable-5-orchestration-guide(successwiki.io) 평가→강제 게이트만 흡수(설치) 등재 — 4계층 중 훅 강제 게이트만 Windows/PowerShell·model-role-division에 맞게 포팅해 실설치(`D:\.claude\hooks\orchestration-gate.py`+settings.json PreToolUse+`scripts\orchestration-gate.ps1` 토글), 11개 합성 페이로드로 실차단 검증. 나머지 3계층(sonnet→opus 리매핑·deep-reasoner/runner·fable.md 선언) 배제. [[하네스-플러그인-흡수-vs-설치]] concept 갱신 — 판단절차 훅층이 처음으로 "설치" 분기 발동한 사례, Windows fail-open 회피법 3단계 기록
- 2026-07-07 | 배선 | 위키 최신성 자동화 2종 — (1) 주간 lint 자동화: `_wiki/scripts/lint-scheduled.ps1`+Windows 작업 `wiki-lint-weekly`(월 10시)→`.lint-last.txt`. (2) 등록 리마인더 훅: `D:\.claude\hooks\wiki-touch-tracker.py`(PostToolUse) — D:\Project\<프로젝트> 소스 수정 시 세션당 1회 "위키 등록 확인" additionalContext, 7/7 발동/억제 검증. reuse-wiki "등록 후" 단계 강제
- 2026-07-07 | Compile | 최근 활성 3개 프로젝트 페이지 재검증·갱신(사용자 지정 스코프) — [[project-dorami]](streaming.service.ts:982→1044 드리프트 교정 + 교훈 "종이호랑이 스펙" 낡은 서사 갱신[8bf2413f로 하드assert 반영] + 리프레시락-코얼레싱·컨테이너힙OOM·골든플로우CI게이트 자산 3건), [[project-naver_turafic_samsung]](crypto.js:16→15 off-by-one 교정 + landing.js Toss시그니처·admin.js 순위귀속 자산 2건), [[project-PJT-v4.7-KIS자동매매]](기존 인용 무드리프트 + credential_store 마스킹·settings_store hot-patch 자산 2건 + 중첩 CLAUDE.md/FastAPI·Flutter·Dante 커버리지 메모). 병렬 ingest 에이전트 초안→검토·경로정규화 후 반영. lint 105/105 유효. 신규 concept 미승격(리프레시락은 향후 concept 편집 대상 플래그)
- 2026-07-07 | Compile | dorami·PJT-v4.7 딥 최신화(사용자 재요청) — [[project-dorami]] 드리프트0(25개 ref 재검증) + 신규 자산 3건(세션부트스트랩401복구 use-auth.ts:84·dev안전백도어 streaming.controller.ts:104·다축리듀서+스냅샷파생 useLiveLayoutMachine.ts:73). [[project-PJT-v4.7-KIS자동매매]] 드리프트0 + 언급만됐던 4개 서브시스템 딥인제스트(api_server 인증+응답봉투·발굴스캔 인플라이트코얼레싱·Dante 스크리너 파이프라인 screener/eod_scan/dante_candidates·TA LLM fail-open/fail-closed 필터+파일뮤텍스·Flutter ApiClient) 자산 6건, 테스트 8/8·4/4 PASS 직접확인. lint 116/116 유효. 승격게이트 충족 concept 후보 3건(refresh-lock-먼저-비교[dorami]·TA필터 provenance게이트[→리스크-관리-상태기계 변형]·Dante 배치발굴-실시간타이밍-분리)은 다음 Compile 대상으로 플래그(project memo 유지)
- 2026-07-07 | Compile | 승격게이트 충족 concept 2건 등재(사용자 승인) — 신규 [[실시간-토큰리프레시-락-먼저-비교]](canonical `dorami/backend/src/modules/auth/auth.service.ts:397` _doRefreshToken NX락-먼저; 클라 코얼레싱 `client.ts:56`; dead-end=compare-before-lock[dorami AGENTS.md Lesson#18]; 함정=리프레시 레이트리밋제외·쿠키maxAge=JWT수명·volatile-lru) + [[리스크-관리-상태기계]]에 "provenance 신호게이트" 변형 추가(`PJT.../common/ta_filter.py:141` 핀=fail-open/자동주입=fail-closed). dorami 역링크·README 개념25→26. lint 119/119 유효. 보류: Dante 배치발굴-실시간타이밍-분리(트레이딩 특화 → mt5/StockDante 유사확인 시 승격)
