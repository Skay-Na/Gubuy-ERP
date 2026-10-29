-- 本地：工资流水按「工资所属月」重新归属
-- 背景：历史工资流水 related_no 为 EXP2026xxxx（开销账导入），真实净利润的
--       payrollBelongsToWindow 只认 PAY-<YYYY-MM> 前缀，否则回落 created_at，
--       导致每月工资都晚记一个月（3 月工资 7741 落在 4 月）。
-- 做法：把 27 条工资流水的 related_no 改为 PAY-<所属月>-<员工ID>，
--       原 EXP 号写入备注留痕。仅改字符串，金额与 balance_after 不动。
-- 员工ID：1=李梅梅 2=刘玉艳 3=任志鹏 11=董庆庆 12=郑玉清 0=外协(杨帅提成)
-- 所属月依据：用户 2026-10-29 确认 2-15 发的 5200 为 2 月工资，其余按「月初发上月」逐批对应
-- 备份：output/backup-local-pre-payrollmonth-20261029.sql.gz

START TRANSACTION;

-- ============ 2026-02 工资（2-15 发） ============
UPDATE erp_db.financial_logs SET related_no='PAY-2026-02-2', remark=CONCAT(remark,'；原流水号 EXP20260215002，2026-10-29 按工资所属月改归 2026-02')
 WHERE id=1902 AND related_no='EXP20260215002' AND amount=1300.00 AND category='工资';
UPDATE erp_db.financial_logs SET related_no='PAY-2026-02-1', remark=CONCAT(remark,'；原流水号 EXP20260215003，2026-10-29 按工资所属月改归 2026-02')
 WHERE id=1903 AND related_no='EXP20260215003' AND amount=1800.00 AND category='工资';
UPDATE erp_db.financial_logs SET related_no='PAY-2026-02-3', remark=CONCAT(remark,'；原流水号 EXP20260215004，2026-10-29 按工资所属月改归 2026-02')
 WHERE id=1904 AND related_no='EXP20260215004' AND amount=2100.00 AND category='工资';

-- ============ 2026-03 工资（4-02 发） ============
UPDATE erp_db.financial_logs SET related_no='PAY-2026-03-3', remark=CONCAT(remark,'；原流水号 EXP20260402001，2026-10-29 按工资所属月改归 2026-03')
 WHERE id=1930 AND related_no='EXP20260402001' AND amount=2631.00 AND category='工资';
UPDATE erp_db.financial_logs SET related_no='PAY-2026-03-2', remark=CONCAT(remark,'；原流水号 EXP20260402002，2026-10-29 按工资所属月改归 2026-03')
 WHERE id=1931 AND related_no='EXP20260402002' AND amount=2892.00 AND category='工资';
UPDATE erp_db.financial_logs SET related_no='PAY-2026-03-1', remark=CONCAT(remark,'；原流水号 EXP20260402003，2026-10-29 按工资所属月改归 2026-03')
 WHERE id=1932 AND related_no='EXP20260402003' AND amount=2218.00 AND category='工资';

-- ============ 2026-04 工资（5-02 发） ============
UPDATE erp_db.financial_logs SET related_no='PAY-2026-04-1', remark=CONCAT(remark,'；原流水号 EXP20260502001，2026-10-29 按工资所属月改归 2026-04')
 WHERE id=1936 AND related_no='EXP20260502001' AND amount=2070.00 AND category='工资';
UPDATE erp_db.financial_logs SET related_no='PAY-2026-04-3', remark=CONCAT(remark,'；原流水号 EXP20260502002，2026-10-29 按工资所属月改归 2026-04')
 WHERE id=1937 AND related_no='EXP20260502002' AND amount=2040.00 AND category='工资';
UPDATE erp_db.financial_logs SET related_no='PAY-2026-04-2', remark=CONCAT(remark,'；原流水号 EXP20260502003，2026-10-29 按工资所属月改归 2026-04')
 WHERE id=1938 AND related_no='EXP20260502003' AND amount=2050.00 AND category='工资';

-- ============ 2026-05 工资（6-03 发） ============
UPDATE erp_db.financial_logs SET related_no='PAY-2026-05-1', remark=CONCAT(remark,'；原流水号 EXP20260603001，2026-10-29 按工资所属月改归 2026-05')
 WHERE id=1946 AND related_no='EXP20260603001' AND amount=2261.00 AND category='工资';
UPDATE erp_db.financial_logs SET related_no='PAY-2026-05-2', remark=CONCAT(remark,'；原流水号 EXP20260603002，2026-10-29 按工资所属月改归 2026-05')
 WHERE id=1947 AND related_no='EXP20260603002' AND amount=2168.00 AND category='工资';
UPDATE erp_db.financial_logs SET related_no='PAY-2026-05-3', remark=CONCAT(remark,'；原流水号 EXP20260603003，2026-10-29 按工资所属月改归 2026-05')
 WHERE id=1948 AND related_no='EXP20260603003' AND amount=2819.00 AND category='工资';

-- ============ 2026-06 工资（7-03 发） ============
UPDATE erp_db.financial_logs SET related_no='PAY-2026-06-1', remark=CONCAT(remark,'；原流水号 EXP20260703001，2026-10-29 按工资所属月改归 2026-06')
 WHERE id=1953 AND related_no='EXP20260703001' AND amount=2455.00 AND category='工资';
UPDATE erp_db.financial_logs SET related_no='PAY-2026-06-3', remark=CONCAT(remark,'；原流水号 EXP20260703002，2026-10-29 按工资所属月改归 2026-06')
 WHERE id=1954 AND related_no='EXP20260703002' AND amount=2188.00 AND category='工资';
UPDATE erp_db.financial_logs SET related_no='PAY-2026-06-2', remark=CONCAT(remark,'；原流水号 EXP20260703003，2026-10-29 按工资所属月改归 2026-06')
 WHERE id=1955 AND related_no='EXP20260703003' AND amount=1854.00 AND category='工资';

-- ============ 2026-07 工资（8-01 发，3 人底薪+提成） ============
UPDATE erp_db.financial_logs SET related_no='PAY-2026-07-3', remark=CONCAT(remark,'；原流水号 EXP20260801001，2026-10-29 按工资所属月改归 2026-07')
 WHERE id=1962 AND related_no='EXP20260801001' AND amount=2018.00 AND category='工资';
UPDATE erp_db.financial_logs SET related_no='PAY-2026-07-2', remark=CONCAT(remark,'；原流水号 EXP20260801002，2026-10-29 按工资所属月改归 2026-07')
 WHERE id=1963 AND related_no='EXP20260801002' AND amount=1648.00 AND category='工资';
UPDATE erp_db.financial_logs SET related_no='PAY-2026-07-1', remark=CONCAT(remark,'；原流水号 EXP20260801003，2026-10-29 按工资所属月改归 2026-07')
 WHERE id=1964 AND related_no='EXP20260801003' AND amount=2236.00 AND category='工资';

-- ============ 2026-08 工资（含 8 月提成与董庆庆几天工资） ============
-- 杨帅空气能提成 840（订单 202608050001 属 8 月，外协无员工ID 用 0）
UPDATE erp_db.financial_logs SET related_no='PAY-2026-08-0', remark=CONCAT(remark,'；原流水号 EXP20260801005，2026-10-29 按工资所属月改归 2026-08')
 WHERE id=1975 AND related_no='EXP20260801005' AND amount=840.00 AND category='工资';
-- 董庆庆 8 月几天工资（8-12 入职，8-21 发）
UPDATE erp_db.financial_logs SET related_no='PAY-2026-08-11', remark=CONCAT(remark,'；原流水号 EXP20260821001，2026-10-29 按工资所属月改归 2026-08')
 WHERE id=1959 AND related_no='EXP20260821001' AND amount=550.00 AND category='工资';
-- 8 月底薪（9-01 发）
UPDATE erp_db.financial_logs SET related_no='PAY-2026-08-3', remark=CONCAT(remark,'；原流水号 EXP20260901001，2026-10-29 按工资所属月改归 2026-08')
 WHERE id=1965 AND related_no='EXP20260901001' AND amount=2178.00 AND category='工资';
UPDATE erp_db.financial_logs SET related_no='PAY-2026-08-2', remark=CONCAT(remark,'；原流水号 EXP20260901002，2026-10-29 按工资所属月改归 2026-08')
 WHERE id=1966 AND related_no='EXP20260901002' AND amount=2059.00 AND category='工资';
UPDATE erp_db.financial_logs SET related_no='PAY-2026-08-1', remark=CONCAT(remark,'；原流水号 EXP20260901003，2026-10-29 按工资所属月改归 2026-08')
 WHERE id=1967 AND related_no='EXP20260901003' AND amount=2067.00 AND category='工资';
-- 8 月提成补差 3957.06（9-01 补录，任志鹏）
UPDATE erp_db.financial_logs SET related_no='PAY-2026-08-3', remark=CONCAT(remark,'；原流水号 EXP20260901004，2026-10-29 按工资所属月改归 2026-08')
 WHERE id=1973 AND related_no='EXP20260901004' AND amount=3957.06 AND category='工资';

-- ============ 2026-09 工资（10-03 发） ============
UPDATE erp_db.financial_logs SET related_no='PAY-2026-09-1', remark=CONCAT(remark,'；原流水号 EXP20261003001，2026-10-29 按工资所属月改归 2026-09')
 WHERE id=1969 AND related_no='EXP20261003001' AND amount=2229.00 AND category='工资';
UPDATE erp_db.financial_logs SET related_no='PAY-2026-09-12', remark=CONCAT(remark,'；原流水号 EXP20261003002，2026-10-29 按工资所属月改归 2026-09')
 WHERE id=1970 AND related_no='EXP20261003002' AND amount=2110.00 AND category='工资';
UPDATE erp_db.financial_logs SET related_no='PAY-2026-09-3', remark=CONCAT(remark,'；原流水号 EXP20261003003，2026-10-29 按工资所属月改归 2026-09')
 WHERE id=1971 AND related_no='EXP20261003003' AND amount=2044.00 AND category='工资';

-- ============ 操作历史留痕 ============
INSERT INTO erp_db.operation_histories (username, title, method, path, changes, created_at) VALUES
('workbuddy', '本地：27 条历史工资流水按「工资所属月」重新归属（related_no EXP→PAY-<月>-<员工ID>，用户2026-10-29确认）', 'POST', '/api/manual/payroll-month-fix',
'[{"detail":"2月3条(1902/1903/1904)→PAY-2026-02；3月3条(1930-1932)→PAY-2026-03；4月3条(1936-1938)→PAY-2026-04；5月3条(1946-1948)→PAY-2026-05；6月3条(1953-1955)→PAY-2026-06；7月3条(1962-1964)→PAY-2026-07"},{"detail":"8月6条(1975杨帅提成840/1959董庆庆550/1965/1966/1967底薪/1973提成补差3957.06)→PAY-2026-08；9月3条(1969/1970/1971)→PAY-2026-09"},{"detail":"只改 related_no 与备注留痕，金额与 balance_after 未动"}]', NOW(3));

COMMIT;

-- ============ 修后校验 ============
SELECT id, related_no, amount FROM erp_db.financial_logs WHERE category='工资' ORDER BY created_at, id;
SELECT '--- 按所属月汇总（应 2-9 月） ---' s;
SELECT SUBSTRING(related_no,5,7) m, COUNT(*) n, SUM(amount) total FROM erp_db.financial_logs WHERE category='工资' AND related_no LIKE 'PAY-%' GROUP BY m ORDER BY m;
SELECT '--- 仍为 EXP 的工资流水（应 0） ---' s;
SELECT COUNT(*) c FROM erp_db.financial_logs WHERE category='工资' AND related_no NOT LIKE 'PAY-%';
SELECT '--- 总额（应 56782.06 不变） ---' s;
SELECT COUNT(*) n, SUM(amount) total FROM erp_db.financial_logs WHERE category='工资';
