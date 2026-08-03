# ARM64네이티브-동적후킹-Frida에뮬레이터

> control-flow-flattening 등으로 정적분석을 막아둔 네이티브(.so) 문자열/로직을, x86_64 안드로이드 에뮬레이터 + Frida로 런타임에 직접 후킹해서 뽑아내는 절차.

## 무엇 / 언제 쓰나
정적 디스어셈블(capstone/objdump/readelf)로 암호화·난독화된 문자열의 실제 디코드 경로를 찾았는데, 그 경로가 control-flow-flattening(상태머신으로 흐름을 잘게 쪼갠 형태)이거나 "항상 거짓"인 가짜 조건문 뒤에 숨겨진 데코이라 정적으로는 못 뚫을 때. 앱을 실제로 돌려서 그 함수의 반환값을 그냥 가로채는 게 더 빠르다.

## 정본 구현 (canonical)
- `guardian-decompile/dynamic-analysis/hook_secureconfig.py` — Frida Python 바인딩으로 device id를 명시(`emulator-5560` 등)해서 spawn+attach, 번들된 agent.js 로드, `SecureConfig.getBaseUrl()` 등 native getter 후킹. `TARGET_DEVICE_ID`를 하드코딩해 실기기와 절대 혼동되지 않게 함(아래 함정 참고).
- `guardian-decompile/dynamic-analysis/frida-agent/agent.js` — `import Java from "frida-java-bridge"` 후 `Java.perform`으로 메서드 구현 교체, `send()`로 반환값 방출.
- `guardian-decompile/dynamic-analysis/frida-agent/bundle.js` — `npx frida-compile agent.js -o bundle.js`로 번들된 산출물(Java 브릿지 포함). `hook_secureconfig.py`가 이 파일을 읽어 `session.create_script()`에 그대로 넘김.
- `guardian-decompile/ANALYSIS_REPORT.md:§8.2` — 정적분석이 어디서 막혔는지(control-flow flattening, 항상거짓 predicate) 판단 근거와 동적분석 전환 결정 기록.
- `guardian-decompile/ANALYSIS_REPORT.md:§9`(직후 절) — 위 절차로 실제 뽑아낸 값(`getBaseUrl() = https://api.gudservice.com`)과 별도 관찰(실기기 netstat)로 교차검증한 사례.

## 셋업 순서 (실측)
1. `D:\Dev\AndroidSdk`(ANDROID_HOME)에 cmdline-tools 없으면 `developer.android.com/studio` 페이지를 curl로 받아 `commandlinetools-win-*.zip` 실제 URL을 grep해서 다운로드(버전 하드코딩 금지 — 자주 바뀜). `cmdline-tools\latest\bin`에 배치.
2. `sdkmanager --licenses`에 `y`를 파이프로 여러 번 흘려보내 전체 동의.
3. **x86_64 시스템 이미지를 받을 것** (`system-images;android-33;google_apis;x86_64`) — arm64-v8a 이미지는 부팅 자체가 안 된다(아래 dead-end).
4. `avdmanager create avd -k "system-images;...;x86_64" --force` (`-d <device>` 옵션은 devices.xml 못 찾는 버그가 있어 생략하는 게 안전).
5. `emulator.exe -avd <name> -no-snapshot -writable-system -port <고정포트>` — 포트를 고정하면 adb 시리얼(`emulator-<port>`)이 예측 가능해져서 실기기와 섞일 위험이 줄어든다.
6. `adb -s emulator-<port> root` (google_apis 비-playstore 이미지는 보통 바로 root 됨) → frida-server(버전은 `pip show frida`로 확인한 python 클라이언트와 **정확히 동일 버전**을 GitHub Releases에서, `frida-server-<ver>-android-x86_64.xz`, lzma로 압축 해제) push → `chmod 755` → 백그라운드 실행.
7. `adb -s emulator-<port> forward tcp:27042 tcp:27042` → `frida.get_device("emulator-<port>")`로 **명시적 device id 지정**(디폴트 `get_usb_device()` 쓰지 말 것 — 아래 함정).
8. spawn → attach → 번들된 script 로드 → resume → 몇 초~수십 초 대기하며 `send()` 메시지 수신.

## 다른 구현 / 변형
- 실기기(루팅 불필요, USB debugging만)에 대해서도 동일 스크립트가 그대로 동작 — 다만 §의 "함정" 항목 때문에 device id 명시가 훨씬 더 중요해짐.

## 주의점 / 함정
- **"실기기·에뮬레이터 동시 연결시 자동 타겟팅 사고"** — `adb devices`/`frida.enumerate_devices()`가 둘 다 보이는 상태에서 시리얼을 명시하지 않으면, 이전에 붙어있던 **실기기가 조용히 선택**될 수 있다. 실제로 이 세션에서 정확히 이 사고가 났다: 에뮬레이터 부팅이 실패한 걸 모르고 진행해서, 분석 대상 앱(SMS/연락처/통화기록을 서버로 업로드하는 것으로 이미 분석된 개인정보 침해성 앱)을 사용자의 실제 갤럭시 S7에 설치·실행시켰고, 부수적으로 기본 전화앱 설정까지 건드렸다. **모든 adb/frida 호출에 `-s <serial>` / device id를 하드코딩하고, 작업 시작 전 `adb devices -l`으로 실기기 존재 여부를 먼저 확인하는 걸 표준 절차로 삼을 것.**
- **"Frida 16+ Java 브릿지 비전역화"** — `Java.perform(...)`만 있는 날것 스크립트를 `session.create_script()`에 그대로 넣으면 `ReferenceError: 'Java' is not defined`로 조용히 실패한다(구버전 Frida 습관 그대로 쓰다 걸림). `frida-java-bridge`를 `frida-compile`로 번들해야 함.
- **"KnoxGate 같은 제조사/무결성 게이트가 예상과 다르게 안 걸림"** — 이 앱은 `Build.MANUFACTURER != "samsung"`이면 즉시 kill하는 게이트가 있는데도, 표준 Google emulator(`sdk_gphone64_x86_64`, 제조사=Google)에서 스플래시~메인화면까지 크래시 없이 통과했다. 정적으로 읽은 "차단 조건"이 실제로는 훨씬 늦게(로그인/온보딩 진행 이후) 평가될 수 있으니, "이 체크에 걸릴 것"이라는 정적 예측을 실측 없이 확정하지 말 것.

## 배제된 접근 (dead-end)
- **arm64-v8a 시스템 이미지로 곧바로 에뮬레이터 부팅** — 앱의 네이티브 라이브러리가 arm64-v8a 전용이라 아키텍처를 맞추려 했으나, x86_64 호스트의 QEMU2는 `FATAL: Avd's CPU Architecture 'arm64' is not supported by the QEMU2 emulator on x86_64 host`로 **부팅 자체가 안 됨**. x86_64 이미지(Android 11+의 google_apis 계열은 ARM 네이티브 라이브러리 번역을 내장)로 바꿔야 실제로 뜬다.

## 관련: [[디컴파일-ViewId-자동화]] · [[TLS-임퍼소네이션-WAF우회]]
