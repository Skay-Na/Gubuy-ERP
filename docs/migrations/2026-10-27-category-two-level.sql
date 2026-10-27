-- 2026-10-27 二级分类体系迁移（生产部署时手动执行一次）
-- 背景：categories 增加 parent_id 支持两级分类；name 唯一索引改为普通索引（允许不同大分类下同名二级分类）
-- AutoMigrate 会自动加 parent_id 列，但不会删除已有的唯一索引，因此唯一索引必须手动处理。

-- 1. 删除 name 唯一索引（幂等：不存在则跳过报错可忽略）
ALTER TABLE categories DROP INDEX idx_categories_name;

-- 2. 加 parent_id 列与索引（若 AutoMigrate 已加列，此句会报 Duplicate column，可忽略）
ALTER TABLE categories
  ADD COLUMN parent_id bigint unsigned NULL COMMENT '父分类ID（NULL=一级大分类）' AFTER name,
  ADD INDEX idx_categories_parent_id (parent_id),
  ADD INDEX idx_categories_name (name);

-- 3. 验证：应看到 idx_categories_name Non_unique=1、idx_categories_parent_id 存在
SHOW INDEX FROM categories;
