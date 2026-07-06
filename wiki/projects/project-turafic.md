# project-turafic

> 5개 전문 Agent가 협력해 네이버/쿠팡 순위를 자율 최적화 + 봇 APK 역공학. | 도메인: 네이버/쿠팡 순위 | 스택: TS, tRPC, React, Frida | 상태: 활성 (Rank_Updator가 파생)

## 핵심 자산 (다른 곳에서 주워올 것)
- `src/frida/hook_signature_functions.js:24` — 서명함수(x-wtm-graphql) 후킹 + 스택트레이스. → [[디컴파일-ViewId-자동화]]
- `src/frida/` — okhttp/retrofit/graphql/crypto/dto 후킹 세트.
- `CLAUDE.md:278-305` — 12개 트래픽 변수 + 유전 알고리즘 진화. → [[사람행동-시뮬레이션]]
- `CLAUDE.md:352-382` — HTTP 418/캡차 오진단 방지 규칙 (canonical). → [[캡차-차단-대응]]
- `.claude/skills/` — db-health/quick-rank/troubleshooter 스킬.

## 의존 개념: [[캡차-차단-대응]] · [[디컴파일-ViewId-자동화]] · [[멀티에이전트-역할오케스트레이션]] · [[네이버-순위-계산]]
