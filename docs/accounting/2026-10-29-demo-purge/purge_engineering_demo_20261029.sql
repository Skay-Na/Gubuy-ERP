-- 2026-10-29 删除本地工程演示数据（用户要求：演示数据全部删除）
-- 范围：engineering_* 6 表全部（10 个 seed-/walkthrough- 项目）、演示入库 106/107、
--       演示库存事件 546-562、演示操作历史（21:00 之后）、受影响 6 品种恢复生产真实值
-- 生产环境工程 6 表本就 0 行、无演示入库，故本次仅清理本地
SET NAMES utf8mb4;

START TRANSACTION;

-- 0. 删除前快照计数（写入操作历史留痕）
SET @eng_projects := (SELECT COUNT(*) FROM engineering_projects);
SET @eng_items := (SELECT COUNT(*) FROM engineering_project_items);
SET @eng_receipts := (SELECT COUNT(*) FROM engineering_receipts);
SET @eng_expenses := (SELECT COUNT(*) FROM engineering_expenses);
SET @eng_shipments := (SELECT COUNT(*) FROM engineering_shipments);
SET @eng_terms := (SELECT COUNT(*) FROM engineering_payment_terms);
SET @demo_inbound := (SELECT COUNT(*) FROM inbound_records WHERE id IN (106,107));
SET @demo_logs := (SELECT COUNT(*) FROM inventory_logs WHERE id >= 546);
SET @demo_hist := (SELECT COUNT(*) FROM operation_histories WHERE created_at >= '2026-10-29 21:00:00');

-- 1. 工程 6 表清空（先子表后主表）
DELETE FROM engineering_project_items WHERE project_id IN (SELECT id FROM engineering_projects);
DELETE FROM engineering_receipts       WHERE project_id IN (SELECT id FROM engineering_projects);
DELETE FROM engineering_expenses       WHERE project_id IN (SELECT id FROM engineering_projects);
DELETE FROM engineering_shipments      WHERE project_id IN (SELECT id FROM engineering_projects);
DELETE FROM engineering_payment_terms  WHERE project_id IN (SELECT id FROM engineering_projects);
DELETE FROM engineering_projects;

-- 2. 演示入库记录（15 号 +4@6800、18 号 +4@7600）
DELETE FROM inbound_records WHERE id IN (106,107);

-- 3. 演示库存事件（入库 547/549 + 工程出库 546/548/550/551/553/555/556/557/558/559/562）
DELETE FROM inventory_logs WHERE id >= 546;

-- 4. 受影响 6 个品种的库存/成本恢复生产真实值
UPDATE products SET avg_cost=0.00,     latest_cost=0.00,     main_stock=0,  store_stock=0 WHERE id=4;
UPDATE products SET avg_cost=5000.00,  latest_cost=5000.00,  main_stock=3,  store_stock=1 WHERE id=15;
UPDATE products SET avg_cost=15800.00, latest_cost=15800.00, main_stock=4,  store_stock=0 WHERE id=18;
UPDATE products SET avg_cost=2949.00,  latest_cost=2949.00,  main_stock=45, store_stock=3 WHERE id=45;
UPDATE products SET avg_cost=99.00,    latest_cost=99.00,    main_stock=0,  store_stock=7 WHERE id=78;
UPDATE products SET avg_cost=1850.00,  latest_cost=1850.00,  main_stock=24, store_stock=3 WHERE id=332;

-- 5. 演示操作历史（21:00 后的 431 条全部为工程演示走查产生）
DELETE FROM operation_histories WHERE created_at >= '2026-10-29 21:00:00';

-- 6. 留痕快照
INSERT INTO operation_histories (username, title, method, path, changes, created_at)
VALUES ('system', '清理本地工程演示数据',
        'SCRIPT', '/admin/maintenance/purge-engineering-demo',
        JSON_OBJECT('engineering_projects', @eng_projects,
                    'engineering_project_items', @eng_items,
                    'engineering_receipts', @eng_receipts,
                    'engineering_expenses', @eng_expenses,
                    'engineering_shipments', @eng_shipments,
                    'engineering_payment_terms', @eng_terms,
                    'demo_inbound_records', @demo_inbound,
                    'demo_inventory_logs', @demo_logs,
                    'demo_operation_histories', @demo_hist,
                    'note', '生产无演示数据，本次仅清理本地；6 品种库存成本按生产真实值恢复'),
        NOW(3));

COMMIT;
