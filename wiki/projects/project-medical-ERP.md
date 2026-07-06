# project-medical-ERP

> 의료기기 유통/판매 업체용 멀티테넌트 SaaS ERP(Turborepo 모노레포). | 도메인: SaaS ERP | 스택: Turborepo, NestJS(api), React+Vite(web), Prisma, Supabase(Auth+Storage+RLS), shadcn/ui | 상태: 활성 (자산 최상)

## 핵심 자산 (다른 곳에서 주워올 것)
- `apps/api/src/common/` — 멀티테넌트 파이프라인 Guard→TenantMiddleware→ResponseInterceptor (canonical). → [[멀티테넌트-요청파이프라인]]
- `apps/api/prisma/schema.prisma:17` — 테넌트 격리 스키마(`Organization`+Plan enum, 모든 모델 organizationId FK).
- ERP 도메인 — `Client`, `Product`+`Inventory`, `Shipment`/`Receiving`($transaction 재고변동), `Settlement`(RECEIVABLE/PAYABLE), `TaxInvoice`. → [[정산-포인트-원장]]
- `apps/web/src/lib/api-client.ts` — axios Bearer 자동첨부, `hooks/use-{domain}.ts` 13개(React Query), `ProtectedRoute`.
- shadcn/ui 전체 Radix 세트 — 디자인 시스템 부트스트랩.

## 주의점
- `apps/web/src/imports/`는 Figma Make 자동생성(TS 에러 71개) — 재사용 비권장.

## 의존 개념: [[멀티테넌트-요청파이프라인]] · [[정산-포인트-원장]] · [[웹앱-인증-세션-계층]]
