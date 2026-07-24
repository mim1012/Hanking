# CLAUDE.md-로드아웃-패턴
> CLAUDE.md 구성을 조립 가능한 fragment(조각)로 모듈화하고, 결정적 설치기가 마커 기반 멱등 append + 코너(corner) 상호배타로 폴더 성격에 맞게 골라 담아 설치하는 패턴.

## 무엇 / 언제 쓰나
프로젝트 폴더마다 성격이 다른데(솔로 실행 vs 멀티에이전트, 비용 통제 유무, 세션 연속성 필요 여부) 전역 규칙 하나로는 모순 없이 담을 수 없을 때. 규칙을 코너별 fragment로 쪼개고, 설치기가 조합 유효성(같은 코너 배타)을 설치 시점에 강제한다. LLM이 구성 내용을 창작하지 않는다 — 번들된 조각을 byte 그대로 삽입하는 것이 핵심.

## 정본 구현 (canonical)
- `D:\.claude\plugins\project-loadout\skills\configure-store\generator\store.py` — 결정적 설치기 전체
  - `store.py:29-31` — `marker()`: `<!-- store:<name>:start/end -->` 마커 쌍 생성. 멱등성·설치 감지·교체의 단일 기반
  - `store.py:61-72` — `check_exclusions()`: 같은 corner = 상호배타. picks 내부 쌍 + picks↔기설치 모두 검사, 위반 시 exit 2로 파일 무변경 거부
  - `store.py:103-137` — `install_fragment()`: 마커 블록 교체-또는-append(멱등) + `files/` 트리 복사 + meta의 `claude_settings_hook` 병합
  - `store.py:78-100` — `merge_claude_settings_hook()`: 대상 `.claude/settings.json`의 `hooks.Stop`에 멱등 병합 — 사용자 훅 보존, 마커 문자열(`coach --hook`) 항목만 dedup 후 정본 교체
  - `store.py:154-191` — `install_multiagent()`: 멀티에이전트 품목만 multi-agent-starter `init.py`에 위임. init.py가 CLAUDE.md를 통째로 쓰므로 기존 조각 블록을 회수했다가 재-append + `.loadout-bak` 백업
- `D:\.claude\plugins\project-loadout\skills\configure-store\generator\fragments\` — 조각 8종(agent-loop·fable5-lowcost·fable5-solo·guard·karpathy·knot·multiagent·session-handoff). 각각 `meta.json`(corner 지정) + `fragment.md`(순수 마크다운, 벤더 중립) + 선택적 `files/`(예: session-handoff의 `SESSION.template.md`)
- `D:\.claude\plugins\project-loadout\skills\configure-store\SKILL.md:34-39` — Do NOT: 마커 없는 야생 설치 금지·코너 배타 우회 금지·installer 트리 안 설치 금지
- 코너 설계 표: `D:\Project\.omc\fragment-design.md:162-171` — 행동 규율(karpathy)·실행 구조(fable5-solo/lowcost/multiagent 상호배타)·자율성·지식 관리·비용 통제·세션 연속성

검증(2026-07-24 실측): 스크래치 폴더에 `--pick karpathy,session-handoff` 설치 exit 0(+딸린 파일 1개 복사), 동일 품목 재설치 = "교체"(마커 1개 유지 = 멱등), `--pick fable5-solo,multiagent` = exit 2 배타 거부(파일 무변경).

## 다른 구현 / 변형
- 원본: sunghopark-loadout(외부)의 store.py를 byte-exact 포팅 — 로직 재작성 없음, fragments 내용물만 이쪽 rules(`D:\.claude\rules\*`)로 재구성 (fragment 소스 매핑은 `D:\project\SESSION.md` 결정 기록 2026-07-09)
- 조건부 문단 변형: 조각 안 `[X도 설치된 경우] …` 문구는 코드가 아니라 에이전트가 파일 안에서 판단 — 조각 간 약결합을 코드 복잡도 없이 달성

## 주의점 / 함정
- **야생 설치** — 조각 내용을 손으로 CLAUDE.md에 붙여넣으면 마커가 없어 멱등 교체·배타 검사·기설치 감지가 전부 무력화된다. 반드시 store.py 경유 (SKILL.md Do NOT 1항)
- **re.sub 백슬래시 이스케이프 해석** — 조각 본문에 `\g`·`\\`가 있으면 문자열 치환 시 정규식 그룹 참조로 해석돼 깨진다. `store.py:111`은 치환자를 lambda로 넘겨 byte-exact 보장
- **위임 스캐폴더의 통째 덮어쓰기** — init.py류가 CLAUDE.md를 새로 쓰면 기존 store 조각이 소실된다. 회수-재부착 + `.loadout-bak` 백업으로 방어 (`store.py:162-191`). 비-store 본문은 백업만 되고 소실됨을 stdout으로 경고
- **이중 지침 소스 드리프트** — 멀티에이전트 템플릿에 카파시 4원칙이 내장돼 있어 karpathy 조각을 같이 담으면 같은 규칙이 두 곳에서 나온다. store.py는 이 경우 karpathy를 자동 생략(`store.py:237-239`). [[하네스-플러그인-흡수-vs-설치]]의 드리프트 함정과 같은 계열

## 배제된 접근 (dead-end)
- **store.py 재작성** — 배제(2026-07-09 결정 기록). 검증된 설치기를 다시 짜면 멱등·배타·백슬래시 처리 같은 엣지를 전부 재검증해야 한다. 재사용 원칙대로 원본 byte-exact 포팅, fragments만 교체
- **전역 CLAUDE.md 단일본 일괄 적용** — 배제. 실행 구조(솔로 vs 멀티에이전트)처럼 서로 모순인 지침을 한 파일에 담을 수 없고, 폴더 성격별 취사선택이 불가능 — 이 한계가 fragment+corner 분해의 출발점

## 관련: [[하네스-플러그인-흡수-vs-설치]] · [[멀티에이전트-역할오케스트레이션]] · [[파일기반-지속메모리]]
