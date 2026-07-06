# MOC — 개념 전체

> 개념(concepts/) 전체 자동 목록. Dataview가 렌더링하므로 수동 갱신 불필요 — 큐레이션된 그룹핑은 [[README|README 인덱스]]와 도메인 지도에서.

## 도메인별 큐레이션 (수동 지도)
[[네이버-트래픽-커머스]] · [[트레이딩]] · [[앱-자동화-리버싱]] · [[웹앱-SaaS]] · [[소셜-수집]] · [[에이전트-인프라]]

## 전체 개념 (자동, 최근 갱신순)
```dataview
TABLE WITHOUT ID file.link AS "개념", dateformat(file.mtime, "yyyy-MM-dd") AS "갱신"
FROM "wiki/concepts"
SORT file.mtime DESC
```

## 최근 7일 변경 (복습용)
```dataview
TABLE WITHOUT ID file.link AS "개념", dateformat(file.mtime, "yyyy-MM-dd") AS "갱신"
FROM "wiki/concepts"
WHERE file.mtime >= date(today) - dur(7 days)
SORT file.mtime DESC
```

## 관련: [[MOC-프로젝트]]
