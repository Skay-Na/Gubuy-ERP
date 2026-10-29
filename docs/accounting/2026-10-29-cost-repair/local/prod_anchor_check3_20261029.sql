SELECT '--- chain gap summary account 2/3/6' AS s;
SELECT account_id, COUNT(*) gaps, MIN(gap) mingap, MAX(gap) maxgap FROM (
SELECT account_id, balance_after - (amount + @p) AS gap, @p:=balance_after
FROM (SELECT account_id, amount, balance_after FROM financial_logs
      WHERE account_id IN (2,3,6)
      ORDER BY account_id, created_at, id) t, (SELECT @p:=NULL) v) s
WHERE ABS(gap)>0.005 GROUP BY account_id;

SELECT '--- account6 six initial flows target' AS s;
SELECT id, account_id, amount, category, remark FROM financial_logs
WHERE account_id=6 AND remark LIKE '%商品 ID: 40%' OR account_id=6 AND id IN (12,15,18,20,30,34);

SELECT '--- BCD-525 inbound/inventory' AS s;
SELECT ir.id, ir.product_id, ir.quantity, ir.unit_cost, ir.total_cost, ir.account_id FROM inbound_records ir WHERE ir.product_id=88;
SELECT id, product_id, unit_cost FROM inventory_logs WHERE product_id=88 ORDER BY id DESC LIMIT 5;
SELECT id, product_id, unit_cost FROM inventory_logs WHERE product_id IN (40,44,47,49,61,66) AND unit_cost IN (2699,3499,2090,3482,5249,6313) ORDER BY id;

SELECT '--- flows 202609240001/240004' AS s;
SELECT id, account_id, related_no, amount, flow_type, category FROM financial_logs
WHERE related_no IN ('202609240001','202609240004');

SELECT '--- current max ids' AS s;
SELECT MAX(id) FROM financial_logs; SELECT account_id, balance_after FROM financial_logs WHERE id IN (SELECT MAX(id) FROM financial_logs GROUP BY account_id);
