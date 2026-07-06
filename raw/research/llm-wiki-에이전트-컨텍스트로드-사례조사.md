# LLM 위키·옵시디언 → 에이전트 컨텍스트 자동 로드 사례조사

> 조사일: 2026-07-05 (insane-search: WebSearch + HN Algolia + X syndication + Reddit old.reddit/engine)
> 선행 조사: [[llm-wiki-x-사례조사]]. 이 문서는 "에이전트가 위키를 자동으로 읽게 만드는 방법"에 집중.

## 1. 원조

- **Karpathy X 포스트** (2026-04-02, 좋아요 6만+): "LLM Knowledge Bases … a large fraction of my recent token throughput is going less into manipulating code, and more into manipulating knowledge." 연구 위키 ~100 아티클/40만 단어를 RAG 없이 운영. https://x.com/karpathy/status/2039805659525644595
- **gist** (`karpathy/442a6bf555914893e9891c11519de94f`, 며칠 만에 5천 스타): raw(불변)/wiki(LLM 소유)/schema(CLAUDE.md·AGENTS.md) 3계층 + `index.md`(카탈로그) + `log.md`(append-only 이력) + Ingest/Query/Lint 사이클. "at small scale the index file is enough" — 소규모는 인덱스 우선 읽기로 충분, 커지면 qmd 하이브리드 검색.

## 2. 컨텍스트 자동 로드 메커니즘 — 실전에서 관찰된 4계층

| 계층 | 메커니즘 | 출처 사례 |
|---|---|---|
| L1 정적 | CLAUDE.md/AGENTS.md/GEMINI.md 자동 로드(룰북) | 전 사례 공통. "CLAUDE.md는 정적 진입점일 뿐, 볼트는 동적 살아있는 이력" (Reddit clsh 스레드) |
| L2 라이프사이클 훅 | SessionStart: QMD 재색인 + 북극성 목표·활성 프로젝트·최근 git 변경·오픈 태스크·전체 파일 목록 주입(~2K tok). UserPromptSubmit: 메시지 분류→라우팅 힌트(~100 tok). PostToolUse: .md 쓰기 후 frontmatter/위키링크/폴더 검증. PreCompact: 트랜스크립트 백업. Stop: 인덱스 갱신·고아 노트 점검 | obsidian-mind(github.com/breferrari/obsidian-mind — Claude Code/Codex/Gemini 3종 배선), Reddit "285줄 운영 매뉴얼 + 5훅" (r/ClaudeAI 1sc65st) |
| L3 명령형 핸드오프 | `/resume`(실행계획+최근 핸드오프 노트 읽기) ↔ `/wrap-up`(계획·부서 파일 갱신 + 핸드오프 노트 생성) = 세션 간 메모리. 의존 그래프 기반 병렬 에이전트(워크트리별)와 결합 | Reddit clsh 사례(r/ClaudeAI 1rv5ox0): 주말 34세션·43핸드오프로 모노레포+npm 3패키지+마케팅 솔로 출시 |
| L4 검색 | index 우선 → 커지면 BM25(bleve)+SQLite. "bm25-first call over vector dbs" 지지 다수 | wuphf(HN 260pt), Karpathy gist(qmd) |

## 3. 핵심 사례

- **obsidian-mind** — 코딩 에이전트 영속 메모리 볼트. 절차(훅)/내용(에이전트 소유 지식) 분리. Claude는 `~/.claude` 메모리 로더, Codex는 `.codex/hooks.json`, Gemini는 `.gemini/settings.json`으로 3종 동시 배선. 계층적 토큰 로딩(always ~2K / on-demand 검색 / triggered ~100).
- **wuphf** (nex-crm, HN 260pt) — 팀·멀티에이전트 공유 위키. 에이전트별 notebook(비공유) → 승격 심사 → wiki 아티클(git 커밋, `archivist` 아이덴티티) → typed triplet/entity fact log. `/lookup`(인용 답변)·`/lint`(모순·고아·낡은 주장 적발). **"Obsidian은 단일 사용자 편집기 — '에이전트A 초안, B 승격, 팀 승인' 상태기계가 에디터 밖에 있어야"** (제작자 HN 답변).
- **Reddit 285줄 매뉴얼 사례** — "볼트 구조가 프롬프트보다 중요. 스키마가 일관되면 Claude 합성이 경이롭고, 아니면 어떤 프롬프트도 소용없다." 링크 철학: "a note without links is a bug".
- **제품화 물결**: OpenKnowledge(inkeep, HN 381pt — Claude/Codex 직결 마크다운 에디터), Wikiwise·Tolaria(맥앱), Memoriki, Hermes Agent 번들 llm-wiki 스킬, VoiceTree(HN — 에이전트용 마크다운 그래프).
- **Claude↔Codex 이식성** (jiradett.medium.com): "instructions/memory/knowledge graph가 마크다운이면 특정 AI에 잠기지 않는다" — Codex는 AGENTS.md 수동 워크플로, Claude는 훅 자동화.

## 4. 우리 _wiki 대비 시사점 (액션 후보)

1. ✅ 이미 함: 3계층·index·`파일:줄` 백링크·글로벌 룰 배선(reuse-wiki.md, Codex AGENTS.md).
2. **SessionStart 훅으로 README 인덱스 자동 주입** — 룰(L1)보다 강한 L2. obsidian-mind 방식.
3. **`log.md` append-only 이력** 도입 (Karpathy gist 표준인데 우리 위키엔 없음).
4. **Lint 사이클 정례화**: 모순·고아 페이지·낡은 `파일:줄` 점검 (massage 폴더 삭제 시 필수).
5. **/resume·/wrap-up 핸드오프 노트** — 프로젝트 세션 연속성 (수주 프로젝트에 특히 유효).
6. 위키가 커지면 **BM25 검색**(qmd/bleve) — 벡터DB 불필요가 커뮤니티 중론.
7. 스웜 병렬 작업 시 **notebook→wiki 승격 플로우**(wuphf) — 에이전트 직쓰기 대신 검토 후 승격.
