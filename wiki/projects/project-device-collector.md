# project-device-collector

> 실기기 브라우저에서 디바이스 핑거프린트를 수집해 Supabase에 저장하는 랜딩(안드로이드 프로필 풀 공급). | 도메인: 수집/인프라 | 스택: 순수 JS(빌드 없음), Supabase REST, Vercel | 상태: 활성

## 핵심 자산 (다른 곳에서 주워올 것)
- `index.html:63` `extractModel`(UA에서 SM-* 모델), `:69` `extractAndroidVer`, `:74` `collect`(cores/memory/screen/dpr/touchPoints/timezone/colorDepth), `:99` Supabase REST insert(`apikey`+`Bearer`, `Prefer: return=minimal`).
- `supabase_setup.sql:5` — Supabase 스키마 + RLS + **프로필 풀 대여(lease) 패턴**(`is_available`/`used_by`/`used_at`, anon insert 정책, service_role select/update).

## 관계
- 여기서 수집한 안드로이드 프로필 풀이 [[project-naver-traffic-android-samsung]]·[[project-electrone_navershopping]]의 실기기 트래픽에 공급됨.

## 의존 개념: [[봇탐지-우회-스택]] · [[IP-로테이션]]
