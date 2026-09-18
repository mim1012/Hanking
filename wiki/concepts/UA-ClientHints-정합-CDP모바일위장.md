# UA↔Client Hints 정합 (CDP 모바일 위장 봇회피 핵심축)

> CDP로 데스크톱 크롬을 모바일로 위장할 때, **UA 문자열과 navigator.userAgentData(Client Hints)가 내적으로 정합**해야 봇 탐지를 통과한다. 불일치(모바일 UA + 데스크톱 userAgentData)가 그 자체로 tell.

## 무엇 / 언제 쓰나
- 데스크톱 서버(VM)에서 Patchright/CDP로 진짜 크롬을 띄워 **모바일** 사이트(예: 네이버 m.search 통합검색)에 자동화 트래픽을 보낼 때.
- 증상: IP를 바꿔도(프록시) 안 풀리고, 세션을 초기화해도 안 풀리는 차단(로그인리다이렉트→429 붕괴). → **원인은 IP도 세션도 아니라 UA/CH 불일치 지문**일 수 있다.

## 정본 구현 (canonical)
- `turafic_update(rebuild-0814-cdp)/shared/mobile-stealth.ts:183` — `setupMobileCDP()`: `Emulation.setUserAgentOverride`에 `userAgentMetadata`(brands/platform:Android/mobile:true/model)를 함께 실어 **navigator.userAgentData를 모바일 값으로 실제 채운다**(UA와 정합).
- `turafic_update(rebuild-0814-cdp)/scripts/strategy-a-runner.ts:728` — 러너 matched 분기: 트라이얼마다 `context.newCDPSession(page)` → `setupMobileCDP`. static 분기(680/791)는 UA만 모바일로 바꾸고 userAgentData는 addInitScript로 덮으려다 실패(불일치 잔존).
- `turafic_update(rebuild-0814-cdp)/scripts/uad-probe.ts` — 검증 프로브: RAW vs MATCHED에서 navigator.userAgentData/getHighEntropyValues 덤프.

## 검증된 성공
- 2026-08-23 카나리 10대 실측: `--header-mode=matched`로 **429 ≈ 0 (30분+ 유지), 성공률 ~46%** (동일 노드 static은 0%성공/65% 429). uad-probe 실측: MATCHED에서 userAgentData `mobile:true / platform:Android` 확인.

## 주의점 / 함정
- **"모바일 UA + 데스크톱 userAgentData" 불일치 = 봇 tell** (이름붙인 실패패턴). UA 문자열만 모바일로 바꾸는 것으로는 부족 — Client Hints(userAgentData)까지 같이 모바일로 정합해야 함.
- CDP는 페이지(newCDPSession)마다, 네비게이션 **전에** setUserAgentOverride를 걸어야 적용됨.
- 잔존축 분리: 이 정합으로 지문차단(429)은 풀리지만 **콜드세션 로그인리다이렉트는 별개 축**(세션 웜업 필요). 정합=지문축 해결, 세션웜=로그인축.

## 배제된 접근 (dead-end)
- **"CDP는 userAgentData를 못 채운다"는 결론은 오판**(2026-08 이전). uad-probe로 반증 — CDP `userAgentMetadata`는 정상 작동. (addInitScript `Object.defineProperty(navigator,'userAgentData',…)` 주입은 Patchright가 무시해 inert였던 게 그 오판의 근거였음.)
- **noCDP(진짜 크롬)+WUI(UI Automation) 전환은 불필요**했음. noCDP가 통했던 이유도 결국 "진짜 크롬이라 UA/CH 자동 정합"이라는 같은 축 — CDP matched로 동일 효과를 얻으면서 CDP의 DOM 접근 편의를 유지.

## 관련: [[봇탐지-우회-스택]] [[사람행동-시뮬레이션]] [[유입수-집계되는-진입]] [[project-turafic]] [[CDP없는-OS레벨-브라우저자동화]] [[sec-ch-ua-grease-불일치-네이버쇼핑검색-로그인게이트]]
