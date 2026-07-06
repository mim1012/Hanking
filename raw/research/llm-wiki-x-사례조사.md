# [raw] LLM Wiki — X 활용 사례 조사 (2026-07-05)

> 외부 조사 원본. LLM은 읽기만 하고 이 파일을 근거로 wiki 페이지를 만든다. 원본 불변.

## 원조 개념 — Karpathy LLM Wiki
- 1,600만 뷰. "AI로 코드 짜지 말고 제2의 뇌를 만들어라."
- 3계층: raw(원본, 사람 소유) / wiki(LLM 생성 마크다운) / CLAUDE.md(스키마)
- 4사이클: Ingest / Compile / Query / Lint
- 철학: "가치는 노드가 아니라 엣지(링크)에 있다"
- 형식: 순수 Markdown (가장 압축적·LLM 가독·사람 감사 가능)

## X 주요 사례
- Allie K. Miller — Claudeopedia: Karpathy 문서 + /last30days + /wiki 스킬. https://x.com/alliekmiller/status/2040884878229565816
- Miles Deutscher — 인생 전체 굴리는 AI 세컨드 브레인. https://x.com/milesdeutscher/status/2041972675418189933
- Greg Isenberg — 옵시디언+Claude Code로 24/7 퍼스널 OS. https://x.com/gregisenberg/status/2026036464287412412
- Sanket — Obsidian CLI 스킬로 "dev brain": 의사결정/버그픽스/스니펫 기록 + Claude hooks 자동화. https://x.com/tinkerersanky/status/2027682782655226164
- Thomas Robb — Granola(회의록)→Obsidian→Claude Code 위키. https://x.com/BreakingSaaS/status/2040432646577086847
- yusaku — 3계층/4사이클 일본어 구현 가이드. https://x.com/yusaku_0426/status/2044198069923930286
- obssnnn — Karpathy 9규칙 요약(raw는 절대 편집 안 함, 3 owners). https://x.com/0xObssnnn/status/2070683754025177238

## 이 저장소에 적용한 결정
- raw = D:\Project 기존 130개+ 프로젝트 (복제 안 하고 절대경로 참조)
- wiki = 재사용 기법 concept 페이지 + 프로젝트 페이지 (canonical `파일:줄` 백링크)
- 목적: 사용자의 "재사용 우선 / 다른 프로젝트에서 주워와라" 원칙 자동화
