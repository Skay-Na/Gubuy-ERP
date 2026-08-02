-- audit-2026-08-02-dryrun-D3.sql
-- 用途：检查 financial_logs.balance_after 是否与 accounts.balance 一致
-- 只读；提供余额重放数据，不修改

USE erp_db;

SELECT
  a.id AS account_id,
  a.name AS account_name,
  a.balance AS current_balance,
  (SELECT balance_after FROM financial_logs
   WHERE account_id = a.id ORDER BY id DESC LIMIT 1) AS last_log_balance,
  (SELECT COUNT(*) FROM financial_logs WHERE account_id = a.id) AS log_count
FROM accounts a
ORDER BY a.id;
