# project-guardian-decompile

> Flutter 발신자표시/스팸차단 앱 GUARDIAN v1.1.2 APK 보안·프라이버시 정적분석. | 도메인: 앱 리버싱 | 스택: jadx, Flutter AOT(libapp.so), OkHttp | 상태: 완료 (참조)

## 핵심 자산 (다른 곳에서 주워올 것)
- `ANALYSIS_REPORT.md:17-166` — APK 분석 리포트 템플릿(DB/권한/exported/보안/API).
- `ANALYSIS_REPORT.md:12` — Flutter 앱 리버싱 방법론(libapp.so AOT, XOR 난독화 base URL). → [[디컴파일-ViewId-자동화]]
- `ANALYSIS_REPORT.md:133` — 안티탬퍼 탐지(libpss.so + so 해시 검증).
- `ANALYSIS_REPORT.md:131` — OkHttp CertificatePinner 인증서 피닝 분석. → [[TLS-임퍼소네이션-WAF우회]]

## 의존 개념: [[디컴파일-ViewId-자동화]] · [[TLS-임퍼소네이션-WAF우회]]
