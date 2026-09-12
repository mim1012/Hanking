# 전략인지-큐-노드-capability-게이트

> 고객이 슬롯별로 여러 실행 전략을 고르면, 큐가 "개통된 전략 중 오늘 시도가 가장 적은 것"을 슬롯당 1행으로 적재하고, 노드는 자기가 실행 가능한 전략(capability 배열)만 claim해 엔진을 디스패치한다. 기존 노드는 시그니처 불변 래퍼로 무중단.

## 무엇 / 언제 쓰나
- 한 작업 단위(슬롯)에 실행 방식이 여럿이고, 실행 노드마다 지원 방식이 다르며(로그인 프로필 유무 등), 기존 함대를 멈추지 않고 새 방식을 점진 개통해야 할 때.
- 핵심 축 4개: ① 레지스트리(`strategy_registry`: 고객 표시 문자·status·`dispatch_enabled` 게이트) ② 선택 집합(`slot_strategy`, source=customer|admin) ③ 큐 적재 라운드로빈(per-(slot,strategy) `attempt_count` 최소) ④ claim capability 필터(노드가 `p_strategies text[]`를 넘김, NULL=기본전략만).
- 2026-09-12 turafic_update 프로덕션 적용 → 로컬 큐모드 카나리 30/30 → XEN-20 노드 D(pc-shopping)+A(m-tonggeom)+기본 페북쉼 병행 실측(리스·텔레메트리·카운터 정확 귀속, 함대 429 불변).

## 정본 구현 (canonical)
- `turafic_update/supabase-migrations/064-strategy-aware-queue.sql:128` `enqueue_claimable_slots` — 슬롯당 큐행 ≤1 불변식 유지 + `:213` `chosen` CTE(선택전략 ∧ `dispatch_enabled` ∧ (production ∨ admin) 중 `attempt_count` 최소, 없으면 `default_strategy`로 fail-open) + `:156` 고아 큐행 재태깅(우리가 적재한 행만; 미개통 전략/기본값 변경으로 남은 행을 기본전략으로 되돌림).
- `…/064-strategy-aware-queue.sql:278` `claim_strategy_slot_v3` — `:352` capability 해석(NULL/빈=기본전략, 미등록만이면 0행 fail-closed), 큐행 strategy 필터, 리스에 strategy 기록, `:585` `strategy_slot_daily` attempt upsert(예외 격리, `ON CONFLICT ON CONSTRAINT` 제약명 지정).
- `…/064-strategy-aware-queue.sql:636` `claim_strategy_slot_v2` — **시그니처 그대로 v3 위임(capability NULL)** = 구 노드 호환. `:654` `report_strategy_slot_v2` — 성공 시 `:786` 실도즈 카운터.
- `…/supabase-migrations/066-customer-strategy-codes-and-rpc.sql:91` `set_slot_strategies` — 코드→internal 검증, customer 호출은 customer 행만 교체(admin 핀 보존·source 승격만), `:116` 슬롯 단위 자문락(`hashtextextended`, bigint 캐스팅 회피). `:68` `get_slot_strategies`. 권한=service_role 단독.
- 러너 `turafic_update/scripts/strategy-a-nocdp-runner.ts:150` `DISPATCH_ALLOW`(env `STRATEGY_DISPATCH_ALLOW`, 빈값=기존 동작) → `:748` `claimSingleSlotsV3`(p_strategies 전달, allowlist 있는 노드는 v2 폴백 금지) → `:1204` `searchCellFor` / `:1216` `willDispatchSocial`(디스패치·스킵·텔레메트리 게이트가 같은 헬퍼) → `:1544` `runStrategyTask` → `:1378` `runSearchRunnerChild`(자식 `--once --dispatch-child=<pid>`, stdout 마지막 줄 JSON, 타임아웃 격리+tree kill).
- 자식 계약 `turafic_update/shoppingtab/search-runner.ts:81` `--once`, `:813` 인자 누락 시 JSON 실패 응답, `:875` JSON 출력 후 stdout 봉인.
- 고객 UI(adpang-production, 에이전트 구현·리뷰 통과·PR #79 머지): `src/lib/slot-strategies.ts`, `src/app/api/sellermate/slot-naver/[id]/strategies/route.ts`(검증 세션+총판 스코프 공용 헬퍼로 소유권), `SellermateNaverContent.tsx` 유입전략 열.

## 다른 구현 / 변형
- 노드 개통 레시피(FleetAgent): 배포 브랜치에 노드 전용 매니페스트 파일 추가 → 노드 `agent.json.manifestFile` 전환(BOM 없이 WriteAllText, 백업) → Machine env `STRATEGY_DISPATCH_ALLOW=<개통전략>,fb-nologin`(기본전략도 넣어야 기존 일감 계속) → `schtasks /run /tn TuraficFleetAgent`. 판정은 DB(리스 strategy·`pc_runner_events.surface`) — 이 에이전트 버전은 러너 stdout 파일을 안 남긴다.
- 로컬 카나리: `--slot-ids=<test슬롯> --strategy=<internal>` + 같은 allowlist. 로컬 슬롯 모드는 report·텔레메트리를 건너뛰므로 DB 흔적 0이 정상.

## 주의점 / 함정
- **"기본값 폴백 착시"** — 러너의 "strategy가 null이면 기존 동작" 안전장치는 DB가 기본전략을 항상 채우는 순간 죽은 코드가 된다. 안전장치는 DB 값이 아니라 **노드 측 명시 옵트인(allowlist)** 에 둬야 한다.
- **"고아 큐행"** — 슬롯당 1행 불변식 + 실행 노드 없는 전략 태깅 = 그 슬롯 영구 정지. 재태깅(retag)과 `dispatch_enabled` 게이트가 짝으로 있어야 한다.
- **"큐 strategy 값 네임스페이스"** — 레거시 값('A')과 internal 값이 한 컬럼에 공존. 레거시는 registry 미등록으로 기본전략 취급.
- **"죽은 캡 카운터 위에 하드캡"** — 기존 일일캡 카운터가 8/22 이후 미기록이라 사실상 무캡 상태였다. 캡을 기본 ON 하면 실도즈(400~1000/일)가 급감한다 → 플래그 기본 OFF, 균등분할은 라운드로빈으로.
- **plpgsql OUT 파라미터 모호성** — RETURNS TABLE 컬럼명(`slot_id`,`strategy`)과 같은 이름을 본문에서 무한정 쓰면 "ambiguous". 인덱스 추론식 대신 `ON CONFLICT ON CONSTRAINT <pkey명>` + 사전 단정.
- **Management API 배치 트랜잭션성은 프로브로 확인** — `create table _t; select 1/0;` 후 잔존 여부. `set local lock_timeout`은 트랜잭션 밖에선 무시된다.
- **SECURITY DEFINER의 RLS 우회는 owner 예외** — 신규 CREATE 함수는 owner=적용 롤이므로 테이블 owner로 `ALTER FUNCTION … OWNER TO` 후 단정.
- **하트비트 게이트** — claim은 `workerNodes` 하트비트(10분)+커밋 일치를 요구한다. 로컬 `.env`의 `SUPABASE_CONTROL_URL`이 다른 프로젝트를 가리키면 하트비트가 엉뚱한 곳으로 가서 "슬롯 없음"만 반복.
- **미개통 전략만 고른 슬롯 + 캡 ON** = 후보 0 → eligible 탈락(아사). 캡 켜기 전 보류 조건을 "실행 가능한 선택이 없으면 기본전략 폴백"으로 완화해야 한다(미해결 티켓).

## 배제된 접근 (dead-end)
- **Option A 전략별 노드풀(노드=1전략, claim에 `p_strategy` 단일 필터)** — 노드 고정 배정이라 처리량 불균형·다중선택 분배 불가 → capability 배열 + 만능 노드로 대체.
- **claim v2 시그니처에 컬럼 추가** — `CREATE OR REPLACE`는 반환형 변경 불가, DROP 순간 함대 claim 실패 → v3 신설 + v2 래퍼.
- **쿨다운 제거를 같은 마이그레이션에 포함** — 429 회귀 시 원인 귀속 불가 → 별도 마이그(065)로 분리.
- **재태깅을 큐 전 행에** — 상류(adpang) 직접 INSERT 행까지 덮어씀 → `strategy_config.source` 가 우리 것인 행만.

## 관련: [[앱-자동업데이트-채널]] · [[실험-그래프-오케스트레이션]] · [[Supabase-마이그레이션-코드배포-분리]] · [[크롬프로필-DPAPI-로그인-이식불가]] · [[러너-인스턴스락-PID재사용]] · [[project-turafic_update]] · [[project-adpang-production]]
