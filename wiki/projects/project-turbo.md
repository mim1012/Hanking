# project-turbo

> vercel/turbo도 turborepo 템플릿도 아님 — 제품 고유명 "Turbo". **카카오T 기사용 앱(`com.kakao.taxi.driver`)에 주입하는 LSPosed/Xposed 후킹 모듈 + 컴패니언 앱**으로, 네트워크/API 레벨에서 예약콜을 자동 수락(봇탐지 우회). | 도메인: 앱 자동화/리버싱(그레이햇 모바일). | 스택: Kotlin(LSPosed XposedBridge, `HttpURLConnection`) + 컴패니언 앱(Hilt, Shizuku, OkHttp, NDK/CMake C++ 무결성) + Express/Supabase 서버(형제 `Media_enhanced-v1.0` server-only 브랜치, Railway). 2모듈 Gradle 모노레포(`:lsposed`+`:app`) | 상태: 활성/실제품(README 2026-03-31, 타깃앱 v2.27→v2.30 추적). origin `github.com/mim1012/turbo`(legacy origin=Vortex). 67k 파일 대부분 벤더/디컴파일(Frida 툴체인, jadx 덤프) — 원본 소스는 93개 `.kt`.

## 핵심 자산 (다른 곳에서 주워올 것)
- **서버 주도 원격 config 3계층 폴백** — `lsposed/src/main/java/com/turbo/ConfigManager.kt:42` — `fetchFromServer(deviceId, kakaoVer) ?: loadCache() ?: decodeEmbedded()`(서버→디스크캐시→내장 Base64). 난독화 클래스명/API URL/키워드를 APK에 하드코딩하지 않고 서버에서 로딩 → 타깃앱 업데이트 시 `PUT /api/v1/config`만, APK 재빌드 불필요. concept 후보 `원격-동적설정-폴백체인`.
- **텔레메트리 배치 + 지수 백오프 재시도** — `ConfigManager.kt:291` `Telemetry` object — 인메모리 큐(max 50, drop-oldest), 데몬 flush 스레드, 실패 배치 prepend 후 60→120→240s 백오프. 원격 로깅 프리미티브.
- **행동 기반 런타임 클래스 로케이터** — `lsposed/src/main/java/com/turbo/AcceptUseCaseLocator.kt:11` — 난독화 메서드를 **이름이 아니라 관측된 부작용**(10초 창 내 `/v2/calls/{id}/accept` OkHttp 요청 매칭 `:75`)으로 찾음 → 버전별 R8 리네임에도 생존. → [[디컴파일-ViewId-자동화]](런타임 행동 매칭 변형)
- **에포크 기반 single-flight 동시성 가드** — `lsposed/src/main/java/com/turbo/SingleAcceptCoordinator.kt:101` — 세션 에포크당 accept 1건만 in-flight, 성공은 종단, 검증 실패 시 클레임 해제. 성공을 API 반환값이 아니라 카운트 delta(`after > before`)로 검증. 재사용 single-flight/idempotency 프리미티브.
- **앱 요청 서명 crypto(PBKDF2 + AES/CBC)** — `app/src/main/java/com/example/twinme/api/StampGenerator.kt:43` — 타깃앱 서명 "stamp" 재현: `PBKDF2WithHmacSHA1(md5(authToken), salt=md5("test"),1024,256)` → `AES/CBC/PKCS5`. → [[웹앱-인증-세션-계층]](요청 서명)
- **디바이스/전화 라이선스 인증(캐시+cert-pinned)** — `app/src/main/java/com/example/twinme/auth/AuthManager.kt:27` — 전화번호 인증, 12h prefs 캐시, OkHttp `CertificatePinner`, 엔드포인트를 네이티브 C++ vault(`NativeBridge.getString`)에서 해석. → [[웹앱-인증-세션-계층]] · [[project-Media_enhanced-v1.0]](같은 백엔드)

## 주의점 / 함정
- **OkHttp가 lsposed에서 `compileOnly`** — 런타임에 `okhttp3.*` 직접 쓰면 `NoClassDefFoundError`. HTTP는 반드시 `java.net.HttpURLConnection`.
- 난독화 클래스 시프트(V→U, X→W 등)는 경향일 뿐 보장 없음. `HookCandidates.kt`가 v2.27/2.29/2.30 fallback 보유. `findClassFlex`가 첫 글자 대소문자 flip(jadx macOS vs Android 런타임 차이).
- 서버(config/telemetry/release)는 [[project-Media_enhanced-v1.0]]와 공유 — 멀티테넌트 백엔드.

## 의존 개념: [[웹앱-인증-세션-계층]] · [[디컴파일-ViewId-자동화]] · [[State-Machine-접근성서비스]] · [[Shizuku-입력주입]]
