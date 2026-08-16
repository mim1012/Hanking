# SSH-공개키인증-Win32버그-우회
> Win32-OpenSSH 서버가 공개키를 인식하고도 인증을 거부하는 버그를 비밀번호+plink로 우회한다.

## 무엇 / 언제 쓰나
원격 Windows PC 플릿(트래픽 봇 워커)에 SSH로 배포/운영 접근을 열 때. `ssh-keygen` 공개키를
`administrators_authorized_keys`에 정상 등록하고 ACL·관리자그룹까지 맞춰도, Win32-OpenSSH 9.5
계열에서 `Server accepts key: ...` 로그 직후 곧바로 `Permission denied
(publickey,password,keyboard-interactive)`가 뜨는 경우 이 페이지를 적용한다.

## 정본 구현 (canonical)
- `sellermate_naver_place_all/rank-check/KOE-REMOTE-SSH.md:28-49` — plink(PuTTY) `-hostkey`
  핀 고정 + `-pw` 비밀번호 인증으로 우회하는 최초 해법(koe PC, `hostname; whoami` 정상 응답으로
  검증됨).
- `turafic_update`(round-penguin worktree) PC-02(`218.148.204.120`)에서 동일 증상 재현 → 같은
  방법으로 재검증(koe와 별개 PC, `plink -ssh -batch -hostkey <핀> -pw <비번>` 접속으로 `hostname`/
  `whoami` 정상 응답 확인 — 독립된 두 번째 실동작 검증).

## 다른 구현 / 변형
- **서버(inbound)뿐 아니라 클라이언트(outbound)에서도 재현** —
  `turafic_update`(round-penguin worktree) PC-04(`118.32.34.247`)에서, 원격 PC가 SSH 배포키로
  `git@github.com`에 나가는 방향에서 동일 증상(`Server accepts key: ... explicit` 로그 직후
  응답 없이 멈춤, `ssh -vT git@github.com`이 타임아웃)이 재현됨 — `PC-INVENTORY.md:43`. 서버 설정
  문제가 아니라 이 PC들에 깔린 Win32-OpenSSH 클라이언트/서버 공통 버전대의 버그임을 시사. 우회는
  SSH 키 자체를 포기하고 GitHub PAT를 `https://<PAT>@github.com/...` 형태로 `git remote set-url`에
  임베드하는 방식(HTTPS 인증으로 완전히 회피) — `PC-INVENTORY.md:42-44`.

## 주의점 / 함정
- **"키 인식 후 거부" 증상** — `ssh -v` 로그에 `Server accepts key: ... ED25519 ... explicit`까지
  뜨고 바로 `No more authentication methods to try. Permission denied`로 끝난다. 흔한 원인(ACL
  오류, 관리자그룹 미포함)과 증상이 똑같아서 오진하기 쉽다.
- **ACL/관리자그룹은 죄가 없을 수 있다** — `icacls`로 `administrators_authorized_keys` 권한이
  `SYSTEM:(F)`, `BUILTIN\Administrators:(F)`뿐임을 확인하고, `net localgroup Administrators`로
  대상 계정이 그룹 멤버임도 확인했는데도 여전히 거부되는 경우가 실측됐다 — 즉 흔히 의심하는 두
  원인이 둘 다 정상이어도 이 버그는 발생한다.
- **PowerShell `net user` 비밀번호 설정 시 큰따옴표+`$`는 변수치환된다** — `net user <계정>
  "pw$Athing"`을 PowerShell에 입력하면 `$A`가 (미정의) 변수로 해석돼 조용히 빈 문자열로 치환되고,
  실제 저장되는 비밀번호가 의도한 것과 달라진다. 작은따옴표로 감싸거나 `` `$ ``로 이스케이프해야
  한다 — 이 함정에 실제로 걸려 첫 plink 인증이 "Configured password was not accepted"로
  실패했었다.

## 다른 구현 / 변형 (scp 파일 배포)
- **같은 PC 플릿에서 `scp`(SFTP 서브시스템 기반) 목적지 경로에 `C:\sellermate\...`처럼 백슬래시를
  쓰면 조용히 실패한다** — 종료코드 0, `-v`에도 에러 없이 "truncating at N"만 찍혀 정상처럼 보이지만
  원격 파일은 갱신되지 않는다(구버전 그대로 남음). `C:/sellermate/...`로 슬래시만 바꾸면 즉시 정상
  반영됨(grep으로 재확인) — `turafic_update`(round-penguin worktree) sellermate-01(SSH 키 인증)에서
  재현·수정, 상세: `PC-INVENTORY.md:61`.

## 배제된 접근 (dead-end)
- **ACL 재설정**(`icacls /inheritance:r` + `/grant:r`) — 폴더 상속 권한이 안 지워진 게 원인일
  거라 가정하고 재확인했으나, 이미 SYSTEM+Administrators만 있는 정상 상태였다. ACL은 원인이
  아니었다.
- **관리자 그룹 재확인**(`net localgroup Administrators`) — 대상 계정이 이미 멤버였다.
  권한그룹 문제도 아니었다.

## 관련: [[IP-로테이션]] · [[project-turafic_update]]
