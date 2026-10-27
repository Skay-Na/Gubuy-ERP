-- 生产迁移：空调二级分类体系（2026-10-28）
-- 前置：先执行 2026-10-27-category-two-level.sql（categories 已有 parent_id 列、name 唯一索引已删）
-- 注意：生产自增 ID 与本地不同，本脚本全部按【名称】匹配，可安全重复执行（幂等）。
-- 效果：新建一级"空调"（排最前），把 挂机/柜机/风管机 挂到其下；
--       多联机→中央空调、空气能→热泵式空气能（改名+挂靠）；新建二级"热风机"。

START TRANSACTION;

-- 1. 建一级"空调"
INSERT INTO categories (name, sort_order, created_at, updated_at)
SELECT '空调', 100, NOW(), NOW() FROM DUAL
WHERE NOT EXISTS (
  SELECT 1 FROM (SELECT id FROM categories WHERE name = '空调' AND parent_id IS NULL AND deleted_at IS NULL) t
);

SET @ac := (SELECT id FROM categories WHERE name = '空调' AND parent_id IS NULL AND deleted_at IS NULL LIMIT 1);

-- 2. 挂子分类（仅当它们还是一级时；sort_order 越大越靠前）
UPDATE categories SET parent_id = @ac, sort_order = 6 WHERE name = '挂机'   AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @ac, sort_order = 5 WHERE name = '柜机'   AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @ac, sort_order = 4 WHERE name = '风管机' AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @ac, sort_order = 3, name = '中央空调'     WHERE name = '多联机' AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @ac, sort_order = 2, name = '热泵式空气能' WHERE name = '空气能' AND parent_id IS NULL AND deleted_at IS NULL;

-- 3. 新建二级"热风机"
INSERT INTO categories (name, parent_id, sort_order, created_at, updated_at)
SELECT '热风机', @ac, 1, NOW(), NOW() FROM DUAL
WHERE @ac IS NOT NULL AND NOT EXISTS (
  SELECT 1 FROM (SELECT id FROM categories WHERE name = '热风机' AND parent_id = @ac AND deleted_at IS NULL) t
);

COMMIT;

-- 验证：应输出 空调 + 六个二级（挂机/柜机/风管机/中央空调/热泵式空气能/热风机）
SELECT id, name, parent_id, sort_order FROM categories
WHERE name = '空调' OR parent_id = (SELECT id FROM (SELECT id FROM categories WHERE name='空调' AND parent_id IS NULL AND deleted_at IS NULL) t)
ORDER BY sort_order DESC;
