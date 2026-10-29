SELECT '--- chain check correct direction (flow_type 1=+ 2=-)' AS s;
SELECT account_id, COUNT(*) gaps FROM (
SELECT account_id, balance_after - (first_bal - first_delta + cum_total) AS gap FROM (
SELECT account_id, amount, balance_after, flow_type,
  SUM(CASE WHEN flow_type=1 THEN amount ELSE -amount END) OVER (PARTITION BY account_id ORDER BY created_at, id ROWS UNBOUNDED PRECEDING) AS cum_total,
  FIRST_VALUE(CASE WHEN flow_type=1 THEN amount ELSE -amount END) OVER (PARTITION BY account_id ORDER BY created_at, id ROWS UNBOUNDED PRECEDING) AS first_delta,
  FIRST_VALUE(balance_after) OVER (PARTITION BY account_id ORDER BY created_at, id ROWS UNBOUNDED PRECEDING) AS first_bal
FROM financial_logs WHERE account_id IN (2,3,6)) t
) s WHERE ABS(gap)>0.005 GROUP BY account_id;

SELECT '--- net check: SUM(dir*amount) vs balance-initial' AS s;
SELECT f.account_id, ROUND(SUM(CASE WHEN f.flow_type=1 THEN f.amount ELSE -f.amount END),2) net,
  a.balance, ROUND(a.balance - SUM(CASE WHEN f.flow_type=1 THEN f.amount ELSE -f.amount END),2) implied_initial
FROM financial_logs f JOIN accounts a ON a.id=f.account_id
WHERE f.account_id IN (2,3,6) GROUP BY f.account_id, a.balance;
