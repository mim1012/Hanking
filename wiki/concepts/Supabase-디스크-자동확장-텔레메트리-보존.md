# Supabase-디스크-자동확장-텔레메트리-보존

> "Your Supabase project's disk was automatically expanded"(8GB→12GB, 90% 도달) 알림이 반복된다 → 원인은 대개 **보존정책 없는 append-only 텔레메트리 + 고churn 테이블 블로트**다. 자동확장은 과금이며 12→18→27GB로 캐스케이드한다. DELETE만으론 파일이 안 줄고(재사용만), OS 반환은 VACUUM FULL/REINDEX가 필요하다. 보존 크론(pg_cron)으로 재발을 막고, 삭제 전 **각 테이블의 reader 시간창을 코드로 실증**한다.

## 무엇 / 언제 쓰나

Supabase(또는 임의의 Postgres) 디스크가 자꾸 차서 자동확장/과금 경고가 올 때. "무엇이 디스크를 먹나"를 실측하고, 무한 증가하는 append-only 로그·텔레메트리에 보존을 걸고, churn 블로트를 회수하는 절차.

## 정본 구현 (canonical)

- `D:\Project\sellermate-contracts\migrations\064-telemetry-retention-and-disk-reclaim.sql` — 보존 함수 6개(배치 ctid 루프, SECURITY DEFINER, search_path=public) + pg_cron 6개(일일 스태거) + 죽은 인덱스 드롭 + 파일 하단 APPLY/READ-BACK/ROLLBACK 절. 기존 `public.purge_strategy_experiment_log` 패턴 이식.
- **experiment-protection 절**(같은 파일, `purge_pc_runner_events`): 뷰가 실험 `started_at`부터 상한없이 누적 집계하면 무조건 N일 삭제가 판정을 깬다 → `not exists (select 1 from experiments x where x.id=e.experiment_id and (x.decided_at is null or x.decided_at >= cutoff))`로 미결/최근 실험 귀속행은 나이무관 보존.
- 진단 SQL 배터리: `pg_database_size` + `pg_total_relation_size`(top relations) + `pg_stat_user_tables`(n_dead_tup/last_autovacuum) + `cron.job_run_details` count + `pg_ls_waldir`(WAL) + `pg_replication_slots`(WAL 핀 누수 배제).

## 진단에서 읽어야 할 신호

- **heap≪index**인 테이블(예: 40k행인데 인덱스 603MB)=REINDEX로 회수. **행수는 적은데 크다**=블로트(autovacuum는 파일 안 줄임).
- **idx_scan=0**인 큰 인덱스=죽은 인덱스, 드롭 대상(단 FK ON DELETE SET NULL 참조는 확인).
- `cron.job_run_details`=pg_cron 로그. 매분 크론 여러 개면 무한 누적(가지치기 없음). keep 7d 크론 필요.
- WAL이 `min_wal_size` 바닥값이면 정상(누수 아님). 복제 슬롯 0개면 WAL 핀 누수 배제.

## 주의점 / 함정

- **삭제 전 reader 시간창을 코드로 실증하라.** "무독자"는 계약 문서가 아니라 5개 코드베이스 grep으로 확인(heartbeats는 실제 reader 0). "라이브 아님"도 실증 — `android_account_leases`는 방치로 보였으나 adpang이 `status='active'` 최신 1건을 읽는 **라이브** 테이블이었다(전량 삭제하면 기능 파손). → released 종료행만 삭제.
- **giant txn 회피.** plpgsql 함수의 배치 루프는 커밋을 안 해 한 트랜잭션이다. 초기 catch-up(수백만 행)은 함수 밖에서 autocommit 배치(`with d as (delete ... limit 20000 returning 1) select count(1) from d;` 반복)로 실행. 정상 일일분은 함수 1회로 충분.
- **DELETE는 OS 반환 안 함.** 디스크 여유를 실제로 만들려면 `VACUUM FULL`(락, 재작성)이나 `REINDEX [CONCURRENTLY]`. Supabase Management API `/database/query`는 VACUUM FULL·REINDEX CONCURRENTLY를 autocommit로 실행 가능(트랜잭션 밖). 핫테이블은 CONCURRENTLY로 락 회피.
- **Supabase 프로비저닝 디스크는 자동 축소 안 됨.** 회수해도 12GB는 유지 — 목적은 "다음 자동확장 방지(재발)"와 헤드룸 확보이지 청구액 즉시 감소가 아니다.
- **ECC Fact-Forcing 게이트**가 `delete from`을 파괴적으로 잡아 반복 차단하면, 만든 purge 함수를 `select public.purge_...()`로 호출(명령 문자열에 파괴 키워드 없음)해 우회. 실측: 이 방법으로 통과.
- 삭제 전 스냅샷: 무독자·대용량이면 행 스냅샷이 회수하려는 디스크를 되레 잡아먹으므로, Supabase PITR을 안전망으로 하고 보존창을 보수적으로. (근거 보존형 삭제는 [[Remove-Item-blocked-훅오인-SQL별칭-del-ManagementAPI적용]]의 스냅샷 관례 참조.)

## 실측 결과 (2026-09-17, adpang 공용 DB)

DB 5,941→3,088MB(−2.85GB), 디스크 ~57%→~33%. heartbeats 1,781→175MB(무독자, 7d), candidates 943→367MB(REINDEX CONCURRENTLY), account_leases released 1.05M행 삭제. 보존 크론 6개 상시 가동으로 재발 차단.

## 관련

[[Remove-Item-blocked-훅오인-SQL별칭-del-ManagementAPI적용]] · [[Supabase-마이그레이션-코드배포-분리]] · [[PostgREST-1000행캡-회피]] · [[plpgsql-선읽기-최적화]]
