# R8-Gson-DTO-keep-함정
> R8 난독화 + Gson 리플렉션 조합에서 DTO keep 규칙을 하나라도 빠뜨리면, 예외 없이 "전 필드 null 객체"가 조용히 반환된다 — 디버그(비난독화)에선 완벽히 되다가 릴리스에서만 죽고, try-catch 폴백도 안 탄다.

## 무엇 / 언제 쓰나
Android 릴리스 빌드(`minifyEnabled true`)에서 Gson(또는 리플렉션 기반 직렬화)을 쓰는 모든
프로젝트. 증상 시그니처: **"디버그는 되는데 릴리스만 안 된다" + NPE가 Gson 호출부가 아니라
한참 뒤 필드 접근부에서 터짐**. Kotlin non-null 타입이어도 Gson은 Unsafe 인스턴스화로
생성자·기본값을 우회하므로 null이 들어온다.

## 정본 구현 (canonical)
- `Vortex` `app/proguard-rules.pro:42-50` — Gson DTO keep 블록. 중첩 데이터클래스는
  `-keep class ...ParsingConfig$* { <fields>; }` 한 줄로 전부 커버.
- `Vortex` `app/src/main/java/com/example/twinme/domain/parsing/ParsingConfig.kt:79-100` —
  심층 방어: `fromJson` 결과의 필수 섹션을 `@Suppress("SENSELESS_COMPARISON")` null 검증 후
  기본값 폴백. keep 규칙이 다시 빠져도 앱이 죽지 않게.
- **검증 방법(빌드 산출물 기준, 2026-09-05 실측)**: ① `mapping.txt`에서
  `DTO클래스 -> 원본이름:` + 필드 원명 보존 확인, ② APK dex 바이트에서 JSON 키 문자열 카운트
  (python zipfile로 dex 추출 → `dex.count(b"field_name")`) — 깨진 빌드는 toString 리터럴만
  1회, 정상 빌드는 필드 엔트리 포함 2회+.

## 다른 구현 / 변형
- `@SerializedName` 전면 부착이 정석이지만 필드 수가 많으면 keep 규칙이 더 경제적.
- 원격 로그로 사후 탐지: 난독화 스택의 `field 'boolean a.b.c' on null object` 패턴 →
  `retrace`(mapping.txt)로 원명 복원해 어느 DTO인지 특정.

## 주의점 / 함정
- **Gson은 매핑 실패에 예외를 던지지 않는다** — `try-catch + 기본값 폴백` 코드가 있어도
  절대 안 탄다. 이것이 이 함정의 핵심이며, keep 누락을 코드 리뷰에서 못 잡는 이유.
- R8은 한술 더 떠 "리플렉션 주입을 못 보므로" 생성자 기본값을 상수 접기(constant folding)
  한다 — 디컴파일하면 필드가 `= false` 상수로 박혀 있음(Vortex 2.8.2 실측).
- 실배포 사고 사례: Vortex 2.8.1~2.8.2에서 `ParsingConfig` DTO keep 누락 →
  FastTree 폴백 경로 진입 시점(환경 변화)부터 전 기기 콜 처리 정지. 잠복 기간이 있어서
  "직전 릴리스 탓"으로 오인됨 — 진단은 실배포 APK 2종 바이너리 diff + 서버 로그 타임라인으로.
- 새 Gson DTO를 추가할 때마다 keep 목록 등록이 필수 — 빌드는 성공하므로 컴파일러가 못 잡는다.

## 배제된 접근 (dead-end)
- "직전 릴리스와의 소스 diff만 보고 원인 찾기" — 잠복 버그는 트리거(환경 변화)와 도입 시점이
  달라 소스 diff에 안 나타난다. 실배포 APK 바이너리 대조 + 런타임 로그가 정답 경로였다.

## 관련
- [[project-Vortex]] · 자동메모리 `vortex-r8-gson-parsingconfig-npe`(진단 전 과정)
