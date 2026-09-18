# sec-ch-ua GREASE 불일치 → 네이버 쇼핑검색탭 로그인게이트 (WebView만 막힘)

> 봇 WebView(또는 CDP)의 **sec-ch-ua GREASE 브랜드가 실제 엔진 major 와 불일치**하면(옛 GREASE 하드코딩이 엔진 자동업뎃에 낡음), 네이버 쇼핑'검색'탭(`*.shopping.naver.com/search/all`)이 게스트를 `nidlogin`으로 리다이렉트한다. UA·userAgentData 가 겉보기 정합이어도 **GREASE 토큰 하나가 tell**. IP·로그인·기기모델과 무관.

## 무엇 / 언제 쓰나
- Android WebView 봇(또는 CDP)으로 네이버 **쇼핑검색탭**(`msearch.shopping.naver.com/search/all` 모바일 / `search.shopping.naver.com/search/all` PC)에 진입할 때. 통검(m.search)·쇼핑'홈'(m.shopping)은 통과하는데 **쇼핑검색탭만** 막히면 이걸 의심.
- 증상: **같은 IP·같은 게스트인데 시스템 크롬은 쇼핑탭 도달, 봇 WebView 만 `nid.naver.com/nidlogin.login?url=…shopping…` 리다이렉트.** 홈은 되므로 "IP 차단"으로 오진하기 쉬움.

## 정본 구현 (canonical)
- `naver_turafic_samsung/android/SamsungTrafficBot/app/src/main/java/com/navertraffic/samsung/strategy/SamsungWebViewManager.kt` — `greasedBrandVersion(major)` + `secChUaHeader(major,secondBrand,secondMajor)` (companion, GREASE 상수 자리 ~:1645): Chromium `GetGreasedUserAgentBrandVersion` 역산. `applyUserAgentMetadata()`(~:1095)가 이 값으로 `setUserAgentMetadata` brands 를 만든다. **엔진 major 에서 자동 유도**(하드코딩 금지).
- `naver_turafic_samsung/.../strategy/BrowserPersona.kt:63` CHROME persona(`secChUa = secChUaHeader(151,…)`), `:140` `buildHonestEnginePersonaSpec`(honest engine 도 같은 헬퍼) — **header(loadUrl)와 metadata(setUserAgentMetadata)를 같은 알고리즘으로 생성**해 둘이 절대 안 어긋나게 한다.
- `sellermate_naver_place_all/rank-check/android/extract-rank.ts:5` — 순위체크기가 쇼핑탭을 **되게** 하는 이유: 봇 WebView 가 아니라 **시스템 크롬(`am start com.android.chrome`)을 CDP로** 몬다(진짜 크롬 GREASE=정합). `:240` `enterShoppingTab`(통검→앵커클릭 리퍼러 유지→msearch), `:127` 앵커 `a[href*="msearch.shopping.naver.com/search/all"]`, chnl_prod_no 매칭.

## GREASE 알고리즘 (재사용 — 실측 검증)
Chromium 은 GREASE 브랜드/버전/순서를 **엔진 major 에서 결정적으로** 만든다:
- `chars=[" ","(",":","-",".","/",")",";","=","?","_"]`(11), `versions=["8","99","24"]`(3), `orders`=[0,1,2]..[2,1,0] 6순열
- brand=`"Not"+chars[m%11]+"A"+chars[(m+1)%11]+"Brand"`, version=`versions[m%3]`, order=`orders[m%6]` (base=[GREASE, "Chromium", secondBrand])
- **실기기 실측 정합**: 크롬138→`"Not)A;Brand";v="8"` 순서[0,1,2] / 크롬151→`"Not=A?Brand";v="99"` 순서[0,2,1]=[GREASE,Google Chrome,Chromium]. z90-1(SM-G930S) WebView 엔진 138→151 자동업뎃으로 낡아 게이트됐던 것.

## 검증된 성공
- 2026-09-18 z90-1 실측: GREASE 를 major 유도로 고친 뒤 봇 WebView 가 `navigator.userAgentData.brands=[Not=A?Brand v99, Google Chrome 151, Chromium 151]`(진짜 크롬151과 동일)을 보냄 → **통검→msearch.shopping/search/all(쇼핑탭)→m.smartstore/…/products(상세) 완주, DB result=success·mid_found=true·entry_method=a11y·게이트0.** 상품 미존재 시엔 `mid_product_not_found_after_shopping_tab_exhausted`(정상 미스, 순위체크기 out_of_range 동형).

## 진단법 (CDP 크롬 vs WebView 대조)
- 봇 WebView: `adb forward tcp:P localabstract:webview_devtools_remote_$(adb shell pidof <pkg>)`.
- 시스템 크롬: 봇 force-stop(포커스 뺏김 방지)→`am start -n com.android.chrome/com.google.android.apps.chrome.Main -d <url>`→소켓 `chrome_devtools_remote` forward. Node22 내장 WebSocket 으로 `Network.requestWillBeSentExtraInfo`(document)에서 **와이어 sec-ch-ua 헤더** 캡처.
- **역방향 오버라이드 A/B로 tell 격리**: `Network.setUserAgentOverride`로 크롬에 봇 헤더를 씌워 게이트되는지 / 봇에 크롬 헤더 씌워 통과하는지. model-only·brand-order·GREASE 를 하나씩 바꿔 원인 좁힘(model 무관·GREASE 결정).

## 주의점 / 함정
- **"IP 플래그" 오진** (이름붙인 실패패턴): 쇼핑'홈'은 되고 '검색탭'만 막히는 걸 IP 차단으로 착각. 실제론 헤더 지문(GREASE). 함대 다른 IP·시스템 크롬이 되는지로 반증.
- **하드코딩 GREASE 는 엔진 자동업뎃에 낡는다**: WebView 엔진은 Play스토어로 조용히 올라가는데 sec-ch-ua 상수는 안 바뀜 → major 에서 유도해야 자동 정합. UA/userAgentData 는 major 만 봐서 겉보기 정합이어도 GREASE 세부(브랜드문자열·버전·순서)가 어긋남.
- **header ≠ metadata 자체가 tell**: loadUrl 헤더맵의 sec-ch-ua 와 `setUserAgentMetadata` 가 다르면(예전: header v8 / metadata v99) getHighEntropyValues 교차검증에 걸림 → 한 알고리즘 소스로 통일.
- 쇼핑'검색'탭은 **직접 goto=즉시 캡차/게이트**. 통검 SERP 에서 앵커 클릭(리퍼러 유지)로만 진입([[유입수-집계되는-진입]] 리얼탭과 결합).

## 배제된 접근 (dead-end)
- **IP 회전 / 로그인 / 기기모델 변경 = 무관** — 전부 시도·격리로 배제. model 을 가짜(SM-S911B)로 둔 채 GREASE 만 맞춰도 통과(model-only 테스트 A 통과), 반대로 진짜 model 이어도 낡은 GREASE 면 게이트(테스트 B).
- **"모바일 쇼핑탭은 무조건 비로그인" 과신** — *상세페이지*는 비로그인 맞지만([[유입수-집계되는-진입]] 참조), 쇼핑'검색탭'은 지문 불량 시 로그인 게이트. 상세 공개 ≠ 검색탭 무관문.

## 관련: [[UA-ClientHints-정합-CDP모바일위장]] [[봇탐지-우회-스택]] [[유입수-집계되는-진입]] [[크롬-CDP-온디바이스-브리지]] [[캡차-차단-대응]] [[네이버-순위-계산]]
