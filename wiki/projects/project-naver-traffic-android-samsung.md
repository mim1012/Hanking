# project-naver-traffic-android-samsung

> 실제 안드로이드폰이 삼성인터넷/WebView로 네이버 모바일 통합검색 전략을 수행하는 Kotlin 앱. | 도메인: 네이버 트래픽(실기기) | 스택: Kotlin, Coroutines, Supabase | 상태: 활성 (naver_turafic_samsung 확장판)

## 핵심 자산 (다른 곳에서 주워올 것)
- `AGENTS.md:149-156` — boss/soldier IP 로테이션 상태머신 (canonical 실기기). → [[IP-로테이션]]
- `AGENTS.md:107-126` — 전략별(G/H) 실검색 URL 파라미터 위장(`sm=`, `ackey` 토큰).
- `AGENTS.md:159-165` — ProtectionDetector(캡차/로그인/429 감지). → [[캡차-차단-대응]]
- `AGENTS.md:249-266` — 실패 원인 분류 플레이북(blacklist vs 429 vs 세션 vs DOM).
- `naver_turafic_samsung/CLAUDE.md:44-49` — BrowserSession 3중 추상화(WebView/Samsung/DryRun).

## 의존 개념: [[IP-로테이션]] · [[캡차-차단-대응]] · [[네이버-순위-계산]]
