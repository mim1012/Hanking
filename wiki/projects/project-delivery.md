# project-delivery

> 배달대행(알파고우/알파고플러스) 일일·주차별 리포트 엑셀을 일 정산서·주급 정산서로 자동 변환하는 데스크톱 정산 프로그램. | 도메인: 정산 도구 | 스택: Python openpyxl, Tk GUI, PyInstaller(exe), msoffcrypto | 상태: 활성

## 핵심 자산 (다른 곳에서 주워올 것)
- `tools/excel_crypto.py:21` `decrypt_if_encrypted` — 열기 암호 걸린 xlsx(OLE)를 메모리 복호화. [[암호xlsx-BadZipFile-File-is-not-a-zip-file-msoffcrypto복호화]]
- `tools/daily_settlement.py:347` `write_summary` — 일 정산 종합. `:295` `fill_summary_sheet`(종합·미지정 시트 공용 계산, 행이 0개면 합계에 0을 써서 순환참조 방지), `:110` `target_raw_sheet`(지역 별칭 + `(Z)` 접미사).
- `tools/weekly_settlement.py:266` `convert` — 주간 정산 변환. `:138` `mission_by_rider`(라이더별 집계), `:181` `write_summary`, `:74` `copy_style`(openpyxl 스타일 복사), `:238` `load_source_workbook`(암호 파일 복호화 포함).
- `tools/rider_store.py:144` `load_store`, `:208` `upsert_rider` — JSON 라이더 스토어(선지급/대출 공제).
- `scripts/build_exe.ps1` — PyInstaller 빌드(msoffcrypto hidden-import 포함). 주의: PyInstaller가 실패해도 마지막 줄 "완료"가 출력된다 → exe 수정시각으로 확인할 것.

## 메모 — 지급 규칙 (이중지급 함정)
- 기사관리 구분: 일급 → 일일 정산 종합에서 지급. 주급·미지정 → 주차별 통합에서 전액 지급(`tools/weekly_combined.py:259`는 일급만 0 처리).
- 2026-10-04 "(Z) 파일 인원이 안 나옴/절반 사라짐": 원인은 일일 종합의 '일급만' 필터였다(Z 지역 기사 대부분 미등록). 이전 에이전트가 필터를 "주급만 제외"로 넓혔지만, 그러면 미지정 기사가 일일+주급에서 **이중지급**된다 → 사용자 결정으로 원래 규칙을 유지하고, 미등록 기사는 '미지정' 시트와 경고창(`tools/unified_app.py:343`)으로 보이게만 했다.
- 같은 지역의 (Z) 파일과 일반 파일은 별개 실적이다(겹치는 기사의 주문 수가 서로 다름) → 합산이 맞다.

## 관련
- 엑셀 리포트 생성은 dorami `settlement.service.ts:113`(ExcelJS)와 같은 목적 다른 스택.

## 의존 개념: [[정산-포인트-원장]] · [[암호xlsx-BadZipFile-File-is-not-a-zip-file-msoffcrypto복호화]]
