-- audit-2026-08-02-dryrun-D2.sql
-- 用途：检查 inventory_logs.warehouse_type 历史错位
-- 只读

USE erp_db;

-- 检查 warehouse_type 枚举值是否合法
SELECT warehouse_type, COUNT(*) AS cnt
FROM inventory_logs
GROUP BY warehouse_type
ORDER BY warehouse_type;

-- 关联订单 delivery_method 与库存流水 warehouse_type
SELECT il.id, il.product_id, il.warehouse_type, il.related_no,
       il.change_qty, o.delivery_method
FROM inventory_logs il
LEFT JOIN orders o ON o.order_no = il.related_no
WHERE il.log_type = 'sale'
  AND il.warehouse_type NOT IN (1, 2, 3)
LIMIT 100;
