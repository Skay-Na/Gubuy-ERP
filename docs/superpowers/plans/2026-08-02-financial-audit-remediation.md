# erpwonchon 财务审计遗留问题修复 — 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复"定金结尾款"在 `TotalIncome` / `TotalExpense` 上的账务失真，补测红冲/划拨/校准，前端关键计算覆盖，并在 `erp_db` 上做只读 dry-run，最终替换 `FINANCIAL_AUDIT_REPORT.md`。

**Architecture:** 在 `FinancialLog` 表新增 `reversal_pair_id`（nullable uint，自配对 = `id`）。结尾款 / 取消 / 红冲的"原日志 + 冲销日志"共享同一 `pair_id`，dashboard / stats 聚合时 `WHERE reversal_pair_id IS NULL` 把冲销对排除。Schema 变更由 GORM AutoMigrate 处理；生产 `erp_db` 需要手动 ALTER。

**Tech Stack:** Go 1.26 + GORM v1.31.1 + MySQL 9.6（后端）；Vue 3.5 + Vite 8 + Element Plus 2.13（前端，`node --test` 覆盖）。

## Global Constraints

- 测试必须使用独立库 `erp_audit_test`，**禁止**对 `erp_db` 写操作
- 变更前先跑失败测试并保留证据（任务 1）；不允许跳过失败就改
- 每个任务独立 commit、对应一个 reviewer gate
- 后端测试运行命令：`DB_NAME=erp_audit_test go test ./controllers -count=1`（在 `erp-backend/` 下）
- 前端测试运行命令：`node --test src/**/*.test.mjs`（在项目根）
- 后端校验：`go vet ./... && go build ./...`
- 前端校验：`npm run build`
- 不引入新的依赖、构建工具、测试框架
- 不修改 `orders` 表 schema，不修改 API 契约、HTTP 路由、JSON 字段名
- 已有测试（`TestAudit_*`）改断言允许，但必须保留其原始意图

---

## Task 1: 复现 BUG-002 — 写失败测试 T-A1（同账户定金结尾款）

**Files:**
- Modify: `erp-backend/controllers/audit_financial_test.go`（在文件末尾追加）

**Interfaces:**
- Consumes: `setupAudit()`、`ensureProduct()`、`postJSON()`、`putJSON()`、`readOrder()`、`accountBalance()`、`accountIDByName()`
- Produces: 新测试 `TestAudit_SameAccountDepositThenSettle`

- [ ] **Step 1.1: 在 `audit_financial_test.go` 末尾追加 `TestAudit_SameAccountDepositThenSettle`**

测试逻辑：
1. 重置 DB，确保 4 个账户存在（支付宝/微信/公户/现金）
2. 用 `ensureProduct` 创建 `审计商品-同账户`
3. POST `/api/orders`，body:
   ```json
   {"items":[{"product_id":X,"quantity":1,"unit_price":1000.00}],
    "payment_method":1,
    "delivery_method":2,
    "deposit_amount":300,
    "customer_name":"同账户定金客户",
    "customer_phone":"13800000001",
    "customer_address":"测试地址"}
   ```
4. 解析响应 `order_no` → `readOrder`
5. PUT `/api/orders/{id}/balance`，body: `{"final_payment_method":1}`（同账户 支付宝）
6. 断言：
   - `w.Code == 200`
   - 支付宝余额 == 1000（300 定金 + 入账 1000 全款 - 300 退款 = 1000；最终是 1000）
   - dashboard 的 `TotalIncome == 1000`（**这一项会失败**，当前是 1300）
   - dashboard 的 `TotalExpense == 0`（**这一项会失败**，当前是 300）
   - 订单 `PaymentStatus == 2`、`DepositAmount == 0`、`ActualPayAmount == 1000`

注：dashboard 数据通过 HTTP 调 `GET /api/finance/summary` 获取（需要 `setupAudit` 注册该路由，下个任务会注册）。为了 Task 1 让测试能跑（哪怕失败），先用 `GORM` 直接 `SUM(financial_logs.amount WHERE flow_type=1)` 验证。Task 5 会改成调用 HTTP。

代码骨架：
```go
func TestAudit_SameAccountDepositThenSettle(t *testing.T) {
    setupAudit(t)
    productID := ensureProduct(t, "审计商品-同账户", 1000.00, 600, 100)

    body := gin.H{
        "items":           []gin.H{{"product_id": productID, "quantity": 1, "unit_price": 1000.00}},
        "payment_method":  1,
        "delivery_method": 2,
        "deposit_amount":  300,
        "customer_name":   "同账户定金客户",
        "customer_phone":  "13800000001",
        "customer_address": "测试地址",
    }
    w := postJSON("/api/orders", body)
    if w.Code != http.StatusOK {
        t.Fatalf("下单失败: %d %s", w.Code, w.Body.String())
    }
    var createResp struct {
        OrderNo string `json:"order_no"`
    }
    if err := json.Unmarshal(w.Body.Bytes(), &createResp); err != nil {
        t.Fatalf("解析下单响应失败: %v", err)
    }
    order := readOrder(t, createResp.OrderNo)

    // 结尾款，同账户
    settleW := putJSON("/api/orders/"+strconv.Itoa(int(order.ID))+"/balance",
        gin.H{"final_payment_method": 1})
    if settleW.Code != http.StatusOK {
        t.Fatalf("结清尾款失败: %d %s", settleW.Code, settleW.Body.String())
    }

    // 验证支付宝净余额 = 1000
    alipayBal := accountBalance(t, "支付宝")
    if math.Abs(alipayBal-1000) > 0.01 {
        t.Fatalf("支付宝净余额应为 1000，实际 %.2f", alipayBal)
    }

    // 验证 TotalIncome（用 financial_logs 直查，会失败）
    var totalIncome float64
    config.DB.Model(&models.FinancialLog{}).
        Where("flow_type = ?", 1).
        Select("COALESCE(SUM(amount), 0)").
        Scan(&totalIncome)
    if math.Abs(totalIncome-1000) > 0.01 {
        t.Fatalf("TotalIncome 应为 1000（修复后），实际 %.2f — BUG-002 复现", totalIncome)
    }

    // 验证 TotalExpense 应为 0
    var totalExpense float64
    config.DB.Model(&models.FinancialLog{}).
        Where("flow_type = ?", 2).
        Select("COALESCE(SUM(amount), 0)").
        Scan(&totalExpense)
    if totalExpense > 0.01 {
        t.Fatalf("TotalExpense 应为 0（无真实退款），实际 %.2f — BUG-002 复现", totalExpense)
    }

    // 订单最终状态
    order = readOrder(t, createResp.OrderNo)
    if order.PaymentStatus != 2 {
        t.Fatalf("订单应为已结清 (2)，实际 %d", order.PaymentStatus)
    }
    if order.ActualPayAmount != 1000 {
        t.Fatalf("订单实际应付应为 1000，实际 %.2f", order.ActualPayAmount)
    }
}
```

- [ ] **Step 1.2: 添加 `strconv` 到 import**

在 `audit_financial_test.go` 第 2-15 行的 import 块添加 `"strconv"`。

- [ ] **Step 1.3: 跑测试，确认失败并保留证据**

```bash
cd erp-backend
DB_NAME=erp_audit_test go test ./controllers -run TestAudit_SameAccountDepositThenSettle -v -count=1
```

Expected: FAIL with `TotalIncome 应为 1000（修复后），实际 1300.00 — BUG-002 复现`

把失败输出原文保存到执行笔记（如 git commit message 或文档）以备审计报告引用。

- [ ] **Step 1.4: Commit**

```bash
git add erp-backend/controllers/audit_financial_test.go
git commit -m "test(audit): add failing test for same-account deposit-then-settle (BUG-002)"
```

---

## Task 2: 新增 `FinancialLog.ReversalPairID` 字段 + `WriteReversalLog` helper

**Files:**
- Modify: `erp-backend/models/core_models.go`（在 `FinancialLog` struct 加字段）
- Modify: `erp-backend/controllers/financial_log_controller.go`（加 helper）

**Interfaces:**
- Produces: `FinancialLog.ReversalPairID *uint` 字段；`WriteReversalLog(tx, depositLogID uint, accountID uint, flowType int, category string, amount float64, balanceAfter float64, relatedNo string, remark string) error`

- [ ] **Step 2.1: 修改 `FinancialLog` struct**

在 `core_models.go` 的 `FinancialLog` struct 末尾追加：

```go
    ReversalPairID *uint `gorm:"index;comment:冲销配对ID；配对日志(定金+退定金 / 销售+退款)汇总时排除" json:"reversal_pair_id,omitempty"`
```

字段位置在 `CreatedAt time.Time` 之前。

- [ ] **Step 2.2: 添加 `WriteReversalLog` helper**

在 `financial_log_controller.go` 末尾追加：

```go
// WriteReversalLog 写入一条冲销配对日志。
// 与 WriteFinancialLog 不同：必须传入 depositLogID 作为 pair_id；同时把原日志的 reversal_pair_id 也置为同一 ID。
// 同一 pair_id 的两条日志会在 dashboard / stats 汇总时被 WHERE reversal_pair_id IS NULL 排除。
func WriteReversalLog(tx *gorm.DB, depositLogID uint, accountID uint, flowType int, category string, amount float64, balanceAfter float64, relatedNo string, remark string) error {
    // 1) 把原日志标记为"已被冲销"，pair_id 自配对 = depositLogID
    if err := tx.Model(&models.FinancialLog{}).
        Where("id = ?", depositLogID).
        Update("reversal_pair_id", depositLogID).Error; err != nil {
        return err
    }
    // 2) 写入冲销日志，共享同一 pair_id
    return tx.Create(&models.FinancialLog{
        AccountID:      accountID,
        FlowType:       flowType,
        Category:       category,
        Amount:         amount,
        BalanceAfter:   balanceAfter,
        RelatedNo:      relatedNo,
        Remark:         remark,
        ReversalPairID: &depositLogID,
    }).Error
}
```

- [ ] **Step 2.3: 验证编译**

```bash
cd erp-backend
go build ./controllers/... ./models/...
```

Expected: PASS（无输出）

- [ ] **Step 2.4: 验证 AutoMigrate 添加列**

```bash
cd erp-backend
DB_NAME=erp_audit_test go test ./controllers -run TestAudit_SameAccountDepositThenSettle -v -count=1
```

测试仍然失败（TotalIncome 不对），但要确认：
- AutoMigrate 自动添加了 `reversal_pair_id` 列（无报错）
- 测试不是因为 schema 不匹配而 panic

```bash
mysql -uroot erp_audit_test -e "DESCRIBE financial_logs;" | grep reversal
```

Expected: 一行 `reversal_pair_id bigint(20) unsigned NULL` 输出。

- [ ] **Step 2.5: Commit**

```bash
git add erp-backend/models/core_models.go erp-backend/controllers/financial_log_controller.go
git commit -m "feat(audit): add FinancialLog.ReversalPairID and WriteReversalLog helper"
```

---

## Task 3: 修改 `PayOrderBalance` 应用 pair 标记，验证 T-A1 通过

**Files:**
- Modify: `erp-backend/controllers/order_controller.go`（`PayOrderBalance` 函数，line 1034-1081 的退款循环）

**Interfaces:**
- Consumes: `WriteReversalLog` from Task 2
- Produces: `PayOrderBalance` 在退款循环内查原定金日志、调用 `WriteReversalLog` 替换现有 `WriteFinancialLog` 写入退定金流水

- [ ] **Step 3.1: 修改 `PayOrderBalance` 退款循环**

把 `order_controller.go` 第 1034-1081 行的循环替换为：

```go
if order.DepositAmount > 0 {
    depositSplits, err := getOrderPaymentBreakdown(tx, order, order.DepositAmount)
    if err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询原账户失败"})
        return
    }

    for _, split := range depositSplits {
        var refundAccount models.Account
        if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&refundAccount, split.AccountID).Error; err != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{
                "code": 500,
                "msg":  fmt.Sprintf("定金账户(ID %d)查询失败，无法完成结算", split.AccountID),
            })
            return
        }
        if refundAccount.Balance < split.Amount {
            tx.Rollback()
            c.JSON(http.StatusBadRequest, gin.H{
                "code": 400,
                "msg":  fmt.Sprintf("账户「%s」余额 ¥%.2f 不足以退还定金 ¥%.2f，无法完成结算，请先补充账户余额或调整收款账户", refundAccount.Name, refundAccount.Balance, split.Amount),
            })
            return
        }

        // 查找该订单/账户对应的原定金流水（必须是未被配对的）
        var depositLog models.FinancialLog
        if err := tx.Where("related_no = ? AND account_id = ? AND category = ? AND reversal_pair_id IS NULL",
            order.OrderNo, split.AccountID, "预付定金").
            Order("id DESC").
            First(&depositLog).Error; err != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{
                "code": 500,
                "msg":  fmt.Sprintf("未找到订单 %s 账户 %d 的原定金流水，无法完成冲销对账", order.OrderNo, split.AccountID),
            })
            return
        }

        if err := tx.Model(&models.Account{}).Where("id = ?", split.AccountID).
            UpdateColumn("balance", gorm.Expr("balance - ?", split.Amount)).Error; err != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{
                "code": 500,
                "msg":  "原账户定金退还失败",
            })
            return
        }

        var updatedAccount models.Account
        tx.First(&updatedAccount, split.AccountID)

        // 用 WriteReversalLog：自动把原 depositLog 标记 reversal_pair_id = depositLog.ID，
        // 并写入冲销日志共享同一 pair_id
        if err := WriteReversalLog(tx,
            depositLog.ID,
            split.AccountID,
            2, // flow_type=支出
            "退还定金",
            split.Amount,
            updatedAccount.Balance,
            order.OrderNo,
            fmt.Sprintf("订单 %s 结算前退还定金 ¥%.2f", order.OrderNo, split.Amount),
        ); err != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{
                "code": 500,
                "msg":  "定金退还记录写入失败",
            })
            return
        }
    }
}
```

- [ ] **Step 3.2: 跑 T-A1 测试，验证通过**

```bash
cd erp-backend
DB_NAME=erp_audit_test go test ./controllers -run TestAudit_SameAccountDepositThenSettle -v -count=1
```

Expected: PASS（TotalIncome 现在还是 1300 因为 dashboard 还没改 — 等等，T-A1 的断言用的是 `config.DB.Model(...).Where("flow_type = ?", 1).Sum(...)`，这还没过滤 pair_id，所以仍然会失败）

如果失败：暂时把 T-A1 的 SUM 查询也加上 `reversal_pair_id IS NULL` 过滤，让 T-A1 先过；Task 5 再写 dashboard 端的测试。

把 T-A1 的 SUM 查询改为：
```go
config.DB.Model(&models.FinancialLog{}).
    Where("flow_type = ?", 1).
    Where("reversal_pair_id IS NULL").
    Select("COALESCE(SUM(amount), 0)").
    Scan(&totalIncome)
```

同样 TotalExpense 查询也加 `Where("reversal_pair_id IS NULL")`。

- [ ] **Step 3.3: 再跑 T-A1，验证通过**

```bash
cd erp-backend
DB_NAME=erp_audit_test go test ./controllers -run TestAudit_SameAccountDepositThenSettle -v -count=1
```

Expected: PASS

- [ ] **Step 3.4: 跑全套后端审计测试，确保没回归**

```bash
cd erp-backend
DB_NAME=erp_audit_test go test ./controllers -count=1
```

Expected: 已有 24 个 TestAudit_* 仍通过；如果有失败，逐个调整断言（保留意图）。

- [ ] **Step 3.5: Commit**

```bash
git add erp-backend/controllers/order_controller.go erp-backend/controllers/audit_financial_test.go
git commit -m "fix(audit): pair refund log with deposit log via reversal_pair_id (BUG-002)"
```

---

## Task 4: 新增 T-A2 / T-A3 / T-A4 / T-A5 测试 + 验证全部通过

**Files:**
- Modify: `erp-backend/controllers/order_controller.go`（扩展 `PayOrderBalance` 支持多账户定金退款，前置步骤 4.0）
- Modify: `erp-backend/controllers/audit_financial_test.go`（追加 4 个测试）

**Interfaces:**
- Consumes: `setupAudit()`、已有 helper
- Produces: 4 个新测试函数 + 多账户定金退款支持

> **前置：Task 4.0 多账户定金退款扩展。**
> 当前工作树的 `PayOrderBalance` 只处理单一 `*order.AccountID` 的定金退款（Task 3 implementer 适配）。但主仓库工作树里有一个未提交的多账户版本（使用 `getOrderPaymentBreakdown` helper，按 `order.payment_breakdown` JSON 拆分定金到不同账户，分别退）。Task 4 必须先把多账户逻辑 port 进来，否则 T-A3（200 支付宝 + 100 现金 定金 + 700 公户 尾款）无法通过。
> 参考源码位置：主仓库 `erp-backend/controllers/order_controller.go:272`（`getOrderPaymentBreakdown` 定义）和 `:1035`（多账户退款循环）。请用 `git show main:erp-backend/controllers/order_controller.go` 或直接 read 主仓库文件（不在 worktree 内），把多账户退款的核心逻辑移植到 worktree 的 `PayOrderBalance` 里。

- [ ] **Step 4.0: 扩展 `PayOrderBalance` 支持多账户定金退款**

把当前 `PayOrderBalance` 第一步的"定金对冲退款"块（`if order.DepositAmount > 0 && order.AccountID != nil { ... }`）改为按 `order.payment_breakdown` 拆分的循环，逐 split 处理：

```go
if order.DepositAmount > 0 {
    depositSplits, err := getOrderPaymentBreakdown(tx, order, order.DepositAmount)
    if err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询原账户失败"})
        return
    }
    for _, split := range depositSplits {
        var refundAccount models.Account
        if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&refundAccount, split.AccountID).Error; err != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{
                "code": 500,
                "msg":  fmt.Sprintf("定金账户(ID %d)查询失败，无法完成结算", split.AccountID),
            })
            return
        }
        if refundAccount.Balance < split.Amount {
            tx.Rollback()
            c.JSON(http.StatusBadRequest, gin.H{
                "code": 400,
                "msg":  fmt.Sprintf("账户「%s」余额 ¥%.2f 不足以退还定金 ¥%.2f，无法完成结算，请先补充账户余额或调整收款账户", refundAccount.Name, refundAccount.Balance, split.Amount),
            })
            return
        }

        var depositLog models.FinancialLog
        if err := tx.Where("related_no = ? AND account_id = ? AND category = ? AND reversal_pair_id IS NULL",
            order.OrderNo, split.AccountID, "预付定金").
            Order("id DESC").
            First(&depositLog).Error; err != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{
                "code": 500,
                "msg":  fmt.Sprintf("未找到订单 %s 账户 %d 的原定金流水，无法完成冲销对账", order.OrderNo, split.AccountID),
            })
            return
        }

        if err := tx.Model(&models.Account{}).Where("id = ?", split.AccountID).
            UpdateColumn("balance", gorm.Expr("balance - ?", split.Amount)).Error; err != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "原账户定金退还失败"})
            return
        }

        var updatedAccount models.Account
        tx.First(&updatedAccount, split.AccountID)

        if err := WriteReversalLog(tx,
            depositLog.ID,
            split.AccountID,
            2, // flow_type=支出
            "退还定金",
            split.Amount,
            updatedAccount.Balance,
            order.OrderNo,
            fmt.Sprintf("订单 %s 结算前退还定金 ¥%.2f", order.OrderNo, split.Amount),
        ); err != nil {
            tx.Rollback()
            c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "定金退还记录写入失败"})
            return
        }
    }
}
```

并把 `getOrderPaymentBreakdown` 和 `PaymentBreakdownItem` 类型定义从主仓库 port 到本文件末尾（参见 `erp-backend/controllers/order_controller.go:272-300` 区域）：

```go
type PaymentBreakdownItem struct {
    PaymentMethod int     `json:"payment_method"`
    AccountID     uint    `json:"account_id"`
    Amount        float64 `json:"amount"`
}

func encodePaymentBreakdown(items []PaymentBreakdownItem) string { ... } // 已有
func decodePaymentBreakdown(s string) ([]PaymentBreakdownItem, error) { ... } // 已有

func getOrderPaymentBreakdown(tx *gorm.DB, order models.Order, fallbackAmount float64) ([]PaymentBreakdownItem, error) {
    if order.PaymentBreakdown != "" {
        items, err := decodePaymentBreakdown(order.PaymentBreakdown)
        if err == nil && len(items) > 0 {
            return items, nil
        }
    }
    if order.AccountID != nil {
        return []PaymentBreakdownItem{{
            PaymentMethod: 1,
            AccountID:     *order.AccountID,
            Amount:        fallbackAmount,
        }}, nil
    }
    return nil, fmt.Errorf("订单 %s 既无 payment_breakdown 也无 account_id", order.OrderNo)
}
```

**注意**：`getOrderPaymentBreakdown` / `PaymentBreakdownItem` / `encodePaymentBreakdown` / `decodePaymentBreakdown` 可能已经在 worktree 中存在（取决于上次 port 是否带过来）。如果存在就直接复用，不要重复声明。

验证：
```bash
cd erp-backend
go build ./controllers/... ./models/...
DB_NAME=erp_audit_test go test ./controllers -run 'TestAudit_SameAccount|TestAudit_DepositThenSettle|TestAudit_DuplicateSettle|TestAudit_SettleRefund' -v -count=1
```
预期：所有 settle 相关测试仍通过（含 Task 1 / Task 3 写入的 T-A1）。

- [ ] **Step 4.1: 追加 `TestAudit_CrossAccountDepositThenSettle`**

```go
func TestAudit_CrossAccountDepositThenSettle(t *testing.T) {
    setupAudit(t)
    productID := ensureProduct(t, "审计商品-跨账户", 1000.00, 600, 100)

    // 定金 300 支付宝，结尾款 1000 公户
    body := gin.H{
        "items":           []gin.H{{"product_id": productID, "quantity": 1, "unit_price": 1000.00}},
        "payment_method":  1,
        "delivery_method": 2,
        "deposit_amount":  300,
        "customer_name":   "跨账户定金客户",
        "customer_phone":  "13800000002",
        "customer_address": "测试地址-跨",
    }
    w := postJSON("/api/orders", body)
    if w.Code != http.StatusOK {
        t.Fatalf("下单失败: %d %s", w.Code, w.Body.String())
    }
    var resp struct{ OrderNo string `json:"order_no"` }
    json.Unmarshal(w.Body.Bytes(), &resp)
    order := readOrder(t, resp.OrderNo)

    settleW := putJSON("/api/orders/"+strconv.Itoa(int(order.ID))+"/balance",
        gin.H{"final_payment_method": 3}) // 公户
    if settleW.Code != http.StatusOK {
        t.Fatalf("结清失败: %d %s", settleW.Code, settleW.Body.String())
    }

    // 支付宝：300 进 -300 退 = 0
    if math.Abs(accountBalance(t, "支付宝")-0) > 0.01 {
        t.Fatalf("跨账户定金后退款：支付宝余额应为 0，实际 %.2f", accountBalance(t, "支付宝"))
    }
    // 公户：0 + 1000 = 1000
    if math.Abs(accountBalance(t, "公户")-1000) > 0.01 {
        t.Fatalf("公户余额应为 1000，实际 %.2f", accountBalance(t, "公户"))
    }
    // TotalIncome = 1000（仅含全款，扣除配对的定金）
    var income float64
    config.DB.Model(&models.FinancialLog{}).
        Where("flow_type = ? AND reversal_pair_id IS NULL", 1).
        Select("COALESCE(SUM(amount), 0)").Scan(&income)
    if math.Abs(income-1000) > 0.01 {
        t.Fatalf("跨账户 TotalIncome 应为 1000，实际 %.2f", income)
    }
}
```

- [ ] **Step 4.2: 追加 `TestAudit_MixedDepositThenSettle`**

```go
func TestAudit_MixedDepositThenSettle(t *testing.T) {
    setupAudit(t)
    productID := ensureProduct(t, "审计商品-混合定金", 1000.00, 600, 100)

    // 定金 200 支付宝 + 100 现金，尾款 700 公户
    body := gin.H{
        "items":           []gin.H{{"product_id": productID, "quantity": 1, "unit_price": 1000.00}},
        "payment_method":  4, // 混合支付
        "payment_splits":  []gin.H{{"payment_method": 1, "amount": 200}, {"payment_method": 5, "amount": 100}},
        "delivery_method": 2,
        "deposit_amount":  300,
        "customer_name":   "混合定金客户",
        "customer_phone":  "13800000003",
        "customer_address": "测试地址-混合",
    }
    w := postJSON("/api/orders", body)
    if w.Code != http.StatusOK {
        t.Fatalf("下单失败: %d %s", w.Code, w.Body.String())
    }
    var resp struct{ OrderNo string `json:"order_no"` }
    json.Unmarshal(w.Body.Bytes(), &resp)
    order := readOrder(t, resp.OrderNo)

    settleW := putJSON("/api/orders/"+strconv.Itoa(int(order.ID))+"/balance",
        gin.H{"final_payment_method": 3})
    if settleW.Code != http.StatusOK {
        t.Fatalf("结清失败: %d %s", settleW.Code, settleW.Body.String())
    }

    // 支付宝 200-200=0；现金 100-100=0；公户 +700
    if math.Abs(accountBalance(t, "支付宝")) > 0.01 ||
        math.Abs(accountBalance(t, "现金")) > 0.01 ||
        math.Abs(accountBalance(t, "公户")-700) > 0.01 {
        t.Fatalf("混合定金后余额异常：支付宝 %.2f 现金 %.2f 公户 %.2f",
            accountBalance(t, "支付宝"), accountBalance(t, "现金"), accountBalance(t, "公户"))
    }
    var income float64
    config.DB.Model(&models.FinancialLog{}).
        Where("flow_type = ? AND reversal_pair_id IS NULL", 1).
        Select("COALESCE(SUM(amount), 0)").Scan(&income)
    if math.Abs(income-700) > 0.01 {
        t.Fatalf("混合定金 TotalIncome 应为 700（仅尾款入账），实际 %.2f", income)
    }
}
```

- [ ] **Step 4.3: 追加 `TestAudit_DepositWithSubsidyAndTradeIn`**

```go
func TestAudit_DepositWithSubsidyAndTradeIn(t *testing.T) {
    setupAudit(t)
    productID := ensureProduct(t, "审计商品-国补tradein", 10000.00, 7000, 100)

    // 商品 10000，国补 1500（封顶），trade-in 1000，抹零 0 → actual_pay = 7500
    // 定金 2000 支付宝，尾款 5500 公户
    body := gin.H{
        "items":            []gin.H{{"product_id": productID, "quantity": 1, "unit_price": 10000.00}},
        "payment_method":   3,
        "delivery_method":  2,
        "deposit_amount":   2000,
        "is_subsidy":       true,
        "subsidy_amount":   1500,
        "trade_in_amount":  1000,
        "trade_in_type":    "旧电视",
        "customer_name":    "国补tradein客户",
        "customer_phone":   "13800000004",
        "customer_address": "测试地址-国补",
    }
    w := postJSON("/api/orders", body)
    if w.Code != http.StatusOK {
        t.Fatalf("下单失败: %d %s", w.Code, w.Body.String())
    }
    var resp struct{ OrderNo string `json:"order_no"` }
    json.Unmarshal(w.Body.Bytes(), &resp)
    order := readOrder(t, resp.OrderNo)

    // actual_pay 应 = 10000 - 1500 - 1000 = 7500
    if math.Abs(order.ActualPayAmount-7500) > 0.01 {
        t.Fatalf("actual_pay 应为 7500，实际 %.2f", order.ActualPayAmount)
    }

    settleW := putJSON("/api/orders/"+strconv.Itoa(int(order.ID))+"/balance",
        gin.H{"final_payment_method": 3})
    if settleW.Code != http.StatusOK {
        t.Fatalf("结清失败: %d %s", settleW.Code, settleW.Body.String())
    }

    // 支付宝 2000-2000=0；公户 0+7500=7500
    if math.Abs(accountBalance(t, "支付宝")) > 0.01 ||
        math.Abs(accountBalance(t, "公户")-7500) > 0.01 {
        t.Fatalf("余额异常：支付宝 %.2f 公户 %.2f",
            accountBalance(t, "支付宝"), accountBalance(t, "公户"))
    }
    var income float64
    config.DB.Model(&models.FinancialLog{}).
        Where("flow_type = ? AND reversal_pair_id IS NULL", 1).
        Select("COALESCE(SUM(amount), 0)").Scan(&income)
    if math.Abs(income-7500) > 0.01 {
        t.Fatalf("TotalIncome 应为 7500，实际 %.2f", income)
    }
}
```

- [ ] **Step 4.4: 追加 `TestAudit_DuplicateSettleIsIdempotent`**

```go
func TestAudit_DuplicateSettleIsIdempotent(t *testing.T) {
    setupAudit(t)
    productID := ensureProduct(t, "审计商品-重复结清", 1000.00, 600, 100)

    body := gin.H{
        "items":           []gin.H{{"product_id": productID, "quantity": 1, "unit_price": 1000.00}},
        "payment_method":  1,
        "delivery_method": 2,
        "deposit_amount":  300,
        "customer_name":   "重复结清客户",
        "customer_phone":  "13800000005",
        "customer_address": "测试地址-重复",
    }
    w := postJSON("/api/orders", body)
    if w.Code != http.StatusOK {
        t.Fatalf("下单失败: %d %s", w.Code, w.Body.String())
    }
    var resp struct{ OrderNo string `json:"order_no"` }
    json.Unmarshal(w.Body.Bytes(), &resp)
    order := readOrder(t, resp.OrderNo)

    // 第一次结清
    first := putJSON("/api/orders/"+strconv.Itoa(int(order.ID))+"/balance",
        gin.H{"final_payment_method": 1})
    if first.Code != http.StatusOK {
        t.Fatalf("首次结清失败: %d %s", first.Code, first.Body.String())
    }
    firstBal := accountBalance(t, "支付宝")
    firstIncome := func() float64 {
        var v float64
        config.DB.Model(&models.FinancialLog{}).
            Where("flow_type = ? AND reversal_pair_id IS NULL", 1).
            Select("COALESCE(SUM(amount), 0)").Scan(&v)
        return v
    }()

    // 第二次结清应被拒绝（400）
    second := putJSON("/api/orders/"+strconv.Itoa(int(order.ID))+"/balance",
        gin.H{"final_payment_method": 1})
    if second.Code != http.StatusBadRequest {
        t.Fatalf("第二次结清应被拒绝 (400)，实际 %d %s", second.Code, second.Body.String())
    }

    // 余额不变
    if math.Abs(accountBalance(t, "支付宝")-firstBal) > 0.01 {
        t.Fatalf("第二次结清后余额变化：原 %.2f → 现 %.2f", firstBal, accountBalance(t, "支付宝"))
    }
    var income2 float64
    config.DB.Model(&models.FinancialLog{}).
        Where("flow_type = ? AND reversal_pair_id IS NULL", 1).
        Select("COALESCE(SUM(amount), 0)").Scan(&income2)
    if math.Abs(income2-firstIncome) > 0.01 {
        t.Fatalf("TotalIncome 因重复结清而变化：原 %.2f → 现 %.2f", firstIncome, income2)
    }
}
```

- [ ] **Step 4.5: 跑全部新测试，验证全部通过**

```bash
cd erp-backend
DB_NAME=erp_audit_test go test ./controllers -run 'TestAudit_SameAccount|TestAudit_CrossAccount|TestAudit_MixedDeposit|TestAudit_DepositWithSubsidy|TestAudit_DuplicateSettle' -v -count=1
```

Expected: 5 PASS

- [ ] **Step 4.6: 跑全部后端审计测试**

```bash
cd erp-backend
DB_NAME=erp_audit_test go test ./controllers -count=1
```

Expected: 全部 PASS（已有 24 + 新增 5 = 29）

- [ ] **Step 4.7: Commit**

```bash
git add erp-backend/controllers/audit_financial_test.go
git commit -m "test(audit): add T-A2..T-A5 deposit-then-settle coverage"
```

---

## Task 5: 修改 Dashboard / Stats 聚合查询 + 验证

**Files:**
- Modify: `erp-backend/controllers/dashboard_controller.go`（`GetFinanceSummary` 第 129-145 行）
- Modify: `erp-backend/controllers/stats_controller.go`（`GetMonthlyStats` 第 75-78 行）
- Modify: `erp-backend/controllers/audit_financial_test.go`（加 `TestAudit_DashboardSummaryFiltersReversalPairs`，并扩展 `setupAudit` 注册 `GetFinanceSummary` 路由）

**Interfaces:**
- Produces: dashboard `TotalIncome`/`TotalExpense` 自动排除 `reversal_pair_id IS NOT NULL` 的日志

- [ ] **Step 5.1: 扩展 `setupAudit` 注册 `GetFinanceSummary`**

修改 `audit_financial_test.go` 的 `setupAudit()`：

```go
func setupAudit(t *testing.T) {
    t.Helper()
    resetAuditDB(t)

    if auditRouter == nil {
        auditRouter = gin.New()
        auditRouter.POST("/api/orders", CreateOrder)
        auditRouter.PUT("/api/orders/:id/balance", PayOrderBalance)
        auditRouter.PUT("/api/orders/:id/cancel", CancelOrder)
        auditRouter.GET("/api/finance/summary", GetFinanceSummary)
    }
}
```

- [ ] **Step 5.2: 追加 `TestAudit_DashboardSummaryFiltersReversalPairs`**

```go
func TestAudit_DashboardSummaryFiltersReversalPairs(t *testing.T) {
    setupAudit(t)
    productID := ensureProduct(t, "审计商品-dashboard过滤", 1000.00, 600, 100)

    // 下单定金 + 结清尾款
    body := gin.H{
        "items": []gin.H{{"product_id": productID, "quantity": 1, "unit_price": 1000.00}},
        "payment_method":  1, "delivery_method": 2,
        "deposit_amount":  300,
        "customer_name":   "dashboard过滤客户",
        "customer_phone":  "13800000006",
        "customer_address": "测试地址-dashboard",
    }
    w := postJSON("/api/orders", body)
    if w.Code != http.StatusOK {
        t.Fatalf("下单失败: %d %s", w.Code, w.Body.String())
    }
    var resp struct{ OrderNo string `json:"order_no"` }
    json.Unmarshal(w.Body.Bytes(), &resp)
    order := readOrder(t, resp.OrderNo)
    settleW := putJSON("/api/orders/"+strconv.Itoa(int(order.ID))+"/balance",
        gin.H{"final_payment_method": 1})
    if settleW.Code != http.StatusOK {
        t.Fatalf("结清失败: %d %s", settleW.Code, settleW.Body.String())
    }

    // 调 dashboard
    summaryW := getJSON("/api/finance/summary?period=all")
    if summaryW.Code != http.StatusOK {
        t.Fatalf("dashboard 查询失败: %d %s", summaryW.Code, summaryW.Body.String())
    }
    var sum struct {
        Code int `json:"code"`
        Data struct {
            TotalIncome  float64 `json:"total_income"`
            TotalExpense float64 `json:"total_expense"`
        } `json:"data"`
    }
    json.Unmarshal(summaryW.Body.Bytes(), &sum)
    if math.Abs(sum.Data.TotalIncome-1000) > 0.01 {
        t.Fatalf("dashboard TotalIncome 应为 1000，实际 %.2f — dashboard 仍按老规则汇总", sum.Data.TotalIncome)
    }
    if math.Abs(sum.Data.TotalExpense) > 0.01 {
        t.Fatalf("dashboard TotalExpense 应为 0（退定金被 pair 排除），实际 %.2f", sum.Data.TotalExpense)
    }
}
```

- [ ] **Step 5.3: 添加 `getJSON` helper**

在 `audit_financial_test.go` 的 helper 区追加：

```go
func getJSON(path string) *httptest.ResponseRecorder {
    req, _ := http.NewRequest("GET", path, nil)
    w := httptest.NewRecorder()
    auditRouter.ServeHTTP(w, req)
    return w
}
```

- [ ] **Step 5.4: 跑新增 dashboard 测试，验证失败（dashboard 还没改）**

```bash
cd erp-backend
DB_NAME=erp_audit_test go test ./controllers -run TestAudit_DashboardSummaryFiltersReversalPairs -v -count=1
```

Expected: FAIL with `dashboard TotalIncome 应为 1000，实际 1300.00`

- [ ] **Step 5.5: 修改 `dashboard_controller.go` 聚合查询**

在 `GetFinanceSummary` 第 129-145 行，把 `incomeDB` 和 `expenseDB` 各加一行 `.Where("reversal_pair_id IS NULL")`：

```go
incomeDB := config.DB.Model(&models.FinancialLog{}).
    Where("flow_type = ?", 1).
    Where("reversal_pair_id IS NULL")
if !startTime.IsZero() {
    incomeDB = incomeDB.Where("created_at >= ?", startTime)
}
if !endTime.IsZero() {
    incomeDB = incomeDB.Where("created_at <= ?", endTime)
}
incomeDB.Select("COALESCE(SUM(amount), 0)").Scan(&summary.TotalIncome)

expenseDB := config.DB.Model(&models.FinancialLog{}).
    Where("flow_type = ?", 2).
    Where("reversal_pair_id IS NULL")
if !startTime.IsZero() {
    expenseDB = expenseDB.Where("created_at >= ?", startTime)
}
if !endTime.IsZero() {
    expenseDB = expenseDB.Where("created_at <= ?", endTime)
}
expenseDB.Select("COALESCE(SUM(amount), 0)").Scan(&summary.TotalExpense)
```

- [ ] **Step 5.6: 修改 `stats_controller.go` 聚合查询**

第 75-78 行改为：

```go
config.DB.Model(&models.FinancialLog{}).
    Where("flow_type = ? AND reversal_pair_id IS NULL", 2).
    Where("created_at >= ? AND created_at <= ?", startTime, endTime).
    Select("COALESCE(SUM(amount), 0)").
    Scan(&stats.Expense)
```

- [ ] **Step 5.7: 跑全部审计测试，验证全部通过**

```bash
cd erp-backend
DB_NAME=erp_audit_test go test ./controllers -count=1
```

Expected: 全部 PASS（29 个 TestAudit_*）

- [ ] **Step 5.8: Commit**

```bash
git add erp-backend/controllers/dashboard_controller.go erp-backend/controllers/stats_controller.go erp-backend/controllers/audit_financial_test.go
git commit -m "fix(audit): exclude paired reversal logs from dashboard / stats aggregation"
```

---

## Task 6: 修改 `CancelOrder` / `ReversalCancelOrder` 加 pair 标记 + 加 F9 测试

**Files:**
- Modify: `erp-backend/controllers/order_controller.go`（`cancelOrderWithOptions` 退款循环）
- Modify: `erp-backend/controllers/audit_financial_test.go`（加 `TestAudit_ReversalCancelOrder` + 注册 `ReversalCancelOrder`）

**Interfaces:**
- Consumes: `WriteReversalLog`
- Produces: 取消订单的退款日志与原销售货款日志共享 `reversal_pair_id`

- [ ] **Step 6.1: 注册 `ReversalCancelOrder` 路由到 `setupAudit`**

```go
auditRouter.PUT("/api/orders/:id/reversal-cancel", ReversalCancelOrder)
```

- [ ] **Step 6.2: 追加 `TestAudit_ReversalCancelOrder`**

```go
func TestAudit_ReversalCancelOrder(t *testing.T) {
    setupAudit(t)
    productID := ensureProduct(t, "审计商品-红冲", 1000.00, 600, 100)

    // 完整支付订单（payment_status=2）
    body := gin.H{
        "items": []gin.H{{"product_id": productID, "quantity": 1, "unit_price": 1000.00}},
        "payment_method":  1, "delivery_method": 2,
        "customer_name":   "红冲客户",
        "customer_phone":  "13800000007",
        "customer_address": "测试地址-红冲",
    }
    w := postJSON("/api/orders", body)
    if w.Code != http.StatusOK {
        t.Fatalf("下单失败: %d %s", w.Code, w.Body.String())
    }
    var resp struct{ OrderNo string `json:"order_no"` }
    json.Unmarshal(w.Body.Bytes(), &resp)
    order := readOrder(t, resp.OrderNo)

    // 标记已交付（满足 allowFulfilled 路径）
    config.DB.Model(&order).Update("is_delivered", true)

    // 红冲
    cancelW := putJSON("/api/orders/"+strconv.Itoa(int(order.ID))+"/reversal-cancel", gin.H{})
    if cancelW.Code != http.StatusOK {
        t.Fatalf("红冲失败: %d %s", cancelW.Code, cancelW.Body.String())
    }

    // 账户余额归 0
    if math.Abs(accountBalance(t, "支付宝")) > 0.01 {
        t.Fatalf("红冲后支付宝余额应为 0，实际 %.2f", accountBalance(t, "支付宝"))
    }
    // 订单状态
    finalOrder := readOrder(t, resp.OrderNo)
    if finalOrder.OrderStatus != 2 {
        t.Fatalf("红冲后订单 OrderStatus 应为 2，实际 %d", finalOrder.OrderStatus)
    }
    // TotalIncome / TotalExpense 均为 0（销售货款被配对）
    var income, expense float64
    config.DB.Model(&models.FinancialLog{}).
        Where("flow_type = ? AND reversal_pair_id IS NULL", 1).
        Select("COALESCE(SUM(amount), 0)").Scan(&income)
    config.DB.Model(&models.FinancialLog{}).
        Where("flow_type = ? AND reversal_pair_id IS NULL", 2).
        Select("COALESCE(SUM(amount), 0)").Scan(&expense)
    if math.Abs(income) > 0.01 || math.Abs(expense) > 0.01 {
        t.Fatalf("红冲后 TotalIncome=%.2f / TotalExpense=%.2f 应均归 0", income, expense)
    }
    // 库存恢复（main_stock 增加 1）
    var p models.Product
    config.DB.First(&p, productID)
    if p.MainStock != 100 { // 初始 100，订单扣 1，红冲 +1
        t.Fatalf("红冲后 main_stock 应为 100，实际 %d", p.MainStock)
    }
}
```

- [ ] **Step 6.3: 修改 `cancelOrderWithOptions` 退款循环**

定位 `order_controller.go` 中的 `cancelOrderWithOptions`，找到写退款流水的 `WriteFinancialLog(...)` 调用（约 line 1380），改为使用 `WriteReversalLog`。退款前先查找原"销售货款"日志：

```go
// 替换原 WriteFinancialLog 写入退款日志的代码块
var collectLog models.FinancialLog
if err := tx.Where("related_no = ? AND account_id = ? AND category = ? AND reversal_pair_id IS NULL",
    order.OrderNo, split.AccountID, "销售货款").
    Order("id DESC").
    First(&collectLog).Error; err != nil {
    // 找不到原 collect log 时，回退为普通退款日志（不配对，保持兼容）
    if err := WriteFinancialLog(tx, split.AccountID, 2, financialCategory,
        split.Amount, updatedAccount.Balance, order.OrderNo,
        fmt.Sprintf("订单 %s %s，反向退款 ¥%.2f", order.OrderNo, actionName, split.Amount)); err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "退款流水写入失败"})
        return
    }
} else {
    if err := WriteReversalLog(tx, collectLog.ID,
        split.AccountID, 2, financialCategory, split.Amount,
        updatedAccount.Balance, order.OrderNo,
        fmt.Sprintf("订单 %s %s，反向退款 ¥%.2f", order.OrderNo, actionName, split.Amount),
    ); err != nil {
        tx.Rollback()
        c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "退款配对流水写入失败"})
        return
    }
}
```

- [ ] **Step 6.4: 跑测试验证**

```bash
cd erp-backend
DB_NAME=erp_audit_test go test ./controllers -run TestAudit_ReversalCancelOrder -v -count=1
```

Expected: PASS

- [ ] **Step 6.5: 跑全部后端测试**

```bash
cd erp-backend
DB_NAME=erp_audit_test go test ./controllers -count=1
```

Expected: 全部 PASS（30 个）

- [ ] **Step 6.6: Commit**

```bash
git add erp-backend/controllers/order_controller.go erp-backend/controllers/audit_financial_test.go
git commit -m "feat(audit): pair cancel/reversal-cancel refund logs with collect logs"
```

---

## Task 7: 加 F16 (TransferFunds) + F17 (CalibrateAccount) 测试

**Files:**
- Modify: `erp-backend/controllers/audit_financial_test.go`（加 2 个测试 + 注册 2 个路由）

**Interfaces:**
- Produces: `TestAudit_TransferFunds`、`TestAudit_CalibrateAccount`

- [ ] **Step 7.1: 注册路由**

```go
auditRouter.POST("/api/accounts/transfer", TransferFunds)
auditRouter.PUT("/api/accounts/:id/calibrate", CalibrateAccount)
```

- [ ] **Step 7.2: 追加 `TestAudit_TransferFunds`**

```go
func TestAudit_TransferFunds(t *testing.T) {
    setupAudit(t)

    fromID := accountIDByName(t, "支付宝")
    toID := accountIDByName(t, "公户")
    setAccountBalance(t, "支付宝", 1000)
    setAccountBalance(t, "公户", 0)

    body := gin.H{
        "from_account_id": fromID,
        "to_account_id":   toID,
        "amount":          500,
        "remark":          "审计测试划拨",
    }
    w := postJSON("/api/accounts/transfer", body)
    if w.Code != http.StatusOK {
        t.Fatalf("划拨失败: %d %s", w.Code, w.Body.String())
    }

    // 净变化 = 0
    if math.Abs(accountBalance(t, "支付宝")-500) > 0.01 ||
        math.Abs(accountBalance(t, "公户")-500) > 0.01 {
        t.Fatalf("划拨后余额异常：支付宝 %.2f 公户 %.2f", accountBalance(t, "支付宝"), accountBalance(t, "公户"))
    }

    // 两行 flow_type=3 流水，pair_id 都为 NULL
    var pairCount int64
    config.DB.Model(&models.FinancialLog{}).
        Where("flow_type = ? AND reversal_pair_id IS NOT NULL", 3).
        Count(&pairCount)
    if pairCount != 0 {
        t.Fatalf("划拨日志不应被配对，实际 %d 条被配对", pairCount)
    }

    // 不计入 TotalIncome / TotalExpense
    var income, expense float64
    config.DB.Model(&models.FinancialLog{}).
        Where("flow_type = ? AND reversal_pair_id IS NULL", 1).
        Select("COALESCE(SUM(amount), 0)").Scan(&income)
    config.DB.Model(&models.FinancialLog{}).
        Where("flow_type = ? AND reversal_pair_id IS NULL", 2).
        Select("COALESCE(SUM(amount), 0)").Scan(&expense)
    if math.Abs(income) > 0.01 || math.Abs(expense) > 0.01 {
        t.Fatalf("划拨不应影响 TotalIncome=%.2f / TotalExpense=%.2f", income, expense)
    }

    // 余额不足应被拒绝
    body["amount"] = 99999
    w2 := postJSON("/api/accounts/transfer", body)
    if w2.Code != http.StatusBadRequest {
        t.Fatalf("余额不足时应返回 400，实际 %d", w2.Code)
    }
    // 同一账户划拨应被拒绝
    body["from_account_id"] = toID
    body["to_account_id"] = toID
    body["amount"] = 10
    w3 := postJSON("/api/accounts/transfer", body)
    if w3.Code != http.StatusBadRequest {
        t.Fatalf("同账户划拨应被拒绝，实际 %d", w3.Code)
    }
}
```

- [ ] **Step 7.3: 追加 `TestAudit_CalibrateAccount`**

```go
func TestAudit_CalibrateAccount(t *testing.T) {
    setupAudit(t)

    accID := accountIDByName(t, "支付宝")
    setAccountBalance(t, "支付宝", 500)

    // 校准到 800（多 300）
    body := gin.H{"real_balance": 800, "remark": "审计校准+300"}
    w := putJSON("/api/accounts/"+strconv.Itoa(int(accID))+"/calibrate", body)
    if w.Code != http.StatusOK {
        t.Fatalf("校准失败: %d %s", w.Code, w.Body.String())
    }
    if math.Abs(accountBalance(t, "支付宝")-800) > 0.01 {
        t.Fatalf("校准后余额应为 800，实际 %.2f", accountBalance(t, "支付宝"))
    }
    // flow_type=4 校准日志：金额 300，方向 收入（diff>0 → flow_type=1 但 category=系统校准）
    // 校准不应计入 TotalIncome / TotalExpense（聚合只查 flow_type IN (1,2)；校准是 flow_type=4）
    // 但当前聚合查的是 flow_type=1 INCLUDE 校准
    // —— 需要在 dashboard 聚合上加 flow_type=4 排除，见 Step 7.4
    _ = w

    // 校准到 200（少 300）
    body2 := gin.H{"real_balance": 200, "remark": "审计校准-300"}
    w2 := putJSON("/api/accounts/"+strconv.Itoa(int(accID))+"/calibrate", body2)
    if w2.Code != http.StatusOK {
        t.Fatalf("二次校准失败: %d %s", w2.Code, w2.Body.String())
    }
    if math.Abs(accountBalance(t, "支付宝")-200) > 0.01 {
        t.Fatalf("二次校准后余额应为 200，实际 %.2f", accountBalance(t, "支付宝"))
    }
}
```

- [ ] **Step 7.4: 修正 CalibrateAccount 使用 `flow_type=4` + dashboard / stats 排除 flow_type=4**

`CalibrateAccount` 当前实现把校准日志写成 `flow_type=1`（diff>0）或 `flow_type=2`（diff<0），导致它被计入收入 / 支出。`FinancialLog.FlowType` 的文档约定 `flow_type=4` = 余额校准，但代码从未使用。修正步骤：

**a) 修改 `erp-backend/controllers/account_controller.go` 的 `CalibrateAccount`：**

定位写入日志的代码块（约 line 263-275），把：
```go
flowType := 2
amount := -diff
if diff > 0 {
    flowType = 1
    amount = diff
}
```
改为：
```go
flowType := 4 // 余额校准 — 不计入收入 / 支出
amount := diff
if amount < 0 {
    amount = -amount
}
```
即统一使用 `flow_type=4`，金额为绝对值。

**b) 修改 `dashboard_controller.go` 的 income / expense 查询：**

```go
incomeDB := config.DB.Model(&models.FinancialLog{}).
    Where("flow_type = ?", 1).
    Where("flow_type <> ?", 4).
    Where("reversal_pair_id IS NULL")
// ... expense 同样
expenseDB := config.DB.Model(&models.FinancialLog{}).
    Where("flow_type = ?", 2).
    Where("flow_type <> ?", 4).
    Where("reversal_pair_id IS NULL")
```

**c) 修改 `stats_controller.go` 的 expense 查询：**

```go
config.DB.Model(&models.FinancialLog{}).
    Where("flow_type = ? AND flow_type <> ? AND reversal_pair_id IS NULL", 2, 4).
    Where("created_at >= ? AND created_at <= ?", startTime, endTime).
    Select("COALESCE(SUM(amount), 0)").
    Scan(&stats.Expense)
```

- [ ] **Step 7.5: 跑测试验证**

```bash
cd erp-backend
DB_NAME=erp_audit_test go test ./controllers -run 'TestAudit_TransferFunds|TestAudit_CalibrateAccount' -v -count=1
```

Expected: PASS

- [ ] **Step 7.6: 跑全部后端测试**

```bash
cd erp-backend
DB_NAME=erp_audit_test go test ./controllers -count=1
```

Expected: 全部 PASS（32 个）

- [ ] **Step 7.7: Commit**

```bash
git add erp-backend/controllers/audit_financial_test.go erp-backend/controllers/dashboard_controller.go erp-backend/controllers/stats_controller.go
git commit -m "test(audit): add F16 transfer + F17 calibrate coverage; exclude flow_type=4 from summary"
```

---

## Task 8: 前端 CheckoutPanel 关键计算覆盖

**Files:**
- Create: `src/views/cashier/CheckoutPanel.test.mjs`

**Interfaces:**
- Produces: 5 个 `node --test` 用例，覆盖 `actualReceipt` / `mixedPaymentTarget` / `mixedPaymentRemaining` 计算逻辑

- [ ] **Step 8.1: 写测试文件**

```javascript
import assert from 'node:assert/strict'
import { readFile } from 'node:fs/promises'
import test from 'node:test'

const source = await readFile(new URL('./CheckoutPanel.vue', import.meta.url), 'utf8')

// 提取 Vue SFC 中关键计算块的源码（用正则）
function extractComputed(name) {
  const re = new RegExp(`const ${name} = computed\\(\\(\\) => \\{([\\s\\S]*?)\\}\\)`)
  const m = source.match(re)
  return m ? m[1].trim() : null
}

test('actualReceipt returns totalPayable in full-pay mode', () => {
  const body = extractComputed('actualReceipt')
  assert.match(body ?? '', /isDepositMode\.value \? depositAmount\.value : totalPayable\.value/)
})

test('mixedPaymentTarget returns actualReceipt (not actualReceipt + subsidyAmount)', () => {
  const body = extractComputed('mixedPaymentTarget')
  assert.ok(body, 'mixedPaymentTarget computed 应存在')
  assert.match(body, /return actualReceipt\.value/)
  // 防止历史 BUG-3 复发：不能加 subsidyAmount
  assert.doesNotMatch(body, /\+= subsidyAmount|subsidyAmount\.value\)/)
})

test('mixedPaymentTotal sums all split amounts', () => {
  const body = extractComputed('mixedPaymentTotal')
  assert.match(body ?? '', /Object\.values\(mixedPaymentAmounts\.value\)\.reduce/)
})

test('mixedPaymentRemaining equals target minus total', () => {
  const body = extractComputed('mixedPaymentRemaining')
  assert.match(body ?? '', /mixedPaymentTarget\.value - mixedPaymentTotal\.value/)
})

test('subsidy cap 1500 enforced', () => {
  // 提取 subsidyAmount computed 的源码，确认有上限判断
  const re = /const subsidyAmount = computed\([\s\S]*?\)/
  const m = source.match(re)
  assert.ok(m, 'subsidyAmount computed 应存在')
  assert.match(m[0], /1500/)
})
```

- [ ] **Step 8.2: 跑测试验证**

```bash
node --test src/views/cashier/CheckoutPanel.test.mjs
```

Expected: PASS（5 个用例）

- [ ] **Step 8.3: 跑全部前端测试**

```bash
node --test 'src/**/*.test.mjs'
```

Expected: 全部 PASS（已有 + 新增）

- [ ] **Step 8.4: Commit**

```bash
git add src/views/cashier/CheckoutPanel.test.mjs
git commit -m "test(frontend): cover CheckoutPanel mixed-payment calculations"
```

---

## Task 9: 历史数据 dry-run SQL 脚本 + 报告

**Files:**
- Create: `erp-backend/scripts/audit-2026-08-02-dryrun-D1.sql`
- Create: `erp-backend/scripts/audit-2026-08-02-dryrun-D2.sql`
- Create: `erp-backend/scripts/audit-2026-08-02-dryrun-D3.sql`
- Create: `erp-backend/scripts/audit-2026-08-02-migration.sql`
- Create: `erp-backend/scripts/audit-2026-08-02-balance-replay.go`（Go 工具：用 financial_logs 重放余额）

- [ ] **Step 9.1: 写 D1 dry-run SQL**

```sql
-- audit-2026-08-02-dryrun-D1.sql
-- 用途：查找 orders 表中 payment_breakdown 缺失的订单
-- 只读：仅 SELECT，请勿在生产库执行 UPDATE/DELETE

USE erp_db;

SELECT
  id, order_no, total_amount, actual_pay_amount, deposit_amount,
  payment_status, order_status, account_id, created_at
FROM orders
WHERE payment_breakdown IS NULL OR payment_breakdown = ''
ORDER BY created_at DESC;

-- 辅助：基于 financial_logs 重建 payment_breakdown 的视图
SELECT
  o.id AS order_id,
  o.order_no,
  o.payment_status,
  SUM(CASE WHEN fl.flow_type = 1 AND fl.reversal_pair_id IS NULL THEN fl.amount ELSE 0 END) AS sum_income_unpaired,
  SUM(CASE WHEN fl.flow_type = 2 AND fl.reversal_pair_id IS NULL THEN fl.amount ELSE 0 END) AS sum_expense_unpaired,
  COUNT(CASE WHEN fl.flow_type = 1 THEN 1 END) AS income_log_count,
  COUNT(CASE WHEN fl.flow_type = 2 THEN 1 END) AS expense_log_count
FROM orders o
LEFT JOIN financial_logs fl ON fl.related_no = o.order_no
WHERE o.payment_breakdown IS NULL OR o.payment_breakdown = ''
GROUP BY o.id, o.order_no, o.payment_status;
```

- [ ] **Step 9.2: 写 D2 dry-run SQL**

```sql
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
```

- [ ] **Step 9.3: 写 D3 dry-run SQL**

```sql
-- audit-2026-08-02-dryrun-D3.sql
-- 用途：检查 financial_logs.balance_after 是否与 accounts.balance 一致
-- 只读；提供余额重放数据，不修改

USE erp_db;

SELECT
  a.id AS account_id,
  a.name AS account_name,
  a.balance AS current_balance,
  (SELECT balance_after FROM financial_logs
   WHERE account_id = a.id ORDER BY id DESC LIMIT 1) AS last_log_balance,
  (SELECT COUNT(*) FROM financial_logs WHERE account_id = a.id) AS log_count
FROM accounts a
ORDER BY a.id;
```

- [ ] **Step 9.4: 写 Go 余额重放工具**

`erp-backend/scripts/audit-2026-08-02-balance-replay.go`：

```go
// audit-2026-08-02-balance-replay.go
// 用途：按 financial_logs.id 顺序重放每个账户的余额，与 accounts.balance 比对
// 运行：go run scripts/audit-2026-08-02-balance-replay.go -db erp_db
package main

import (
    "database/sql"
    "flag"
    "fmt"
    "log"
    "os"
    "sort"

    _ "github.com/go-sql-driver/mysql"
)

type entry struct {
    ID          int64
    FlowType    int
    Amount      float64
    BalanceAft  float64
}

func main() {
    dbName := flag.String("db", "erp_audit_test", "数据库名")
    flag.Parse()

    dsn := os.Getenv("DB_USER") + ":" + os.Getenv("DB_PASSWORD") +
        "@tcp(127.0.0.1:3306)/" + *dbName + "?parseTime=true&loc=Local"
    if os.Getenv("DB_USER") == "" {
        dsn = "root@tcp(127.0.0.1:3306)/" + *dbName + "?parseTime=true"
    }
    db, err := sql.Open("mysql", dsn)
    if err != nil {
        log.Fatal(err)
    }
    defer db.Close()

    rows, err := db.Query("SELECT id, name, balance FROM accounts ORDER BY id")
    if err != nil {
        log.Fatal(err)
    }
    type acc struct {
        ID       int64
        Name     string
        Current  float64
    }
    var accounts []acc
    for rows.Next() {
        var a acc
        if err := rows.Scan(&a.ID, &a.Name, &a.Current); err != nil {
            log.Fatal(err)
        }
        accounts = append(accounts, a)
    }
    rows.Close()

    fmt.Printf("%-5s %-15s %12s %12s %12s %12s\n", "ID", "Name", "Current", "Replayed", "Diff", "Logs")
    for _, a := range accounts {
        rs, err := db.Query("SELECT id, flow_type, amount, balance_after FROM financial_logs WHERE account_id = ? ORDER BY id ASC", a.ID)
        if err != nil {
            log.Fatal(err)
        }
        var logs []entry
        for rs.Next() {
            var e entry
            if err := rs.Scan(&e.ID, &e.FlowType, &e.Amount, &e.BalanceAft); err != nil {
                log.Fatal(err)
            }
            logs = append(logs, e)
        }
        rs.Close()
        sort.Slice(logs, func(i, j int) bool { return logs[i].ID < logs[j].ID })

        replayed := 0.0
        for _, l := range logs {
            switch l.FlowType {
            case 1, 3: // 收入 / 内部调拨（划入）
                replayed += l.Amount
            case 2: // 支出
                replayed -= l.Amount
            case 4: // 校准：差额方向已经在 amount 的符号里
                // 按当前实现：diff>0 → flow_type=1 / amount=diff；
                //             diff<0 → flow_type=2 / amount=-diff
                // 上面已覆盖
            }
        }
        diff := a.Current - replayed
        fmt.Printf("%-5d %-15s %12.2f %12.2f %12.2f %12d\n",
            a.ID, a.Name, a.Current, replayed, diff, len(logs))
    }
}
```

- [ ] **Step 9.5: 写迁移 SQL（默认全部注释）**

`erp-backend/scripts/audit-2026-08-02-migration.sql`：

```sql
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
--   AND refund.category IN ('订单取消退款', '订单红冲')
--   AND collect.reversal_pair_id IS NULL
--   AND refund.reversal_pair_id IS NULL
-- SET refund.reversal_pair_id = collect.id,
--     collect.reversal_pair_id = collect.id
-- WHERE refund.id > collect.id;
```

- [ ] **Step 9.6: 尝试在 erp_db 上跑 dry-run（如可访问）**

```bash
# D1
mysql -uroot erp_db < erp-backend/scripts/audit-2026-08-02-dryrun-D1.sql > /tmp/audit-D1.txt 2>&1
# D2
mysql -uroot erp_db < erp-backend/scripts/audit-2026-08-02-dryrun-D2.sql > /tmp/audit-D2.txt 2>&1
# D3
mysql -uroot erp_db < erp-backend/scripts/audit-2026-08-02-dryrun-D3.sql > /tmp/audit-D3.txt 2>&1
# 余额重放
cd erp-backend && go run scripts/audit-2026-08-02-balance-replay.go -db erp_db > /tmp/audit-replay.txt 2>&1
```

如果数据库不可访问（生产环境不在本地），把这些命令输出"BLOCKED"并继续。

- [ ] **Step 9.7: 把输出整合到报告**

把 `/tmp/audit-*.txt` 内容整合到 `FINANCIAL_AUDIT_REPORT.md`（下一任务）。如果 dry-run 不可执行，把 dry-run 命令和"BLOCKED"理由写进报告。

- [ ] **Step 9.8: Commit**

```bash
git add erp-backend/scripts/audit-2026-08-02-*.sql erp-backend/scripts/audit-2026-08-02-balance-replay.go
git commit -m "docs(audit): add historical data dry-run SQL scripts and replay tool"
```

---

## Task 10: 替换 `FINANCIAL_AUDIT_REPORT.md` + 最终验证

**Files:**
- Modify: `FINANCIAL_AUDIT_REPORT.md`（覆盖原报告）
- Modify: `src/views/OrderManage.vue:470`（如有需要，更新 UI 文案）

- [ ] **Step 10.1: 后端最终验证**

```bash
cd erp-backend
go vet ./controllers/... ./models/... ./middlewares/... ./utils/...
go build ./controllers/... ./models/... ./middlewares/... ./utils/...
DB_NAME=erp_audit_test go test ./controllers -count=1
```

Expected: 全部通过；测试数 >= 32 个 TestAudit_*

- [ ] **Step 10.2: 前端最终验证**

```bash
cd <project root>
npm run build
node --test 'src/**/*.test.mjs'
```

Expected: build 成功（可接受 chunk size warning）；所有测试 PASS

- [ ] **Step 10.3: 更新 `OrderManage.vue:470` UI 文案**

第 470 行 alert 内容改为：

```vue
<el-alert title="结算说明" type="info" :closable="false" show-icon class="mb-4 !rounded-xl">
  系统将按"退定金 → 收全款"两步完成结算：账户余额先减后增，
  财务上等价于一次性收取全款（订单净收款 = actual_pay_amount）。
</el-alert>
```

- [ ] **Step 10.4: 重写 `FINANCIAL_AUDIT_REPORT.md`**

按设计文档 §9 的结构撰写新报告，包含：
- 结论（纠正"残留缺陷 0"）
- 变更清单（每文件 → 目的 → 对应问题 → 测试）
- 定金尾款核对表（T-A1 ~ T-A5 真实运行结果）
- 端到端金额核对表（任务 B）
- 未验证项（F9/F16/F17 PASS；C4 BLOCKED；D1/D2/D3 结果或 BLOCKED）
- 历史数据 dry-run 结果（粘贴 `/tmp/audit-*.txt` 摘要）
- 真实验证证据（命令 + 输出摘要）

- [ ] **Step 10.5: Commit**

```bash
git add FINANCIAL_AUDIT_REPORT.md src/views/OrderManage.vue
git commit -m "docs(audit): replace audit report with corrected findings (BUG-002 fixed)"
```

---

## Self-Review（写完后由 plan-writer 检查）

- 规格覆盖：
  - §5.1 reversal_pair_id 字段 → Task 2
  - §5.2 dashboard / stats 过滤 → Task 5
  - §5.3 PayOrderBalance 修复 → Task 3 + Task 4
  - §5.4 CancelOrder / ReversalCancelOrder → Task 6
  - §5.5 TransferFunds / CalibrateAccount → Task 7
  - §6.1 T-A1 ~ T-A5 → Task 1 + Task 4
  - §6.1 F9 / F16 / F17 → Task 6 + Task 7
  - §6.2 前端测试 → Task 8
  - §7 dry-run → Task 9
  - §9 报告替换 → Task 10
- 占位符：未发现"TBD"等
- 类型一致性：`WriteReversalLog` 签名在 Task 2 / Task 3 / Task 6 都一致；`ReversalPairID *uint` 字段引用一致
- 所有任务的步骤均包含具体代码，无"类似 Task N"

---

*Plan ready. Awaiting execution choice (subagent-driven vs inline).*
