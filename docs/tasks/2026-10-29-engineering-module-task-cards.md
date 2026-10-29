# 工程模块 · 并行开发任务卡

| 项 | 内容 |
| --- | --- |
| 文档编号 | TASK-ENG-001 |
| 版本 | v1.0（**开发执行基线**） |
| 日期 | 2026-10-29 |
| 上游文档 | SRS-ENG-001 v1.1（需求）、SPEC-ENG-001 v1.1（设计）、TECH-ENG-001 v1.1（技术方案）、**API-ENG-001 v1.1（接口契约，冻结）** |
| 卡片数 | 16 张（T01 ~ T16），分 6 个波次 |
| 已知约定 | 工程单不计算提成；工程费用独立成账，不写 `financial_logs` / 不改账户余额 |
| 适用对象 | 执行本模块的后端与前端开发人员（可多人并行） |

---

## 0. 使用规则（开工前必读）

### 0.1 一人一卡，卡自包含

每张卡写清了**读哪份文档的哪一节、改哪些文件、写什么函数、怎么验、不许碰什么**。拿到卡不需要再问别人上下文，也不需要读其他卡。

### 0.2 四条硬铁律

| 编号 | 铁律 | 违反后果 |
| --- | --- | --- |
| H1 | **不写 `orders` / `order_items` / `financial_logs` / `accounts` / `account_id=7`** | 直接打回，工程模块的全部价值就是"与零售/国补/经营开销零耦合" |
| H2 | **不改接口契约的路径、方法、字段名**。契约是冻结基线，前端按它写、后端按它实现 | 任一方擅自改名 = 双方同时返工 |
| H3 | **不改别人卡的文件**（文件所有权见 §2）。需要改别人的文件，先提出来，由该卡负责人改 | 并行冲突、覆盖他人工作 |
| H4 | **每卡交付前必须自检通过**（编译 + 单测 + 手工走查），不通过不交付 | 上一轮已要求：不要让用户当测试员 |

### 0.3 卡片状态标记

```
[ ] 未开始    [~] 进行中    [!] 阻塞    [x] 完成
```

每张卡完成后在标题前打 `[x]` 并附一句"实测结论"（做了什么、验到什么程度）。

---

## 1. 并行泳道总览

### 1.1 波次与依赖

```
Wave 0  ┌─ T01 后端地基（模型/迁移/路由）
        └─ T02 前端地基（api/路由/菜单/空壳）        ← 与 T01 完全并行，不互相依赖

Wave 1  ┌─ T03 service 纯函数层 + 单测
        ├─ T04 列表 + 汇总接口
        └─ T05 跨模块联动（库存流水/审计/提示）      ← 只依赖 T01

Wave 2  ┌─ T06 项目主档 CRUD + 编号生成              ← 依赖 T03
        └─ T07 费用后端（4 接口）                    ← 依赖 T06，但与 T08 起并行

Wave 3  ┌─ T08 清单 + 节点整表保存（一致性核心）     ← 依赖 T06
        ├─ T09 回款 + 冲销 + 自动结清                ← 依赖 T08
        └─ T10 出库 + 库存扣减 + 流水                ← 依赖 T08（与 T09 并行）

Wave 4  ┌─ T11 前端列表页（6 卡 + 筛选 + 表/卡）
        ├─ T12 前端详情容器 + 概览 + 出库 Tab
        ├─ T13 前端清单 Tab + 回款 Tab
        ├─ T14 前端费用 Tab + 台账页
        └─ T15 前端弹层群（建档/回款/冲销/出库/作废）  ← 前端 5 卡可全并行，靠 §2.3 契约解耦

Wave 5  └─ T16 测试补齐 + 巡检 8 条 SQL + 回归自检     ← 依赖全部
```

### 1.2 关键路径（决定最短工期）

```
T01 → T06 → T08 → T09 → T16
```

**关键路径上全是"钱"相关的逻辑**（建档 → 清单/节点一致性 → 回款），这也符合风险分布：这四张卡出错，账就是错的；其他卡出错，最多是页面显示不好看。

### 1.3 人力配置建议

| 配置 | 排法 | 说明 |
| --- | --- | --- |
| 2 人（1 后端 + 1 前端） | 后端串行 T01→T03→T06→T08→T09→T10→T07→T16；前端 T02→T11→T12→T13→T14→T15 | 后端是瓶颈，前端可提前对 mock 开发 |
| 3 人（2 后端 + 1 前端） | 后端 A 走关键路径 T01→T06→T08→T09；后端 B 走 T03→T04→T05→T07→T10；前端同上 | 推荐。T07 费用后端整条支线交给 B，不占关键路径 |
| 4 人（2 后端 + 2 前端） | 前端拆两路：A 做 T11+T15，B 做 T12+T13+T14 | 前端并行度高，但 T12 的 Tab 容器需要等组件到位才能联调 |

> **前端可先行动**：T02 完成后即可用 mock 数据做 T11~T15，不必等后端接口。契约 §3 已给出完整响应示例，直接抄成 mock。

### 1.4 为什么这样拆（三处刻意的设计）

1. **T07 费用后端被单独拎出来并行**：费用只依赖"项目存在"，与清单、节点、回款、出库全无耦合。抽出来做，可以让前端在拿到费用接口后先跑通「费用 Tab + 台账页」，不必等回款链路。这是本模块唯一的额外并行机会。
2. **T11~T15 按"文件所有权"拆，不按"页面"拆**：一个页面由多人写必然冲突。这里改成"列表页归 T11、详情容器归 T12、Tab 内容归 T13/T14、弹层归 T15"，靠冻结的 props 契约对接。
3. **T05 跨模块联动独立成卡**：它要改的是全站公共文件（`operation_history.go`、`InventoryLog.vue`、记账页），风险等级与工程模块本身不同——单独一张卡，单独评审，改坏了能精准回滚。

---

## 2. 文件所有权与冻结契约

### 2.1 后端文件所有权

| 文件 | 归属卡 | 说明 |
| --- | --- | --- |
| `erp-backend/models/engineering.go` | **T01** | 6 个模型，只此一卡可改 |
| `erp-backend/config/db.go` | **T01** | 仅追加 6 行 AutoMigrate |
| `erp-backend/routers/router.go` | **T01** | 仅新增 engineering 路由组，接口全量注册 |
| `erp-backend/services/engineering_service.go` | **T03**（T06/T08/T09/T10 只调用，不改） | 导出函数签名见 §2.4 |
| `erp-backend/controllers/engineering_project_controller.go` | **T04**（列表/汇总）+ **T06**（CRUD/状态） | 两卡共用一个文件：T04 先建文件写列表与汇总，T06 追加其余函数，**不得重写对方函数** |
| `erp-backend/controllers/engineering_finance_controller.go` | **T08**（清单/节点）+ **T09**（回款） | 同上，T08 先建，T09 追加 |
| `erp-backend/controllers/engineering_shipment_controller.go` | **T10** | — |
| `erp-backend/controllers/engineering_expense_controller.go` | **T07** | — |
| `erp-backend/controllers/engineering_controller_test.go` | **T16**（T03 可先写 service 单测到 `services/engineering_service_test.go`） | 两文件不冲突 |
| `erp-backend/middlewares/operation_history.go` | **T05** | 只做追加，改一行都要评审 |

### 2.2 前端文件所有权

| 文件 | 归属卡 |
| --- | --- |
| `src/api/engineering.js` | **T02** |
| `src/router/index.js` | **T02** |
| `src/layout/Index.vue` | **T02** |
| `src/views/engineering/Index.vue` | **T11** |
| `src/views/engineering/Detail.vue` | **T12** |
| `src/views/engineering/Expenses.vue` | **T14** |
| `src/composables/useEngineeringList.js` | **T11** |
| `src/composables/useEngineeringProject.js` | **T12** |
| `src/composables/useEngineeringExpenses.js` | **T14** |
| `components/engineering/EngineeringStatCards.vue`、`FilterBar`、`ListTable`、`ListCards`、`StatusTag` | **T11** |
| `components/engineering/EngineeringOverviewPanel.vue`、`MobileRow.vue` | **T12** |
| `components/engineering/EngineeringItemTable.vue`、`PaymentPanel.vue` | **T13** |
| `components/engineering/EngineeringExpensePanel.vue`、`ExpenseDialog.vue`、`ExpenseTag.vue`、`ExpenseSummary.vue` | **T14** |
| `components/engineering/EngineeringProjectDrawer.vue`、`ReceiptDialog.vue`、`ShipmentDialog.vue`、`VoidDialog.vue` | **T15** |
| `src/views/InventoryLog.vue`、`src/views/Bookkeeping.vue` | **T05** |

> 组件目录统一 `src/components/engineering/`，下文简写为 `components/engineering/`。

### 2.3 跨卡组件契约（**冻结，不得改名改形**）

**`EngineeringStatusTag`**（T11 产出，T12/T13/T14 使用）

```js
props: { status: { type: Number, required: true } }   // 1跟进中 2已签约 3供货中 4已验收 5已结清 9已作废
// 无 emits。内部自行映射文案与色值，禁止外部传色值
```

**`EngineeringExpenseTag`**（T14 产出，T14 内部 + T11 列表净利率提示用）

```js
props: { category: { type: Number, required: true } }
// 同时导出命名常量：EXPENSE_CATEGORIES = [{value:1,label:'安装人工',color:'...'}, ...] 共 7 项
```

**`EngineeringExpenseSummary`**（T14 产出，T14 内部使用；Tab 与台账页共用）

```js
props: {
  summary: { type: Object, required: true },   // { total, paid, unpaid, year_total, count }
  byCategory: { type: Array, default: () => [] }, // [{ category, amount }] 恒 7 项
  showYear: { type: Boolean, default: true },   // 项目 Tab 传 false（项目维度无"本年"概念）
  activeCategory: { type: [Number, null], default: null }
}
emits: ['pick-category']   // 参数：category 或 null（取消）
```

**`EngineeringExpenseDialog`**（T14 产出，T14 的项目 Tab 与台账页共用）

```js
props: {
  modelValue: { type: Boolean, required: true },
  projectId: { type: Number, default: null },   // null = 公共费用模式，项目可下拉选择
  projectOptions: { type: Array, default: () => [] }, // [{ id, project_no, name }] 仅公共模式用
  expense: { type: Object, default: null }      // 传对象 = 编辑，传 null = 新增
}
emits: ['update:modelValue', 'saved']           // saved 携带保存后的 expense 对象
```

**`EngineeringMobileRow`**（T12 产出，T13/T14 复用）

```js
props: {
  title: { type: String, required: true },
  rows: { type: Array, required: true },   // [{ label, value, emphasis?: 'red'|'green'|'normal' }]
  actions: { type: Array, default: () => [] } // [{ text, type?: 'primary'|'danger', key }]
}
emits: ['action']  // 参数：action.key
```

**`Detail.vue` 对 Tab 组件的调用约定**（T12 负责接线，T13/T14 按此实现）

```vue
<EngineeringItemTable     :project="project" @refresh="reload" />
<EngineeringPaymentPanel  :project="project" @refresh="reload" @open-receipt="..." />
<EngineeringShipmentDialog :model-value="..." :project="project" :items="project.items" @saved="reload" />
<EngineeringExpensePanel  :project="project" @refresh="reload" />
```

> `project` 恒为详情接口 `GET /projects/:id` 的 `data` 原样对象，**不在子组件里重新拉取项目**。子组件只负责渲染 + 调自己的写接口，写成功后 `emit('refresh')`，由 Detail 统一重拉。

### 2.4 后端 service 导出函数契约（**冻结**）

```go
package services

// 金额与口径（纯函数，无 DB）
func RoundMoney(v float64) float64                      // 四舍五入到 2 位
func CalcContractTotal(itemsTotal, install, freight, other, discount float64) float64
func CalcEquipmentProfit(contractTotal, costTotal float64) float64
func CalcEquipMarginRate(contractTotal, costTotal float64) float64        // 返回百分数，合同额为 0 → 0
func CalcNetProfit(contractTotal, costTotal, expenseTotal float64) float64
func CalcNetMarginRate(contractTotal, costTotal, expenseTotal float64) float64

// 状态机（纯函数）
func CanTransit(from, action string) (to string, ok bool)   // action: sign/accept/revert/void
func StatusLabel(status string) string

// 校验（纯函数，返回可展示的中文错误）
func ValidateTerms(contractTotal float64, terms []TermInput) error   // 合计必须等于合同额
func ValidateItems(items []ItemInput) error                          // 数量>0、单价>=0
func ValidateExpense(e ExpenseInput) error                           // 金额>0、类别 1~7、日期合法性
func ValidateShipment(items []ItemInput, ships []ShipInput) error     // 累计发货不得超订购

// 逾期（纯函数）
func CalcOverdue(terms []TermSnapshot, today time.Time) (amount float64, count int, maxDays int)
```

> T06/T08/T09/T10 只**调用**这些函数，**不得**在自己文件里重算金额或另写一份校验。这是"口径只有一处"的技术保障。

---

## 3. 任务卡

> ### 本轮实施结论（2026-10-29 收口，T03~T16 全部完成）
>
> 各卡 DoD 勾选项不逐条回填，以本块为准（每次验收都有对应的脚本输出或截图留痕）：
>
> | 卡 | 结论 | 证据 |
> | --- | --- | --- |
> | T03 | service 纯函数层 + 单测通过 | `go test ./services/ -run TestEngineering` 全绿；金额/状态机/逾期/净利口径集中在 `engineering_service.go` |
> | T04 | 列表 + 汇总接口 | `output/cp2-check.py` 断言汇总 12 字段与列表逐项一致；`only_pending` 筛选与 `/summary` 同口径（477,580 == 477,580 已复核） |
> | T05 | 跨模块联动 | 出库写 `inventory_logs`（类型 engineering），记账页只有提示条、无工程费用入口；`operation_histories` 有 before 快照 |
> | T06 | 主档 CRUD + 状态流转 + 编号生成 | 建档幂等（`client_request_id`），编号 `GC-YYYYMMDD-NNN` 并发无重 |
> | T07 | 费用独立成账 | `engineering_expenses` **结构上无 account_id/financial_log_id**；`docs/accounting/engineering-isolation-audit.sql` 第 5 条断言该字段不存在 |
> | T08 | 清单 + 节点整表保存 | 三道平账点全部 409/400 拦截，事务回滚，库内无脏数据 |
> | T09 | 回款登记 + 冲销 + 自动结清 | 收满自动转「已结算」（P7 演示项目可复现）；冲销留 `reversed_receipt_id` 链 |
> | T10 | 出库 + 库存扣减 + 自动供货中 | 出库后项目自动进「供货中」（P5 演示项目 5 台发 3 台） |
> | T11 | 列表页 | 11 列在 1440px 视口零横向溢出（`inner=body=scroll=1070`）；逾期三重信号；6 卡数字与后端一致 |
> | T12 | 详情页容器 + 概览 + 出库 Tab | 5 个 Tab 与 URL 同步，刷新/分享保持当前 Tab |
> | T13 | 清单 Tab + 回款 Tab | 整表提交，节点 30/60/10、30/50/20、50/50 预设 |
> | T14 | 费用 Tab + 开销台账页 | 与经营开销两套账：台账页独立路由 `/admin/engineering/expenses`，7 类 chip + 公共费用筛选 |
> | T15 | 弹层群 | 建档抽屉 / 回款 / 冲销 / 出库 / 作废五个弹层真实点击走查通过 |
> | T16 | 巡检 + 回归 + 走查 | `engineering-isolation-audit.sql` 9 条断言全部 0 行；上线后新增流水 0 笔（工程零污染）；PC/移动端视觉走查完成；**出库→验收→结清全链路在 UI 里真实点通（12 项断言全过）** |
>
> **本轮修复的五个真问题**（都是走查/全量回归抓出来的，不是开发时发现的）：
> 1. `views/engineering/Detail.vue` 解构时漏了 `addShipment`，出库提交抛 `ReferenceError` 被 catch 静默吞掉 —— 出库按钮点了没反应，接口一次都没发出。已补；现点击后能正常到达后端并进入「库存不足 → 确认负库存」确认态。
> 2. 列表行 DTO 的待回款/逾期没有排除已作废项目，勾选「含已作废」时会出现一笔永远追不回的待回款并标红，与 `/summary` 口径（只算状态 1~4）不一致。已在 `buildEngProjectListItem` 里对状态 9 归零。
> 3. **收齐款就把「出库」「验收」跳过了**（2026-10-29 晚发现）。`CreateEngineeringReceipt` 的自动结清条件写成 `status IN (2,3,4)`，等于刚签约甚至没发货的项目，只要客户先打全款就直接置「已结清」，`accept_date` 永远是空 —— 库里的 10 号项目就是这样从「供货中」跳到 5 的。SRS §4.1 状态表写的是 **5 只能由 4 已验收自动触发**。已改为只认状态 4，并配套在验收动作里补「若已收齐则同事务直接结清」，否则已收齐的项目会卡在「已验收」不动。前端提示同步区分「已确认验收」与「已验收，款项已收齐，项目自动结清」。
> 4. **16 个工程接口一条 Swagger 注解都没有**，`go test ./...` 一跑即挂 `TestSwaggerMatchesRegisteredAPIRoutes`（另外还翻出 3 条 10-28 加的 finance 钻取接口注解写了但 swagger.json 停在 10-26 没重新生成）。已补齐 16 个接口注解（13 个 query 参数、3 处路径参数、9 个请求体）并重新生成文档，契约测试转绿。教训：**只跑单测不算回归，`go test ./...` 必须跑**。
> 5. 走查脚本暴露的「误报」也顺手记下，避免下次误判：出库弹窗按设计 §8 **打开即默认勾选并填满待发数**，再点一次勾选框反而会清成 0 —— 不是 bug。

---

### [x] T01 后端地基：6 张表 + 迁移 + 路由骨架 + 权限

> **实测结论（2026-10-29 20:52）**：`models/engineering.go`（6 模型）+ `config/db.go` 追加 AutoMigrate + `routers/router.go` 在既有 `adminAuth` 分组内注册 16 条路由（该分组已挂 AdminAuthRequired + OperationHistory，无需重复挂）。`go build ./...` 通过；后端重启后 6 张表全部建成（`engineering_projects/items/payment_terms/receipts/shipments/expenses`），`engineering_expenses.project_id` 实测可空、`engineering_projects.expense_total` 存在；接口权限实测：无 token 401、员工 token 403、管理员 200+501 占位；回归 `GET /api/expenses`、`GET /api/orders`、`GET /api/products` 均 200。

| 项 | 内容 |
| --- | --- |
| 批次 | B1 |
| 依赖 | 无 |
| 并行 | 与 T02 完全并行 |
| 角色 | 后端 |
| 产出 | `models/engineering.go`、`config/db.go`（追加）、`routers/router.go`（追加） |

**输入**：TECH-ENG-001 §2.1（表结构全文）、§1.2（既有文件改动）；API-ENG-001 §2（响应包络与枚举）

**实现要点**

1. 新建 `models/engineering.go`，按技术方案 §2.1 逐字落地 6 个模型：`EngineeringProject`、`EngineeringProjectItem`、`EngineeringPaymentTerm`、`EngineeringReceipt`、`EngineeringShipment`、`EngineeringExpense`；字段名、`gorm` tag、`json` tag、索引、注释全部照抄，**不自行增删字段**。
2. `EngineeringExpense.ProjectID` 必须是 `*uint`（可空 = 公共费用）；`EngineeringReceipt.ReversedReceiptID` 自关联；金额字段统一 `float64` + `decimal(12,2)`。
3. `config/db.go` 的 `AutoMigrate(...)` 追加 6 个模型，**只追加**，不动既有列表顺序。
4. `routers/router.go` 新增分组：

```go
// 注意：现有代码把 auth 分组（已挂 AuthRequired + OperationHistoryMiddleware）
// 里的管理员校验写成逐路由挂 middlewares.AdminAuthRequired()。
// 本模块整组挂是 gin 等价写法，更不容易漏；评审若要求完全对齐既有风格，
// 改成逐路由追加同名中间件即可，但两种写法不可混用。
eng := auth.Group("/admin/engineering")
eng.Use(middlewares.AdminAuthRequired())   // 员工 token 一律 403
{
    eng.GET("/summary", ...)
    eng.GET("/projects", ...)
    eng.POST("/projects", ...)
    eng.GET("/projects/:id", ...)
    eng.PUT("/projects/:id", ...)
    eng.DELETE("/projects/:id", ...)
    eng.PUT("/projects/:id/status", ...)
    eng.PUT("/projects/:id/items", ...)
    eng.PUT("/projects/:id/payment-terms", ...)
    eng.POST("/projects/:id/receipts", ...)
    eng.POST("/projects/:id/receipts/:rid/reverse", ...)
    eng.POST("/projects/:id/shipments", ...)
    eng.GET("/expenses", ...)
    eng.POST("/expenses", ...)
    eng.PUT("/expenses/:eid", ...)
    eng.DELETE("/expenses/:eid", ...)
}
```

5. 本卡只要求**路由可注册、编译通过**：handler 先写 `c.JSON(200, gin.H{"code": 501, "msg": "not implemented"})` 占位，由后续卡逐个替换。**不要在本卡写业务逻辑**。
6. 启动后端验证 `AutoMigrate` 建表成功，6 张表出现在 `gubuy-erp-db` 中。

**验收（DoD）**

- [ ] `go build ./...` 通过
- [ ] 后端启动无报错，`information_schema` 中 6 张 `engineering_*` 表存在，字段与文档一致（重点核对 `project_id` 可空、`expense_total` 存在）
- [ ] 16 条路由全部注册（`gin` 启动日志或 `/debug` 可见）
- [ ] 用员工 token 请求任一接口返回 403；管理员 token 返回 501 占位

**自测**

```bash
cd erp-backend && go build ./... && go vet ./...
# 起服务后
curl -s -H "Authorization: Bearer <admin_token>" http://127.0.0.1:8081/api/admin/engineering/summary
```

**禁区**

- 不动 `models/core_models.go`（一个字段都不加）
- 不动 `controllers/order_controller.go`、`bookkeeping_*`、`dashboard_*`、`payroll_*`、`finance_*`、任何 `subsidy_*`
- 不写业务逻辑到 handler 里

---

### [x] T02 前端地基：api 层 + 3 条路由 + 菜单 + 空壳页

> **实测结论（2026-10-29 20:55）**：`src/api/engineering.js` 16 个函数与契约 §4 逐字一致（`grep -c "^export const"` = 16）；`src/router/index.js` 3 条路由，`engineering/expenses` 已排在 `engineering/:id` 之前；`src/layout/Index.vue` PC 与移动端菜单各新增「工程」（`Suitcase` 图标，位于「订单」之后）；3 个骨架页 `views/engineering/{Index,Detail,Expenses}.vue` 就位。`npm run build` 通过；Vite 开发服务器实测三个新模块与 api 文件均 200。

| 项 | 内容 |
| --- | --- |
| 批次 | B10（前端部分） |
| 依赖 | 无（按契约写，不等后端） |
| 并行 | 与 T01 完全并行 |
| 角色 | 前端 |
| 产出 | `src/api/engineering.js`、`src/router/index.js`、`src/layout/Index.vue`、3 个空壳页面 |

**输入**：API-ENG-001 §4（前端调用映射，**16 个函数，与 16 个接口 1:1**）；SPEC-ENG-001 §2.1 菜单、§2.2 路由

**实现要点**

1. `src/api/engineering.js` 按契约 §4 **逐字**实现全部函数（项目 12 个 + 费用 4 个 = 16 个），风格对齐 `src/api/orders.js`，统一用 `src/utils/request.js`。
2. `src/router/index.js` 新增 3 条路由，**`/admin/engineering/expenses` 必须注册在 `/admin/engineering/:id` 之前**（Vue Router 静态优先，顺序错了台账页会被详情页吞掉）。全部挂在管理端布局下，员工端不加。
3. `src/layout/Index.vue` 的 `menuItems` 与 `mobileMenuItems` **各加一项**「工程」，位置在「订单」之后，图标 `Suitcase`。
4. 三个空壳页面 `Index.vue` / `Detail.vue` / `Expenses.vue` 各写最小可用骨架（标题 + 一行占位文案），保证路由可跳通、无报错。**具体内容由 T11/T12/T14 填充，本卡不写业务 UI**。
5. 菜单高亮：`el-menu` 的 `default-active` 需能正确匹配 `/admin/engineering` 及其子路由。

**验收（DoD）**

- [ ] `npm run build` 通过
- [ ] 侧边栏出现「工程」，点击进入列表空壳；移动端菜单也有；`/terminal` 无入口
- [ ] 直接访问 `/admin/engineering/expenses` 进的是台账空壳，**不是**详情页（顺序验证）
- [ ] 刷新 `/admin/engineering/123?tab=payments` 不白屏、不报错
- [ ] `engineering.js` 中函数名与契约 §4 完全一致，**共 16 个**，一个不多一个不少

**自测**

```bash
npm run build
# dev 起来后手动点：菜单 → 工程 → 工程开销（验证顺序）
```

**禁区**

- 不在本卡写页面业务 UI（会和 T11/T12/T14 冲突）
- 不改 `src/utils/request.js`
- **不得复用既有经营开销接口** `POST /api/admin/expenses`（那是日常开销，混用即混账）；工程费用一律走 `/api/admin/engineering/expenses`

---

### [x] T03 service 纯函数层 + 单测

| 项 | 内容 |
| --- | --- |
| 批次 | B2 |
| 依赖 | T01（模型存在即可，不依赖 DB 连接） |
| 并行 | 与 T04、T05 |
| 角色 | 后端（最懂账的人做这张卡） |
| 产出 | `services/engineering_service.go`、`services/engineering_service_test.go` |

**输入**：TECH-ENG-001 §4（金额口径 4.1~4.6）、§6（状态机）；SRS §6（规则 R1~R15）、F10.1（净利口径）

**实现要点**

1. 按 §2.4 冻结签名实现全部导出函数，**一个不多一个不少**（后续卡按签名调用，多写少写都会编译失败）。
2. 金额一律先 `RoundMoney` 再参与下一步，避免 `0.1+0.2` 类误差累积；`CalcContractTotal` 中折让可以为负向抵扣，结果**允许为负**（真实业务有零元赠送单），不做下限截断。
3. `ValidateTerms` 比较用**分**为单位（`int64(math.Round(v*100))`）而非浮点直接 `==`，差额报错文案带具体差额：`付款节点合计 ¥860,000.00 与合同总额 ¥870,000.00 不一致，差额 ¥10,000.00`。
4. `CanTransit` 严格实现技术方案 §6 的状态机表：`sign`（1→2）、`accept`（3→4）、`revert`（仅退一步）、`void`（任意态→9，状态 5 结清后不允许作废）。
5. `CalcOverdue`：仅统计 `planned_amount > received_amount` 且 `planned_date < today` 的节点；`received_amount >= planned_amount` 视为该节点已收满，不计逾期。
6. 单测覆盖至少 30 个用例：金额四舍五入边界（0.005 → 0.01）、折让大于合计、节点合计差 0.01、状态机全部合法/非法转移、逾期跨月/跨年、净利为负、合同额为 0 时毛利率返回 0（不 panic）。

**验收（DoD）**

- [ ] `go build ./...` 通过，`go test ./services/ -run TestEngineering -v` 全绿
- [ ] 单测 ≥ 30 个断言组，覆盖金额/状态机/校验/逾期/净利五类
- [ ] 用真实数据手算一遍：合同 860,000 − 成本 690,000 − 费用 46,800 = 净利 123,200，净利率 14.33%，函数输出一致

**自测**

```bash
cd erp-backend && go test ./services/ -v -run TestEngineering
```

**禁区**

- service 里**不出现** `gorm` / `*gin.Context` / `http` 任何依赖（纯函数才可测）
- 不写 DB 查询（查库的聚合在 controller 里，见技术方案 §4.6）

---

### [x] T04 列表 + 汇总接口

| 项 | 内容 |
| --- | --- |
| 批次 | B3 |
| 依赖 | T01；T03 的 `CalcOverdue`/`CalcEquipMarginRate`（可先用桩，联调时换真） |
| 并行 | 与 T03、T05 |
| 角色 | 后端 |
| 产出 | `controllers/engineering_project_controller.go`（仅 `ListProjects`、`GetSummary`）、`routers` 对应 handler 替换 |

**输入**：API-ENG-001 §3.1（汇总）、§3.2（列表）；SPEC-ENG-001 §4.3（汇总卡口径）、§4.4（列表列与筛选）

**实现要点**

1. `GET /summary` 返回 10 个字段（见契约 §3.1），其中 `overdue_amount`/`overdue_term_count` 用 `CalcOverdue` 计算；`year_received` 按 `payment_date` 本年；`avg_margin_rate`、`gross_profit` **仅统计状态 ≥2**。
2. `GET /projects` 支持契约 §3.2 全部筛选参数：`keyword`（项目名/编号/甲方，`LIKE` 双字段）、`status`（多选）、`owner_id`、`project_type`、`overdue=1`、`pending_only=1`、`include_void`、`sort_by`、`page`、`page_size`。`total` 在响应顶层。
3. 列表行需返回派生字段：`pending_amount`、`received_ratio`、`net_profit`、`net_margin_rate`、`overdue_days`、`shipped_qty`、`ordered_qty`，**全部 SQL 或内存单遍算，禁止 N+1 循环查库**。
4. 逾期筛选（`overdue=1`）用子查询 `EXISTS (SELECT 1 FROM engineering_payment_terms t WHERE t.project_id = p.id AND t.planned_date < CURDATE() AND t.received_amount < t.planned_amount)`，不要先拉全量再内存过滤。
5. 软删除默认排除（`deleted_at IS NULL`）；`include_void=1` 才带状态 9。

**验收（DoD）**

- [ ] 造 3 个项目（1 个逾期、1 个无逾期、1 个作废），各筛选组合返回结果正确
- [ ] 汇总卡 6 个数字与列表逐项对得上（手工核算）
- [ ] 100 条项目时列表接口 P95 < 300ms，`EXPLAIN` 无全表扫描
- [ ] `total` 与 `page_size=10` 分页行为正确，超出页码返回空数组而非报错

**自测**

```bash
curl -s -H "Authorization: Bearer $TOKEN" "http://127.0.0.1:8081/api/admin/engineering/projects?page=1&page_size=10&sort_by=pending_amount"
```

**禁区**

- 不在本卡写创建/更新/删除（属 T06）
- 不把汇总卡的数算在前端（口径必须单一来源）

---

### [x] T05 跨模块联动：库存流水类型 + 审计 + 记账页提示

| 项 | 内容 |
| --- | --- |
| 批次 | B10 |
| 依赖 | T01（路由已注册） |
| 并行 | 与 T03、T04 |
| 角色 | 后端 + 前端（一人即可，改动小但敏感） |
| 产出 | `middlewares/operation_history.go`（追加）、`src/views/InventoryLog.vue`、`src/views/Bookkeeping.vue` |

**输入**：TECH-ENG-001 §7.1（12 条审计映射）、§7.2（pathParamTable 分支）、§7.3（评审拦截项）；SRS R11（禁止混账）

**实现要点**

1. `operationRoutes` **追加** 12 条（技术方案 §7.1 全文照抄），不修改、不重排任何既有条目。**只做追加**——这个文件是全站审计入口，改错会影响所有模块。
2. `pathParamTable` 的 `case "id"` 分支追加 `engineering` 前缀判断；`case "rid"` 追加 `engineering_receipts`；`case "eid"` 追加 `engineering_expenses`。
3. `InventoryLog.vue`：① 类型筛选下拉加一项「工程出库」（value `engineering`）；② 类型映射 map 加 `engineering → 工程出库`，色值用中性灰蓝，与销售/采购区分开。
4. `Bookkeeping.vue`（经营开销入口）顶部加一行说明，**不新增工程费用入口**：
   > 工程开销请到「工程 → 工程开销」记录，避免两边重复记账。
5. 写一段 `curl` 自测清单，验证审计快照真的产生（技术方案 §7 要求）。

**验收（DoD）**

- [ ] 新建 1 个项目、改 1 次清单、录 1 笔费用，`operation_histories` 各产生一条记录，`before` 快照含金额字段
- [ ] 库存流水页能按「工程出库」筛选，且既有 5 种类型（销售/入库/出库/调拨/盘点）显示不变
- [ ] 记账页顶部提示可见，且**没有**新增工程费用按钮
- [ ] `git diff middlewares/operation_history.go` 只有 `+` 行，**没有 `-` 行**（评审硬性要求）

**自测**

```bash
cd erp-backend && go build ./... && git diff --stat middlewares/operation_history.go
# 检查是否只有新增
git diff middlewares/operation_history.go | grep -c "^-" # 期望 0（除文件头）
```

**禁区**

- 修改任何既有审计条目
- 让工程费用出现在经营开销列表里（SRS R11 硬拦截）

---

### [x] T06 项目主档：创建 / 详情 / 更新 / 删除 / 状态流转 / 编号生成

| 项 | 内容 |
| --- | --- |
| 批次 | B4 |
| 依赖 | T03（service 函数）、T01 |
| 并行 | 与 T07 |
| 角色 | 后端 |
| 产出 | `controllers/engineering_project_controller.go`（追加 CRUD 与状态 5 个函数） |

**输入**：API-ENG-001 §3.3~§3.7；SPEC-ENG-001 §5.1（详情头部）、§5.2（概览 Tab）；SRS F1/F2/F9

**实现要点**

1. **编号生成** `GC-YYYYMMDD-NNN`：当日流水号，唯一索引兜底；冲突时重试最多 3 次（每次序号 +1），3 次失败返回 500 `项目编号生成失败，请重试`。**禁止用 `MAX(id)+1`**（并发必撞）。
2. 创建：费用项默认 0，`contract_total` 由 `CalcContractTotal` 算（此时清单为空即 install+freight+other−discount）；状态固定为 1「跟进中」；`client_request_id` 幂等（重复提交返回 409，不重复建单）。
3. 详情 `GET /projects/:id`：一次返回项目 + items + terms + receipts + shipments + expenses + 派生字段（`gross_profit`、`net_profit`、`net_margin_rate`、`pending_amount`、`received_ratio`）。**expenses 只含本项目的，不含公共费用**（SRS F10.1）。
4. 更新 `PUT /projects/:id`：只允许改主档字段（名称/甲方/联系人/负责人/预计额/费用项/备注）；**清单、节点、回款、出库、费用一律不在此接口改**。已结清（状态 5）项目金额字段锁定，只允许改备注与联系方式（R10）。
5. 状态流转 `PUT /projects/:id/status`：走 `CanTransit`；`sign` 前必须校验"合同额 = 节点合计"（调 `ValidateTerms`）；`void` 需 `reason` 必填并写审计；任何状态变更前 `SELECT ... FOR UPDATE` 锁项目行。
6. 删除：仅允许状态 1「跟进中」且无回款无出库无费用的项目**硬删**；其余一律提示"请先作废"，返回 400。

**验收（DoD）**

- [ ] 并发建 10 单（脚本），编号无重复、无空洞报错
- [ ] 重复提交同一 `client_request_id` 只建 1 单，第二次返回 409
- [ ] 无节点时点「签约」被拦截，提示文案为可展示中文
- [ ] 有回款的项目删除被拒；状态 1 空项目可删
- [ ] 详情接口一次请求返回全部 5 类子数据，无 N+1

**自测**

```bash
go test ./controllers/ -run TestProjectCRUD -v
```

**禁区**

- 不在本卡写清单/节点保存逻辑（T08）
- 不写回款（T09）、出库（T10）、费用（T07）

---

### [x] T07 费用后端：台账查询 + 新增 + 编辑 + 删除（独立成账的核心卡）

| 项 | 内容 |
| --- | --- |
| 批次 | B8 |
| 依赖 | T06（项目存在） |
| 并行 | 与 T08、T09、T10 全部并行 |
| 角色 | 后端 |
| 产出 | `controllers/engineering_expense_controller.go` |

**输入**：API-ENG-001 §3.13~§3.16；SRS F10（全节）、F11（全节）、R11~R15；TECH-ENG-001 §4.6（费用汇总口径 SQL）

**实现要点**

1. `GET /expenses` 台账：支持 `keyword`（项目名/收款方/备注）、`project_id`（含 `0` = 只看公共费用）、`category`（多选）、`date_from`/`date_to`、`pay_status`、`amount_min`/`amount_max`、分页。返回 `summary`（`total`/`paid`/`unpaid`/`year_total`/`count`）+ `by_category`（**恒 7 项，0 金额也返回**）+ `list`。
2. 所有聚合**在 SQL 里完成**（技术方案 §4.6 两条 SQL），禁止把全量费用拉进内存 `reduce`。
3. 新增 `POST /expenses`：`ValidateExpense` 校验（金额 > 0、类别 1~7、日期不晚于今天、`category=7 其他` 时 `remark` 必填）；`project_id` 为空 = 公共费用；已作废项目（状态 9）新增返回 400；已结清（状态 5）允许但响应带 `warning` 字段（前端弹提示，不拦截）。
4. 编辑 `PUT /expenses/:eid`：允许改全部业务字段；**允许改所属项目**（费用录错项目是真实场景），改了要在同一事务里重算**新旧两个项目**的 `expense_total`。
5. 删除 `DELETE /expenses/:eid`：软删除（`gorm.DeletedAt`），同事务重算 `expense_total`。
6. **`expense_total` 同事务刷新**：`SELECT FOR UPDATE` 锁项目行 → `SELECT SUM(amount) WHERE project_id=? AND deleted_at IS NULL` → `UPDATE engineering_projects SET expense_total=?`。新事务内完成，失败整笔回滚（E5）。
7. 硬铁律：本卡代码里**不得出现** `financial_logs`、`accounts`、`account_id`、`Bookkeeping` 任何字样（R11 评审拦截项）。`pay_method=公户` 只是存值，不动余额。
8. **路径易错点**：系统里**已存在**经营开销接口 `POST/PUT /api/admin/expenses`（`controllers.CreateExpense`）。工程费用必须走 `/api/admin/engineering/expenses`，**不能复用也不能改既有那个**——两者是两套账，路径混了就是混账。

**验收（DoD）**

- [ ] 录 1 笔 ¥3,000 安装人工 → 项目 `expense_total` 变 3,000、净利同步减 3,000、台账合计 +3,000
- [ ] 删除该笔 → 三个数全部回到原值，`operation_histories` 有 before 快照
- [ ] 录 1 笔公共费用（`project_id=null`）→ 台账合计与「本年工程开销」增加，**任何项目净利不变**（SRS F11 验收 3）
- [ ] `category=7` 不填备注被拦截，提示"选择「其他」时必须填写备注说明"
- [ ] 已作废项目新增费用返回 400；已结清项目新增成功且响应含 `warning`
- [ ] `by_category` 恒 7 项（含 0 金额项）
- [ ] 编辑时改所属项目，新旧两个项目 `expense_total` 都正确
- [ ] `grep -i "financial_logs\|account_id" controllers/engineering_expense_controller.go` 无结果

**自测**

```bash
go test ./controllers/ -run TestExpense -v
grep -c "financial_logs" controllers/engineering_expense_controller.go   # 期望 0
```

**禁区**

- 写任何账户余额变动（这是本模块最高优先级的禁区）
- 在费用里算提成（Q4 已确认不算）
- 把公共费用摊到项目（Q 已确认不摊，R13）

---

### [x] T08 清单 + 付款节点整表保存（一致性核心）

| 项 | 内容 |
| --- | --- |
| 批次 | B5 |
| 依赖 | T06（项目 CRUD）、T03（`ValidateTerms`/`CalcContractTotal`） |
| 并行 | 与 T07 |
| 角色 | 后端（关键路径，最高优先级） |
| 产出 | `controllers/engineering_finance_controller.go`（新建，仅 items + terms 两个函数） |

**输入**：API-ENG-001 §3.8、§3.9；TECH-ENG-001 §4.1~§4.3（重算与一致性闸门）、§5.1（事务边界）；SRS R1~R5

**实现要点**

1. **两道闸门，缺一不可**：
   - 清单保存后 `contract_total` 变了 → 必须校验"新合同额 = 现有节点合计"，不等则**整笔回滚**并返回差额；
   - 节点保存时校验"节点合计 = 当前合同额"，不等则回滚。
   两者都在同一事务内，先 `SELECT ... FOR UPDATE` 锁项目行（技术方案 §5.1-①）。
2. 清单整表保存：全量替换（先删本项目 items 再插入），但**已发货行不允许删**（`shipped_qty > 0` 的行若不在提交列表里 → 400 `行「XXX」已发货 6 台，不能删除`）；数量小于已发货量同样拦截。
3. `unit_cost`（成本单价）为快照：创建/编辑时可传，不传则取 `products.purchase_price`（若无则 0）；**后续商品进价变化不影响已保存的清单**（毛利才稳定）。
4. 节点整表保存：`planned_amount` 之和必须等于 `contract_total`；`sort` 按提交顺序重排；已产生回款的节点**不允许删除，也不允许把 `planned_amount` 调到低于已收金额**。
5. 重算字段一次写回：`items_total`、`cost_total`、`contract_total`、`ordered_qty`、`gross_profit`、`net_profit`、`net_margin_rate`（都走 service 函数，不手写算式）。
6. 保存成功返回**最新完整项目对象**（前端直接 replace，不必二次拉取）。

**验收（DoD）**

- [ ] 改清单单价导致合同额变化，节点合计未同步 → 返回 400 且带差额，库内**任何字段都没变**（事务回滚验证）
- [ ] 节点合计故意差 0.01 → 拦截；改成 0.01 差额后补齐 → 通过
- [ ] 已发货行删除被拦截；数量改小于已发货被拦截
- [ ] 两人并发保存同一项目清单（脚本模拟）→ 串行执行，无数据错乱，后写的金额与节点仍自洽
- [ ] 巡检 SQL ③（合同额 = 节点合计）返回 0 行

**自测**

```sql
-- 正常应 0 行
SELECT p.id FROM engineering_projects p
  LEFT JOIN engineering_payment_terms t ON t.project_id = p.id
 WHERE p.deleted_at IS NULL AND p.status >= 2
 GROUP BY p.id HAVING p.contract_total <> COALESCE(SUM(t.planned_amount),0);
```

**禁区**

- 允许"合同额变了但节点没跟上"的任何写法（这是本模块最致命的 bug）
- 在清单里直接改 `contract_total`（必须重算）

---

### [x] T09 回款登记 + 冲销 + 自动结清

| 项 | 内容 |
| --- | --- |
| 批次 | B6 |
| 依赖 | T08（节点存在且合计已对齐） |
| 并行 | 与 T10 |
| 角色 | 后端（关键路径） |
| 产出 | `controllers/engineering_finance_controller.go`（追加 2 个函数） |

**输入**：API-ENG-001 §3.10、§3.11；TECH-ENG-001 §5.2-③④⑦（并发）；SRS F5/F6

**实现要点**

1. 登记回款用**原子条件更新**（不能"先查再改"）：

```sql
UPDATE engineering_payment_terms
   SET received_amount = received_amount + ?
 WHERE id = ? AND received_amount + ? <= planned_amount;
-- RowsAffected == 0 → 回滚 + 409「该节点应收已变化，请刷新后重试」
```

项目层同样条件更新 `received_amount + ? <= contract_total`。
2. `client_request_id` 唯一索引，命中重复键返回 409 `该笔回款已登记`（防双击/重试重复入账）。
3. 冲销：不删原记录，插入**负数金额**的新记录（`reversed_receipt_id` 指向原记录），同时把节点与项目的 `received_amount` 减回去（下界 0）；同一笔只能冲销一次（重复冲销 400）。
4. 自动结清：每次登记后判断 `received_amount >= contract_total`（分单位比较）→ 条件更新 `WHERE status IN (2,3,4)` 置 5 并写 `settled_at`；`RowsAffected=0` 静默返回（已被别人置过）。
5. 回款金额 > 0；`payment_date` 不晚于今天；`term_id` 必须属于本项目。
6. 全部在单事务内：`receipts(insert) + terms(update) + projects(update)`。

**验收（DoD）**

- [ ] 节点计划 200,000，已收 150,000，再登 60,000 → 被拦截（超收）
- [ ] 同一 `client_request_id` 提交两次 → 只入账一次，第二次 409
- [ ] 两人并发登记同一节点最后一笔（脚本各发一次）→ 只有一笔成功，另一笔 409，**不出现双倍入账**
- [ ] 收满合同额 → 状态自动跳 5，`settled_at` 有值
- [ ] 冲销一笔 → 节点与项目已收同步减少，原记录仍在，新记录为负数
- [ ] 巡检 SQL ①② 返回 0 行

**自测**

```sql
-- ① 项目已收 = 回款合计（应 0 行）
SELECT p.id FROM engineering_projects p LEFT JOIN engineering_receipts r ON r.project_id=p.id
 WHERE p.deleted_at IS NULL GROUP BY p.id HAVING p.received_amount <> COALESCE(SUM(r.amount),0);
```

**禁区**

- 用"先 SELECT 再 UPDATE"的方式改已收金额（并发必错）
- 物理删除回款记录（必须负数冲销）

---

### [x] T10 出库 + 库存扣减 + 流水 + 自动供货中

| 项 | 内容 |
| --- | --- |
| 批次 | B7 |
| 依赖 | T08（清单与已发数量） |
| 并行 | 与 T09 |
| 角色 | 后端 |
| 产出 | `controllers/engineering_shipment_controller.go` |

**输入**：API-ENG-001 §3.12；TECH-ENG-001 §5.1（事务）、§5.3-⑤⑥；SRS F7

**实现要点**

1. 复用既有 `WriteInventoryLog(...)`（不新造轮子），`log_type = "engineering"`，`related_no` = 项目编号 `project_no`，`remark` 写 `项目名 + 客户名`。
2. 库存按 `warehouse_type` 分门店（`store_stock`）/云仓（`cloud_stock`）；`SELECT ... FOR UPDATE` 锁商品行 → 算 `before/after` → `UPDATE products SET store_stock = store_stock - ?` → 写流水，全程同事务。
3. **超订购量拦截**：累计发货（含本次）> 该行 `quantity` → 400 `行「XXX」订购 12 台，已发 10 台，本次最多可发 2 台`。
4. **负库存**：库存不足时若 `allow_negative=false` → 409 `库存不足（当前 3 台，需发 5 台）。确认允许负库存请勾选后重试`；`allow_negative=true` 时放行并在流水 `remark` 追加 `负库存出库(已确认)`，同时写审计。
5. 出库成功后：`project_items.shipped_qty += qty`、`projects.shipped_qty += qty`；若有任一发货则状态 2「已签约」→ 3「供货中」（条件更新 `WHERE status=2`）。
6. 状态 9（作废）项目出库直接 400（事务内先 `SELECT status FOR UPDATE`）。
7. 出库日期不晚于今天；数量 > 0。

**验收（DoD）**

- [ ] 出库 6 台 → 商品库存 -6、`inventory_logs` 新增一条 `engineering` 流水、清单已发 +6、项目已发 +6、状态 2→3
- [ ] 超订购量被拦截，文案带具体可发数量
- [ ] 库存 3 台发 5 台（未勾允许）→ 409；勾选后 → 成功且库存为 -2，流水备注含"负库存出库"
- [ ] 作废项目出库 → 400
- [ ] 巡检 SQL ④⑤ 返回 0 行
- [ ] 零售开单同时扣同一商品，行为与改动前一致（回归）

**自测**

```bash
go test ./controllers/ -run TestShipment -v
```

**禁区**

- 自己写一套库存扣减 SQL（必须复用既有写法）
- 修改 `products` 之外的商品字段（价格、名称等一律不碰）

---

### [x] T11 前端列表页（6 汇总卡 + 筛选 + 表/卡 + 分页）

| 项 | 内容 |
| --- | --- |
| 批次 | B9-1 |
| 依赖 | T02（api 与路由）；后端可对 mock |
| 并行 | 与 T12、T13、T14、T15 |
| 角色 | 前端 |
| 产出 | `views/engineering/Index.vue`、`composables/useEngineeringList.js`、`components/engineering/EngineeringStatCards.vue`、`FilterBar.vue`、`ListTable.vue`、`ListCards.vue`、`StatusTag.vue` |

**输入**：SPEC-ENG-001 §4（全节）、§7（移动端）、§8（文案）、§2.3（视觉规范）

**实现要点**

1. 6 张汇总卡严格按 §4.3 口径与可点击行为；第 5 张「本年工程开销」点击跳 `/admin/engineering/expenses?year=当前年`；净利率为负时数字用 `#dc2626`。
2. 筛选条：关键词（防抖 300ms）、状态多选、负责人、项目类型、只看逾期、只看待回款、含作废；**筛选状态写入 URL query**，从详情 `history.back()` 回来筛选与页码不丢（§2.2 硬要求）。
3. 表格列按 §4.4；每行合同额下方小字显示净利率；逾期项目**三重信号**：行首红条 + 待回款红字带"逾期 N 天" + 状态区红标签。
4. 移动端（<768px）切卡片流（`ListCards` + 加载更多），不出现横向滚动表格。
5. 空状态：无数据时显示「还没有工程项目，点右上角新建」+ 主按钮；筛选无结果时显示「没有符合条件的项目」+ 清除筛选。
6. `StatusTag.vue` 是**全模块唯一色值来源**，严格按 §2.3 导出 props，不改形。

**验收（DoD）**

- [ ] 6 张卡数字与列表筛选结果口径一致（点击卡片能筛出对应列表）
- [ ] 逾期项目三种信号同时出现
- [ ] 进详情后浏览器返回，筛选与页码保留
- [ ] 375px 宽度无横向滚动，卡片可点进详情
- [ ] 权限：员工账号访问被路由拦截（既有守卫即可，不新增逻辑）

**禁区**

- 自己算汇总数字（一律用 `/summary`）
- 改 `api/engineering.js`（T02 所有）

---

### [x] T12 详情页容器 + 概览 Tab + 出库 Tab

| 项 | 内容 |
| --- | --- |
| 批次 | B9-2 |
| 依赖 | T02 |
| 并行 | 与 T11/T13/T14/T15 |
| 角色 | 前端 |
| 产出 | `views/engineering/Detail.vue`、`composables/useEngineeringProject.js`、`components/engineering/EngineeringOverviewPanel.vue`、`MobileRow.vue` |

**输入**：SPEC-ENG-001 §5.1、§5.2、§5.5（头部按状态的主操作）、§5.7（出库 Tab）

**实现要点**

1. 头部：项目名 + 编号 + 状态标签 + 甲方 + 三个数字（合同额/已回款/待回款），移动端两行堆叠。主操作按钮按状态动态显示（§5.5 表），点击弹对应弹层（T15 的组件，按 §2.3 契约接线）。
2. 5 个 Tab 用 `?tab=` query 驱动（`overview|items|payments|shipments|expenses`），默认 `overview`；非法值回落 overview。Tab 括号计数（清单行数、已收节点/总节点、出库单数、费用笔数）。
3. 概览 Tab：
   - 基本信息卡、金额构成卡（设备/安装/运输/其他/折让 → 合同额）
   - **盈利卡显示两层**：设备毛利（金额 + 毛利率）、项目净利（金额 + 净利率），净利率 < 10% 橙色提示、< 0 红色「负毛利」
   - 底部 11px 灰字：`净利已扣除本项目实际支出，不含公共费用分摊`
   - 回款进度条 + 发货进度条
   - 下一步建议文案（逾期 N 个节点合计 ¥X）
4. 出库 Tab：汇总一行（已出库 X 台 / 共 Y 台，门店/云仓拆分）+ 出库明细表 + 「新增出库」按钮；移动端用 `MobileRow`。
5. Detail 只负责"拉项目 + 分发数据 + 统一 reload"，**不实现各 Tab 的业务逻辑**；子组件写成功后 `emit('refresh')` → Detail 重拉。
6. 概览 Tab 数据全部来自详情接口，**不额外请求**。

**验收（DoD）**

- [ ] 直接访问 `/admin/engineering/1?tab=expenses` 刷新后停在费用 Tab
- [ ] 头部三数字 + 两个进度条 + 盈利卡与数据库一致（手算核对）
- [ ] 状态 1 时只显示「编辑 / 签约 / 作废」，状态 5 时只显示「查看」类操作
- [ ] 移动端 5 个 Tab 横向可滚不换行，内容卡片化

**禁区**

- 在 Detail 里写清单/节点/费用的业务校验（属 T13/T14）
- 改 `Index.vue`（T11 所有）

---

### [x] T13 清单 Tab + 回款 Tab

| 项 | 内容 |
| --- | --- |
| 批次 | B9-3 |
| 依赖 | T02；后端 T08/T09（可先 mock） |
| 并行 | 与 T11/T12/T14/T15 |
| 角色 | 前端 |
| 产出 | `components/engineering/EngineeringItemTable.vue`、`PaymentPanel.vue` |

**输入**：SPEC-ENG-001 §5.3（清单）、§5.4（回款）、§5.6（回款弹窗交互由 T15 实现）

**实现要点**

1. 清单：**整表编辑、一次提交**（不允许逐格保存）。编辑态支持加行/删行/改数量单价，底部实时汇总"合计金额 / 合计成本 / 毛利率"，并**实时显示与合同额的差额**（前端预览，权威值以后端返回为准）。
2. 已发货行在编辑态禁用删除、数量不可小于已发货量，给出原因 tooltip。
3. 回款：节点表（节点名/比例/计划金额/计划日期/已收/未收/状态）+ 回款记录表（日期/金额/方式/票据号/备注/操作人/冲销按钮）；顶部"已收/待收"汇总 + 逾期节点高亮。
4. 节点编辑模式：支持快捷预设 30/60/10、30/50/20、50/50 一键生成；合计与合同额不一致时**提交按钮禁用并显示差额**。
5. 冲销按钮仅对"非冲销记录"且"未被冲销过"的显示；点击走 `ElMessageBox.confirm` 二次确认。
6. 所有金额 `¥` + 千分位 + 2 位小数；所有写操作成功后 `emit('refresh')`。

**验收（DoD）**

- [ ] 清单改单价 → 合计与毛利率实时变；提交后后端返回的最新项目对象被采用
- [ ] 节点合计差 0.01 时提交按钮禁用并提示差额
- [ ] 快捷预设一键填出 3 个节点且合计正确
- [ ] 已发货行删除按钮禁用，tooltip 说明原因
- [ ] 375px 下清单与节点均为卡片化（`MobileRow`）

**禁区**

- 逐格保存
- 在前端做权威金额计算并写回后端

---

### [x] T14 费用 Tab + 工程开销台账页（"单独账目"的落地面）

| 项 | 内容 |
| --- | --- |
| 批次 | B9-4 |
| 依赖 | T02；后端 T07（可先 mock） |
| 并行 | 与 T11/T12/T13/T15 |
| 角色 | 前端 |
| 产出 | `views/engineering/Expenses.vue`、`composables/useEngineeringExpenses.js`、`components/engineering/EngineeringExpensePanel.vue`、`ExpenseDialog.vue`、`ExpenseTag.vue`、`ExpenseSummary.vue` |

**输入**：SPEC-ENG-001 §5.5（费用 Tab）、§6（台账页全节）、§8（文案）

**实现要点**

1. 费用 Tab：汇总条（合计/已付/未付）+ 七类占比 chip（点击筛选，选中紫底白字）+ 明细表 + 「录入费用」按钮。`showYear=false`。
2. 台账页：顶部 4 数字（合计/已付/未付/本年开销）+ 类别分布 + 筛选（关键词、项目含"公共费用（不分摊）"、类别多选、日期区间、付款状态、金额区间）+ 明细表 + 分页。
3. `ExpenseDialog` **一个组件服务两个入口**：`projectId` 传数字 = 项目费用（所属项目锁定只读）；传 `null` = 可下拉选项目或选「公共费用（不分摊）」。**不允许写两个弹窗**。
4. 表单校验：金额 > 0 且 ≤ 9,999,999.99；类别必选；`category=7` 时备注必填；日期不晚于今天；已付时付款方式必填；未付时付款日期置灰。
5. 已结清（状态 5）项目录费用时弹提示：`项目已结清，补录费用后净利会变化，是否继续？`（提示不拦截）。
6. 表单底部固定一行提示（防混账）：`工程费用独立记账，不会计入经营开销，也不改变任何账户余额`。
7. 删除费用二次确认，文案明确"删除后项目净利将变化"。

**验收（DoD）**

- [ ] 录一笔安装人工 → Tab 汇总、概览盈利卡净利、台账合计三处同步变化
- [ ] 台账录一笔公共费用 → 「本年工程开销」增加，任何项目净利不变
- [ ] 项目维度的"本年开销"不显示（`showYear=false` 生效）
- [ ] 类别 chip 点击筛选、再点取消，明细与合计联动
- [ ] 金额填 0 或负数被拦；选「其他」不填备注被拦
- [ ] 375px 下台账表格转卡片，无横向滚动

**禁区**

- 在费用弹窗里提供"同时记入经营开销"的勾选项（R11 硬禁止）
- 写两个费用弹窗

---

### [x] T15 弹层群：建档抽屉 / 回款 / 冲销 / 出库 / 作废

| 项 | 内容 |
| --- | --- |
| 批次 | B9-5 |
| 依赖 | T02 |
| 并行 | 与 T11/T12/T13/T14 |
| 角色 | 前端 |
| 产出 | `components/engineering/EngineeringProjectDrawer.vue`、`ReceiptDialog.vue`、`ShipmentDialog.vue`、`VoidDialog.vue` |

**输入**：SPEC-ENG-001 §4.6（建档抽屉）、§5.4.3（回款弹窗）、§5.7.2（出库弹窗）、§5.6（作废）

**实现要点**

1. 建档抽屉三分区（基本信息 / 甲方信息 / 金额构成），必填校验：项目名称、项目类型、甲方单位、负责人；预计合同额 ≥ 0。提交成功关闭并跳详情。
2. 回款弹窗：节点下拉（默认带出当前逾期节点）、金额（默认 = 该节点未收额，可改但不得超未收）、到账日期（默认今天，不晚于今天）、收款方式、票据号、备注。提交时禁用按钮防双击（`client_request_id` 由前端生成）。
3. 出库弹窗：选清单行 → 显示订购/已发/可发 → 填本次数量（≤ 可发）；门店/云仓切换显示当前库存；库存不足时出现红色提示 + 复选框`我确认允许负库存出库`（不勾则提交按钮禁用）。
4. 作废弹窗：原因必填（≥ 5 字），二次确认文案写明不可撤销 + 已录回款/出库会保留。
5. 全部弹层：移动端宽度 100%、底部按钮吸底；`Esc` 关闭前若有改动需确认。

**验收（DoD）**

- [ ] 建档必填项为空时提交按钮禁用或提示
- [ ] 回款金额超过节点未收额被拦，文案带可收上限
- [ ] 连续点击提交按钮只发一次请求
- [ ] 出库库存不足时复选框逻辑正确（勾了才能提交）
- [ ] 作废必须填原因，取消不作废
- [ ] 375px 下弹层不溢出

**禁区**

- 在弹层里自己拼接口路径（用 `api/engineering.js`）
- 改 `ExpenseDialog`（属 T14）

---

### [x] T16 测试补齐 + 巡检 8 条 SQL + 全量回归自检

| 项 | 内容 |
| --- | --- |
| 批次 | B11 |
| 依赖 | T01~T15 全部 |
| 并行 | 无（收口卡） |
| 角色 | 全员（谁写谁补测，由一人收口） |
| 产出 | `controllers/engineering_controller_test.go`、`scripts/check-engineering-consistency.sql`、自检报告 |

**输入**：TECH-ENG-001 §5.4（8 条巡检 SQL）、§8（测试点清单）、§9（验收流程）

**实现要点**

1. 补齐 controller 层单测：至少覆盖技术方案 §8 的 20 个测试点（金额一致性 5 个、并发 6 个、状态机 4 个、费用 3 个、权限 2 个）。
2. 落 `scripts/check-engineering-consistency.sql`，8 条 SQL（①项目已收=回款合计 ②节点已收=节点回款合计 ③合同额=节点合计 ④清单已发=出库合计 ⑤项目已发=清单已发合计 ⑥`expense_total`=费用合计 ⑦费用金额>0 ⑧作废项目无新增费用），每条正常返回 0 行，文件头注释写清"0 行 = 正常"。
3. 手工全链路走查一遍：建档 → 清单 → 节点 → 签约 → 回款 → 出库 → 费用 → 验收 → 结清 → 作废，各一次并记录结果。
4. **回归验证（必做）**：零售开单、国补管理、经营看板、库存流水页、工资条五个页面数据与改动前完全一致（截图或 SQL 比对）。
5. 核对硬铁律：`git diff` 中不出现 `financial_logs`、`accounts`、`order_items` 的写操作。

**验收（DoD）**

- [ ] `go build ./...`、`go vet ./...`、`go test ./...` 全绿
- [ ] `npm run build` 通过，无 console 报错
- [ ] 8 条巡检 SQL 全部 0 行
- [ ] 全链路走查 11 步全部通过并留记录
- [ ] 5 个既有页面回归无差异
- [ ] 员工端 `/terminal` 无任何工程入口

**自测**

```bash
cd erp-backend && go test ./... && cd .. && npm run build
mysql -h127.0.0.1 -P3307 -uroot -p erp_db < scripts/check-engineering-consistency.sql
```

---

## 4. 集成检查点

| 检查点 | 时机 | 必须验到 | 通过后放行 |
| --- | --- | --- | --- |
| **CP1** | T01 + T02 完成 | 表建好、16 路由通、前端 3 页可跳、菜单对 | 后端与前端各自往下并行 |

> **CP1 已通过（2026-10-29 20:55）**：6 张表建成；16 条路由全部注册（启动日志逐条可见）；权限三态实测 401/403/501；「工程」菜单 PC + 移动端就位；`/admin/engineering`、`/admin/engineering/expenses`、`/admin/engineering/1?tab=payments` 三条路由可直达（骨架页）；既有经营开销、订单、商品、零售链路回归 200。
| **CP2** | T03 + T04 + T06 完成 | 建档 → 列表可见 → 详情可开；汇总卡数字对得上 | 开始 T08（一致性核心） |
| **CP3** | T08 + T09 + T10 + T07 完成 | 全后端链路 curl 跑通；8 条巡检 SQL **全部 0 行** | 前端开始真接口联调 |
| **CP4** | T11~T15 完成 | 5 个页面 + 6 个弹层全部可用；移动端无横向滚动；筛选返回不丢 | 进入 T16 收口 |

> CP3 是**最重要的一次卡点**：8 条巡检 SQL 不全 0，说明账已经错了，绝不允许带着错账往下联调。

---

## 5. 风险与回退

| 风险 | 概率 | 影响 | 对策 |
| --- | --- | --- | --- |
| 合同额与节点合计失配 | 中 | 高（账错） | T08 双闸门 + 巡检 SQL ③ 兜底；CP3 卡点必查 |
| 并发双倍入账 | 中 | 高 | 原子条件更新 + `client_request_id` 唯一索引；单测覆盖 |
| 工程费用被误记到经营开销 | 中 | 中（口径污染） | 记账页提示 + 费用表单固定提示 + 评审 grep 拦截（R11） |
| `operation_history.go` 改错影响全站审计 | 低 | 高 | T05 单独成卡、只追加、`git diff` 检查无 `-` 行 |
| 路由顺序错导致台账页被详情页吞 | 中 | 低 | T02 验收专门一条；写死在卡里 |
| 前端 5 卡并行冲突 | 中 | 中 | 文件所有权矩阵 + 冻结 props 契约 |
| 负库存出库无审计 | 低 | 中 | 强制 `allow_negative` 落库 + 流水备注 + 审计 |

**回退方案**

1. **功能下线**：删前端菜单项 + 3 条路由 → 页面不可达，后端接口与 6 张表保留，对既有功能零影响。
2. **彻底回退**：`DROP` 6 张 `engineering_*` 表（无外部依赖，零售/国补/看板均不引用）；`inventory_logs` 中 `log_type='engineering'` 的记录删或保留均可（不影响既有查询）。**回退前必须全库备份**。
3. **单卡回退**：T05 若出错，`git revert` 该卡提交即可，不牵连工程模块主体。

---

## 6. 交付完成定义（整体 DoD）

以下 8 条全满足才算这个模块交付完成：

- [ ] 16 张卡全部 `[x]`，每卡附实测结论
- [ ] 4 个检查点 CP1~CP4 全部通过
- [ ] 8 条巡检 SQL 全部 0 行（连续 3 天各跑一次仍 0 行）
- [ ] 全链路手工走查 11 步通过
- [ ] 5 个既有页面（零售开单 / 国补 / 看板 / 库存流水 / 工资条）数据零差异
- [ ] 员工端无工程入口；员工 token 访问工程接口 403
- [ ] 代码中不存在对 `financial_logs` / `accounts` / `order_items` 的写操作
- [ ] 移动端 375px 与 PC 端功能一致（先 PC 后移动，两端都验）

---

## 附录 A：每卡领取时的标准动作

1. 读本卡 + 卡上标注的文档章节（不超过 3 节，不用读全文档）
2. 在任务标题前标 `[~]`
3. 按"实现要点"逐条做，遇到与文档冲突**先提出来**，不自行改契约
4. 自测跑通本节「自测」命令
5. 逐条勾 DoD，附一句实测结论，标 `[x]`
6. 提交前 `git diff` 自查：只改本卡文件、无遗留调试代码、无 `console.log`/`fmt.Println`

## 附录 B：文案统一来源

所有报错、提示、确认语一律取自 SPEC-ENG-001 §8 文案表。**开发不得自创措辞**——同一个意思出现两种说法，用户会以为是两件事。
