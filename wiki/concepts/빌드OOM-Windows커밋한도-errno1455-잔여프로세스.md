# 빌드 OOM인데 RAM은 남음 = Windows 커밋 한도 고갈(errno 1455), 잔여 node/에이전트가 범인

> Gradle/Kotlin(kapt) 빌드가 `insufficient memory for the Java Runtime Environment` / `os::commit_memory ... failed; error=... errno=1455` / `Native memory allocation (mmap) failed to map ...bytes`로 죽는데 **물리 RAM은 몇 GB 남아 있는** 경우 — 이건 힙 부족도 코드 에러도 아니라 **Windows 커밋 한도(RAM+페이지파일) 고갈**이다. 멀티에이전트 세션이 남긴 **잔여 node/python/claude/codex 프로세스가 커밋을 수 GB씩 예약**해 천장에 닿은 것. 힙을 낮춰도 안 풀린다(커밋 자체가 벽). 잔여 프로세스를 정리하거나 재부팅해서 커밋을 비우면 같은 코드가 그대로 빌드된다.

## 무엇 / 언제 쓰나
Windows 개발 머신(특히 Claude Code/OMC 같은 멀티에이전트를 오래 돌린)에서 `gradlew assembleRelease`·`kaptGenerateStubs`·kotlinc가 갑자기 OOM으로 죽을 때. 조금 전까지 같은 코드로 빌드가 됐고 바뀐 게 사소할 때 특히. "컴파일 에러"로 오인하기 쉬운 함정.

## 정본 진단·복구 (실측 2026-09-20, Vortex 릴리스 빌드)
1. **증상 식별** — 빌드 로그 tail에 `# There is insufficient memory for the Java Runtime Environment to continue.` + `os::commit_memory(...) failed; error='...' (DOS error/errno=1455)` + `Native memory allocation (mmap) failed to map 528482304 bytes. Error detail: G1 virtual space`. Gradle는 이걸 `> Task :app:kaptGenerateStubsReleaseKotlin FAILED ... Compilation error. See log for more details`로 감싸서 **가짜 "컴파일 에러"처럼 보인다.** `hs_err_pid*.log`가 프로젝트 루트에 떨어짐(첫 줄에 insufficient memory 명시).
2. **errno 1455 = ERROR_COMMITMENT_LIMIT** — 물리 RAM이 아니라 **커밋 charge**(RAM+페이지파일) 초과. 물리 여유가 5GB여도 커밋이 천장이면 mmap이 실패한다.
3. **커밋 실측** (PowerShell):
   ```powershell
   $os = Get-CimInstance Win32_OperatingSystem
   "CommitLimit(MB): " + [math]::Round($os.TotalVirtualMemorySize/1024) +
   " / Used: " + [math]::Round(($os.TotalVirtualMemorySize-$os.FreeVirtualMemory)/1024) +
   " / Free: " + [math]::Round($os.FreeVirtualMemory/1024)
   ```
   실측: 58,771MB 중 58,171MB 사용, **여유 600MB** → Kotlin 데몬이 ~528MB 못 잡아 죽음.
4. **범인 찾기 = 커밋(Private) 상위 프로세스** (WS 아님, `PrivateMemorySize64`로 정렬):
   ```powershell
   Get-Process | Sort-Object PrivateMemorySize64 -Descending |
     Select-Object -First 20 Name,@{n='PrivMB';e={[math]::Round($_.PrivateMemorySize64/1MB)}},Id
   ```
   실측: **node 156개 = 합 11.5GB**, python 16개=1.4GB, claude 10+개=~6.5GB, codex 5개=0.4GB — 여러 세션 잔여가 커밋을 다 먹고 있었다.
5. **복구** — (a) 잔여 dev 프로세스 정리(단 현재 하네스 자신·사용자 다른 작업일 수 있어 신중), 또는 (b) **재부팅이 가장 확실**. 재부팅 후 커밋 여유 600MB→18,256MB, `gradlew assembleRelease` 그대로 통과(1m23s, EXIT 0). 코드는 한 줄도 안 고침.

## 주의점 / 함정
- **"Compilation error"를 코드 탓으로 오인 금지.** Gradle이 kapt 데몬 크래시(OOM)를 컴파일 에러로 포장한다. 진짜 컴파일 에러면 `compileDebugKotlin`이 먼저 실패했을 것 — 직전에 통과했다면 OOM을 먼저 의심하고 `hs_err_pid*.log`/커밋 여유를 본다.
- **힙 축소로 안 풀린다.** 커밋 charge가 벽이면 `-Xmx`를 낮춰도 컴파일러가 필요로 하는 최소 커밋조차 없을 수 있고, 애초에 남의 프로세스가 커밋을 예약한 게 원인이라 내 빌드 힙만 줄여선 근본 해결이 아니다.
- **Gradle 데몬 하나 죽이는 걸론 부족할 수 있다.** `gradlew --stop`+java 1개 kill로 600MB 회수했으나 여전히 OOM. 커밋을 GB 단위로 비워야 한다(잔여 node 수십~수백 개).
- **PowerShell `-P`/`-D` 인자 따옴표 함정** — `-Pkotlin.daemon.jvmargs="-Xmx1g"`처럼 값만 따옴표 치면 PS가 `.daemon.jvmargs=...`를 별도 토큰(=task명)으로 쪼갠다. 토큰 **전체**를 `'-Pkotlin.daemon.jvmargs=-Xmx1g'`로 감싸거나 배열/`--%`로 넘길 것.
- **사용자 소유 프로세스(실행 중 VM·게임)는 함부로 안 죽인다** — 커밋을 크게 먹어도 세션이 시작하지 않은 것은 재부팅 승인을 받거나 사용자가 닫게 한다.

## 배제된 접근 (dead-end)
- **`-Xmx` 낮춰 재시도** — 커밋 천장 문제라 효과 없음(위).
- **잔여 프로세스 무차별 kill** — 현재 Claude Code 하네스 자신이 node/claude 프로세스라, 전부 죽이면 세션이 끊긴다. 오래된 것만 골라 죽이거나 재부팅이 안전.

## 관련: [[디컴파일-ViewId-자동화]](jadx 힙 OOM은 물리 힙 문제로 다름) · [[project-Vortex]] · [[러너-인스턴스락-PID재사용]]
