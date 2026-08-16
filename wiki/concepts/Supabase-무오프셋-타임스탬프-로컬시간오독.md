# Supabase-무오프셋-타임스탬프-로컬시간오독
> `timestamp without time zone` 컬럼을 PostgREST가 오프셋 없이 반환하면, KST 호스트의 JS `Date.parse()`가 UTC 값을 로컬시간으로 오독해 9시간을 통째로 밀린다.

## 무엇 / 언제 쓰나
Supabase(PostgREST) 테이블에 `timestamp without time zone` 컬럼(예: 하트비트, `lastSeenAt`류)이 있고,
Node.js 서버가 `Date.parse(row.column)`이나 `new Date(row.column)`으로 "지금으로부터 얼마나 지났나"를
계산해 online/offline 판정을 내리는 코드를 작성/리뷰할 때 이 페이지를 적용한다. 서버 호스트가 KST(UTC+9)이고
데이터 자체는 UTC로 기록되는 조합에서 발생한다.

## 정본 구현 (canonical)
- `naver_turafic_samsung/server/lib/admin.js:793` `parseSupabaseTimestampMs()` — 문자열 끝에
  `Z`나 `±hh:mm` 오프셋이 없으면 `Z`를 붙여 UTC로 명시 해석한 뒤 `Date.parse()`. `getPcFleetStatus()`
  (`:819`)의 온라인 판정(`now - lastHeartbeatMs < PC_WORKER_ONLINE_MS`)과 클라이언트로 내려주는
  `lastHeartbeat` 값(`toISOString()`으로 재직렬화해 `Z` 보존) 양쪽에 적용.
- `naver_turafic_samsung/server/dev-server.test.js:2071` — 오프셋 없는 최근(30초 전) UTC 타임스탬프를
  워커노드 행으로 주입해 `online:true`, `lastSeenSec<60`을 검증하는 회귀 테스트. 71/71 전체 테스트
  통과 확인, 수정 전/후 실제 `/admin/api/pc-fleet` 응답(curl)으로도 재검증 — 8개 PC 전부
  `online:false, lastSeenSec≈27000~32000` → `online:true, lastSeenSec` 정상 범위(2026-08-16 세션).

## 원인 체인
`turafic_update/scripts/strategy-a-runner.ts`의 `sendHeartbeat()`가 `new Date().toISOString()`(UTC, `Z`
포함)으로 Supabase `workerNodes.lastHeartbeat`에 쓰지만, 그 컬럼이 `timestamp without time zone`이라
Postgres/PostgREST가 저장·조회 과정에서 오프셋을 잘라내 `"2026-08-16T16:50:57.083"`(오프셋 없음)로
돌려준다. 이 문자열을 오프셋 없이 `Date.parse()`하면 ECMA-262 규칙상 **서버 프로세스의 로컬 타임존**으로
해석되므로, KST(UTC+9) 호스트에서는 실제 UTC 16:50:57을 "KST 16:50:57"(=UTC 07:50:57)로 오인해
계산상 9시간 더 지난 것처럼 보인다.

## 주의점 / 함정
- **"온라인인데 전부 오프라인으로 뜬다"는 하트비트/스케줄 로직 버그가 아니라 파싱 버그일 수 있다** —
  `lastSeenSec`이 대략 32,400초(9시간)의 배수/근사치로 찍히면 우선 이 패턴부터 의심할 것. 실제로
  하트비트 자체(cadence, 재시도, DB insert)는 전부 정상이었다.
- **`.env` 변수명 불일치와 증상이 겹쳐서 오진하기 쉽다** — 이 세션에서도 처음엔 `SUPABASE_URL` vs
  `SUPABASE_REST_URL` 변수명 불일치("Supabase 연동 안 됨" 에러)를 "그" 버그로 오인했다. 그건 실제로는
  `README.md:117-129`에 문서화된 대로 매 실행 시 셸에서 `$env:SUPABASE_REST_URL`을 직접 지정하는
  **의도된 설계**였고(`.env` 자동로딩 없음), 진짜 버그는 그 다음(연동은 되는데 전부 오프라인)이었다.
  증상이 순차적으로 겹칠 때 첫 에러를 고쳤다고 끝난 게 아니라는 신호로 취급할 것.
- **클라이언트로 내려주는 값도 같이 정규화해야 한다** — 서버 쪽 online 판정만 고치고 API 응답의
  `lastHeartbeat` 원본 문자열을 그대로 패스스루하면, 브라우저가 그 값을 다시 로컬시간으로 오독해
  화면 표시(`toLocaleString()`)가 여전히 9시간 밀린다. 파싱한 ms를 `new Date(ms).toISOString()`으로
  재직렬화해 `Z`를 보존한 뒤 내려줘야 한다.

## 배제된 접근 (dead-end)
- **`.env`에 `SUPABASE_URL`/`SUPABASE_PUBLISHABLE_KEY`를 `SUPABASE_REST_URL`/`SUPABASE_ANON_KEY`로
  바꿔 저장하려는 시도** — 처음엔 이게 근본 수정이라고 판단했으나, `README.md`를 확인한 결과 이
  dev-server는 `.env` 자동로딩을 아예 안 하고 실행 시 셸 환경변수 주입을 쓰도록 설계돼 있었다.
  `.env` 파일 수정은 애초에 아무 효과가 없었을 것 — 코드/문서를 먼저 읽지 않고 설정 파일부터
  고치려 한 게 헛다리였다.

## 관련: [[project-naver_turafic_samsung]] · [[project-turafic_update]]
