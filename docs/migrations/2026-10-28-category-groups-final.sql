-- 生产迁移：分类体系大分组最终版（2026-10-28）
-- 前置：先执行 2026-10-27-category-two-level.sql（categories 已有 parent_id 列）。
-- 注意：生产自增 ID 与本地不同，本脚本全部按【名称】匹配，可安全重复执行（幂等）。
-- 口径：商品 category_id 一律不动，只改 categories 的 parent_id / name / sort_order。
-- 效果：
--   新建一级：冰洗(90)、厨房电器(75)、厨房小电(70)、环境电器(65)、生活电器(55)
--   一级排序（越大越靠前）：空调=100(勿动) 冰洗=90 洗衣机=85 热水器=80 厨房电器=75
--     厨房小电=70 环境电器=65 电视机=60 生活电器=55 全屋智能=50 门锁=45
--   挂靠：冰箱/冷柜→冰洗；壁挂炉→热水器；电暖油丁/风扇/净水器/茶吧机→环境电器；
--     灶具/烟机/洗碗机/消毒柜/微蒸烤/微波炉→厨房电器；电饭煲/电压力锅/电磁炉/电饼铛/
--     电煮锅/电热锅/电火锅/空气炸锅/电水壶/面包机/破壁机/炒锅→厨房小电；清洁电器→生活电器
--   新建二级（空的，现有商品不动）：电热水器、燃气热水器（挂热水器）
--   洗衣机/电视机/全屋智能/门锁保持一级；空调及其二级分类不改动。

START TRANSACTION;

-- 1. 更新存量一级分类 sort_order（越大越靠前）
UPDATE categories SET sort_order = 85 WHERE name = '洗衣机'   AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET sort_order = 80 WHERE name = '热水器'   AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET sort_order = 60 WHERE name = '电视机'   AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET sort_order = 50 WHERE name = '全屋智能' AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET sort_order = 45 WHERE name = '门锁'     AND parent_id IS NULL AND deleted_at IS NULL;

-- 2. 新建一级分类（幂等：不存在才插入）
INSERT INTO categories (name, sort_order, created_at, updated_at)
SELECT '冰洗', 90, NOW(), NOW() FROM DUAL
WHERE NOT EXISTS (
  SELECT 1 FROM (SELECT id FROM categories WHERE name = '冰洗' AND parent_id IS NULL AND deleted_at IS NULL) t
);
INSERT INTO categories (name, sort_order, created_at, updated_at)
SELECT '厨房电器', 75, NOW(), NOW() FROM DUAL
WHERE NOT EXISTS (
  SELECT 1 FROM (SELECT id FROM categories WHERE name = '厨房电器' AND parent_id IS NULL AND deleted_at IS NULL) t
);
INSERT INTO categories (name, sort_order, created_at, updated_at)
SELECT '厨房小电', 70, NOW(), NOW() FROM DUAL
WHERE NOT EXISTS (
  SELECT 1 FROM (SELECT id FROM categories WHERE name = '厨房小电' AND parent_id IS NULL AND deleted_at IS NULL) t
);
INSERT INTO categories (name, sort_order, created_at, updated_at)
SELECT '环境电器', 65, NOW(), NOW() FROM DUAL
WHERE NOT EXISTS (
  SELECT 1 FROM (SELECT id FROM categories WHERE name = '环境电器' AND parent_id IS NULL AND deleted_at IS NULL) t
);
INSERT INTO categories (name, sort_order, created_at, updated_at)
SELECT '生活电器', 55, NOW(), NOW() FROM DUAL
WHERE NOT EXISTS (
  SELECT 1 FROM (SELECT id FROM categories WHERE name = '生活电器' AND parent_id IS NULL AND deleted_at IS NULL) t
);

-- 3. 取各父分类 ID（按名称）
SET @bx   := (SELECT id FROM categories WHERE name = '冰洗'     AND parent_id IS NULL AND deleted_at IS NULL LIMIT 1);
SET @rsq  := (SELECT id FROM categories WHERE name = '热水器'   AND parent_id IS NULL AND deleted_at IS NULL LIMIT 1);
SET @hj   := (SELECT id FROM categories WHERE name = '环境电器' AND parent_id IS NULL AND deleted_at IS NULL LIMIT 1);
SET @cf   := (SELECT id FROM categories WHERE name = '厨房电器' AND parent_id IS NULL AND deleted_at IS NULL LIMIT 1);
SET @cfxd := (SELECT id FROM categories WHERE name = '厨房小电' AND parent_id IS NULL AND deleted_at IS NULL LIMIT 1);
SET @sh   := (SELECT id FROM categories WHERE name = '生活电器' AND parent_id IS NULL AND deleted_at IS NULL LIMIT 1);

-- 4. 挂靠二级分类（仅当它们还是一级时，幂等；二级 sort_order 在各自父分类内取不重复值，越大越靠前）
-- 冰洗
UPDATE categories SET parent_id = @bx, sort_order = 2 WHERE name = '冰箱' AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @bx, sort_order = 1 WHERE name = '冷柜' AND parent_id IS NULL AND deleted_at IS NULL;
-- 热水器（壁挂炉挂入；新建空的二级 电热水器/燃气热水器，现有商品不动）
UPDATE categories SET parent_id = @rsq, sort_order = 3 WHERE name = '壁挂炉' AND parent_id IS NULL AND deleted_at IS NULL;
INSERT INTO categories (name, parent_id, sort_order, created_at, updated_at)
SELECT '电热水器', @rsq, 2, NOW(), NOW() FROM DUAL
WHERE @rsq IS NOT NULL AND NOT EXISTS (
  SELECT 1 FROM (SELECT id FROM categories WHERE name = '电热水器' AND parent_id = @rsq AND deleted_at IS NULL) t
);
INSERT INTO categories (name, parent_id, sort_order, created_at, updated_at)
SELECT '燃气热水器', @rsq, 1, NOW(), NOW() FROM DUAL
WHERE @rsq IS NOT NULL AND NOT EXISTS (
  SELECT 1 FROM (SELECT id FROM categories WHERE name = '燃气热水器' AND parent_id = @rsq AND deleted_at IS NULL) t
);
-- 环境电器
UPDATE categories SET parent_id = @hj, sort_order = 4 WHERE name = '电暖油丁' AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @hj, sort_order = 3 WHERE name = '风扇'     AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @hj, sort_order = 2 WHERE name = '净水器'   AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @hj, sort_order = 1 WHERE name = '茶吧机'   AND parent_id IS NULL AND deleted_at IS NULL;
-- 厨房电器
UPDATE categories SET parent_id = @cf, sort_order = 6 WHERE name = '灶具'   AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @cf, sort_order = 5 WHERE name = '烟机'   AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @cf, sort_order = 4 WHERE name = '洗碗机' AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @cf, sort_order = 3 WHERE name = '消毒柜' AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @cf, sort_order = 2 WHERE name = '微蒸烤' AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @cf, sort_order = 1 WHERE name = '微波炉' AND parent_id IS NULL AND deleted_at IS NULL;
-- 厨房小电
UPDATE categories SET parent_id = @cfxd, sort_order = 12 WHERE name = '电饭煲'   AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @cfxd, sort_order = 11 WHERE name = '电压力锅' AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @cfxd, sort_order = 10 WHERE name = '电磁炉'   AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @cfxd, sort_order = 9  WHERE name = '电饼铛'   AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @cfxd, sort_order = 8  WHERE name = '电煮锅'   AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @cfxd, sort_order = 7  WHERE name = '电热锅'   AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @cfxd, sort_order = 6  WHERE name = '电火锅'   AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @cfxd, sort_order = 5  WHERE name = '空气炸锅' AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @cfxd, sort_order = 4  WHERE name = '电水壶'   AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @cfxd, sort_order = 3  WHERE name = '面包机'   AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @cfxd, sort_order = 2  WHERE name = '破壁机'   AND parent_id IS NULL AND deleted_at IS NULL;
UPDATE categories SET parent_id = @cfxd, sort_order = 1  WHERE name = '炒锅'     AND parent_id IS NULL AND deleted_at IS NULL;
-- 生活电器
UPDATE categories SET parent_id = @sh, sort_order = 1 WHERE name = '清洁电器' AND parent_id IS NULL AND deleted_at IS NULL;

COMMIT;

-- 验证：输出完整两级分类树（一级在前按 sort_order 降序，二级紧跟父分类）
SELECT c.id, c.name, c.sort_order, c.parent_id,
       (SELECT p.name FROM categories p WHERE p.id = c.parent_id) AS parent_name
FROM categories c
WHERE c.deleted_at IS NULL
ORDER BY (c.parent_id IS NULL) DESC, c.sort_order DESC, c.id ASC;
