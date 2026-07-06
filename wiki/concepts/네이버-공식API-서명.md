# 네이버 공식 API 서명 (검색광고 HMAC)

> 네이버 검색광고 API HMAC-SHA256 서명 — 삽질 포인트인데 검증된 구현. 순위/키워드 SaaS 공통.

## 무엇 / 언제 쓰나
네이버 검색광고/데이터랩/키워드 공식 API를 붙일 때. 서명 포맷이 까다로워 canonical을 그대로 이식.

## 정본 구현 (canonical)
- `adpang-production/src/lib/naver-searchad-api.ts:34` `generateSearchAdSignature()` — `message = "{timestamp}.{method}.{uri}"` → `createHmac('sha256', secretKey)` → base64. 헤더 `:161-164` (`X-Timestamp`, `X-API-KEY`, `X-Customer`, `X-Signature`). Python 예제를 Node로 이식, 주석에 원본 대조 포함.
- 계열 유틸: `src/lib/naver-datalab-api.ts`, `naver-keyword-api.ts`.

## 다른 구현 / 변형
- 동일 유틸이 `adpang-landing-page/src/lib/`, `sellermate` 계열에도 복제됨.
- 리드젠 쪽 Naver Search/Datalab: `leadgen-lab/src/research_collect.py:124` `naver_get`, `:137` `naver_post`.

## 관련: [[네이버-순위-계산]] · [[project-adpang-production]]
