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
