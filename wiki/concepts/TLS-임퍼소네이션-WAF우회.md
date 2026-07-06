# TLS 임퍼소네이션 / WAF 우회

> curl_cffi로 브라우저 JA3/JA4 지문을 복제해 HTTP 레벨에서 WAF(Cloudflare/Akamai/DataDome)를 우회.

## 무엇 / 언제 쓰나
브라우저 없이(경량) 차단된 사이트 본문·API를 가져올 때. 브라우저 스텔스([[봇탐지-우회-스택]])의 HTTP 레벨 대안.

## 정본 구현 (canonical)
- `warm-agent/insane_fetch.py` — JA3 지문 로테이션(`:26`, `chrome124,safari17_0,edge101`), per-host 세션 풀로 `cf_clearance`/`_abck` 쿠키 성숙(`:66`), 사이트 루트 warmup(`:94`), challenge 마커 판정기(`:114`), 지문 순환 재시도(`:173`). insane-search Phase-2를 트리밍해 vendoring.

## 다른 구현 / 변형
- **insane-search 스킬** (`D:\.claude\plugins\...\insane-search`) — 전체 격자(TLS×URL변환×referer) + Playwright 폴백. 단건은 이걸 직접 쓰는 게 나음.
- 앱단 반대편(인증서 피닝 분석): `guardian-decompile/ANALYSIS_REPORT.md:131` (OkHttp CertificatePinner).

## 주의점 / 함정
- curl_cffi ≥0.15 필요(최신 Chrome 지문).
- 429는 terminal 아님 — 백오프 후 재시도.

## 관련: [[봇탐지-우회-스택]] · [[project-warm-agent]]
