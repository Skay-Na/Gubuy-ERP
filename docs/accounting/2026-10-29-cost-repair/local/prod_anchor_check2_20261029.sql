SELECT '--- yangjie order_items' AS s;
SELECT oi.id, oi.order_id, o.order_no, oi.product_id, oi.unit_cost, oi.quantity
FROM order_items oi JOIN orders o ON oi.order_id=o.id
WHERE o.order_no='202610020002';

SELECT '--- A-class products order_items unit_cost distribution' AS s;
SELECT p.name, oi.unit_cost, COUNT(*) cnt
FROM order_items oi JOIN products p ON oi.product_id=p.id
WHERE p.name IN ('MHSR180SN8-S1(真省)','BCD-525WUSGPZM','BC/BD-100KMC','MG10S50T','F60-33DN6')
GROUP BY p.name, oi.unit_cost ORDER BY p.name, oi.unit_cost;

SELECT '--- B-class products order_items unit_cost distribution' AS s;
SELECT p.name, oi.unit_cost, COUNT(*) cnt
FROM order_items oi JOIN products p ON oi.product_id=p.id
WHERE p.name IN ('35B2','46B2','35PH2','35GW/F2','72PH2','72QM1P')
GROUP BY p.name, oi.unit_cost ORDER BY p.name, oi.unit_cost;

SELECT '--- cost flows by related_no (A-class + yangjie)' AS s;
SELECT id, account_id, related_no, amount, flow_type, category, remark
FROM financial_logs
WHERE related_no IN ('202610020002','202609010001','202609070002','202608100001','202609300001','202609300002')
ORDER BY related_no, id;

SELECT '--- cost flows B-class 39 related_no' AS s;
SELECT id, account_id, related_no, amount, flow_type
FROM financial_logs
WHERE account_id=3 AND related_no IN (
'ORD20260608165054749280','ORD20260608174144110897','ORD20260608203003684056','ORD20260614110810166496','ORD20260614111549256227','ORD20260614111658292924','ORD20260615134406676574','ORD20260616153645550298','ORD20260617175351626817','ORD20260619190709437447','ORD20260622144836590140','ORD20260622180450986371','ORD20260624091951242924','202606260001','202606270001','202606280001','202606290003','202606290004','202607040002','202607060003','202607060004','202607070004','202607130001','202607190001','202607200001','202608310001','202603300001','202604270001','202605280013','202605280014','202605280015','202605280016','202605280017','202605280018','202605300002','202605300003','202605300004')
ORDER BY related_no, id;

SELECT '--- account6 initial stock flows (first 15)' AS s;
SELECT id, account_id, amount, flow_type, category, remark, created_at
FROM financial_logs WHERE account_id=6 ORDER BY id LIMIT 15;

SELECT '--- chain gap summary account 2/3/6' AS s;
SELECT account_id, COUNT(*) gaps, MIN(gap) mingap, MAX(gap) maxgap FROM (
SELECT account_id, balance_after - (amount + @p) AS gap, @p:=balance_after
FROM (SELECT account_id, amount, balance_after FROM financial_logs
      WHERE account_id IN (2,3,6) AND deleted_at IS NULL
      ORDER BY account_id, created_at, id) t, (SELECT @p:=NULL) v) s
WHERE ABS(gap)>0.005 GROUP BY account_id;

SELECT '--- inbound_records for the 11 products' AS s;
SELECT ir.id, ir.product_id, p.name, ir.quantity, ir.unit_cost, ir.total_cost, ir.account_id, ir.created_at
FROM inbound_records ir JOIN products p ON ir.product_id=p.id
WHERE p.name IN ('MHSR180SN8-S1(真省)','BCD-525WUSGPZM','BC/BD-100KMC','MG10S50T','F60-33DN6','35B2','46B2','35PH2','35GW/F2','72PH2','72QM1P')
ORDER BY ir.id;
