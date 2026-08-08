# project-starcraft-dashboard

> 스타크래프트 유즈맵 "초월자 강화기" 보스클리어 스크린샷 → EasyOCR 자동인식 → tkinter 대시보드. 숨고 외주(15만원/3~5일, 클라이언트 정은석). | 도메인: 게임 유틸/로컬 OCR | 스택: Python 3.11, EasyOCR(torch), tkinter, PyInstaller onefile | 상태: 진행중(2026-08-08 세션 종료 시점 — 빌드 크래시 수정 재빌드 중)

## 핵심 자산 (다른 곳에서 주워올 것)
- `starcraft_dashboard/starcraft_dashboard/src/ocr_engine.py:14` — `imread_unicode()`: Windows에서 `cv2.imread`가 비-ASCII(한글) 경로를 조용히 `None` 반환하는 버그를 `np.fromfile + cv2.imdecode`로 우회. **주의**: 이번 세션엔 실패한 대안(dead-end)을 시도하지 않고 원인 파악 후 바로 이 방식으로 고쳐서, `_wiki/AGENTS.md` 승격 게이트(검증된 성공 + 이름 붙인 실패 + 배제된 dead-end 3종)의 dead-end 항목을 못 채움 → concept 미승격, 이 프로젝트 메모로만 유지. 다음에 같은 문제 재현 시 dead-end 하나(예: 경로를 `os.fsencode`로만 바꿔보고 실패) 채우면 concept로 승격 가능.
- `starcraft_dashboard/starcraft_dashboard/src/data_manager.py:16` — PyInstaller onefile 앱에서 "사용자가 UI로 수정하는 설정 파일"을 영속화하는 패턴: `_bundle_dir()`(읽기 전용 원본, frozen이면 `sys._MEIPASS`) vs `_external_dir()`(exe 옆, 쓰기 가능) 분리 + 최초 실행 시 `shutil.copy`로 시딩. `sys._MEIPASS`에 직접 쓰면 프로세스 종료 시 통째로 사라져 사용자 변경사항이 재실행마다 초기화되는 실패를 사전에 방지. 아직 실동작(빌드된 exe로) 검증 전 — 검증되면 concept 승격 후보.
- `starcraft_dashboard/starcraft_dashboard/HANDOFF.md:1` — 숨고 외주 인수인계 문서 템플릿(요구사항표 + 폴더구조 + 즉시테스트방법 + 채팅이력 + 남은작업). 다른 숨고 외주 건 인계 문서 작성 시 재사용 가능한 구조.

## 주의점 / 함정 (이 프로젝트 한정 메모, 게이트 미충족으로 concept 미승격)
- **PyInstaller onefile + scipy(easyocr 의존)**: `--onefile`로 빌드하면 `ModuleNotFoundError: No module named 'scipy._cyutility'`로 실행 즉시 크래시(`빌드 성공(exit 0)`과 `실행 가능`은 별개임을 실증 — exe가 뜬 것처럼 보이는 "Unhandled exception in script" 팝업 창이 따로 뜸). `--collect-all scipy` 추가해 재빌드 시도했으나 이 세션 종료 시점까지 실행 검증 미완료(다음 세션에서 이어서 확인 필요).
- 전역 Python 환경(가상환경 아님)에 여러 무관 프로젝트 패키지(PyQt5, paddleocr, pandas, psycopg2 등)가 섞여있어 PyInstaller onefile exe가 467MB로 과도하게 커짐 — venv 격리 없이 빌드하면 무관 의존성까지 딸려 들어가는 사례.

## 의존 개념: (없음 — 이번 세션 승격 없음)
