# project-guardian-decompile

> Flutter 발신자표시/스팸차단 앱 GUARDIAN v1.1.2 APK 보안·프라이버시 분석(정적+동적). | 도메인: 앱 리버싱 | 스택: jadx, Flutter AOT(libapp.so), OkHttp, Frida, Android emulator | 상태: 완료 (참조, 동적분석까지 확장됨 2026-08-03)

## 핵심 자산 (다른 곳에서 주워올 것)
- `ANALYSIS_REPORT.md:17-166` — APK 분석 리포트 템플릿(DB/권한/exported/보안/API).
- `ANALYSIS_REPORT.md:12` — Flutter 앱 리버싱 방법론(libapp.so AOT, 난독화 base URL). → [[디컴파일-ViewId-자동화]]
- `ANALYSIS_REPORT.md:131` — OkHttp CertificatePinner 인증서 피닝 분석. → [[TLS-임퍼소네이션-WAF우회]]
- `ANALYSIS_REPORT.md:169-231`(§8, 2026-08-03 추가) — **상용 앱실드 SDK "ProShield"(com.pss.security) 분석 패턴**: libapp.so를 AES/GCM+AndroidKeyStore로 런타임 복호화 로드, 삼성 Knox 게이트(비정품 단말 강제종료), 벤더 자체서버 heartbeat/verify/kill-report(android_id 포함) 텔레메트리. 다른 앱에서 유사 SDK(앱실드/DRM류) 만나면 이 패턴으로 먼저 식별.
- `ANALYSIS_REPORT.md:198-210`(§8.2) — **control-flow-flattening + 항상-거짓 predicate + 매직넘버 데코이** 조합의 네이티브 문자열 은닉 기법 실사례(radare2 없이 capstone 파이썬 스크립트로 ARM64 정적 분석한 절차 포함). 정적 분석으로 못 뚫는 지점과 그 판단 근거(다음은 Frida 동적분석) 기록.

- `ANALYSIS_REPORT.md:§9`(2026-08-03 추가) — **base URL 동적 확정 + 도메인 OSINT**: 정적분석 실패 후 x86_64 에뮬레이터+Frida로 `getBaseUrl()="https://api.gudservice.com"` 확보(실기기 netstat 관찰 IP와 교차검증). WHOIS(Privacy Protect+1984 ehf로 등록자 완전 익명화, 등록일 2026-03-12)와 crt.sh 인증서 이력(2021-10-22부터, 서브도메인 `admin.`/`partner-api.` 확인) 대조에서 **도메인 재등록(드롭캐치) 이력 불일치** 발견. → 절차 자체는 [[ARM64네이티브-동적후킹-Frida에뮬레이터]] concept로 분리 등재.
- `dynamic-analysis/hook_secureconfig.py`, `dynamic-analysis/frida-agent/` — 재사용 가능한 Frida 후킹 스크립트(canonical, 위 concept 참고).
- `ANALYSIS_REPORT.md:§10`(2026-08-03 추가) — **Blutter로 libapp.so(Dart AOT) 정적 복원**: 앱의 실제 클린아키텍처 구조 전체 확보, `ApiClient.post/get`의 `requiresAuth=true` 기본값+로그인 단일 예외를 코드로 확정(로그인 제외 전 엔드포인트 토큰 필수). 절차는 [[Flutter-DartAOT-Blutter정적복원]] concept로 분리 등재.

## 의존 개념: [[디컴파일-ViewId-자동화]] · [[TLS-임퍼소네이션-WAF우회]] · [[ARM64네이티브-동적후킹-Frida에뮬레이터]] · [[Flutter-DartAOT-Blutter정적복원]]
