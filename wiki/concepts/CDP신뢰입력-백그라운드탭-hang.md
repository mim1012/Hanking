# CDP 신뢰입력(dispatchMouseEvent/Touch)은 백그라운드 탭에서 hang
> CDP `Input.dispatchMouseEvent`/`dispatchTouchEvent`/`synthesizeScrollGesture`는 포그라운드가 아닌 Chrome 탭에서 응답까지 2분+ 걸리거나 무한 대기한다. 헤드리스·백그라운드 자동화의 클릭/스크롤은 in-page JS로 처리하라.

## 무엇 / 언제 쓰나
CDP로 Chrome을 몰아 순위체크·스크래핑·E2E를 돌릴 때, 요소 클릭/페이지네이션/스크롤을 어떻게 발생시킬지 고를 때. 두 방식이 있다:
- **in-page JS**: `el.click()`, `window.scrollTo()` 등 `Runtime.evaluate`로 실행. isTrusted=false(합성). 백그라운드에서도 즉시 반환.
- **CDP 신뢰입력**: `Input.dispatchMouseEvent`(mouseMoved→Pressed→Released), `Input.dispatchTouchEvent`, `Input.synthesizeScrollGesture`. isTrusted=true(진짜 입력). 단 **탭이 포그라운드가 아니면 블록/hang**.

## 정본 구현 (canonical)
- `sellermate_naver_place_all/rank-check/android-app/app/src/main/java/com/navertraffic/rankchecker/ChromeDevToolsSocket.kt:82` — `clickAt(x,y)` = CDP 신뢰 마우스 클릭(mouseMoved→mousePressed→mouseReleased). 이게 백그라운드에서 hang하는 주범.
- `sellermate_naver_place_all/rank-check/android-app/app/src/main/java/com/navertraffic/rankchecker/ChromeDevToolsBridge.kt:142` — `clickPage()`가 페이지네이션 버튼을 **in-page JS `buttons[i].click()`으로 눌러 즉시 반환**하는 정본 패턴(hang 회피). clickAt은 JS click이 실패했을 때만 쓰는 폴백으로 둔다.

## 다른 구현 / 변형
- 클릭이 isTrusted를 요구하는 핸들러(일부 SPA·안티봇)면 JS click이 무시될 수 있다. 그럴 땐 탭을 `Page.bringToFront`로 포그라운드로 올린 뒤 clickAt, 또는 짧은 타임아웃을 걸고 실패시 폴백.

## 주의점 / 함정
- **"죽은 코드처럼 보이는 clickAt 폴백"을 함부로 활성화하지 말 것.** JS click을 우선하고 clickAt을 폴백으로 둔 구조는 버그가 아니라 hang 회피용 의도적 설계다. sellermate rankchecker에서 clickPage를 clickAt 우선으로 바꾼 vc82가 백그라운드 hang→함대 stall(59분 미완 스캔, 30분 0적재)을 냈고 vc83으로 롤백했다.
- 실측(2026-09-14): PC Chrome 152 포그라운드 2분+/백그라운드 무한, 안드로이드 Chrome 138 clickPage hang.

## 배제된 접근 (dead-end)
- `Input.synthesizeScrollGesture`로 무한스크롤 유도 → 응답 2분+ 또는 무한 대기(백그라운드). 페이지네이션 트리거로 신뢰 불가.
- `window.scrollBy/scrollTo`(JS 스크롤) → hang은 없지만 사이트의 lazy 페이지네이션(예: 네이버 msearch)을 트리거하지 못하는 경우 있음.

## 관련: [[크롬-CDP-온디바이스-브리지]] · [[UA-ClientHints-정합-CDP모바일위장]]
- [[크롬-CDP-온디바이스-브리지]]: 같은 CDP Input 계열의 다른 실패(Android 크롬에서 dispatchTouchEvent no-op). 이 페이지(hang)와 함께 "CDP 합성입력은 온디바이스/백그라운드에서 신뢰 불가"라는 한 묶음.
