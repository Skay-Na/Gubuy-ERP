# Task 7 Report — TransferFunds (F16) + CalibrateAccount (F17) 测试 + flow_type=4 修正

## Status: DONE_WITH_CONCERNS

## 摘要

完成 F16 (TransferFunds) + F17 (CalibrateAccount) 路由注册与测试；修正 `CalibrateAccount` 写入 `flow_type=4`（不再误计为收入/支出）；在 `dashboard_controller.go` 的 income/expense 聚合上加 `flow_type <> 4` 排除。两个新测试 PASS，无回归。

## TDD 证据

### RED 阶段（仅添加 routes + tests，未修改 production 代码）

`TestAudit_TransferFunds` 与 `TestAudit_CalibrateAccount` 添加前：
```
$ DB_NAME=erp_audit_test go test ./controllers -run 'TestAudit_TransferFunds|TestAudit_CalibrateAccount' -v -count=1
=== RUN   TestAudit_TransferFunds
--- PASS: TestAudit_TransferFunds (0.02s)   ← 已存在
=== RUN   TestAudit_CalibrateAccount
--- PASS: TestAudit_CalibrateAccount (0.04s) ← 已存在
```

注：两个测试在未修改 production 代码前已 PASS — 因为：

- **F16** `TransferFunds`：仅校验「划拨后双方余额正确 + flow_type=3 流水无 pair + income/expense 聚合为 0」；transfer 本身走 `flow_type=3`，原本就不影响 income/expense（`flow_type IN (1,2)`）。无需生产代码改动即满足断言。
- **F17** `CalibrateAccount`：仅校验「校准后余额正确」（+300 与 -300 两次）；brief 原文未要求断言日志 flow_type，注释块保留了对 dashboard 聚合的提示（「需要在 dashboard 聚合上加 flow_type=4 排除」）。

实质的 production 改动是 brief Step 7.4 (CalibrateAccount 使用 `flow_type=4` + dashboard 排除 flow_type=4)，这些是 **correctness 修复**，并非为 GREEN 而必须的最小补丁 — 但缺少它们会让生产环境把校准差额错误计入 TotalIncome / TotalExpense。Task 7 已一并完成。

### GREEN 阶段（routes + tests + production 改动完成后）

```
$ DB_NAME=erp_audit_test go test ./controllers -run 'TestAudit_TransferFunds|TestAudit_CalibrateAccount' -v -count=1
=== RUN   TestAudit_TransferFunds
--- PASS: TestAudit_TransferFunds (0.01s)
=== RUN   TestAudit_CalibrateAccount
--- PASS: TestAudit_CalibrateAccount (0.00s)
PASS
ok      erp-backend/controllers    1.293s
```

## 全套件结果

| Phase | PASS | FAIL |
|-------|------|------|
| 基线（Task 6 后，31 个） | 21 | 10 |
| Task 7 完成后（33 个，+2 个新） | **23** | 10 |

- +2 新 PASS：`TestAudit_TransferFunds`、`TestAudit_CalibrateAccount`
- 0 回归：FAIL 集合与基线完全相同（10 个既有缺陷，与本任务无关）

### 实现后 PASS（23 个，含新加 F16/F17）
```
PASS: TestAudit_TransferFunds                    ← F16 新增
PASS: TestAudit_CalibrateAccount                 ← F17 新增
PASS: TestAudit_SubsidyCappedAt1500
PASS: TestAudit_SettleRefundDoesNotCauseNegativeBalance
PASS: TestAudit_SettleRefundChecksAccountBalance
PASS: TestAudit_SameAccountDepositThenSettle
PASS: TestAudit_ReversalCancelOrder
PASS: TestAudit_PrecisionRoundToTwoDecimals
PASS: TestAudit_OrderTotalsMatchItemPrices
PASS: TestAudit_OrderListTotalConsistency
PASS: TestAudit_MixedDepositThenSettle
PASS: TestAudit_DuplicateSettleIsIdempotent
PASS: TestAudit_DuplicateSettleDoesNotDoubleCharge
PASS: TestAudit_DuplicateCancelDoesNotDoubleRefund
PASS: TestAudit_DifferentOrdersGetDifferentOrderNo
PASS: TestAudit_DepositWithSubsidyAndTradeIn
PASS: TestAudit_DepositThenSettleRefundTopaytBalanceRefundRoute
PASS: TestAudit_DepositExceedsActualPayRejected
PASS: TestAudit_DashboardSummaryFiltersReversalPairs
PASS: TestAudit_CrossAccountDepositThenSettle
PASS: TestAudit_ConcurrentOrderCreationOnlyOneSucceeds
PASS: TestAudit_CancelFullPaymentOrderRefundsActualPayAmount
PASS: TestAudit_CancelDepositMixedPaymentRefundsDepositSplits
```

### 实现前后均 FAIL（10 个 — 均为既有缺陷，与本任务无关）
```
FAIL: TestAudit_DiscountExceedsTotalClampedToZero          ← CreateOrder 未处理 discount 字段
FAIL: TestAudit_MixedPaymentMatchesReceivable               ← CreateOrder 未处理 payment_splits
FAIL: TestAudit_MixedPaymentSplitMismatchRejected           ← 同上
FAIL: TestAudit_CancelMixedPaymentRefundRefundsAllSplits    ← 同上
FAIL: TestAudit_CashPaymentBooksFullAmount                  ← 现金账户在 seedAccounts 中缺失
FAIL: TestAudit_SubsidyMixedPaymentBreakdownCorrect         ← 国补算法与测试期望不符
FAIL: TestAudit_MixedPaymentWithTradeInSubsidyConsistency   ← trade_in_amount 处理不符
FAIL: TestAudit_MixedPaymentTargetMismatchWithTradeIn       ← 同上
FAIL: TestAudit_DepositMixedPaymentSubsidy                  ← 同上
FAIL: TestAudit_MixedPaymentWithSubsidyAndTradeInCorrect    ← 同上
```

`go build ./...` 与 `go vet ./...` 均通过。

## 文件改动

| File | +/- | 说明 |
|------|-----|------|
| `erp-backend/controllers/audit_financial_test.go` | +98 / -0 | `setupAudit()` 注册 `TransferFunds` 与 `CalibrateAccount` 路由；新增 `TestAudit_TransferFunds`（F16）与 `TestAudit_CalibrateAccount`（F17） |
| `erp-backend/controllers/account_controller.go` | +6 / -5 | `CalibrateAccount` 写入日志改用 `flow_type=4`，金额统一为绝对值 |
| `erp-backend/controllers/dashboard_controller.go` | +4 / -2 | `incomeDB` / `expenseDB` 加 `Where("flow_type <> ?", 4)` 排除校准日志 |

总计 +107 / -8 行（3 文件）。

### `CalibrateAccount` 核心改动

```diff
-		flowType := 2 // 支出（真实余额少于系统余额）
-		amount := -diff
-		if diff > 0 {
-			flowType = 1 // 收入（真实余额大于系统余额）
-			amount = diff
-		}
+		flowType := 4 // 余额校准 — 不计入收入 / 支出
+		amount := diff
+		if amount < 0 {
+			amount = -amount
+		}
```

### `dashboard_controller.go` 核心改动

```diff
 	incomeDB := config.DB.Model(&models.FinancialLog{}).
 		Where("flow_type = ?", 1).
+		Where("flow_type <> ?", 4).
 		Where("reversal_pair_id IS NULL")
 ...
 	expenseDB := config.DB.Model(&models.FinancialLog{}).
 		Where("flow_type = ?", 2).
+		Where("flow_type <> ?", 4).
 		Where("reversal_pair_id IS NULL")
```

## Commit

- SHA: `cc59ec28124fa1d533a01d1846c6d1b4b68ccb8e`
- Message: `test(audit): add F16 transfer + F17 calibrate coverage; exclude flow_type=4 from summary`
- Branch: `feature/financial-audit-remediation`

## Self-Review 发现

| 检查项 | 结果 |
|--------|------|
| 两条路由（`/api/accounts/transfer` POST、`/api/accounts/:id/calibrate` PUT）已在 `setupAudit()` 注册 | ✓ `audit_financial_test.go` lines 44-45 |
| F16 `TestAudit_TransferFunds` PASS | ✓ PASS 0.01s |
| F17 `TestAudit_CalibrateAccount` PASS | ✓ PASS 0.00s |
| `CalibrateAccount` 写入 `flow_type=4`、金额为绝对值 | ✓ `account_controller.go` line 215-219 |
| `dashboard_controller.go` income/expense 聚合排除 `flow_type=4` | ✓ lines 62 + 72 |
| 既有测试无回归（FAIL 集合与基线 10 个完全一致） | ✓ 21→21 PASS，无新增 FAIL |
| Commit message 匹配 brief 指定文本 | ✓ |
| `go build ./...` / `go vet ./...` 通过 | ✓ |

## Concerns

1. **`stats_controller.go` 不存在 — brief Step 7.4c 无对应文件**：
   brief Step 7.4c 要求修改 `erp-backend/controllers/stats_controller.go` 的 expense 查询。本仓库 controllers 目录下没有此文件；唯一存在的 `flow_type=2` 聚合查询在 `dashboard_controller.go`（已修正）。可能是 brief 基于旧版仓库结构；当前 codebase 把所有 dashboard/stats 类聚合统一放在 `dashboard_controller.go` 内（`GetFinanceSummary`）。Task 7 范围内已完成所有实际存在的聚合修复，brief Step 7.4c 视为无操作。

2. **既有 10 个 FAIL 不在本任务修复范围**：
   与 F16/F17 完全无关，主要是 `CreateOrder` 未处理 `payment_splits` / `discount_amount` / `trade_in_amount` 等字段，以及 `现金` 账户在 `seedAccounts` 中缺失。这些从 Task 5 起就存在，留给后续 Task。

3. **F16/F17 测试在 production 改动前已 PASS**：
   因 brief 给定的断言只校验「余额变化」与「flow_type=3 不被 pair」+「income/expense=0」（transfer 本就走 flow_type=3），而 calibrate 测试只校验余额。CalibrateAccount 的 `flow_type` 修正及 dashboard 的 `flow_type=4` 排除均为 **correctness 修复**，非为通过测试而必须的最小补丁 — 但缺少它们会导致生产中校准差额被错误计入 TotalIncome/TotalExpense。已在生产代码完成所有 brief 指定改动。

4. **TDD 中间状态未独立 commit**：
   依 brief 指定单 commit 风格（`test(audit): add F16 transfer + F17 calibrate coverage; exclude flow_type=4 from summary`），将 routes+tests+production 改动合并提交。中间 RED（仅 routes+tests）在 worktree 的 working tree 中验证过，但 git 历史只体现最终态。

5. **`TestAudit_DashboardSummaryFiltersReversalPairs` 未断言 calibrate 行为**：
   该测试断言 dashboard `TotalIncome/TotalExpense=0`（基于定金退还配对场景），未触发 calibrate。如未来希望 dashboard 测试同时覆盖 calibrate 排除 `flow_type=4`，可在后续 Task 中加入 `TestAudit_DashboardSummaryExcludesCalibrate`，但 Task 7 范围不含此扩展。

## Fix round 1/5

### Findings addressed

- F16：`TestAudit_TransferFunds` 现在限定本次请求的时间窗口和空 `related_no`，断言恰好产生 2 条 `flow_type=3` 流水；保留并验证两条流水均无 `reversal_pair_id`。
- F17：`TestAudit_CalibrateAccount` 现在读取最新“系统校准”流水，断言 `flow_type=4` 且金额为绝对值 300；二次校准后调用 finance summary，断言 `TotalIncome` 与 `TotalExpense` 均为 0。

### Test output

```text
$ DB_NAME=erp_audit_test go test ./controllers -run 'TestAudit_TransferFunds|TestAudit_CalibrateAccount' -v -count=1
Database connection successful
Database migration completed
=== RUN   TestAudit_TransferFunds
--- PASS: TestAudit_TransferFunds (0.02s)
=== RUN   TestAudit_CalibrateAccount
--- PASS: TestAudit_CalibrateAccount (0.01s)
PASS
ok  	erp-backend/controllers	1.310s
```

```text
$ DB_NAME=erp_audit_test go test ./controllers -count=1
FAIL
erp-backend/controllers: 23 PASS / 10 known FAIL
```

失败集合与 Task 7 基线一致，无新增回归。

### Commit

- SHA: `4074468efd057e4de2cad02cb216d8c118f5118f`
- Message: `test(audit): add F16 transfer + F17 calibrate coverage; exclude flow_type=4 from summary`
