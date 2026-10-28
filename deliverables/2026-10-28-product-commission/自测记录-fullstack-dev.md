# 自测记录 · 单商品单独设置提成

> 实现人：寇豆码（fullstack-dev） 2026-10-28
> 契约依据：技术方案.md（2026-10-28 冻结版），未偏离任何冻结项

## 一、验收标准逐条自测

| # | 验收标准 | 自测方法 | 结果 | 证据 |
|---|---|---|---|---|
| 1 | Product 新增 3 字段，AutoMigrate 自动加列 | go build ./... 通过；测试环境（docker mysql:8.0，端口 3307）InitDB AutoMigrate 成功建列 | ✅ | 测试连接输出 "Database connection successful / migration completed" |
| 2 | calcOrderCommission 契约 16 条用例 | `go test ./controllers/ -run 'Commission|Commissionable' -v` | ✅ 全过 | 16 个 TestCalcOrderCommission* 全 PASS（含 ①mode=0 逐分对拍旧函数 ⑤赠品不发固定提成 ⑧固定金额不受折扣 ⑨no_commission 残留 mode 防御 ⑪全额折扣百分比=0固定照发） |
| 3 | 旧 commissionableOrderAmount 测试不回归 | 同上命令 | ✅ | TestCommissionableOrderAmount 5 子用例 PASS |
| 4 | 开单循环双变量累计、商会单分支不动 | 代码走查：order_controller.go:506-507 双变量+快照，755 商会分支公式原样（仅变量名等价替换），734-749 其余分支结构不变 | ✅ | 走查记录见下节 |
| 5 | Create/Update DTO 三字段 + 校验 + 归零 | normalizeProductCommission（product_controller.go:49）：no_commission 优先静默归零；mode∉{0,1,2}→400「提成模式非法」；mode=1 越界→400「请设置提成比例（大于0且不超过1）」；mode=2 ≤0→400「请设置每台固定提成金额」；Update map 全量写三键（:414-416） | ✅ | 代码走查 + 与契约 §5.2/§5.4 逐字核对 |
| 6 | operation_history 三标签 + 中文格式化 | operation_history_controller.go:554-556 三标签；:1014 fixed 归入货币；:1018 commission_mode 中文映射；commission_rate 复用既有百分比 case | ✅ | 代码走查 |
| 7 | PC/手机表单三选一 + 条件校验 + 回填 | ProductManage.vue PC:361-384、手机:453-476 同步改造；formRules 条件校验 :985-1004；openAddDialog 重置 :1182；handleEdit 回填（undefined 回退 0）:1200-1202；submitProduct 状态落回 :1367-1372；提示含「商会订单不使用商品单独提成」 | ✅ | 代码走查 |
| 8 | 列表角标 mode=1「提成X%」/ mode=2「提成¥N/台」 | commissionBadge（:1409-1416），PC:145、手机:227 两处接入，与「无提成」互斥 | ✅ | 代码走查 |
| 9 | 前端构建无报错 | `npm run build`（vite） | ✅ | ✓ built in 3.17s |
| 10 | 后端全量无回归 | `go test ./...` | ✅（1 个既有失败与本改动无关） | 见下「异常场景」 |

## 二、异常场景自测

| 场景 | 期望 | 结果 |
|---|---|---|
| mode=0 老商品开单（含折扣摊入/国补/全排除/无折扣 4 变体） | 与旧公式逐分一致 | ✅ 用例1 四子用例对拍 `commissionableOrderAmount`+`moneyCents` 完全相等 |
| 商会单（OrderType=2） | 资金池公式不变 | ✅ chamberCommissionCents 分支一行未改（等价变量替换，数学恒等） |
| 老板客户整单 NoCommission | 提成=0 | ✅ 分支结构未动 |
| 员工不存在 | 提成=0 | ✅ 用例10 + 分支未动 |
| no_commission 商品 mode 残留非 0 | 两部分均不计 | ✅ 用例9 |
| 3×0.17 元×0.03 逐项取分 | 0.03（无累计漂移，汇总取分会得 0.02） | ✅ 用例12 |
| 病态输入 defaultSales>totalAmount | min 钳制不放大 | ✅ 用例16 |

## 三、mode=0 兼容性走查结论

- **累计口径**：老商品 mode=0 → customSales=0，defaultSales ≡ 原 commissionableSales（赠品成交价在累计前已置 0、NoCommission 商品均排除，与旧逻辑完全同源）。
- **提成公式**：calcOrderCommission 默认段 `moneyCents(commissionBase×min(defaultSales,total)/total×rate)/100` 与旧 `moneyCents(commissionableOrderAmount(...)*rate)/100` 数学同构，测试用例 1/14 直接调用旧函数对拍逐分相等。
- **商会单**：`chamberCommissionCents(defaultSales+customSales)`，两者之和即原 commissionableSales，金额恒等；chamberCommissionCents 及资金池分摊零改动。
- **手动改提成接口（2117-2165）**：一行未动。
- **历史订单**：无任何重算/回填逻辑，仅开单时计算一次。

## 四、自测命令输出摘要

```
$ go build ./...                                   # 通过（无输出）
$ go test ./controllers/ -run 'Commission|Commissionable' -v
    --- PASS: TestCalcOrderCommissionMode0MatchesLegacyFormula (4 子用例)
    --- PASS: TestCalcOrderCommissionMode1TwoRates / MixedMode0And1 / Mode2MultiQty
    --- PASS: TestCalcOrderCommissionGiftSkipsFixedCommission / Mode1ShrinksWithDiscount
    --- PASS: TestCalcOrderCommissionSubsidyKeepsSalesBasis / Mode2NotAffectedByDiscount
    --- PASS: TestCalcOrderCommissionNoCommissionResidualMode / NoEmployee
    --- PASS: TestCalcOrderCommissionFullDiscount / Mode1CentRounding / GiftMode1
    --- PASS: TestCalcOrderCommissionDefaultOnlyEqualsLegacy / FixedOnly / ClampsDefaultSales
    --- PASS: TestCommissionableOrderAmount (5 子用例，回归)
PASS  ok  erp-backend/controllers 4.5s
$ go test ./...                                    # controllers/models/services 全 ok；
    # 仅 routers TestSwaggerMatchesRegisteredAPIRoutes 失败：
    # GET /api/finance/sales-analysis 未写入 Swagger —— 既有问题（finance_sales_controller.go
    # 与 router.go:124 均非本次改动文件），与本功能无关
$ npm run build                                    # ✓ built in 3.17s，无报错
```

## 五、涉及文件清单

| 文件 | 类型 | 关键位置 |
|---|---|---|
| erp-backend/models/core_models.go | 修改 | :66-73 Product 三字段（tag 按契约逐字） |
| erp-backend/controllers/order_commission.go | **新建** | CommissionItemInput + calcOrderCommission 纯函数 |
| erp-backend/controllers/order_commission_test.go | **新建** | 契约 16 条用例 |
| erp-backend/controllers/order_controller.go | 修改 | :506-507 双变量+项快照；:655 快照收集；:751-761 提成段（商会分支公式未动） |
| erp-backend/controllers/product_controller.go | 修改 | :18-66 DTO 三字段 + normalizeProductCommission；:297/:366 Create/Update 校验；:414-416 Update map 三键 |
| erp-backend/controllers/operation_history_controller.go | 修改 | :554-556 标签；:1014/:1018 格式化 |
| src/views/ProductManage.vue | 修改 | PC 表单 :361-384 / 手机 :453-476；角标 :145/:227 + :1409；productForm :966-968；formRules :985-1004；回填 :1200-1202；提交同步 :1367-1372 |

复用确认：moneyCents、commissionableOrderAmount（保留供测试对拍）、chamberCommissionCents、operationCurrency/operationNumber 均复用，无重复实现。

## 六、未覆盖项 / 遗留问题

1. `go test ./...` 中 routers 包 Swagger 测试失败为既有问题（sales-analysis 路由缺注解），不属本卡范围，待主理人裁决是否另开卡修复。
2. 测试环境为本地 docker mysql:8.0（3307）；生产 MariaDB 10.6 的手工 ALTER SQL 已在技术方案 §3.2 归档，未在本机验证 MariaDB 方言（AutoMigrate 双通道兼容，风险极低）。
3. 测试期间检测到并行会话在工作区写入 dashboard_breakdown_controller.go（19:19-19:20 短暂破坏 controllers 包编译，后由其作者修复），本记录中所有测试结果均在可用编译状态下取得。
