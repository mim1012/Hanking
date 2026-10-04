# 암호xlsx-BadZipFile-File-is-not-a-zip-file-msoffcrypto복호화

> openpyxl이 `.xlsx`를 `zipfile.BadZipFile: File is not a zip file`(또는 `is_zipfile()` False → "일반 .xlsx 구조가 아닙니다")로 못 연다 → '열기 암호'가 걸린 OLE 컨테이너(첫 8바이트 `D0 CF 11 E0 A1 B1 1A E1`)다. msoffcrypto로 메모리(BytesIO)에 복호화한 뒤 `load_workbook`에 넘긴다.

## 무엇 / 언제 쓰나
거래처가 열기 암호를 걸어 보내는 엑셀(예: 알파고플러스 일일/주급 정산 리포트, 카카오톡 수신)을 사람 손 없이 자동 처리할 때. 확장자는 `.xlsx`라도 일반 xlsx는 `PK\x03\x04`(zip)로 시작하고, 암호 파일은 OLE 매직으로 시작한다 — 헤더 8바이트로 판정한다.

## 정본 구현 (canonical)
- `delivery/tools/excel_crypto.py:21` `decrypt_if_encrypted` — 헤더 8바이트로 OLE 판정(`:34`), 일반 파일은 받은 Path를 그대로 반환. `OfficeFile()` 생성 실패·비OOXML·비암호면 안내 ValueError(`:45-51`). 암호 목록(`:10`, 신→구 순)을 같은 OfficeFile 객체로 순회하되 `InvalidKeyError`·`DecryptionError`만 잡는다(`:53-62`).
- 호출부: `delivery/tools/daily_settlement.py:152` `open_alphago`, `delivery/tools/weekly_settlement.py:246` `load_source_workbook`.
- PyInstaller: msoffcrypto는 `OfficeFile()` 안에서 `msoffcrypto.format.ooxml`을 지연 import한다. 그래서 `--hidden-import msoffcrypto --hidden-import msoffcrypto.format.ooxml`이 필요하다(`delivery/scripts/build_exe.ps1:6`).
- 검증(2026-10-04): 카카오톡 원본 9개(암호 걸림)를 복호화해 정산한 결과가 원본 대비 인원(110/110, 88/88)·라이더별 금액 모두 일치. exe 재빌드 후 기동 스모크 PASS, 번들에 msoffcrypto/olefile/cryptography 포함 확인.

## 주의점 / 함정
- **「암호 틀림」으로 위장된 다른 오류**: 암호 루프에서 `except Exception: continue`로 잡으면, frozen exe에서 cryptography 백엔드가 빠진 것 같은 진짜 버그까지 "암호를 확인할 수 없습니다"로 보고된다. 다음 암호로 넘어가는 건 InvalidKeyError/DecryptionError뿐이어야 한다.
- **구버전 .xls도 OLE 매직**: 확장자만 .xlsx로 바뀐 .xls(또는 깨진 OLE)도 암호 경로로 들어온다. `officefile.format != "ooxml" or not officefile.is_encrypted()`이거나 `OfficeFile()` 자체가 실패하면 "엑셀에서 .xlsx로 다시 저장" 안내를 낸다.
- 틀린 암호는 `load_key`가 아니라 `decrypt()` 끝의 zip 검사에서 InvalidKeyError로 드러난다. 같은 OfficeFile로 다음 암호를 다시 시도해도 상태가 남지 않는다(구암호 파일로 검증).
- file-like 입력은 헤더를 읽기 전에 `seek(0)`.

## 배제된 접근 (dead-end)
- `load_workbook(path)` 직행(09-21 배포본): 암호 파일에서 BadZipFile. 주급 로더는 `is_zipfile` 실패 시 "엑셀에서 암호 해제 후 다시 저장하라"는 안내만 했다 → 매일 수작업이 필요했다.
- 암호 루프 `except Exception` 일괄 처리(이전 에이전트 1차 수정): 위 「암호 틀림 위장」 때문에 리뷰에서 반려되어 예외를 좁혔다.

## 관련: [[project-delivery]]
