# project-sellermate_naver_place_all

> 단일 PC에서 4개 플랫폼(쇼핑/플레이스/쿠팡 순위 + 트래픽)을 순차 처리하는 모노레포. | 도메인: 네이버/쿠팡 순위·트래픽 | 스택: TS(tsx), Patchright, Supabase | 상태: 활성

## 핵심 자산 (다른 곳에서 주워올 것)
- `place-check/check-place-rank-core.ts:675` — 플레이스 순위 로직 (canonical). → [[네이버-순위-계산]]
- `place-check/utils/humanBehavior.ts` — 사람행동 시뮬 (canonical). → [[사람행동-시뮬레이션]]
- `rank-check/extractors/` — MID 추출 팩토리 (canonical). → [[네이버-순위-계산]]
- `rank-check/CLAUDE.md` — 쿠팡 Akamai 우회(로그인 세션 + 워밍업). → [[봇탐지-우회-스택]]
- `REPORT-프로그램-구동-로직.md:35-71` — PC 단위 원자적 슬롯 락 패턴.

## 의존 개념: [[네이버-순위-계산]] · [[사람행동-시뮬레이션]] · [[봇탐지-우회-스택]] · [[IP-로테이션]]
