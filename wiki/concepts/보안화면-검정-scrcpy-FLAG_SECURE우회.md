# 보안화면 검정(scrcpy/screencap) = FLAG_SECURE, 루팅폰 벗기기

> scrcpy(또는 screencap)에서 특정 앱 화면만 **검은 화면**으로 나오면 그 앱이 `FLAG_SECURE`를 건 것이다. 루팅폰(Magisk+Zygisk)이면 LSPosed 포크 Vector + DisableFlagSecure 모듈로 system_server 레벨에서 플래그를 벗겨 scrcpy·MediaProjection(TeamViewer 등)에 정상 렌더시킨다. 예: KakaoT Driver 예약콜 리스트/상세(`ReservationActivity`/`ReservationDetailActivity`)가 scrcpy에 검정.

## 무엇 / 언제 쓰나
자동화/리버싱 대상 앱의 화면을 원격 관찰·캡처·녹화(scrcpy, TeamViewer, MediaProjection 기반 미러링)하려는데 그 화면만 검게 나올 때. 원인이 네트워크·GPU·미러링 도구가 아니라 대상 앱이 창에 `WindowManager.LayoutParams.FLAG_SECURE`를 걸어 보안 서피스를 캡처에서 배제한 것임을 먼저 판별하고, 루팅폰에서 이를 무력화하는 절차. 이번 세션 실기기 검증 환경: **S22 Ultra SM-S908N, Magisk 30.6 루팅, Android 14 OneUI**.

## 정본 구현 (canonical) — 실기기 검증됨, 재현 명령/경로가 앵커
> 코드 패턴이 아니라 기기 운영 기법이라 `파일:줄` 앵커가 없다. 아래 명령·모듈 패키지명·CLI 경로가 재현 앵커다.

1. **Zygisk 활성** — `magisk --sqlite "INSERT OR REPLACE INTO settings (key,value) VALUES ('zygisk',1)"` → 재부팅. (LSPosed 계열은 Zygote 주입이 전제라 Zygisk 필수.)
2. **LSPosed 포크 Vector 설치** — `magisk --install-module Vector-*.zip` → 재부팅. Vector는 JingMatrix가 유지하는 LSPosed 포크(현재 프로젝트명 Vector). 모듈 디렉터리는 `/data/adb/modules/zygisk_vector`.
3. **DisableFlagSecure 모듈 APK 설치** — 패키지 `io.github.lsposed.disableflagsecure`(표시명 "Enable Screenshot"), `adb install`로 설치.
4. **Vector 매니저에서 활성 + 스코프 지정** — DisableFlagSecure 활성화 후 **스코프에 System Framework(system_server) + 시스템 UI 체크 → 적용 → 재부팅**. 재부팅 후 system_server 훅이 로드되면 `FLAG_SECURE`가 벗겨져 scrcpy·MediaProjection(TeamViewer 등)에 정상 렌더된다.

- **Vector CLI 조작** — 매니저 GUI 없이도 `sh /data/adb/modules/zygisk_vector/cli modules enable <pkg>` / `... status` / `... scope ls <pkg>`로 상태 확인·활성 가능. **단 DisableFlagSecure는 module.prop에 고정 스코프가 박혀 있어 CLI `scope set`을 거부** → system_server/시스템UI 스코프 체크는 매니저 GUI에서 해야 한다.

## 보조 관측 (픽셀 불필요 · 무루팅도 가능)
- `FLAG_SECURE`는 **접근성 트리를 막지 못한다.** `adb shell uiautomator dump`로 콜목록·경로·요금·상태가 텍스트로 전부 나온다(화면 픽셀 없이도 내용 확보). → [[디컴파일-ViewId-자동화]] · [[카카오T-예약콜-실시간감지]]
- **단 dump는 순간적으로 UiAutomation을 점유**해 접근성 서비스 타이밍에 간섭할 수 있다. 반응속도를 실측할 때는 dump 대신 `logcat`을 쓴다.

## 주의점 / 함정
- **삼성 Knox의 `screencap` 별도 차단 — screencap-only 검증 = false negative (명명된 실패, 2회 오판)** — DisableFlagSecure가 창 layer의 `FLAG_SECURE`를 제거해도 삼성은 `adb shell screencap`(CLI) 경로를 Knox로 **별도** 차단해서 그 영역을 여전히 검게 반환한다. **screencap 결과만 보고 "우회 실패"로 판단하면 오판이다.** 검증은 반드시 **scrcpy(라이브 디스플레이 캡처)나 PC 데스크톱 스크린샷**으로 실제 렌더를 확인할 것. 이번 세션 실측: 같은 상태에서 `screencap`=검정인데 scrcpy=정상 렌더 확인(데스크톱 캡처로 대조).
- **Vector 매니저 "고정 스코프" 체크박스 GUI 글리치 (명명된 실패)** — DisableFlagSecure의 고정 스코프 체크박스가 적용·재부팅 후에도 unchecked로 표시되는 표시 버그가 있다. 실제 훅은 적용돼 있으므로 체크박스 표시만 믿고 "스코프 미적용"이라 판단하면 오판. 실렌더(scrcpy) 여부로 판정할 것.

## 배제된 접근 (dead-end)
- **삼성에서 root `screencap`으로 FLAG_SECURE 화면 픽셀 획득** — 불가능. Knox가 `screencap` 경로를 layer 플래그와 무관하게 별도로 차단하므로, DisableFlagSecure로 플래그를 벗겨도 `screencap`은 계속 검게 반환한다. 픽셀이 필요하면 scrcpy/MediaProjection(라이브 디스플레이 캡처) 경로로 얻어야 한다.

## 곁가지 운영 메모 — 루팅폰 재부팅 후 adb-over-tcp 자동복구 (별도 concept 아님)
같은 관찰 워크플로(원격 루팅폰)를 돌리려면 재부팅 후 adb tcp가 자동으로 다시 열려야 한다. 이건 별개 기법이라 여기 메모로만 남긴다.
- `persist.adb.tcp.port`는 일반 `setprop`이 SELinux로 거부 → Magisk `resetprop`로 설정. `/data/adb/service.d/adb-tcp.sh`가 boot_completed 대기 → resetprop 5555 → adbd 재기동 → 포트 드리프트 시에만 재적용하는 keeper. PC측 재연결 워처와 짝.
- **함정: 삼성은 wireless-debug 설정이 CE(Credential Encrypted) 저장소**라 재부팅 후 최초 1회 화면 잠금해제(PIN) 전에는 adb tcp가 안 열린다 — ping은 되지만 5555는 refused. "부팅됐는데 왜 안 붙나"의 답은 대개 "아직 PIN 해제 전".

## 관련: [[카카오T-예약콜-실시간감지]] · [[디컴파일-ViewId-자동화]] · [[Shizuku-입력주입]] · [[State-Machine-접근성서비스]] · [[project-Vortex]]
