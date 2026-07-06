# project-electrone_navershopping

> 외부 엔진 task JSON을 읽어 Patchright로 네이버 쇼핑 검색/클릭을 수행하는 트래픽 러너. | 도메인: 네이버 트래픽 | 스택: Node18/TS, Patchright, Electron28, @anthropic-ai/sdk | 상태: 활성 (핵심 엔진)

## 핵심 자산 (다른 곳에서 주워올 것)
- `shared/mobile-stealth.ts` — 모바일 스텔스/핑거프린트 (canonical). → [[봇탐지-우회-스택]]
- `ipRotation.ts` — ADB IP 로테이션 완성형 (canonical). → [[IP-로테이션]]
- `captcha/ReceiptCaptchaSolverPRB.ts` — 영수증 캡차 Claude Vision 솔버 (canonical). → [[캡차-차단-대응]]
- `rank-check-shopping.ts` — 쇼핑 순위 체크. → [[네이버-순위-계산]]
- `CLAUDE.md:51-59` — 5계층 봇탐지 명세. `:72-80` 서버 구동 자동 업데이트/롤백. `:113-119` esbuild `__name` 폴리필.

## 의존 개념: [[봇탐지-우회-스택]] · [[IP-로테이션]] · [[캡차-차단-대응]] · [[네이버-순위-계산]] · [[사람행동-시뮬레이션]]
