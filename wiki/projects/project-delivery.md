# project-delivery

> 배달대행(알파고우) 주차별 리포트 엑셀을 주급 정산서로 자동 변환하는 데스크톱 정산 프로그램. | 도메인: 정산 도구 | 스택: Python openpyxl, GUI, PyInstaller(exe) | 상태: 활성

## 핵심 자산 (다른 곳에서 주워올 것)
- `tools/weekly_settlement.py:263` `convert` — 주간 정산 변환. `:136` `mission_by_rider`(라이더별 집계), `:179` `write_summary`, `:72` `copy_style`(openpyxl 스타일 복사).
- `tools/rider_store.py:144` `load_store`, `:208` `upsert_rider` — JSON 라이더 스토어(선지급/대출 공제).
- `scripts/build_exe.ps1` — PyInstaller 빌드.

## 관련
- 엑셀 리포트 생성은 dorami `settlement.service.ts:113`(ExcelJS)와 같은 목적 다른 스택.

## 의존 개념: [[정산-포인트-원장]]
