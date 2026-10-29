-- ===== 修账后校验 =====
SELECT '--- 1. balance vs accounts (expect 2=22716.00, 3=67366.12, 6=-306467.20)' AS s;
SELECT a.id, a.name, a.balance, ROUND(SUM(CASE WHEN f.flow_type=1 THEN f.amount ELSE -f.amount END),2) AS net
FROM accounts a LEFT JOIN financial_logs f ON f.account_id=a.id
WHERE a.id IN (2,3,6) GROUP BY a.id, a.name, a.balance;

SELECT '--- 2. chain gaps (expect none)' AS s;
SELECT account_id, COUNT(*) gaps FROM (
SELECT account_id, balance_after - (first_bal - first_delta + cum_total) AS gap FROM (
SELECT account_id, amount, balance_after,
  SUM(CASE WHEN flow_type=1 THEN amount ELSE -amount END) OVER (PARTITION BY account_id ORDER BY created_at, id ROWS UNBOUNDED PRECEDING) AS cum_total,
  FIRST_VALUE(CASE WHEN flow_type=1 THEN amount ELSE -amount END) OVER (PARTITION BY account_id ORDER BY created_at, id ROWS UNBOUNDED PRECEDING) AS first_delta,
  FIRST_VALUE(balance_after) OVER (PARTITION BY account_id ORDER BY created_at, id ROWS UNBOUNDED PRECEDING) AS first_bal
FROM financial_logs WHERE account_id IN (2,3,6)) t) s
WHERE ABS(gap)>0.005 GROUP BY account_id;

SELECT '--- 3. spot: yangjie & wangwei' AS s;
SELECT related_no, account_id, amount, remark FROM financial_logs WHERE related_no IN ('202610020002','202608070001') AND flow_type=2 AND category='销售订单采购成本';

SELECT '--- 4. spot: A-class flows' AS s;
SELECT related_no, account_id, amount FROM financial_logs WHERE related_no IN ('202608100001','202609010001','202609070002','202609300001','202609300002') AND flow_type=2 AND category='销售订单采购成本';

SELECT '--- 5. spot: B-class repaired flows (expect 1999/2994/4700/3299/6199/4759)' AS s;
SELECT related_no, amount FROM financial_logs WHERE related_no IN ('ORD20260608165054749280','202606260001','202609240001','202609240004','202603300001','202605280013','202607130001','ORD20260608203003684056') AND flow_type=2 AND category='销售订单采购成本';

SELECT '--- 6. account6 initial flows' AS s;
SELECT id, amount FROM financial_logs WHERE account_id=6 AND id IN (12,15,18,20,30,34) ORDER BY id;

SELECT '--- 7. products avg_cost' AS s;
SELECT id, name, avg_cost FROM products WHERE id IN (32,88,122,138,206,40,44,47,49,61,66) ORDER BY id;

SELECT '--- 8. order_items spot' AS s;
SELECT o.order_no, oi.product_id, oi.unit_cost FROM order_items oi JOIN orders o ON oi.order_id=o.id
WHERE o.order_no IN ('202610020002','202608070001','202609010001','202609300002','202605280013','202603300001','202607130001','202609240004');

SELECT '--- 9. old-value residue check (expect all zero)' AS s;
SELECT
 (SELECT COUNT(*) FROM order_items WHERE product_id=32 AND unit_cost=5566.67) AS oi_32,
 (SELECT COUNT(*) FROM order_items WHERE product_id=206 AND unit_cost IN (549.50,824.25)) AS oi_206,
 (SELECT COUNT(*) FROM order_items WHERE product_id=47 AND unit_cost IN (2090.00,1992.00)) AS oi_47,
 (SELECT COUNT(*) FROM order_items WHERE product_id=44 AND unit_cost=3499.00) AS oi_44,
 (SELECT COUNT(*) FROM order_items WHERE product_id=61 AND unit_cost IN (5249.00,4540.00)) AS oi_61,
 (SELECT COUNT(*) FROM order_items WHERE product_id=49 AND unit_cost=3482.00) AS oi_49,
 (SELECT COUNT(*) FROM order_items WHERE product_id=66 AND unit_cost=6313.00) AS oi_66,
 (SELECT COUNT(*) FROM financial_logs WHERE related_no IN ('202610020002','202608070001','202608100001','202609010001','202609070002','202609300001','202609300002') AND flow_type=2 AND category='销售订单采购成本' AND amount IN (3723.40,1843.27,3711.11,1855.56,249.50,549.50,824.25,1799.33,4379.20)) AS fl_old,
 (SELECT COUNT(*) FROM products WHERE id IN (32,88,122,138,206,44,47,49,61,66) AND avg_cost IN (5566.67,4379.20,249.50,1799.33,824.25,3499.00,2090.00,3482.00,5249.00,6313.00)) AS prod_old;
