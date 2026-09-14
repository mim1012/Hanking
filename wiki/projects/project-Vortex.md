# project-Vortex

> KakaoT Driver 콜을 접근성 API로 자동 수락하는 Android 서비스(상태머신 + Hilt). | 도메인: 앱 자동화/리버싱 | 스택: Kotlin, Hilt, Shizuku, Coroutines/StateFlow, Supabase | 상태: 활성 (자산 밀도 최고)

## 핵심 자산 (다른 곳에서 주워올 것)
- `docs/INPUT_INJECTION_DETECTION.md` — 입력주입 vs 봇탐지 분석 (canonical). → [[Shizuku-입력주입]]
- `ShizukuConnectionManager.kt:246` — Shizuku 셸 탭. 3단계 클릭 폴백.
- `docs/VIEW_ID_REFERENCE.md`, `docs/ViewIdlist.md` — 디컴파일 view ID 레퍼런스. → [[디컴파일-ViewId-자동화]]
- `fasttree/` — 별도 프로세스 UI 트리 조회(UIAutomation + AIDL).
- `monitoring/AccessibilityDeathTracker.kt` — 접근성 서비스 death 추적.
- `.github/workflows/release.yml` — APK → Supabase Storage → GitHub Release 자동화.

## 루팅 브랜치 — LSPosed/Frida 우회 서브시스템 (폴더 `D:\Project\vortex_rooting`, 브랜치 `feature/rooting-implementation`)
같은 Vortex 레포의 별도 브랜치 체크아웃. `app/`(접근성 서비스)는 Vortex main과 동일(위 자산이 커버)하나, **`lsposed/` Xposed 후킹 + `frida/` 디컴파일-매핑 워크플로우는 main에 없는 별도 서브시스템**이라 여기 노트로 기록(별도 페이지 아님). [[project-turbo]](카카오T LSPosed)와 같은 계열.
- **case-flip 난독화 클래스 탐색** — `vortex_rooting/lsposed/src/main/java/com/vortex/kakaobypass/MainHook.kt:75` `findClassFlex()` — jadx/ProGuard 대소문자 불일치(`A7.d→a7.d`) 대응해 첫 글자 case flip 재시도. [[project-turbo]]의 `findClassFlex`와 동일 패턴(형제 계보).
- **WebSocket 생명주기 후킹** — `vortex_rooting/lsposed/src/main/java/com/vortex/kakaobypass/MainHook.kt:426` `hookAllMethods(wsClass, "disconnect", …)` — 2045줄 단일 후킹 레지스트리(토큰 캡처, throttleFirst-5s 제거, Play Integrity 캡처, SSL 언핀, 예약 자동 리프레시 13개 훅).
- **LSPosed 모듈 스켈레톤** — `vortex_rooting/ROOTING_IMPLEMENTATION.md:52` + `lsposed/src/main/assets/xposed_init` — 엔트리 `com.vortex.kakaobypass.MainHook`, `handleLoadPackage` 패키지필터 + `param.result=true`로 KakaoT 5초 리프레시 리미터 우회. 재사용 Xposed 모듈 템플릿.
- **함정: OkHttp `compileOnly` → 런타임 `NoClassDefFoundError`** — `vortex_rooting/lsposed/src/main/java/com/vortex/kakaobypass/MainHook.kt:1859` — `java.net.HttpURLConnection` 폴백 필요(타입은 컴파일되나 번들 안 됨). [[project-turbo]]와 동일 함정.

## accept_flow 텔레메트리 & 관측성 (2026-07-12, 2.7.8 Phase 0)
Supabase `twinme_accept_flow_events`(jsonb `details`)에 수락 단계별 이벤트를 남기는 원격 계측 파이프라인. 7일 실측(24대)으로 성능 병목을 **추측이 아닌 데이터**로 규명한 사례.

- **실측 결론(재사용 인사이트)** — 수락 매크로의 병목은 파싱/스레드가 아니라 **확정(step3) 단계 경쟁 패배**였다. step3 종단결과: `already_assigned`(이미 배차) 1,229 / `cancelled` 1,041 / 성공(`confirmed_reservation_text_detected`) 21. 타임아웃은 7일 2건뿐 → "느려서 타임아웃" 가설 기각. 클릭 자체는 shizuku_tap p50 56ms로 빠르고, 측정된 지연은 step2 수락버튼 대기(p90 1.6s)와 dispatchGesture 경로의 `Thread.sleep(300)`(전체 13.6%)뿐.
- **함정: 이벤트 로그 게이트 필드 오분류** — `RemoteLogger.kt:1335` `AcceptFlowGuards.shouldLog`가 종단여부를 `stage=="final"`로 판정했으나, 실제 종단 이벤트는 `stage="step3_confirm"` + `outcome="final"`로 방출됨. 그 결과 성공/실패 종단 로그가 샘플링·세션예산·**dedup에 삭제**(선행 `dispatch_success`가 `monitor:stage:callKey` 공유 dedup 키를 오염 → 뒤따르는 성공 행 소멸)돼 성공이 저집계됐다. 수정: 게이트를 `outcome=="final"` 기준으로 재조정 → 종단 이벤트만 가드 우회·즉시 flush, 고빈도 `analyze`(2.7.7 flood 방지 대상)는 전 가드 유지. **교훈: 이벤트 로그 억제 게이트는 방출부의 실제 필드값과 반드시 대조 검증할 것.**
- **클라이언트 측정 지연 체인** — `created_at`은 45초 배치 삽입 시각이라 밀리초 지연 측정 불가. 해결: `SystemClock.elapsedRealtime()`(단조시계) 마크를 `details` jsonb에 실어 배치와 무관하게 측정. `AcceptLatencyTracker.kt:14` `details()`가 `call_appeared_to_confirm_ms` + split(appeared→click→accept→confirm) 생성. 앵커: `AnalyzingHandler.kt:345`(eligible 확정=appeared_at), `WaitingForConfirmHandler.kt:455`(종단 이벤트에 부착), 마크 필드 `StateContext.kt:104`, 리셋 `CallAcceptEngineImpl.kt:260/320/1056`.
- **함정: created_at으로 클라 지연 측정 금지** — 배치 인제스트(45s/40건, 2.7.7)를 쓰면 서버 삽입시각이 flush 창에 양자화된다. 지연은 반드시 클라 단조시계 필드로.
- **DB 조회 경로** — Supabase Management API `POST /v1/projects/{ref}/database/query`로 읽음(대시보드 토큰 필요). urllib 기본 UA는 Cloudflare 1010 차단 → `User-Agent: curl/8.4.0` 헤더 필요. 프로젝트 ref `czqnybgoaeihwvgdtvgn`, 리전 풀러엔 tenant 미존재(직결 대신 Management API 사용).

## hawng-driver 브랜치 — vortex3 전용 + 경쟁앱(touchassist) 벤치마킹 (2026-08-12)
브랜치 전체가 `com.example.vortex3`/"Vortex3"로 영구 리브랜딩(applicationId+앱이름, 일회성 suffix 아님). 16KB 페이지 정렬(Android 15+)은 Sentry SDK 7.3.0→8.47.0 + AGP 8.1.1→8.5.2 + Gradle 8.5→8.7 조합으로 해결(SDK만으론 불충분, `zipalign -c -P 16`으로 검증).
- **기능 갭 분석 기준선** — [[project-touchassist]](경쟁 예약콜 자동화 앱 디컴파일) 대비 Vortex 미보유: (1) [[행정동-기반-지역필터]] "맞춤콜"(인천공항 하드코딩 키워드뿐, 일반 지역필터 없음), (2) 맞춤콜 실시간 팝업 감지→자동거절 상태머신 자체가 없음, (3) `NotificationListenerService` 채널 없음(화면꺼짐/백그라운드 콜 선감지 불가). 우선순위: 지역필터 > 알림감청 > uiautomator dump 폴백 > OCR/반경필터(낮음).
- **비루팅 실시간감지 한계 재확인** — [[카카오T-예약콜-실시간감지]] — "요청 0" fresh 리딩(e1 리더)은 root/LSPosed 코드주입 전제라 Vortex(Shizuku=ADB shell 권한뿐)에서 재현 불가. 서버 자체가 ~3초 캐시라 화면 기반 폴링과 실질 격차는 크지 않음.

## 의존 개념: [[Shizuku-입력주입]] · [[디컴파일-ViewId-자동화]] · [[State-Machine-접근성서비스]] · [[행정동-기반-지역필터]] · [[카카오T-예약콜-실시간감지]] · [[보안화면-검정-scrcpy-FLAG_SECURE우회]]

## 2·3단계 수락 안정화 (2026-09-14)
- `Vortex/app/src/main/java/com/example/twinme/domain/state/AcceptButtonTiming.kt:3` — 버튼 안정화250ms·단계간300ms·재시도1200ms·최초포함3회 정책. TouchAssistant의 세대검증 재탐색과 Overrun의 이벤트 후 대기/현재 root 재시도 패턴 참고. 실제 경쟁앱 자료는 `Vortex/apk_extractor/overun-tuto`(overun 철자)에도 있다.
- `Vortex/app/src/main/java/com/example/twinme/domain/state/handlers/DetectedCallHandler.kt:702` — 입력 성공/프로세스 pending을 구분하고 함수 안 연타를 제거. 기존 코드 Android 재현에서 pending 결과에도3회 전송, 감지bounds를 무시하고540,2080 탭 확인. 수정은 실제 버튼bounds를 사용한다.
- `Vortex/app/src/androidTest/java/com/example/twinme/AcceptanceTimingUiTest.kt:46` / `:154` — 실기기 로컬 fixture에서 정상버튼 실탭, 무반응시 제한 재시도, 3단계 완료 판정 검증. 실제 카카오T 배차 성공률 검증은 아님. → [[State-Machine-접근성서비스]] · [[Shizuku-입력주입]]
- 맞춤콜 추가 검증: 수락은 공통 타이밍 적용, 별도 거절 경로는 실패 입력도500ms 간격/최대4회 예산에 포함하도록 수정. JVM120·Android 로컬19 통과. 근거 `Vortex/docs/ACCEPT_TIMING_STABILITY.md`.
