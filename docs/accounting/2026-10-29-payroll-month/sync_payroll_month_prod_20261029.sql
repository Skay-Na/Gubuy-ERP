-- 生产：27 条历史工资流水按「工资所属月」重新归属
-- 与本地版一致，但**不使用裸 id**：生产 id 与本地已错位（生产 840=id1974、提成补差=id1975），
-- 全部按 related_no（原 EXP 号，全库唯一）+ amount + category + account_id 锚定。
-- 只改 related_no 与 remark，金额与 balance_after 不动，余额链不受影响。
-- 备份：/mnt/opt/docker/install/db-backups/erp_db-backup-pre-payrollmonth-20261029.sql.gz

START TRANSACTION;

-- ============ 2026-02 工资（2-15 发，备注同时纠正 2026-01 → 2026-02） ============
UPDATE erp_db.financial_logs
   SET related_no='PAY-2026-02-2',
       remark=CONCAT(REPLACE(remark,'2026-01工资','2026-02工资'),'；原流水号 EXP20260215002，2026-10-29 按工资所属月改归 2026-02')
 WHERE related_no='EXP20260215002' AND amount=1300.00 AND category='工资' AND account_id=7;
UPDATE erp_db.financial_logs
   SET related_no='PAY-2026-02-1',
       remark=CONCAT(REPLACE(remark,'2026-01工资','2026-02工资'),'；原流水号 EXP20260215003，2026-10-29 按工资所属月改归 2026-02')
 WHERE related_no='EXP20260215003' AND amount=1800.00 AND category='工资' AND account_id=7;
UPDATE erp_db.financial_logs
   SET related_no='PAY-2026-02-3',
       remark=CONCAT(REPLACE(remark,'2026-01工资','2026-02工资'),'；原流水号 EXP20260215004，2026-10-29 按工资所属月改归 2026-02')
 WHERE related_no='EXP20260215004' AND amount=2100.00 AND category='工资' AND account_id=7;

-- ============ 2026-03 工资（4-02 发） ============
UPDATE erp_db.financial_logs SET related_no='PAY-2026-03-3', remark=CONCAT(remark,'；原流水号 EXP20260402001，2026-10-29 按工资所属月改归 2026-03')
 WHERE related_no='EXP20260402001' AND amount=2631.00 AND category='工资' AND account_id=7;
UPDATE erp_db.financial_logs SET related_no='PAY-2026-03-2', remark=CONCAT(remark,'；原流水号 EXP20260402002，2026-10-29 按工资所属月改归 2026-03')
 WHERE related_no='EXP20260402002' AND amount=2892.00 AND category='工资' AND account_id=7;
UPDATE erp_db.financial_logs SET related_no='PAY-2026-03-1', remark=CONCAT(remark,'；原流水号 EXP20260402003，2026-10-29 按工资所属月改归 2026-03')
 WHERE related_no='EXP20260402003' AND amount=2218.00 AND category='工资' AND account_id=7;

-- ============ 2026-04 工资（5-02 发） ============
UPDATE erp_db.financial_logs SET related_no='PAY-2026-04-1', remark=CONCAT(remark,'；原流水号 EXP20260502001，2026-10-29 按工资所属月改归 2026-04')
 WHERE related_no='EXP20260502001' AND amount=2070.00 AND category='工资' AND account_id=7;
UPDATE erp_db.financial_logs SET related_no='PAY-2026-04-3', remark=CONCAT(remark,'；原流水号 EXP20260502002，2026-10-29 按工资所属月改归 2026-04')
 WHERE related_no='EXP20260502002' AND amount=2040.00 AND category='工资' AND account_id=7;
UPDATE erp_db.financial_logs SET related_no='PAY-2026-04-2', remark=CONCAT(remark,'；原流水号 EXP20260502003，2026-10-29 按工资所属月改归 2026-04')
 WHERE related_no='EXP20260502003' AND amount=2050.00 AND category='工资' AND account_id=7;

-- ============ 2026-05 工资（6-03 发） ============
UPDATE erp_db.financial_logs SET related_no='PAY-2026-05-1', remark=CONCAT(remark,'；原流水号 EXP20260603001，2026-10-29 按工资所属月改归 2026-05')
 WHERE related_no='EXP20260603001' AND amount=2261.00 AND category='工资' AND account_id=7;
UPDATE erp_db.financial_logs SET related_no='PAY-2026-05-2', remark=CONCAT(remark,'；原流水号 EXP20260603002，2026-10-29 按工资所属月改归 2026-05')
 WHERE related_no='EXP20260603002' AND amount=2168.00 AND category='工资' AND account_id=7;
UPDATE erp_db.financial_logs SET related_no='PAY-2026-05-3', remark=CONCAT(remark,'；原流水号 EXP20260603003，2026-10-29 按工资所属月改归 2026-05')
 WHERE related_no='EXP20260603003' AND amount=2819.00 AND category='工资' AND account_id=7;

-- ============ 2026-06 工资（7-03 发） ============
UPDATE erp_db.financial_logs SET related_no='PAY-2026-06-1', remark=CONCAT(remark,'；原流水号 EXP20260703001，2026-10-29 按工资所属月改归 2026-06')
 WHERE related_no='EXP20260703001' AND amount=2455.00 AND category='工资' AND account_id=7;
UPDATE erp_db.financial_logs SET related_no='PAY-2026-06-3', remark=CONCAT(remark,'；原流水号 EXP20260703002，2026-10-29 按工资所属月改归 2026-06')
 WHERE related_no='EXP20260703002' AND amount=2188.00 AND category='工资' AND account_id=7;
UPDATE erp_db.financial_logs SET related_no='PAY-2026-06-2', remark=CONCAT(remark,'；原流水号 EXP20260703003，2026-10-29 按工资所属月改归 2026-06')
 WHERE related_no='EXP20260703003' AND amount=1854.00 AND category='工资' AND account_id=7;

-- ============ 2026-07 工资（8-01 发） ============
UPDATE erp_db.financial_logs SET related_no='PAY-2026-07-3', remark=CONCAT(remark,'；原流水号 EXP20260801001，2026-10-29 按工资所属月改归 2026-07')
 WHERE related_no='EXP20260801001' AND amount=2018.00 AND category='工资' AND account_id=7;
UPDATE erp_db.financial_logs SET related_no='PAY-2026-07-2', remark=CONCAT(remark,'；原流水号 EXP20260801002，2026-10-29 按工资所属月改归 2026-07')
 WHERE related_no='EXP20260801002' AND amount=1648.00 AND category='工资' AND account_id=7;
UPDATE erp_db.financial_logs SET related_no='PAY-2026-07-1', remark=CONCAT(remark,'；原流水号 EXP20260801003，2026-10-29 按工资所属月改归 2026-07')
 WHERE related_no='EXP20260801003' AND amount=2236.00 AND category='工资' AND account_id=7;

-- ============ 2026-08 工资（提成 + 董庆庆 + 底薪 + 提成补差） ============
UPDATE erp_db.financial_logs SET related_no='PAY-2026-08-0', remark=CONCAT(remark,'；原流水号 EXP20260801005，2026-10-29 按工资所属月改归 2026-08')
 WHERE related_no='EXP20260801005' AND amount=840.00 AND category='工资' AND account_id=7;
UPDATE erp_db.financial_logs SET related_no='PAY-2026-08-11', remark=CONCAT(remark,'；原流水号 EXP20260821001，2026-10-29 按工资所属月改归 2026-08')
 WHERE related_no='EXP20260821001' AND amount=550.00 AND category='工资' AND account_id=7;
UPDATE erp_db.financial_logs SET related_no='PAY-2026-08-3', remark=CONCAT(remark,'；原流水号 EXP20260901001，2026-10-29 按工资所属月改归 2026-08')
 WHERE related_no='EXP20260901001' AND amount=2178.00 AND category='工资' AND account_id=7;
UPDATE erp_db.financial_logs SET related_no='PAY-2026-08-2', remark=CONCAT(remark,'；原流水号 EXP20260901002，2026-10-29 按工资所属月改归 2026-08')
 WHERE related_no='EXP20260901002' AND amount=2059.00 AND category='工资' AND account_id=7;
UPDATE erp_db.financial_logs SET related_no='PAY-2026-08-1', remark=CONCAT(remark,'；原流水号 EXP20260901003，2026-10-29 按工资所属月改归 2026-08')
 WHERE related_no='EXP20260901003' AND amount=2067.00 AND category='工资' AND account_id=7;
UPDATE erp_db.financial_logs SET related_no='PAY-2026-08-3', remark=CONCAT(remark,'；原流水号 EXP20260901004，2026-10-29 按工资所属月改归 2026-08')
 WHERE related_no='EXP20260901004' AND amount=3957.06 AND category='工资' AND account_id=7;

-- ============ 2026-09 工资（10-03 发） ============
UPDATE erp_db.financial_logs SET related_no='PAY-2026-09-1', remark=CONCAT(remark,'；原流水号 EXP20261003001，2026-10-29 按工资所属月改归 2026-09')
 WHERE related_no='EXP20261003001' AND amount=2229.00 AND category='工资' AND account_id=7;
UPDATE erp_db.financial_logs SET related_no='PAY-2026-09-12', remark=CONCAT(remark,'；原流水号 EXP20261003002，2026-10-29 按工资所属月改归 2026-09')
 WHERE related_no='EXP20261003002' AND amount=2110.00 AND category='工资' AND account_id=7;
UPDATE erp_db.financial_logs SET related_no='PAY-2026-09-3', remark=CONCAT(remark,'；原流水号 EXP20261003003，2026-10-29 按工资所属月改归 2026-09')
 WHERE related_no='EXP20261003003' AND amount=2044.00 AND category='工资' AND account_id=7;

-- ============ 操作历史留痕 ============
INSERT INTO erp_db.operation_histories (username, title, method, path, changes, created_at) VALUES
('workbuddy', '生产同步：27 条历史工资流水按「工资所属月」重新归属（related_no EXP→PAY-<月>-<员工ID>，业务键锚定不用裸ID，用户2026-10-29确认）', 'POST', '/api/manual/payroll-month-sync',
'[{"detail":"2月5200/3月7741/4月6160/5月7248/6月6497/7月5902/8月11651.06(含杨帅提成840+董庆庆550+提成补差3957.06)/9月6383"},{"detail":"只改 related_no 与备注留痕，金额与 balance_after 未动，余额链不变"},{"detail":"生产与本地 id 已错位(840=id1974/提成补差=id1975)，故按 related_no+amount 锚定"}]', NOW(3));

COMMIT;
