# erpwonchon 财务审计遗留问题修复 — 设计文档

> 状态：草案 v1
> 日期：2026-08-02
> 作者：OpenCode
> 任务来源：`/Users/nanjiakai/Desktop/OpenCode_财务审计遗留问题修复与复核任务.md`
> 前置：现有 `FINANCIAL_AUDIT_REPORT.md` 声称"残留缺陷 0"，但未覆盖同账户定金结尾款、红冲、资金划拨、余额校准、前端 E2E、生产历史数据一致性。本设计纠正该结论并修复真正残留的问题。

---

## 1. 目标

1. 修复"定金结尾款"在同账户、跨账户、混合定金场景下的账务失真（`TotalIncome` 重复计入、`TotalExpense` 出现虚构退款）。
2. 在独立测试库（`erp_audit_test`）内对每一种定金结尾款场景做端到端金额核对：订单 → payment_breakdown → 账户余额 → financial_logs → dashboard/stats 一致性。
3. 补测此前标注 `NOT RUN` 的功能：红冲、资金划拨、余额校准。
4. 用 `node --test` 覆盖前端收银关键计算函数。
5. 对生产库 `erp_db` 做 **只读** dry-run，输出可审阅迁移方案，不执行任何写入。
6. 用更新后的中文复核报告替换现有 `FINANCIAL_AUDIT_REPORT.md`，如实反映每项状态。

## 2. 范围

### 2.1 包含

- 后端 `PayOrderBalance`（`erp-backend/controllers/order_controller.go:982-1177`）的"退定再结全额"流程修复账务语义
- 新增 `FinancialLog.ReversalPairID` 字段及配套 GORM migration
- `Dashboard.GetFinanceSummary`（`dashboard_controller.go:81`）、`Stats.GetMonthlyStats`（`stats_controller.go:32`）聚合查询按 `reversal_pair_id IS NULL` 过滤
- `CancelOrder` / `ReversalCancelOrder` 的退款日志同步标记 pair
- `TransferFunds`、`CalibrateAccount` 的审计可见性验证
- 新增 8 个回归测试（同账户、跨账户、混合定金、定金+国补+trade-in、重复结尾款幂等、红冲、划拨、校准）
- 前端 `CheckoutPanel.vue`、`Cashier.vue` 关键计算函数的 `node --test` 覆盖
- 历史数据 dry-run SQL 脚本（默认不执行）+ 报告
- `FINANCIAL_AUDIT_REPORT.md` 替换

### 2.2 不包含

- 改动 `erp_db` 任何数据（明确禁止 UPDATE/DELETE/INSERT/TRUNCATE/DROP）
- 改动 API 契约、HTTP 路由、请求/响应字段名
- 改动支付方式枚举（`payment_method` 1/2/3/4/5）
- 引入新的前端框架或构建依赖
- 不修改 `Order.payment_breakdown` JSON 结构（保持当前"创建时写入、结清时覆盖为全款拆分"的语义；详见 §5.7）

## 3. 业务模型选择

**采用"保留退定再结全额，修复账务语义"。**

- UI 文案（`OrderManage.vue:470`）明确告诉用户"系统将先退还定金，再按全款金额入账"，这是产品曾有的设计意图。
- 不采用"保留定金、仅收尾款"模型，避免业务侧推翻现有 UI。
- 修复目标：在维持两笔流水（定金→退定金、销售货款）的账务可追溯前提下，让 `TotalIncome` / `TotalExpense` 聚合结果等于客户实付净额。

## 4. 根因

### 4.1 当前实现的账务链路

订单应收 1000、定金 300、结尾款 700（同账户 支付宝）：

```text
步骤 1（CreateOrder）：account.balance += 300；financial_logs +1 行
    {flow_type=1, category=预付定金, amount=300, related_no=ORD001}
步骤 2（PayOrderBalance 第一步）：account.balance -= 300；financial_logs +1 行
    {flow_type=2, category=退还定金, amount=300, related_no=ORD001}
步骤 3（PayOrderBalance 第二步）：account.balance += 1000；financial_logs +1 行
    {flow_type=1, category=销售货款, amount=1000, related_no=ORD001}
```

### 4.2 问题

- 账户余额净变化 = +1000 ✓ 正确
- `financial_logs WHERE flow_type=1` SUM = 300 + 1000 = **1300** ✗ 虚增 300
- `financial_logs WHERE flow_type=2` SUM = **300** ✗ 虚构退款
- `dashboard.TotalIncome` = 1300（虚增）
- `dashboard.TotalExpense` = 300（虚构）
- 订单 `payment_breakdown` 在结清后被覆盖为 `[支付宝 1000]`，丢失定金 300 的明细

### 4.3 为何之前审计未发现

`FINANCIAL_AUDIT_REPORT.md` 的 T6 故意使用跨账户（定金 300 在支付宝，尾款 1000 在公户），规避了同账户问题。`setupAudit()` 注册的路由仅含 `CreateOrder` / `PayOrderBalance` / `CancelOrder`，未注册 `ReversalCancelOrder`，也未对 `GetFinanceSummary` 做断言。`TestAudit_*` 集合未断言 `TotalIncome == actual_pay_amount`。

## 5. 修复设计

### 5.1 核心机制：`reversal_pair_id`

`FinancialLog` 增加列：

```go
type FinancialLog struct {
    ...
    ReversalPairID *uint `gorm:"index;comment:冲销配对ID，配对日志(定金+退定金 / 销售+退款)在汇总时被过滤" json:"reversal_pair_id,omitempty"`
}
```

`reversal_pair_id` 的语义：

- NULL：正常日志，参与 `TotalIncome` / `TotalExpense` 汇总
- 非 NULL：该日志是某个"冲销对"的一部分。**两条**日志（被冲销方 + 冲销方）共享同一个 ID；dashboard / stats 汇总时 `WHERE reversal_pair_id IS NULL` 即可把它们都排除

写入策略：

| 场景 | 被冲销方（先写） | 冲销方（后写） | pair_id 取值 |
|---|---|---|---|
| 结尾款退定金 | 原定金日志 | 新退定金日志 | = 原定金日志.ID |
| 取消订单全退 | 原销售货款日志 | 新退款日志 | = 原销售货款日志.ID |
| 红冲（履约后）| 原销售货款日志 | 新红冲退款日志 | = 原销售货款日志.ID |

实现要点：在 `tx` 内先 `UPDATE financial_logs SET reversal_pair_id = id WHERE id = ?`（把原日志标记为被冲销），再写入冲销日志并设相同 `reversal_pair_id`。事务保证原子性。

### 5.2 聚合查询修改

`dashboard_controller.go:129-145` 的两段查询：

```go
// 修改前
incomeDB := config.DB.Model(&models.FinancialLog{}).Where("flow_type = ?", 1)
incomeDB.Select("COALESCE(SUM(amount), 0)").Scan(&summary.TotalIncome)
expenseDB := config.DB.Model(&models.FinancialLog{}).Where("flow_type = ?", 2)
expenseDB.Select("COALESCE(SUM(amount), 0)").Scan(&summary.TotalExpense)

// 修改后
incomeDB := config.DB.Model(&models.FinancialLog{}).
    Where("flow_type = ?", 1).
    Where("reversal_pair_id IS NULL")
expenseDB := config.DB.Model(&models.FinancialLog{}).
    Where("flow_type = ?", 2).
    Where("reversal_pair_id IS NULL")
```

`stats_controller.go:75-78` 同样增加 `reversal_pair_id IS NULL`。

### 5.3 PayOrderBalance 修改

退款循环内，对每个 split：

```go
// 1) 找到对应的原定金日志
var depositLog models.FinancialLog
if err := tx.Where("related_no = ? AND account_id = ? AND category = ? AND reversal_pair_id IS NULL",
    order.OrderNo, split.AccountID, "预付定金").
    Order("created_at DESC, id DESC").
    First(&depositLog).Error; err != nil {
    // 若找不到（异常路径），回滚报错
    tx.Rollback()
    c.JSON(500, gin.H{"code": 500, "msg": "未找到原定金流水，无法冲销对账"})
    return
}

// 2) 标记原定金日志为"已冲销"
if err := tx.Model(&depositLog).Update("reversal_pair_id", depositLog.ID).Error; err != nil {
    tx.Rollback()
    c.JSON(500, gin.H{"code": 500, "msg": "定金日志冲销标记失败"})
    return
}

// 3) 扣减账户余额（同现有代码 + 负余额校验）
...

// 4) 写退定金流水，并标注同一 pair_id
WriteFinancialLog(tx, split.AccountID, 2, "退还定金", split.Amount,
    updatedAccount.Balance, order.OrderNo, remark, /* reversalPairID */ depositLog.ID)
```

需要扩展 `WriteFinancialLog` 签名，支持可选 `reversalPairID` 参数（使用变参或新增 helper 函数 `WriteReversalLog`）。

### 5.4 CancelOrder / ReversalCancelOrder 修改

`cancelOrderWithOptions` 中的退款循环同样需要先查原销售货款日志（同 `related_no + account_id + category="销售货款"`），再写退款日志并共享 pair_id。

### 5.5 TransferFunds / CalibrateAccount 修改

- 划拨：现有实现写两行 `flow_type=3` 流水，本身对称（in + out 各一条），对 `TotalIncome`/`TotalExpense` 无影响（`flow_type=3` 不计入）。但需要在测试中验证：
  - 两条流水 `reversal_pair_id` 都为 NULL
  - 净账户变化 = 0
  - 不计入 `TotalIncome` / `TotalExpense`
- 校准：写一行 `flow_type=4` 流水。`TotalIncome` / `TotalExpense` 不涉及（聚合只查 `flow_type IN (1,2)`）。需测试 diff 流水、direction、balance_after、连续校准幂等。

### 5.6 GORM AutoMigrate

`models/core_models.go` 加字段后，`config/db.go` 的 `AutoMigrate` 会自动给 `financial_logs` 表加列（开发 / 测试环境）。**生产 `erp_db` 需要手动 ALTER TABLE 配合维护窗口执行**（详见 §7.4 迁移 SQL 中首段）。

### 5.7 payment_breakdown 行为

用户选择"保留并追加结清记录"。本设计的解读是：

- **财务流水表 (`financial_logs`)**：保留完整历史（定金、退定金、销售货款、可能的退款 / 红冲全部保留），通过 `reversal_pair_id` 标记冲销对。这是"保留历史"的载体。
- **订单表 (`orders.payment_breakdown`)**：保持当前"创建时写入、结清时覆盖为全款拆分"的行为，不做 JSON 合并。理由：
  - `payment_breakdown` 是 refund 路径的"当前可退状态"读取源（`getOrderPaymentBreakdown`），如果合并定金 + 尾款，cancel / 红冲会重复退定金
  - 已结清订单退款时需要知道"当前实际收款账户"，合并后该信息模糊
  - 完整审计信息已在 `financial_logs` 中可按 `related_no` 重放
- 不引入 `Order.OriginalPaymentBreakdown` 等新字段；如需查询历史定金拆分，直接基于 `financial_logs` 重建。

如果业务侧后续要求订单详情页同时显示"定金拆分 + 尾款拆分"，由前端在订单详情接口中通过 `related_no` 查询 financial_logs 并聚合展示，本设计不预先改造。

## 6. 测试设计

### 6.1 后端（`erp-backend/controllers/audit_financial_test.go`）

新增用例：

| ID | 名称 | 断言点 |
|---|---|---|
| T-A1 | `TestAudit_SameAccountDepositThenSettle` | 1000 订单、定金 300 支付宝、结清 700 支付宝；TotalIncome=1000、TotalExpense=0、支付宝余额=1000、原定金日志被配对 |
| T-A2 | `TestAudit_CrossAccountDepositThenSettle` | 1000 订单、定金 300 支付宝、结清 1000 公户；TotalIncome=1000、支付宝=0、公户=1000 |
| T-A3 | `TestAudit_MixedDepositThenSettle` | 1000 订单、定金 200 支付宝+100 现金、结清 700 公户；TotalIncome=1000、三账户余额正确 |
| T-A4 | `TestAudit_DepositWithSubsidyAndTradeIn` | 国补+trade-in+定金+结清；汇总=actual_pay |
| T-A5 | `TestAudit_DuplicateSettleIsIdempotent` | 第二次结尾款返回 400；不重复增加余额、不重复写日志 |
| F9 | `TestAudit_ReversalCancelOrder` | 完整支付订单红冲；订单状态=2、库存恢复、账户按 payment_breakdown 拆分退款、TotalIncome=0、TotalExpense=0 |
| F16 | `TestAudit_TransferFunds` | 划拨后两账户净变化=0、两行 flow_type=3 流水、各自 balance_after 正确、不计入 TotalIncome/TotalExpense |
| F17 | `TestAudit_CalibrateAccount` | 校准余额=目标、diff 流水正确、连续校准幂等 |

扩展 `setupAudit()` 注册 `ReversalCancelOrder`、`TransferFunds`、`CalibrateAccount`、`GetFinanceSummary`、`GetFinancialLogs`、`GetMonthlyStats` 路由。

### 6.2 前端（`src/**/*.test.mjs`）

- `src/utils/dashboard.test.mjs`：已有
- `src/views/SystemSettings.test.mjs`：已有
- `src/views/cashier/`：新增 `mixedPaymentTarget` 计算逻辑覆盖（同/异账户、定金模式、国补场景）
- `src/utils/receipt.test.mjs`：已有

### 6.3 前端收银 E2E 范围

按用户选择：用 `node --test` 覆盖关键计算函数，不引入 Playwright。报告里 C4 标注为 `BLOCKED`（无浏览器 E2E 工具），但列出已覆盖的计算函数列表。

## 7. 历史数据 dry-run

### 7.1 D1：payment_breakdown 缺失订单

```sql
SELECT
  id, order_no, total_amount, actual_pay_amount, deposit_amount,
  payment_status, order_status, account_id, created_at
FROM orders
WHERE payment_breakdown IS NULL OR payment_breakdown = ''
ORDER BY created_at DESC;
```

辅助 SQL：基于 `financial_logs` 重建每个订单的支付拆分（按 `related_no + flow_type=1 AND reversal_pair_id IS NULL` 聚合）。

### 7.2 D2：warehouse_type 历史错位

```sql
-- 关联订单的库存流水
SELECT il.id, il.product_id, il.warehouse_type, il.related_no, il.change_qty, o.delivery_method
FROM inventory_logs il
LEFT JOIN orders o ON o.order_no = il.related_no
WHERE il.log_type = 'sale' AND il.warehouse_type NOT IN (1, 2, 3);
```

预期结论：枚举已统一（1/2/3），错位订单数 = 0；如果发现历史错位则单独列出。

### 7.3 D3：financial_logs.balance_after 漂移

按账户分组，逐账户按 ID 顺序重放：

```sql
SELECT id, account_id, flow_type, amount, balance_after, created_at
FROM financial_logs
WHERE account_id = ?
ORDER BY id ASC;
```

按项目规则重算余额（amount 正负取决于 flow_type，参考现有聚合），与 `accounts.balance` 比对，输出差异金额。

### 7.4 迁移 SQL

单独存放在 `erp-backend/scripts/audit-2026-08-02-migration.sql`，默认前缀 `-- SAFETY: COMMENT OUT FOR DRY-RUN`，所有 DDL/DML 用 `-- TODO(uncomment-after-review):` 注释包裹。报告里说明需要：
1. 先做 `mysqldump` 全量备份
2. 在 `erp_db` 维护窗口执行
3. 执行后跑 §7.3 的对账 SQL 确认

## 8. 验证清单（任务 §7）

变更前：
- `git status` 记录 ✓（已在执行）
- 跑新增失败测试 → 修复前 BUG-002 必须 fail

变更后：
- `go vet ./...`
- `go build ./...`
- `DB_NAME=erp_audit_test go test ./controllers -count=1`
- `npm run build`
- `node --test src/**/*.test.mjs`

## 9. 报告交付（任务 §8）

替换 `FINANCIAL_AUDIT_REPORT.md`，结构：

```
1. 结论（纠正"残留缺陷 0"为"残留缺陷 1 → 已修复"）
2. 变更清单（文件 → 修改目的 → 对应问题 → 测试）
3. 定金尾款核对表（T-A1 ~ T-A5 全部结果）
4. 端到端金额核对表（任务 B）
5. 未验证项与真实状态（F9/F16/F17/C4/D1/D2/D3 全部 PASS/FAIL/BLOCKED/NOT RUN）
6. 历史数据 dry-run 报告与迁移方案
7. 真实验证证据（执行的命令与输出摘要）
```

## 10. 完成标准（任务 §9 自检）

- [ ] 同账户、跨账户、混合定金结尾款均先复现、再修复、再验证
- [ ] 定金结尾款不再产生虚假收入/支出
- [ ] 订单、账户余额、财务流水、汇总统计一致
- [ ] 红冲、资金划拨、余额校准均完成实际测试
- [ ] 前端关键计算函数有单元测试覆盖
- [ ] 生产历史数据仅完成只读 dry-run，未被改写
- [ ] 所有缺陷有修复前失败证据 + 修复后回归证据
- [ ] 最终报告如实反映所有未覆盖项

---

## 11. 风险与回退

| 风险 | 影响 | 回退方案 |
|---|---|---|
| `reversal_pair_id` 字段加失败 | AutoMigrate 不生效 | 手动 `ALTER TABLE financial_logs ADD COLUMN reversal_pair_id BIGINT UNSIGNED NULL, ADD INDEX idx_reversal_pair_id` |
| 历史日志已有部分 pair 缺失 | 聚合结果偏差 | dry-run 报告里提供补 pair 的 SQL（默认注释） |
| 现有 `TestAudit_DepositThenSettleRefundTopaytBalanceRefundRoute` 等用例因新逻辑失败 | 测试不绿 | 调整断言以匹配新语义（保留原意图：账户余额正确，补充 TotalIncome/Expense 断言） |
| 修改后 `PayOrderBalance` 找不到原定金日志（异常路径） | 500 报错 | 在退款循环外加容错：若 `DepositAmount > 0` 但查不到定金日志，写一行 `flow_type=4` 校准流水说明差额，避免阻断业务；写完后再继续 |
| 前端 UI 文案与新账务语义不符 | 用户困惑 | `OrderManage.vue:470` 文案补充"账户余额会先减后增，财务上等价于一次性收全款" |

## 12. 不在本设计范围内

- 引入 Prometheus / Sentry / 审计大盘
- 替换 GORM AutoMigrate 为独立 migration 工具
- 改造前端收银 UI 视觉
- 替换财务流水聚合为 OLAP（ClickHouse / DuckDB）

---

*待用户审阅后进入 writing-plans 阶段。*
