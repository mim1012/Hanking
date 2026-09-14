# DOM 스캔 "원시 앵커 수" 기준 조기종료 → 깊은 순위 NOT_FOUND 반복
> lazy-load 리스트를 스크롤하며 DOM으로 순위를 읽을 때, 완료 판정을 `querySelectorAll(...).length`(광고·중복 앵커 포함 원시 수)로 하면 실제 유기 상품이 절반도 안 로드된 시점에 멈춘다. 완료 판정은 **필터 후 유기 상품 수 + 바닥 도달**로 하고, 스캔 스레드에는 세대 취소·시간예산·부분결과 보존·계측을 같이 넣는다.

## 무엇 / 언제 쓰나
- 증상: 특정 순위대(예: 190~500위) 슬롯이 매일 100회+ 스캔되는데 전부 NOT_FOUND, history가 며칠 전에 동결. "네이버 차단"으로 오진하기 쉽다.
- 감별: 클린 IP 실기기에서 프로덕션 매칭 규칙 그대로 하네스를 돌려 찾히면 앱 결함(이 페이지), 안 찾히면 사이트/IP.
- 적용: Chrome CDP/WebView로 msearch 같은 lazy-load 리스트를 페이지네이션+스크롤로 읽는 모든 러너.

## 정본 구현 (canonical) — sellermate_naver_place_all vc85(0.4.68), 2026-09-15
- `rank-check/android-app/app/src/main/java/com/navertraffic/rankchecker/ChromeCdpRanker.kt:600` — DOM 스캔 JS가 inventory 필터 통과 + `chnl_prod_no` + `organic_expose_order` 보유 앵커만 `organicCount++` (needles 비교보다 앞).
- `ChromeCdpRanker.kt:608` — `atBottom=(scrollY+innerHeight)>=(scrollHeight-80)` 바닥 판정을 JSON으로 반환.
- `ChromeCdpRanker.kt:459` 부근 — 스크롤 루프 종료 우선순위: organic≥40 → 바닥&&무증가3(`:95`) → 무증가10 → 스크롤 24회 → 페이지 예산 25s → 세대 만료.
- `ChromeCdpRanker.kt:82-83` — 스캔 총예산 330s(재시도 간 공유) / DOM 예산 240s. `:109 abortAll()` 세대 취소로 타임아웃 후 고아 스레드가 다음 그룹의 Chrome을 건드리지 못하게.
- `ChromeCdpRanker.kt:352 partialForAbortReason` — 예산/전환실패/빈페이지로 못 훑은 슬롯만 `partial`(짧은 백오프), 결과 끝(`no_button`)은 정상 완주.
- `ChromeCdpRanker.kt:200,216` + `ChromeDevToolsBridge.kt:79 resolveTargetIdExact` — 그룹 시작 시 `closeAllTabs` 후 키워드 정확 매칭 탭 ID를 한 번 고정(generic "아무 msearch 탭" 폴백 금지).
- `ChromeCdpRanker.kt:41 ScanDiagnostics` → `rankchecker_job_runs.metadata`(source/api_reason/pages_reached/per_page[{p,switch_ms,scrolls,anchors,organic,found,bottom}]/elapsed_ms/abort_reason/tab_url/app_version_code). 함대 로그 없이도 원격 판별 가능.
- `TelemetryApi.kt:49,75` — startJob(POST)/finishJob(PATCH) 경합 latch.
- 재현 하네스 `rank-check/test/spike-trusted-paging.mjs`(`--needles <cpno>`로 프로덕션 매칭 규칙 재현), 관찰 `rank-check/test/watch-deeprank-rollout.mjs`, 카나리 큐 투입 `rank-check/test/requeue-slots.mjs --assign <worker>`.

## 다른 구현 / 변형
- vc84 이전(`git show e24b8d6:…/ChromeCdpRanker.kt` runDomFallback) — `anchorCount>=40` break + 24회 고정 attempt + 240s 그룹 타임아웃이 부분결과 폐기. 비교용.

## 주의점 / 함정
- **원시 앵커 인플레이션**: 같은 상품이 `lst*N`+`prd_ext` 두 앵커로 존재하고 광고(`lst*A/P/D`)·`blc_*` 블록까지 셀렉터에 잡혀 원시 115 vs 유기 21(실측). "40개 도달"이 스크롤 1~2회 만에 참이 된다.
- **부분결과 폐기 타임아웃**: 그룹 타임아웃 콜백이 전부 NOT_FOUND로 finish하면 이미 찾은 FOUND까지 사라진다. 타임아웃은 안전망으로만 두고 내부 예산이 먼저 반환하게.
- **탭 고정 경합**: 인텐트마다 새 탭이 열리고 이전 탭이 남는 Chrome에서 `waitForUrl("msearch")`는 남은 탭 때문에 즉시 참. 그 직후 URL 매칭으로 타깃을 고르면 이전 키워드 탭을 스캔한다(카나리에서 실제 발생: 밀키트 스캔이 무선충전기 page7 탭을 읽음). 그룹 시작 시 탭 전부 닫기 + 정확 매칭 대기.
- **org.json null → "null" 문자열**: `optString`은 JSON null에 `"null"`을 돌려준다. 상호명이 `"null"`로 780/780건 저장됐었다. `isNull(key)` 가드 필수.
- **Android 8+ ANDROID_ID는 앱별**: `adb shell settings get secure android_id`와 앱이 보는 값이 다르다. 워커 ID는 `rankchecker_devices`에서 읽는다.
- **재설치 직후 이중 기동**: MY_PACKAGE_REPLACED + am start로 같은 그룹이 두 번 시작될 수 있다 — 세대 취소가 하나를 폐기해야 한다.

## 배제된 접근 (dead-end)
- **신뢰 입력으로 페이지네이션(CDP Input / `input tap` / focus+ENTER)** — 9/14 "JS click은 isTrusted 아니라 page2+ 안 뜸" 가설은 LTE 실기기 실측으로 기각(JS `button.click()`으로 page2~7 정상). CDP Input은 백그라운드 탭 hang([[CDP신뢰입력-백그라운드탭-hang]]). 문제는 클릭이 아니라 스크롤 완료 판정이었다.
- **ncaptcha 토큰 캡처·재사용** — single-use라 418 지속([[project-sellermate_naver_place_all]] 9/14 메모).
- **WebView 토큰 폴백(vc81)** — found 12%, 그룹당 수 분 낭비. 삭제.

## 관련: [[CDP신뢰입력-백그라운드탭-hang]] · [[크롬-CDP-온디바이스-브리지]] · [[순위체크-대시보드-배포-게이트]] · [[project-sellermate_naver_place_all]]
