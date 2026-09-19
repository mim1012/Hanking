# 안드로이드 상품 탭: title 검색(블로그 오탐) → href-mid 좌표 스캔 탭
> 네이버 SERP/쇼핑탭에서 봇이 상품 대신 m.blog.naver.com 블로그 카드를 탭해 상세 미착지("상세 착지 미확인" 반복) → 상품명 title 검색 대신 앵커 href의 상품ID(nv_mid/mid)로 실제 상품 앵커를 찾아 그 좌표를 실제 탭한다.

## 무엇 / 언제 쓰나
- 안드로이드에서 네이버 쇼핑/통검의 특정 상품(mid/nv_mid)을 실제 손가락 탭으로 열어야 할 때.
- 증상: 로그에 "상세 착지 미확인"·"상품 미노출" 반복, 실제 탭 URL이 `m.blog.naver.com/...`(상품 키워드를 촘촘히 반복하는 블로그 글).

## 정본 구현 (canonical) — in-app WebView + href-mid 좌표 스캔 (실기기 검증)
- `naver_turafic_samsung/android/SamsungTrafficBot/app/src/main/java/com/navertraffic/samsung/strategy/NaverPageScripts.kt:450` — `midScanJs`: 앵커 href의 mid(`nv_mid=`/`searchGate`/`data-shp-contents-id`/`/products/`…)로 상품 앵커를 점수화(title 매칭은 최후 폴백 score 20), `scrollIntoView`+`elementFromPoint` 히트테스트로 실제 탭 좌표(css x,y)+href 반환. 광고 앵커(`lst*A/P/D`) 제외.
- `naver_turafic_samsung/.../strategy/SamsungWebViewManager.kt:1004` midScanJs 호출 · `:708` parseMidScanHit · `:819` `gestures.tapCssPoint(freshHit)` — WebView가 자기 화면 위치·밀도를 알아 CSS→화면좌표 변환 후 시스템 입력(dispatchGesture)으로 탭.
- `naver_turafic_samsung/.../strategy/SamsungBrowserStrategyD.kt:144` `clickMidLinkOnShoppingTab` — 쇼핑탭 진입 후 이 좌표탭 사용. 검증: 실기기 3대(z90-1/2/3) 스마트스토어 상세 착지(2026-09-19).
- 절대 화면좌표 1점 탭 프리미티브: `BrowserAccessibilityService.kt:301` `tapScreenPoint`(dispatchGesture — 합성 MotionEvent가 아니라 실입력 파이프라인이라 HTTP 490/영수증 캡차 회피).

## 다른 구현 / 변형
- 탭 직전 재스캔으로 스테일 좌표 방지: midScanJs를 탭 순간 다시 돌려 신선 좌표 사용(스캔~탭 사이 지연로딩 이미지로 레이아웃 밀림 보정). 같은 원리 [[State-Machine-접근성서비스]](파싱 시점 좌표 재사용 금지).

## 주의점 / 함정
- **"title 검색 블로그 오탐"**: 상품 탭을 `BrowserAccessibilityService.kt:31` `clickByTitle`→`:119` `findBestNode`(상품명 단어 최다 노드)로 하면, 상품 키워드를 촘촘히 반복하는 **블로그 카드가 짧은 쇼핑카드보다 점수 높아** 선택→m.blog.naver.com 착지, 상세 0%. 접근성 트리는 href/도메인을 안 줘서 에디토리얼 필터(텍스트 길이·날짜 바이라인·"블로그" 조상 표지)로도 못 거른다(휴리스틱 누수). → 반드시 href-mid 좌표 스캔으로 전환.
- 좌표는 CSS 뷰포트 기준 → `tapCssPoint`가 WebView rect+density로 화면좌표 변환한다. 외부 브라우저엔 이 기준이 없다(아래 dead-end).

## 배제된 접근 (dead-end)
- **외부 시스템 브라우저(Chrome) + 접근성 title 검색**(CDP DOM 읽기 병행해도): 외부 Chrome엔 우리 WebView rect가 없어 midScanJs의 CSS 좌표를 화면좌표로 변환할 기준(콘텐츠영역 top offset=상태바+주소창, 스크롤 시 접힘/가변, devicePixelRatio)이 없다 → 좌표탭 불가 → 탭을 접근성 title 검색에 맡길 수밖에 없고 → 블로그 오탐. 탭 재사용(`Browser.EXTRA_APPLICATION_ID`)·CDP DOM 캐러셀 인지(`src=mid`)를 넣어도 "어느 노드를 탭하나"는 안 풀린다. 2026-09-19 실기기 확인 후 외부 Chrome 변종 폐기, in-app WebView로 통일.
- **CDP Input.dispatchTouchEvent 직접 클릭**: Android 크롬에서 no-op/hang + 합성입력=탐지 tell → 입력은 접근성 실제 제스처로. 상세 [[크롬-CDP-온디바이스-브리지]] · [[CDP신뢰입력-백그라운드탭-hang]].

## 관련: [[크롬-CDP-온디바이스-브리지]] · [[CDP없는-OS레벨-브라우저자동화]] · [[State-Machine-접근성서비스]] · [[유입수-집계되는-진입]] · [[DOM스캔-원시앵커수-조기종료-깊은순위-NOT_FOUND]] · [[앱-자동업데이트-채널]] · [[project-naver_turafic_samsung]]
