-- audit-2026-08-02-dryrun-D1.sql
-- 用途：查找 orders 表中 payment_breakdown 缺失的订单
-- 只读：仅 SELECT，请勿在生产库执行 UPDATE/DELETE

USE erp_db;

SELECT
  id, order_no, total_amount, actual_pay_amount, deposit_amount,
  payment_status, order_status, account_id, created_at
FROM orders
WHERE payment_breakdown IS NULL OR payment_breakdown = ''
ORDER BY created_at DESC;

-- 辅助：基于 financial_logs 重建 payment_breakdown 的视图
SELECT
  o.id AS order_id,
  o.order_no,
  o.payment_status,
  SUM(CASE WHEN fl.flow_type = 1 AND fl.reversal_pair_id IS NULL THEN fl.amount ELSE 0 END) AS sum_income_unpaired,
  SUM(CASE WHEN fl.flow_type = 2 AND fl.reversal_pair_id IS NULL THEN fl.amount ELSE 0 END) AS sum_expense_unpaired,
  COUNT(CASE WHEN fl.flow_type = 1 THEN 1 END) AS income_log_count,
  COUNT(CASE WHEN fl.flow_type = 2 THEN 1 END) AS expense_log_count
FROM orders o
LEFT JOIN financial_logs fl ON fl.related_no = o.order_no
WHERE o.payment_breakdown IS NULL OR o.payment_breakdown = ''
GROUP BY o.id, o.order_no, o.payment_status;
