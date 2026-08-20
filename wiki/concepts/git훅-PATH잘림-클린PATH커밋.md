# git훅-PATH잘림-클린PATH커밋

> Windows에서 husky pre-commit이 `'"node"'은(는) 내부 또는 외부 명령...`으로 깨질 때, PATH를 최소 클린 세트로 교체하고 커밋하면 훅 체인이 정상 동작한다.

## 무엇 / 언제 쓰나
- 이 PC(PC_1M)의 시스템 PATH에 잘못 등록된 항목(리터럴 `$env:PATH` 문자열 등)이 섞여 있어, `git commit` → sh(훅) → node(lint-staged) → cmd(.bin 심 스크립트)로 내려가는 과정에서 **손자 cmd 프로세스의 %PATH%가 ~245바이트로 잘려 nodejs 경로가 유실**된다. 훅 안에서 `node scripts/...`(sh 직접 자식)는 되는데 `npx lint-staged`(cmd 손자)만 죽는 비대칭이 시그니처.
- 증상: "Secret scan passed." 다음 줄에서 `'"node"' is not recognized` → pre-commit failed. 같은 실패가 어떤 리포에서든(husky+lint-staged 조합) 재현될 수 있다.

## 정본 구현 (canonical)
- 커밋 직전 PowerShell에서:
```powershell
$env:Path = 'C:\Program Files\nodejs;C:\Program Files\Git\cmd;C:\Program Files\Git\usr\bin;C:\Windows\system32;C:\Windows'
git commit -m "..."
```
- 2026-08-19 adpang-production에서 실증: 이 방식으로 secret scan + lint-staged(eslint --fix, prettier --write) 전 훅 통과, 다수 커밋 성공. lint-staged 설정 위치: `adpang-production/package.json` (`lint-staged` 키).

## 다른 구현 / 변형
- 진단 명령: Git Bash에서 `cmd //c "echo %PATH%"` → 출력이 수백 바이트에서 끊기고 nodejs가 없으면 이 문제. `ORIGINAL_PATH`/`PATH` env에 리터럴 `$env:PATH` 항목이 보이면 오염 확정.

## 주의점 / 함정
- **"sh→cmd PATH 잘림"** — bash에서 `which node`는 되는데 cmd 자식은 node를 못 찾는다. bash가 스스로 실행하는 건 PATH 변환이 필요 없어서 되고, 네이티브 손자에게 넘길 때만 잘린다. "node 설치가 깨졌다"로 오진하기 쉽다.
- `--no-verify`로 우회하지 말 것 — 시크릿 스캔·포맷 훅이 통째로 빠진다. 근본 해결은 시스템 PATH에서 깨진 항목 제거.

## 배제된 접근 (dead-end)
- **샌드박스 해제 후 커밋(dangerouslyDisableSandbox)** — 샌드박스가 env를 오염시킨다는 가설이었으나 동일 실패. 원인은 샌드박스가 아니라 머신 PATH 자체.
- **단순 재시도** — 같은 명령 3회 동일 실패. 환경 문제라 재시도 무의미.

## 관련: [[윈도우서버-git-자동배포]] · [[project-adpang-production]]
