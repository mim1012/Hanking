# 크롬프로필-DPAPI-로그인-이식불가

> Chrome 프로필 폴더를 다른 Windows PC/사용자로 복사해도 로그인은 따라가지 않는다 — 쿠키 값이 원 사용자의 DPAPI 키로 암호화돼 있어 새 PC에서는 복호화 불가(=로그아웃 상태). 로그인이 필요한 경로는 그 PC의 콘솔에서 직접 로그인해야 한다.

## 무엇 / 언제 쓰나
- 로그인 세션이 필요한 브라우저 자동화를 여러 노드로 확장할 때. "로컬에서 되는 프로필을 zip으로 노드에 심는다"는 계획을 세우기 전에 읽을 것.
- 2026-09-12 실측(turafic_update, XEN-20): 로컬 로그인 프로필 `login_00`(캐시 제외 165MB, zip 34MB)을 노드에 이식 → 로그인 불필요한 pc-shopping(D)은 성공, 로그인 필요한 m-shopping(B)은 2/2 `LOGIN_REDIRECT`. 같은 프로필이 로컬에서는 31/31 로그인 상태였다.

## 정본 구현 (canonical)
- 프로필 게이트 `turafic_update/scripts/strategy-a-nocdp-runner.ts:1247` `enforceShoppingProfileGate` — 로그인 필요 셀(m-shopping/pc-shopping)이 allowlist에 있으면 `Default/Network/Cookies`(Chrome 122+) 또는 `Default/Cookies` 존재를 확인하고 없으면 그 셀을 allowlist에서 제거. **존재만 보므로 "로그아웃된 프로필"은 못 잡는다** — 로그인 상태는 실행 결과(`LOGIN_REDIRECT`)로만 드러난다.
- 이식 절차 자체(캐시 폴더 제외 zip → scp → Expand-Archive)는 pc-shopping처럼 **로그인이 필요 없는 셀의 웜 프로필**로는 유효하다(쿠키 갱신 확인). 로그인 이식 목적으로는 무효.

## 다른 구현 / 변형
- 노드 로그인 확보: 노드 콘솔 세션(autologon/RDP)에서 해당 프로필 경로로 크롬을 띄워 수동 로그인 1회 → 이후 러너가 같은 경로를 `--profile`로 사용. 계정 정보가 필요하므로 사람 작업.
- 페북 로그인 프로필(`social_meta`)도 같은 제약. [[브라우저-세션쿠키-영속화]]는 **같은 PC·같은 사용자**에서의 영속화 기법이며 이식과는 다른 문제.

## 주의점 / 함정
- **"프로필 존재 ≠ 로그인"** — 폴더·Cookies 파일이 있고 크기도 정상인데 로그인이 풀려 있다. 게이트를 통과한 뒤 도즈가 전부 로그인벽으로 소모된다(고객 슬롯 순번 낭비). 개통 전 그 노드에서 로그인 필요 셀 1회 스모크가 필수.
- **Chrome 122+ 쿠키 경로** — `Default/Network/Cookies`. `Default/Cookies`만 검사하면 정상 프로필을 오탐해 셀을 꺼버린다(리뷰에서 실제로 잡힌 회귀).
- 프로필 복사 시 `SingletonLock`·캐시 폴더는 제외. 복사본으로 크롬이 뜨긴 하므로 "동작한다"고 오판하기 쉽다.

## 배제된 접근 (dead-end)
- **로컬 로그인 프로필 zip 이식으로 노드 로그인 확보** — 위 실측대로 로그아웃 상태. DPAPI가 사용자 SID·머신 키에 묶여 있어 파일 복사로는 넘어가지 않는다.
- (미시도, 참고) CDP `Network.setCookie`로 로컬 쿠키 값을 노드에 주입하는 방식은 nocdp 함대 원칙(CDP=탐지 신호)과 충돌해 검토하지 않았다.

## 관련: [[전략인지-큐-노드-capability-게이트]] · [[브라우저-세션쿠키-영속화]] · [[CDP없는-OS레벨-브라우저자동화]] · [[project-turafic_update]]
