-- =====================================================================
-- 生产修账 2026-10-29（同步本地已确认修账 + 王伟单新增确认）
-- 范围：①杨洁(张甜)202610020002 真省 8350 ②王伟202608070001 真省 8350（用户本次确认）
--       ③A类4单零成本稀释 ④B类6品种期初进价+已售成本连带
-- 规则：全部按业务键（order_no / related_no + 旧金额 + 品种守卫）锚定，不依赖裸 ID；
--       排除红冲/退款单（张雷/王攀登/高添 3 单不动）；
--       balance_after 不搬运本地值，按生产流水全量重放。
-- =====================================================================
START TRANSACTION;

-- ---------- ①+② 真省 180L 成本 5566.67→8350（杨洁 + 王伟） ----------
UPDATE order_items oi JOIN orders o ON oi.order_id=o.id
SET oi.unit_cost=8350.00
WHERE o.order_no IN ('202610020002','202608070001') AND oi.product_id=32 AND oi.unit_cost=5566.67;

UPDATE financial_logs SET amount=5585.10, remark='订单 202610020002 采购成本修正 ¥5566.67→¥8350.00（真省180L 按 8-07 入库单价；公户收款 ¥10100.00 分摊）'
WHERE related_no='202610020002' AND account_id=3 AND flow_type=2 AND category='销售订单采购成本' AND amount=3723.40;
UPDATE financial_logs SET amount=2764.90, remark='订单 202610020002 采购成本修正 ¥5566.67→¥8350.00（真省180L 按 8-07 入库单价；本单公户实收部分 ¥5000.00 分摊）'
WHERE related_no='202610020002' AND account_id=3 AND flow_type=2 AND category='销售订单采购成本' AND amount=1843.27;
UPDATE financial_logs SET amount=5566.67, remark='订单 202608070001 采购成本修正 ¥5566.67→¥8350.00（真省180L 按 8-07 入库单价；公户收款 ¥10000.00 分摊）'
WHERE related_no='202608070001' AND account_id=3 AND flow_type=2 AND category='销售订单采购成本' AND amount=3711.11;
UPDATE financial_logs SET amount=2783.33, remark='订单 202608070001 采购成本修正 ¥5566.67→¥8350.00（真省180L 按 8-07 入库单价；本单公户实收部分 ¥5000.00 分摊）'
WHERE related_no='202608070001' AND account_id=3 AND flow_type=2 AND category='销售订单采购成本' AND amount=1855.56;

-- ---------- ③ A类 4 单（零成本库存稀释，用户 2026-10-29 确认实价） ----------
UPDATE order_items oi JOIN orders o ON oi.order_id=o.id SET oi.unit_cost=1099.00
WHERE o.order_no IN ('202609010001','202609070002') AND oi.product_id=206 AND oi.unit_cost IN (549.50,824.25);
UPDATE order_items oi JOIN orders o ON oi.order_id=o.id SET oi.unit_cost=499.00
WHERE o.order_no='202608100001' AND oi.product_id=122 AND oi.unit_cost=249.50;
UPDATE order_items oi JOIN orders o ON oi.order_id=o.id SET oi.unit_cost=2699.00
WHERE o.order_no='202609300001' AND oi.product_id=138 AND oi.unit_cost=1799.33;
UPDATE order_items oi JOIN orders o ON oi.order_id=o.id SET oi.unit_cost=5049.00
WHERE o.order_no='202609300002' AND oi.product_id=88 AND oi.unit_cost=4379.20;

UPDATE financial_logs SET amount=499.00, remark='订单 202608100001 采购成本修正（零成本库存稀释，按用户2026-10-29确认实价）：¥249.50→¥499.00'
WHERE related_no='202608100001' AND account_id=2 AND flow_type=2 AND category='销售订单采购成本' AND amount=249.50;
UPDATE financial_logs SET amount=1099.00, remark='订单 202609010001 采购成本修正（零成本库存稀释，按用户2026-10-29确认实价）：¥549.50→¥1099.00'
WHERE related_no='202609010001' AND account_id=3 AND flow_type=2 AND category='销售订单采购成本' AND amount=549.50;
UPDATE financial_logs SET amount=1099.00, remark='订单 202609070002 采购成本修正（零成本库存稀释，按用户2026-10-29确认实价）：¥824.25→¥1099.00'
WHERE related_no='202609070002' AND account_id=3 AND flow_type=2 AND category='销售订单采购成本' AND amount=824.25;
UPDATE financial_logs SET amount=2699.00, remark='订单 202609300001 采购成本修正（零成本库存稀释，按用户2026-10-29确认实价）：¥1799.33→¥2699.00'
WHERE related_no='202609300001' AND account_id=3 AND flow_type=2 AND category='销售订单采购成本' AND amount=1799.33;
UPDATE financial_logs SET amount=5049.00, remark='订单 202609300002 采购成本修正（零成本库存稀释，按用户2026-10-29确认实价）：¥4379.20→¥5049.00'
WHERE related_no='202609300002' AND account_id=3 AND flow_type=2 AND category='销售订单采购成本' AND amount=4379.20;

-- BCD-525 入库价更正（4台 5474→5049）
UPDATE inbound_records SET unit_cost=5049.00, total_cost=20196.00 WHERE id=101 AND product_id=88 AND unit_cost=5474.00;
UPDATE inventory_logs SET unit_cost=5049.00 WHERE product_id=88 AND unit_cost=5474.00;

-- ---------- ④ B类 6 品种期初建库存（5-16 批次） ----------
UPDATE inbound_records SET unit_cost=2213.00, total_cost=115076.00 WHERE id=12 AND product_id=40 AND unit_cost=2699.00;
UPDATE inbound_records SET unit_cost=2994.00, total_cost=74850.00  WHERE id=15 AND product_id=44 AND unit_cost=3499.00;
UPDATE inbound_records SET unit_cost=1999.00, total_cost=133933.00 WHERE id=18 AND product_id=47 AND unit_cost=2090.00;
UPDATE inbound_records SET unit_cost=3299.00, total_cost=85774.00  WHERE id=20 AND product_id=49 AND unit_cost=3482.00;
UPDATE inbound_records SET unit_cost=4700.00, total_cost=136300.00 WHERE id=30 AND product_id=61 AND unit_cost=5249.00;
UPDATE inbound_records SET unit_cost=6199.00, total_cost=24796.00  WHERE id=34 AND product_id=66 AND unit_cost=6313.00;

UPDATE financial_logs SET amount=115076.00, remark='期初建库存成本修正（用户2026-10-29逐品种确认真实进价）：¥2699.00→¥2213.00 x 52件' WHERE id=12 AND account_id=6 AND amount=140348.00;
UPDATE financial_logs SET amount=74850.00,  remark='期初建库存成本修正（用户2026-10-29逐品种确认真实进价）：¥3499.00→¥2994.00 x 25件' WHERE id=15 AND account_id=6 AND amount=87475.00;
UPDATE financial_logs SET amount=133933.00, remark='期初建库存成本修正（用户2026-10-29逐品种确认真实进价）：¥2090.00→¥1999.00 x 67件' WHERE id=18 AND account_id=6 AND amount=140030.00;
UPDATE financial_logs SET amount=85774.00,  remark='期初建库存成本修正（用户2026-10-29逐品种确认真实进价）：¥3482.00→¥3299.00 x 26件' WHERE id=20 AND account_id=6 AND amount=90532.00;
UPDATE financial_logs SET amount=136300.00, remark='期初建库存成本修正（用户2026-10-29逐品种确认真实进价）：¥5249.00→¥4700.00 x 29件' WHERE id=30 AND account_id=6 AND amount=152221.00;
UPDATE financial_logs SET amount=24796.00,  remark='期初建库存成本修正（用户2026-10-29逐品种确认真实进价）：¥6313.00→¥6199.00 x 4件'  WHERE id=34 AND account_id=6 AND amount=25252.00;

-- B类已售明细（订单号白名单，红冲/退款单已排除）
UPDATE order_items oi JOIN orders o ON oi.order_id=o.id SET oi.unit_cost=2994.00
WHERE oi.product_id=44 AND oi.unit_cost=3499.00 AND o.order_no IN
('ORD20260622180450986371','202606260001','202606290003','202606290004');
UPDATE order_items oi JOIN orders o ON oi.order_id=o.id SET oi.unit_cost=1999.00
WHERE oi.product_id=47 AND oi.unit_cost=2090.00 AND o.order_no IN
('ORD20260608165054749280','ORD20260614110810166496','ORD20260614111549256227','ORD20260614111658292924','ORD20260616153645550298','ORD20260617175351626817','ORD20260619190709437447','ORD20260622144836590140','ORD20260624091951242924','202606280001','202607060003','202607060004','202607070004','202607190001');
UPDATE order_items oi JOIN orders o ON oi.order_id=o.id SET oi.unit_cost=1999.00
WHERE oi.product_id=47 AND oi.unit_cost=1992.00 AND o.order_no IN
('202605280013','202605280014','202605280015','202605280016','202605280017','202605280018','202605300002','202605300003','202605300004');
UPDATE order_items oi JOIN orders o ON oi.order_id=o.id SET oi.unit_cost=4700.00
WHERE oi.product_id=61 AND oi.unit_cost=5249.00 AND o.order_no IN
('ORD20260608203003684056','ORD20260615134406676574','202606270001','202607040002','202608310001','202609240001');
UPDATE order_items oi JOIN orders o ON oi.order_id=o.id SET oi.unit_cost=4700.00
WHERE oi.product_id=61 AND oi.unit_cost=4540.00 AND o.order_no IN ('202603300001','202604270001');
UPDATE order_items oi JOIN orders o ON oi.order_id=o.id SET oi.unit_cost=3299.00
WHERE oi.product_id=49 AND oi.unit_cost=3482.00 AND o.order_no IN ('ORD20260608174144110897','202609240004');
UPDATE order_items oi JOIN orders o ON oi.order_id=o.id SET oi.unit_cost=6199.00
WHERE oi.product_id=66 AND oi.unit_cost=6313.00 AND o.order_no IN ('202607130001','202607200001');

-- B类成本流水 39 条（按 related_no + 旧金额守卫，公户）
UPDATE financial_logs SET amount=1999.00 WHERE related_no IN
('ORD20260608165054749280','ORD20260614110810166496','ORD20260614111549256227','ORD20260614111658292924','ORD20260616153645550298','ORD20260617175351626817','ORD20260619190709437447','ORD20260622144836590140','ORD20260624091951242924','202606280001','202607060003','202607060004','202607070004','202607190001')
AND account_id=3 AND flow_type=2 AND category='销售订单采购成本' AND amount=2090.00;
UPDATE financial_logs SET amount=1999.00 WHERE related_no IN
('202605280013','202605280014','202605280015','202605280016','202605280017','202605280018','202605300002','202605300003','202605300004')
AND account_id=3 AND flow_type=2 AND category='销售订单采购成本' AND amount=1992.00;
UPDATE financial_logs SET amount=2994.00 WHERE related_no IN
('ORD20260622180450986371','202606260001','202606290003','202606290004')
AND account_id=3 AND flow_type=2 AND category='销售订单采购成本' AND amount=3499.00;
UPDATE financial_logs SET amount=4700.00 WHERE related_no IN
('ORD20260615134406676574','202606270001','202607040002','202608310001','202609240001')
AND account_id=3 AND flow_type=2 AND category='销售订单采购成本' AND amount=5249.00;
UPDATE financial_logs SET amount=4759.00, remark='订单 ORD20260608203003684056 采购成本修正：72PH2 ¥5249→¥4700（本单另含其他商品成本 ¥59.00，合计 ¥5308.00→¥4759.00）'
WHERE related_no='ORD20260608203003684056' AND account_id=3 AND flow_type=2 AND category='销售订单采购成本' AND amount=5308.00;
UPDATE financial_logs SET amount=4700.00 WHERE related_no IN ('202603300001','202604270001')
AND account_id=3 AND flow_type=2 AND category='销售订单采购成本' AND amount=4540.00;
UPDATE financial_logs SET amount=3299.00 WHERE related_no IN ('ORD20260608174144110897','202609240004')
AND account_id=3 AND flow_type=2 AND category='销售订单采购成本' AND amount=3482.00;
UPDATE financial_logs SET amount=6199.00 WHERE related_no IN ('202607130001','202607200001')
AND account_id=3 AND flow_type=2 AND category='销售订单采购成本' AND amount=6313.00;

-- B类 inventory_logs 全事件统一真实进价
UPDATE inventory_logs SET unit_cost=2213.00 WHERE product_id=40 AND unit_cost=2699.00;
UPDATE inventory_logs SET unit_cost=2994.00 WHERE product_id=44 AND unit_cost=3499.00;
UPDATE inventory_logs SET unit_cost=1999.00 WHERE product_id=47 AND unit_cost IN (2090.00,1992.00);
UPDATE inventory_logs SET unit_cost=3299.00 WHERE product_id=49 AND unit_cost=3482.00;
UPDATE inventory_logs SET unit_cost=4700.00 WHERE product_id=61 AND unit_cost IN (5249.00,4540.00);
UPDATE inventory_logs SET unit_cost=6199.00 WHERE product_id=66 AND unit_cost=6313.00;

-- ---------- ⑤ products.avg_cost ----------
UPDATE products SET avg_cost=8350.00 WHERE id=32 AND avg_cost=5566.67;
UPDATE products SET avg_cost=5049.00 WHERE id=88 AND avg_cost=4379.20;
UPDATE products SET avg_cost=499.00  WHERE id=122 AND avg_cost=249.50;
UPDATE products SET avg_cost=2699.00 WHERE id=138 AND avg_cost=1799.33;
UPDATE products SET avg_cost=1099.00 WHERE id=206 AND avg_cost=824.25;
UPDATE products SET avg_cost=2994.00 WHERE id=44 AND avg_cost=3499.00;
UPDATE products SET avg_cost=1999.00 WHERE id=47 AND avg_cost=2090.00;
UPDATE products SET avg_cost=3299.00 WHERE id=49 AND avg_cost=3482.00;
UPDATE products SET avg_cost=4700.00 WHERE id=61 AND avg_cost=5249.00;
UPDATE products SET avg_cost=6199.00 WHERE id=66 AND avg_cost=6313.00;

-- ---------- ⑥ 余额链全量重放（账户 2/3/6，期初 0 已验证） ----------
UPDATE financial_logs f
JOIN (
  SELECT id, first_bal - first_delta + cum_total AS new_bal FROM (
    SELECT id,
      SUM(CASE WHEN flow_type=1 THEN amount ELSE -amount END) OVER (PARTITION BY account_id ORDER BY created_at, id ROWS UNBOUNDED PRECEDING) AS cum_total,
      FIRST_VALUE(CASE WHEN flow_type=1 THEN amount ELSE -amount END) OVER (PARTITION BY account_id ORDER BY created_at, id ROWS UNBOUNDED PRECEDING) AS first_delta,
      FIRST_VALUE(balance_after) OVER (PARTITION BY account_id ORDER BY created_at, id ROWS UNBOUNDED PRECEDING) AS first_bal
    FROM financial_logs WHERE account_id IN (2,3,6)
  ) x
) s ON f.id = s.id
SET f.balance_after = ROUND(s.new_bal, 2);

UPDATE accounts SET balance = (
  SELECT ROUND(SUM(CASE WHEN flow_type=1 THEN amount ELSE -amount END),2)
  FROM financial_logs WHERE account_id = accounts.id
) WHERE id IN (2,3,6);

-- ---------- ⑦ 修账快照 ----------
INSERT INTO historical_cost_repairs (product_id, order_no, previous_unit_cost, unit_cost, repaired_order_count, repaired_sale_count, stock_qty, cost_baseline, remark, created_at) VALUES
(32, '202610020002', 5566.67, 8350.00, 1, 0, 1, 0, '杨洁（张甜）202610020002 成本更正：5566.67（16,700入库成本误摊3台库存）→8350.00（8-07入库单价）；经用户2026-10-29确认；生产同步', NOW(3)),
(32, '202608070001', 5566.67, 8350.00, 1, 0, 1, 0, '王伟 202608070001 成本更正：5566.67→8350.00（与杨洁单同款稀释，生产同步时发现并经用户2026-10-29确认补修）', NOW(3)),
(206, '202609010001', 549.50, 1099.00, 1, 0, 0, 0, '零成本库存稀释修正：员工增库0元库存摊薄移动加权，按用户2026-10-29确认实价更正（生产同步）', NOW(3)),
(206, '202609070002', 824.25, 1099.00, 1, 0, 0, 0, '零成本库存稀释修正：员工增库0元库存摊薄移动加权，按用户2026-10-29确认实价更正（生产同步）', NOW(3)),
(122, '202608100001', 249.50, 499.00, 1, 0, 0, 0, '零成本库存稀释修正：员工增库0元库存摊薄移动加权，按用户2026-10-29确认实价更正（生产同步）', NOW(3)),
(138, '202609300001', 1799.33, 2699.00, 1, 0, 0, 0, '零成本库存稀释修正：员工增库0元库存摊薄移动加权，按用户2026-10-29确认实价更正（生产同步）', NOW(3)),
(88, '202609300002', 4379.20, 5049.00, 1, 0, 0, 0, '零成本库存稀释修正：员工增库0元库存摊薄移动加权，按用户2026-10-29确认实价更正（生产同步）', NOW(3)),
(40, 'INITIAL-STOCK', 2699.00, 2213.00, 0, 0, 0, 0, '期初建库存进价修正（5-16批次）：35B2 2699.00→2213.00，用户2026-10-29核对表确认（生产同步；avg_cost已是2213无需改）', NOW(3)),
(44, 'INITIAL-STOCK', 3499.00, 2994.00, 0, 0, 0, 0, '期初建库存进价修正（5-16批次）：46B2 3499.00→2994.00，用户2026-10-29核对表确认（生产同步）', NOW(3)),
(47, 'INITIAL-STOCK', 2090.00, 1999.00, 0, 0, 0, 0, '期初建库存进价修正（5-16批次）：35PH2 2090.00→1999.00，用户2026-10-29核对表确认（生产同步）', NOW(3)),
(49, 'INITIAL-STOCK', 3482.00, 3299.00, 0, 0, 0, 0, '期初建库存进价修正（5-16批次）：35GW/F2 3482.00→3299.00，用户2026-10-29核对表确认（生产同步）', NOW(3)),
(61, 'INITIAL-STOCK', 5249.00, 4700.00, 0, 0, 0, 0, '期初建库存进价修正（5-16批次）：72PH2 5249.00→4700.00，用户2026-10-29核对表确认（生产同步）', NOW(3)),
(66, 'INITIAL-STOCK', 6313.00, 6199.00, 0, 0, 0, 0, '期初建库存进价修正（5-16批次）：72QM1P 6313.00→6199.00，用户2026-10-29核对表确认（生产同步）', NOW(3)),
(44, 'BATCH-INITCOST', 3499.00, 2994.00, 0, 0, 0, 0, '已售成本修正（期初进价连带）：46B2 6月系统单4条，用户2026-10-29核对表确认（生产同步）', NOW(3)),
(47, 'BATCH-INITCOST', 2090.00, 1999.00, 0, 0, 0, 0, '已售成本修正（期初进价连带）：35PH2 6月系统单14条，用户2026-10-29核对表确认（生产同步）', NOW(3)),
(47, 'BATCH-INITCOST', 1992.00, 1999.00, 0, 0, 0, 0, '已售成本修正（期初进价连带）：35PH2 5月导入单9条，用户2026-10-29核对表确认（生产同步）', NOW(3)),
(49, 'BATCH-INITCOST', 3482.00, 3299.00, 0, 0, 0, 0, '已售成本修正（期初进价连带）：35GW/F2 2条，用户2026-10-29核对表确认（生产同步）', NOW(3)),
(61, 'BATCH-INITCOST', 5249.00, 4700.00, 0, 0, 0, 0, '已售成本修正（期初进价连带）：72PH2 6条，用户2026-10-29核对表确认（生产同步）', NOW(3)),
(61, 'BATCH-INITCOST', 4540.00, 4700.00, 0, 0, 0, 0, '已售成本修正（期初进价连带）：72PH2 3-4月导入单2条，用户2026-10-29核对表确认（生产同步）', NOW(3)),
(66, 'BATCH-INITCOST', 6313.00, 6199.00, 0, 0, 0, 0, '已售成本修正（期初进价连带）：72QM1P 2条，用户2026-10-29核对表确认（生产同步）', NOW(3));

SET @bal2 = (SELECT balance FROM accounts WHERE id=2);
SET @bal3 = (SELECT balance FROM accounts WHERE id=3);
SET @bal6 = (SELECT balance FROM accounts WHERE id=6);

INSERT INTO operation_histories (username, title, method, path, changes, created_at) VALUES
('workbuddy', '生产同步：杨洁单+王伟单真省成本8350+A类稀释4单+B类期初进价6品种（用户2026-10-29确认；余额链按生产数据全量重放）', 'POST', '/api/manual/cost-fix-prod-sync',
CONCAT('[{"t":"accounts","id":2,"after_balance":',@bal2,'},{"t":"accounts","id":3,"after_balance":',@bal3,'},{"t":"accounts","id":6,"after_balance":',@bal6,'},{"detail":"杨洁202610020002 oi5566.67→8350 fl689 3723.40→5585.10 fl690 1843.27→2764.90"},{"detail":"王伟202608070001 oi 5566.67→8350 fl 3711.11→5566.67 fl 1855.56→2783.33"},{"detail":"A类4单5明细+BCD-525入库5474→5049"},{"detail":"B类6品种：inbound6条+台账流水6条+已售39条+成本流水39条+inventory全事件+avg_cost"}]'), NOW(3));
COMMIT;
