-- T16 账单隔离巡检：工程模块与日常经营账必须零耦合
-- 每一条都应该返回 0 行 / 0 值；有输出即为越界。
SELECT '【1】工程费用是否漏进资金流水（related_no/remark 带工程编号）' AS chk, COUNT(*) AS bad
FROM financial_logs
WHERE related_no LIKE 'ENG%' OR related_no LIKE 'GC-%'
   OR remark LIKE '%工程费用%' OR remark LIKE '%工程开销%';

SELECT '【2】资金流水是否被标成工程来源' AS chk, COUNT(*) AS bad
FROM financial_logs WHERE source_type LIKE '%engineer%';

SELECT '【3】工程回款是否冒充销售货款进公户' AS chk, COUNT(*) AS bad
FROM financial_logs WHERE related_no LIKE 'GC-%' AND flow_type IN ('income', 'in');

SELECT '【4】订单表是否被工程单污染（订单号/备注/客户名带工程编号）' AS chk, COUNT(*) AS bad
FROM orders
WHERE order_no LIKE '%GC-2026%' OR trade_in_remark LIKE '%GC-2026%' OR customer_name LIKE '%工程单%';

SELECT '【5】工程费用表是否存在资金账户字段（结构上就不该有）' AS chk, COUNT(*) AS bad
FROM information_schema.columns
WHERE table_schema = 'erp_db' AND table_name = 'engineering_expenses'
  AND column_name IN ('account_id', 'financial_log_id', 'log_id');

SELECT '【6】已签约项目的节点合计 ≠ 合同额（平账破坏）' AS chk, COUNT(*) AS bad
FROM engineering_projects p
WHERE p.deleted_at IS NULL AND p.status IN (2, 3, 4)
  AND p.contract_total > 0
  AND ABS(p.contract_total - (SELECT COALESCE(SUM(t.planned_amount), 0)
                              FROM engineering_payment_terms t
                              WHERE t.project_id = p.id)) > 0.01;

SELECT '【7】回款流水合计 ≠ 项目已实收（派生字段失真）' AS chk, COUNT(*) AS bad
FROM engineering_projects p
WHERE p.deleted_at IS NULL AND p.status IN (2, 3, 4, 5)
  AND ABS(p.received_amount - (SELECT COALESCE(SUM(r.amount), 0)
                               FROM engineering_receipts r
                               WHERE r.project_id = p.id)) > 0.01;

-- 回归基线：工程模块上线日 = 2026-10-29。校验只覆盖「上线后新增的流水」，
-- 因为本机库 2026-10-25 之前的历史数据存在既有的余额链断点（见最后一条信息项），
-- 那些与本次模块无关，混在一起校验只会掩盖真问题。
SELECT '【8】上线后新增流水的净额≠余额（回归）' AS chk, COUNT(*) AS bad FROM (
  SELECT account_id, SUM(amount) AS net,
         (SELECT balance_after FROM financial_logs f2
           WHERE f2.account_id = f1.account_id AND f2.created_at >= '2026-10-29'
           ORDER BY f2.created_at DESC, f2.id DESC LIMIT 1) AS last_bal
  FROM financial_logs f1
  WHERE account_id <> 6 AND created_at >= '2026-10-29'
  GROUP BY account_id
) x
WHERE ABS(net - COALESCE(last_bal, 0)) > 0.01;

SELECT '【9】上线后新增流水的余额链断裂（回归）' AS chk, COUNT(*) AS bad FROM (
  SELECT account_id, balance_after, amount,
         LAG(balance_after) OVER (PARTITION BY account_id ORDER BY created_at, id) AS prev_bal
  FROM financial_logs
  WHERE account_id <> 6 AND created_at >= '2026-10-29'
) t
WHERE prev_bal IS NOT NULL AND ABS(prev_bal + amount - balance_after) > 0.01;

-- 信息项（不计入通过/失败）：全库历史余额链断点，供后续清账参考
select 'info_legacy_chain_breaks' as chk, count(*) as cnt from (
  select account_id, balance_after, amount,
         lag(balance_after) over (partition by account_id order by id) as prev_bal
  from financial_logs
  where account_id <> 6
) t
where prev_bal is not null and abs(prev_bal + amount - balance_after) > 0.01;

-- 正向佐证：工程模块自身的账目规模（这些是应该存在的）
select 'engineering_scale' as chk,
       (select count(*) from engineering_projects where deleted_at is null) as projects,
       (select count(*) from engineering_receipts) as receipts,
       (select count(*) from engineering_expenses where deleted_at is null) as expenses,
       (select count(*) from engineering_shipments) as shipments;
