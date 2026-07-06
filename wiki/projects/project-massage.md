# project-massage

> 마사지샵 탐색 + 인증/업주승인 + 관리자/커뮤니티 운영 Next.js 앱. | 도메인: 업소 디렉터리 | 스택: Next.js16/React19/TS, Prisma7/PostgreSQL | 상태: 종료(2026-07-05 환불로 계약 종료 — 코드는 재사용 자산으로 유효)

## 핵심 자산 (다른 곳에서 주워올 것)
- `_wiki/raw/snapshots/massage/src/lib/auth/password.ts:6` — scrypt 해시(무의존) (canonical). `session.ts:13` HttpOnly 쿠키. `guards.ts:22` 역할 가드. 보안 리뷰(2026-07-04) CRITICAL 0 판정. → [[웹앱-인증-세션-계층]]
- `_wiki/raw/snapshots/massage/src/lib/security/rate-limit.ts:204` — Upstash REST 공유 레이트리미터(INCR/PEXPIRE NX/PTTL) + 인메모리 폴백, `:164` 접두사 env 자동탐색, `:39` 위조 안전 클라이언트 IP. → [[서버리스-공유-레이트리밋]]
- `_wiki/raw/snapshots/massage/src/lib/server/in-flight.ts:1` — 동시 동일요청 코얼레싱 17줄 유틸(동시접속 100명 해법). → [[요청-코얼레싱-인플라이트]]
- `_wiki/raw/snapshots/massage/src/lib/region-compat.ts:3` — 신·구 지역코드 호환 필터(환불 사고의 해법). → [[레거시-코드체계-호환-필터]]
- `_wiki/raw/snapshots/massage/scripts/run-tests.mjs:60` — `resolveTestDatabaseUrl`: 테스트 DB명이 아니면 실행 거부하는 가드 러너(운영 DB 오염 방지). 시드+`@/*` 별칭 등록 포함.
- `_wiki/raw/snapshots/massage/tests/` prod-*.spec.ts 9종 — 운영환경 스모크 매트릭스 패턴: `_wiki/raw/snapshots/massage/tests/prod-public-matrix.spec.ts:13` 지역×테마×정렬 필터 행렬, 보안 퍼즈·권한 네거티브 매트릭스·SEO 크롤러 스모크.
- `_wiki/raw/snapshots/massage/src/app/api/admin/` — 관리자 CRUD 패턴(shops/notices/qna/reviews/approvals), 승인 워크플로우.
- `_wiki/raw/snapshots/massage/prisma/schema.prisma` — 21개 모델(User:38, Shop:77, Review, QnA, Notice, AuditLog:217, SiteSettings:250). 업소 디렉터리 도메인 모델.

## 스냅샷 (원본 폴더 삭제 대비)
- 위 핵심 자산 전체(인증 3종·레이트리밋·인플라이트·지역호환·테스트러너·prod 스펙 9종·admin CRUD·schema)를
  `raw/snapshots/massage/`에 원본 경로 구조 그대로 보존(2026-07-06, 50파일 134KB, 해시 대조 완료).
  원본 폴더 삭제(2026-07-06 승인)에 대비해 위 참조는 스냅샷 경로 기준으로 재지향됨.
  코드 전체 이력(90+ 테스트, 커밋 로그 포함)은 GitHub `mim1012/massage`에 전부 푸시되어 있음(미푸시 0).

## 교훈 (포스트모템 2026-07-05)
관리자 부산 필터 회귀 → 환불. 원인: 화면별로 따로 구현된 지역 필터 + exact match + 깨끗한 픽스처만으로 테스트. 상세는 [[레거시-코드체계-호환-필터]] 주의점과 글로벌 룰 `D:\.claude\rules\contract-project-gates.md`.

## 의존 개념: [[웹앱-인증-세션-계층]] · [[서버리스-공유-레이트리밋]] · [[요청-코얼레싱-인플라이트]] · [[레거시-코드체계-호환-필터]]
