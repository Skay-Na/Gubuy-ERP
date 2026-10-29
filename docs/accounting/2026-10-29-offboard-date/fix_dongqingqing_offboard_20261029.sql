-- ============================================================
-- 补录董庆庆（id=11）离职日期，消除 9/10 月虚假应发
-- 背景：董庆庆 2026-08-12 入职、8-14~8-20 有打卡、8-21 领 550 后离开，
--       2026-10-03 才在系统删号（删号日≠离职日），last_working_date 为空，
--       导致 employeeWorkedDuringMonth 判定其 9 月/10 月仍在职，
--       工资条出现「2026-09 应发 2000 / 未发」的虚假欠款。
-- 处理：last_working_date=2026-08-20（最后打卡日），employment_status=departed
-- 注意：8 月金额不受影响——8 月已有手工月度考勤（leave_days=27），
--       人工考勤优先于自动推算，应发仍为 516。
-- 执行：本地 + 生产同 SQL（业务键守卫，不用裸 id）
-- 日期：2026-10-29
-- ============================================================

START TRANSACTION;

UPDATE erp_db.employees
SET last_working_date = '2026-08-20',
    employment_status = 'departed',
    updated_at = NOW(3)
WHERE name = '董庆庆'
  AND entry_date = '2026-08-12'
  AND last_working_date IS NULL;

SET @affected := ROW_COUNT();

INSERT INTO erp_db.operation_histories (username, title, method, path, changes, created_at)
VALUES (
  'workbuddy',
  CONCAT('补录董庆庆离职日期 2026-08-20（删号日 10-03 非离职日），消除 9/10 月虚假应发；影响 ', @affected, ' 行'),
  'SCRIPT',
  '/admin/manual/employee-offboard-date',
  JSON_OBJECT(
    'employee', '董庆庆',
    'employee_id_local_and_prod', 11,
    'entry_date', '2026-08-12',
    'before', JSON_OBJECT('last_working_date', NULL, 'employment_status', 'active'),
    'after', JSON_OBJECT('last_working_date', '2026-08-20', 'employment_status', 'departed'),
    'reason', '最后打卡日 8-20，8-21 领 550 后离开；deleted_at=10-03 为账号清理时间',
    'affected_rows', @affected,
    'note', '8 月应发不变（人工月度考勤 leave_days=27 优先）'
  ),
  NOW(3)
);

COMMIT;

SELECT id, name, entry_date, last_working_date, employment_status, deleted_at
FROM erp_db.employees WHERE name IN ('董庆庆', '刘玉艳');
