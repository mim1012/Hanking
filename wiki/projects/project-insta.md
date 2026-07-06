# project-insta

> 다계정 인스타그램 포스팅 자동화 데스크톱 앱(콘텐츠·이미지 자동생성 + 예약 발행). | 도메인: 소셜 자동화 | 스택: Electron31, Playwright, sharp, OpenAI API | 상태: 활성

## 핵심 자산 (다른 곳에서 주워올 것)
- `src/main/poster.js:82,123` — 계정별 persistent context 세션 디렉토리 (canonical). → [[브라우저-세션쿠키-영속화]]
- `poster.js:87` `clickFirst(candidates)`, `:102` `anyVisible` — 회복력 있는 다중 셀렉터 클릭/가시성 판정.
- `src/main/network.js:24` `resetMobileData` — ADB 모바일 데이터 IP 로테이션. → [[IP-로테이션]]
- `src/main/image.js:78` `processImage` — sharp 이미지 파이프라인(랜덤 리사이즈, 여백 extend, SVG 텍스트 오버레이).
- `src/main/store.js:67` `encryptSecret` — Electron safeStorage 자격증명 암호화.
- `src/main/gpt.js:46` — OpenAI 캡션/이미지 생성. `runner.js:37` — 큐/워커 + 랜덤 딜레이.

## 의존 개념: [[브라우저-세션쿠키-영속화]] · [[IP-로테이션]] · [[봇탐지-우회-스택]]
