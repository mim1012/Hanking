# usage-widget

> AI 모델(Claude/Codex/DeepSeek/Kimi/GLM) 남은 사용량을 표시하는 Windows 플로팅 위젯. | 도메인: 에이전트 인프라/데스크톱 유틸 | 스택: Python 3.11, tkinter, requests | 상태: 동작 (2026-07-24)

## 핵심 자산 (다른 곳에서 주워올 것)
- `usage-widget/widget.py:88` — Claude/Codex OAuth 토큰 자동 refresh + 인증파일 회수 기록
- `usage-widget/widget.py:451` — 프로바이더 병렬 조회(fetch_all, 스레드 fan-out) + 정규화 결과 스키마
- `usage-widget/widget.py:478` — tkinter 프레임리스 always-on-top 위젯 (드래그 이동, 위치 영속화, 우클릭 메뉴, 게이지 바)

## 의존 개념: [[AI-구독-사용량-조회]] · [[파일기반-지속메모리]]
