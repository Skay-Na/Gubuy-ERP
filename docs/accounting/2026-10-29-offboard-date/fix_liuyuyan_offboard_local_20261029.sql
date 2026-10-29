-- ============================================================
-- 补录刘玉艳（id=2）离职日期 + 9 月离职月底薪折算  【本地版】
-- 背景：刘玉艳 2026-09-03、09-04 最后两天打卡后离开（全公司 09-05 起仅李梅梅/郑玉清有打卡），
--       2026-10-03 才在系统删号（删号日≠离职日），last_working_date 为空，
--       employeeWorkedDuringMonth 依据 deleted_at=10-03 判定她 9 月、10 月仍在职，
--       工资条出现「2026-09 应发 2000 / 未发 / 差 2000」的虚假欠款。
-- 处理：
--   1) last_working_date=2026-09-04（最后打卡日）、employment_status=departed
--   2) 补 2026-09 手工月度考勤 leave_days=30
--      → 扣款天数 = 30 - 4（每月免扣 4 天）= 26 天
--      → 扣款 = 26 × (2000/30) = 1733.33，应发 = 2000 - 1733.33 = 266.67 → 抹零 266
--      → 等效在职工历日 9-01~9-04 共 4 天，与「9 月初离职」一致
--   3) 9 月实发 0（10-03 发薪批次 PAY-2026-09-* 只有李梅梅/郑玉清/任志鹏），
--      修正后 9 月真实欠款 = 266 元
-- 执行：本地执行本文件；生产用 sync_liuyuyan_offboard_prod_20261029.sql
-- 日期：2026-10-29
-- ============================================================

START TRANSACTION;

-- 1) 补离职日期（业务键守卫：姓名 + 入职日 + 尚未登记离职日）
UPDATE erp_db.employees
SET last_working_date = '2026-09-04',
    employment_status = 'departed',
    updated_at = NOW(3)
WHERE name = '刘玉艳'
  AND entry_date = '2026-05-01'
  AND last_working_date IS NULL;

SET @emp_rows := ROW_COUNT();
SET @emp_id := (SELECT id FROM erp_db.employees WHERE name = '刘玉艳' AND entry_date = '2026-05-01' LIMIT 1);

-- 2) 9 月离职月底薪折算（手工月度考勤，人工优先于自动推算）
INSERT INTO erp_db.monthly_attendances (employee_id, month, leave_days, actual_days, late_count, created_at, updated_at)
SELECT @emp_id, '2026-09', 30, 0, 0, NOW(3), NOW(3)
FROM DUAL
WHERE @emp_id IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM erp_db.monthly_attendances WHERE employee_id = @emp_id AND month = '2026-09'
  );

SET @att_rows := ROW_COUNT();

-- 3) 留痕
INSERT INTO erp_db.operation_histories (username, title, method, path, changes, created_at)
VALUES (
  'workbuddy',
  CONCAT('补录刘玉艳离职日期 2026-09-04（删号日 10-03 非离职日）+ 9 月离职月底薪折算为 266；员工行 ', @emp_rows, '、考勤行 ', @att_rows),
  'SCRIPT',
  '/admin/manual/employee-offboard-date',
  JSON_OBJECT(
    'employee', '刘玉艳',
    'employee_id', @emp_id,
    'entry_date', '2026-05-01',
    'last_check_in', '2026-09-04 08:39:20',
    'deleted_at', '2026-10-03 13:10:02',
    'before', JSON_OBJECT('last_working_date', NULL, 'employment_status', 'active', 'sept_leave_days', NULL),
    'after', JSON_OBJECT('last_working_date', '2026-09-04', 'employment_status', 'departed', 'sept_leave_days', 30),
    'reason', '9-03/9-04 为最后打卡日，9-05 起全公司仅李梅梅/郑玉清打卡；deleted_at=10-03 为账号清理时间',
    'calc', JSON_OBJECT('base_salary', 2000, 'daily', 66.6667, 'deduction_days', 26, 'final_salary', 266, 'equivalent_worked_days', 4),
    'paid_sept', 0,
    'real_arrears', 266,
    'affected_rows', @emp_rows
  ),
  NOW(3)
);

COMMIT;

-- 校验
SELECT id, name, entry_date, last_working_date, employment_status, deleted_at
FROM erp_db.employees WHERE name IN ('董庆庆', '刘玉艳');

SELECT id, employee_id, month, leave_days, actual_days
FROM erp_db.monthly_attendances WHERE employee_id = @emp_id ORDER BY month;
