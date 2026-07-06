# project-wishcat

> 위시캣 프리랜서용 — LLM이 견적 초안만 생성하고 전송은 사람 승인 후 수동인 안전 우선 도구(92개 테스트). | 도메인: 견적 자동화 | 스택: Python3.12, SQLite(WAL/FK), Patchright 스텔스, Anthropic API | 상태: 활성

## 핵심 자산 (다른 곳에서 주워올 것)
- `src/wishket_autobid/auth/browser.py:1` — Patchright 스텔스 영속 프로필(구글 SSO 우회) (canonical). → [[브라우저-세션쿠키-영속화]]
- `src/wishket_autobid/review/state_machine.py:20` — 원자적 CAS 검토 상태머신(`begin_send()` CAS로 TOCTOU/이중제출 차단, events 감사). 재사용성 매우 높은 승인→전송 안전 게이트.
- `quoter/cost_guard.py`, `citation_check.py`, `tech_guard.py` — LLM 안전 가드 순수모듈(비용/인용/과장 방지).
- `db/schema.py`, `db/repositories.py` — 멱등 마이그레이션 DB 계층.

## 의존 개념: [[브라우저-세션쿠키-영속화]] · [[TLS-임퍼소네이션-WAF우회]]
