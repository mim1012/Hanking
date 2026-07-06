# MOC — 프로젝트 전체

> 프로젝트 페이지(projects/) 전체 자동 목록. Dataview가 렌더링하므로 수동 갱신 불필요 — 도메인별 그룹핑은 [[README|README 인덱스]]에서.

## 전체 프로젝트 (자동, 최근 갱신순)
```dataview
TABLE WITHOUT ID file.link AS "프로젝트", dateformat(file.mtime, "yyyy-MM-dd") AS "갱신"
FROM "wiki/projects"
SORT file.mtime DESC
```

## 최근 7일 변경 (복습용)
```dataview
TABLE WITHOUT ID file.link AS "프로젝트", dateformat(file.mtime, "yyyy-MM-dd") AS "갱신"
FROM "wiki/projects"
WHERE file.mtime >= date(today) - dur(7 days)
SORT file.mtime DESC
```

## 관련: [[MOC-개념]]
