-- audit-2026-08-02-migration.sql
-- 用途：生产 erp_db 历史数据迁移（添加 reversal_pair_id 列 + 重建 payment_breakdown）
-- 默认全部注释；执行前需：
--   1. mysqldump 全量备份 erp_db
--   2. 在维护窗口内执行
--   3. 执行后跑 audit-2026-08-02-dryrun-D3.sql 对账

USE erp_db;

-- TODO(uncomment-after-review):
-- ALTER TABLE financial_logs
--   ADD COLUMN reversal_pair_id BIGINT UNSIGNED NULL,
--   ADD INDEX idx_financial_logs_reversal_pair_id (reversal_pair_id);

-- TODO(uncomment-after-review):
-- -- 为已发生的退定金操作补充 pair_id（关联原定金日志）
-- UPDATE financial_logs refund
-- JOIN financial_logs deposit
--   ON deposit.related_no = refund.related_no
--   AND deposit.account_id = refund.account_id
--   AND deposit.category = '预付定金'
--   AND refund.category = '退还定金'
--   AND deposit.reversal_pair_id IS NULL
--   AND refund.reversal_pair_id IS NULL
-- SET refund.reversal_pair_id = deposit.id,
--     deposit.reversal_pair_id = deposit.id
-- WHERE refund.id > deposit.id;

-- TODO(uncomment-after-review):
-- -- 为已发生的订单取消/红冲退款补充 pair_id（关联原销售货款日志）
-- UPDATE financial_logs refund
-- JOIN financial_logs collect
--   ON collect.related_no = refund.related_no
--   AND collect.account_id = refund.account_id
--   AND collect.category = '销售货款'
--   AND refund.category IN ('订单退款')
--   AND collect.reversal_pair_id IS NULL
--   AND refund.reversal_pair_id IS NULL
-- SET refund.reversal_pair_id = collect.id,
--     collect.reversal_pair_id = collect.id
-- WHERE refund.id > collect.id;
