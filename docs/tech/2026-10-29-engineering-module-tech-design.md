# 工程模块 技术方案

| 项 | 内容 |
| --- | --- |
| 文档编号 | TECH-ENG-001 |
| 版本 | v1.1（新增工程费用模型与接口，毛利口径改为净利口径） |
| 日期 | 2026-10-29 |
| 上游文档 | SRS-ENG-001 v1.1（需求）、SPEC-ENG-001 v1.1（产品设计） |
| 配套文档 | `2026-10-29-engineering-module-api-contract.md`（接口契约 v1.1，前后端共同唯一依据） |
| 技术栈 | Go 1.26 + Gin + GORM / MySQL 8.0 / Vue 3 + Vite + Element Plus + Tailwind |
| 下一阶段 | 并行开发任务卡（确认本文档后拆分） |

**变更记录**

| 版本 | 变更 |
| --- | --- |
| v1.0 | 5 表模型、12 接口、毛利口径（只减设备成本） |
| **v1.1** | **新增 `engineering_expenses` 表与 4 个费用接口；`engineering_projects` 新增 `expense_total`；毛利口径拆为「设备毛利 / 项目净利」；新增对账 SQL ⑥；审计路由 +4 条；事务清单 +3 行** |

---

## 1. 代码落位

### 1.1 新增文件

| 文件 | 职责 | 预估规模 |
| --- | --- | --- |
| `erp-backend/models/engineering.go` | **6 个**模型定义（不塞进 `core_models.go`，避免该文件继续膨胀） | ~210 行 |
| `erp-backend/services/engineering_service.go` | 金额重算、状态机、校验、逾期判定、**净利计算**（纯函数 + 事务内校验，不碰 HTTP） | ~480 行 |
| `erp-backend/controllers/engineering_project_controller.go` | 列表 / 汇总 / 创建 / 详情 / 更新 / 删除 / 状态流转 | ~400 行 |
| `erp-backend/controllers/engineering_finance_controller.go` | 清单整表保存 / 节点整表保存 / 回款登记 / 冲销 | ~360 行 |
| `erp-backend/controllers/engineering_shipment_controller.go` | 出库（含库存扣减与流水） | ~200 行 |
| **`erp-backend/controllers/engineering_expense_controller.go`** | **费用台账查询 / 新增 / 编辑 / 删除（含 `expense_total` 同事务重算）** | **~220 行** |
| `erp-backend/controllers/engineering_controller_test.go` | 单测（金额、状态机、并发保护） | ~620 行 |

### 1.2 既有文件改动（清单式，不能漏）

| 文件 | 改动 | 风险 |
| --- | --- | --- |
| `config/db.go` | `AutoMigrate` 列表追加 **6 个**模型 | 低（纯追加） |
| `routers/router.go` | 新增 `eng := auth.Group("/admin/engineering")` 分组，挂 `AdminAuthRequired`；组内注册 `projects/*` 与 `expenses/*` | 低 |
| `middlewares/operation_history.go` | ① `operationRoutes` 追加 **12 条**路由映射；② `pathParamTable` 追加 `engineering` 前缀分支（含 `expenses`） | 中（该文件为全局审计，改错影响所有模块，必须只做追加） |
| `src/views/InventoryLog.vue` | ① 类型筛选下拉加「工程出库」；② 类型映射 map 加 `engineering` | 低 |
| `src/router/index.js` | 新增 **3 条**路由（列表 / 台账 / 详情），**台账必须在 `:id` 之前** | 低（顺序错会导致台账页被详情页吞掉） |
| `src/layout/Index.vue` | `menuItems` 与 `mobileMenuItems` 各加一项「工程」 | 低 |
| **`src/views/Bookkeeping.vue`（或经营开销入口）** | **不新增工程费用入口**；仅在页面顶部加一行说明"工程开销请到「工程 → 工程开销」记录，避免重复" | **低但必须做**，否则老板会两边都录一遍 |

### 1.3 明确不改的文件（评审拦截项）

- `models/core_models.go` 中 `Order` / `OrderItem`：**一个字段都不加**；
- 任何国补相关文件（`subsidy_*`）；
- `controllers/order_controller.go`、`bookkeeping_*`、`dashboard_*`、`payroll_*`、`finance_*`；
- `ai_agent_data.go` 的数据表白名单（一期不向 AI 开放工程表）。

> **v1.1 新增红线（用户明确要求）**：工程模块**全程零写入** `accounts`（账户余额）与 `financial_logs`（资金流水），也**不得**把工程费用记入 `account_id=7`（经营开销）。工程开销独立成表 `engineering_expenses`，与日常经营开销彻底分离。这一条列为代码评审的一级拦截项——见到任何 `FinancialLog{...}` 出现在工程代码里，直接打回。

### 1.4 分层原则

```
router（权限 + 路径）
  └─ controller（HTTP 解析、参数校验、响应包络、事务边界）
       └─ service（纯业务规则：金额重算、状态机矩阵、一致性校验）
            └─ GORM（无原生 SQL 拼接，聚合查询除外）
```

- 事务边界一律在 controller（与现有 `order_controller.go` 风格一致：`tx := config.DB.Begin()` … `tx.Commit()` / `tx.Rollback()`）；
- service 层函数签名接收 `tx *gorm.DB`，便于在事务内调用，也便于单测直接跑纯函数。

---

## 2. 数据模型

### 2.1 关系

```
engineering_projects (1)
├── (N) engineering_project_items     商品清单行
├── (N) engineering_payment_terms     付款节点
│        └── (N) engineering_receipts  回款记录（term_id 非空即挂节点）
├── (N) engineering_shipments         出库记录（→ inventory_logs 同事务写入）
└── (N) engineering_expenses          工程费用（project_id 可空 = 公共费用）

engineering_receipts.reversed_receipt_id → engineering_receipts.id（冲销自关联）
所有子表 → products(product_id) / employees(owner_employee_id, operator_id) 只读引用

注：engineering_expenses.project_id 是唯一允许为 NULL 的外键关联，
    NULL 表示"公共费用"——不属于任何项目，只进台账与「本年工程开销」。
    因此该表不能用 GORM 的级联/必填约束，权重全在应用层。
```

### 2.2 GORM 模型（最终以本定义为准，AutoMigrate 落库）

```go
// EngineeringProject 工程项目主表
type EngineeringProject struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	ProjectNo   string `gorm:"type:varchar(50);uniqueIndex;not null;comment:项目编号 GC-YYYYMMDD-NNN" json:"project_no"`
	Name        string `gorm:"type:varchar(100);not null;comment:项目名称" json:"name"`
	ProjectType int    `gorm:"type:tinyint;not null;default:1;comment:1住宅精装 2商业综合体 3酒店公寓 4政企学校 5其他" json:"project_type"`

	ClientName    string `gorm:"type:varchar(150);not null;default:'';comment:甲方单位" json:"client_name"`
	ClientContact string `gorm:"type:varchar(50);not null;default:'';comment:甲方联系人" json:"client_contact"`
	ClientPhone   string `gorm:"type:varchar(20);not null;default:'';comment:联系电话" json:"client_phone"`
	Address       string `gorm:"type:varchar(255);not null;default:'';comment:项目地址" json:"address"`

	OwnerEmployeeID *uint     `gorm:"index;comment:销售负责人" json:"owner_employee_id"`
	OwnerEmployee   *Employee `gorm:"foreignKey:OwnerEmployeeID" json:"owner_employee,omitempty"`
	ContractNo      string    `gorm:"type:varchar(50);not null;default:'';comment:甲方合同号" json:"contract_no"`
	SignDate        *time.Time `gorm:"type:date;index;comment:签约日期" json:"sign_date"`
	AcceptDate      *time.Time `gorm:"type:date;comment:验收日期" json:"accept_date"`
	SettledAt       *time.Time `gorm:"comment:结清时间" json:"settled_at"`

	EstimatedAmount float64 `gorm:"type:decimal(12,2);not null;default:0;comment:预计合同额(跟进阶段)" json:"estimated_amount"`
	ItemsTotal      float64 `gorm:"type:decimal(12,2);not null;default:0;comment:设备总额(清单汇总)" json:"items_total"`
	InstallFee      float64 `gorm:"type:decimal(12,2);not null;default:0;comment:安装费" json:"install_fee"`
	FreightFee      float64 `gorm:"type:decimal(12,2);not null;default:0;comment:运输费" json:"freight_fee"`
	OtherFee        float64 `gorm:"type:decimal(12,2);not null;default:0;comment:其他费用" json:"other_fee"`
	DiscountAmount  float64 `gorm:"type:decimal(12,2);not null;default:0;comment:折让(正数=让利)" json:"discount_amount"`
	ContractTotal   float64 `gorm:"type:decimal(12,2);not null;default:0;comment:合同总额=设备+安装+运输+其他-折让" json:"contract_total"`
	CostTotal       float64 `gorm:"type:decimal(12,2);not null;default:0;comment:设备成本合计(快照)" json:"cost_total"`
	ExpenseTotal    float64 `gorm:"type:decimal(12,2);not null;default:0;comment:本项目工程费用合计(冗余,不含公共费用)" json:"expense_total"`
	ReceivedAmount  float64 `gorm:"type:decimal(12,2);not null;default:0;comment:已实收合计(冗余,与回款记录一致)" json:"received_amount"`

	OrderedQty int `gorm:"type:int;not null;default:0;comment:订购台数(清单汇总)" json:"ordered_qty"`
	ShippedQty int `gorm:"type:int;not null;default:0;comment:已出库台数" json:"shipped_qty"`

	Status     int        `gorm:"type:tinyint;not null;default:1;index;comment:1跟进中 2已签约 3供货中 4已验收 5已结清 9已作废" json:"status"`
	VoidReason string     `gorm:"type:varchar(200);not null;default:'';comment:作废原因" json:"void_reason"`
	VoidedAt   *time.Time `gorm:"comment:作废时间" json:"voided_at"`
	Remark     string     `gorm:"type:varchar(500);not null;default:'';comment:备注" json:"remark"`

	ClientRequestID *string `gorm:"type:varchar(64);uniqueIndex;comment:幂等键" json:"client_request_id,omitempty"`

	Items     []EngineeringProjectItem `gorm:"foreignKey:ProjectID" json:"items,omitempty"`
	Terms     []EngineeringPaymentTerm `gorm:"foreignKey:ProjectID" json:"terms,omitempty"`
	Receipts  []EngineeringReceipt     `gorm:"foreignKey:ProjectID" json:"receipts,omitempty"`
	Shipments []EngineeringShipment    `gorm:"foreignKey:ProjectID" json:"shipments,omitempty"`
	Expenses  []EngineeringExpense     `gorm:"foreignKey:ProjectID" json:"expenses,omitempty"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// EngineeringProjectItem 设备清单行（行金额不落库，实时计算）
type EngineeringProjectItem struct {
	ID        uint    `gorm:"primaryKey" json:"id"`
	ProjectID uint    `gorm:"index;not null" json:"project_id"`
	ProductID uint    `gorm:"index;not null" json:"product_id"`
	Product   *Product `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	Quantity  int     `gorm:"type:int;not null;comment:订购数量" json:"quantity"`
	UnitPrice float64 `gorm:"type:decimal(12,2);not null;comment:工程单价" json:"unit_price"`
	UnitCost  float64 `gorm:"type:decimal(12,2);not null;default:0;comment:成本单价快照" json:"unit_cost"`
	ShippedQty int    `gorm:"type:int;not null;default:0;comment:已出库数量" json:"shipped_qty"`
	Remark    string  `gorm:"type:varchar(200);not null;default:''" json:"remark"`
	SortOrder int     `gorm:"type:int;not null;default:0" json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// EngineeringPaymentTerm 付款节点
type EngineeringPaymentTerm struct {
	ID             uint       `gorm:"primaryKey" json:"id"`
	ProjectID      uint       `gorm:"index;not null" json:"project_id"`
	Name           string     `gorm:"type:varchar(50);not null;comment:节点名称" json:"name"`
	Ratio          *float64   `gorm:"type:decimal(5,2);comment:计划比例%" json:"ratio"`
	PlannedAmount  float64    `gorm:"type:decimal(12,2);not null;comment:计划金额" json:"planned_amount"`
	PlannedDate    *time.Time `gorm:"type:date;index;comment:计划回款日" json:"planned_date"`
	ReceivedAmount float64    `gorm:"type:decimal(12,2);not null;default:0;comment:该节点累计实收(冗余)" json:"received_amount"`
	SortOrder      int        `gorm:"type:int;not null;default:0" json:"sort_order"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

// EngineeringReceipt 回款记录（只增不改，冲销生成负数记录）
type EngineeringReceipt struct {
	ID              uint       `gorm:"primaryKey" json:"id"`
	ProjectID       uint       `gorm:"index;not null" json:"project_id"`
	TermID          uint       `gorm:"index;not null" json:"term_id"`
	Term            *EngineeringPaymentTerm `gorm:"foreignKey:TermID" json:"term,omitempty"`
	Amount          float64    `gorm:"type:decimal(12,2);not null;comment:金额(冲销为负)" json:"amount"`
	PaymentDate     time.Time  `gorm:"type:date;index;not null;comment:到账日期" json:"payment_date"`
	Method          int        `gorm:"type:tinyint;not null;default:1;comment:1银行转账 2承兑汇票 3现金 4支票 5其他" json:"method"`
	BillNo          string     `gorm:"type:varchar(50);not null;default:'';comment:票据号/流水号" json:"bill_no"`
	Remark          string     `gorm:"type:varchar(200);not null;default:''" json:"remark"`
	ReversedReceiptID *uint    `gorm:"index;comment:被冲销的原记录ID" json:"reversed_receipt_id"`
	OperatorID      *uint      `gorm:"index" json:"operator_id"`
	OperatorName    string     `gorm:"type:varchar(100);not null;default:''" json:"operator_name"`
	ClientRequestID *string    `gorm:"type:varchar(64);uniqueIndex;comment:幂等键" json:"client_request_id,omitempty"`
	CreatedAt       time.Time  `json:"created_at"`
}

// EngineeringShipment 出库记录
type EngineeringShipment struct {
	ID            uint      `gorm:"primaryKey" json:"id"`
	ProjectID     uint      `gorm:"index;not null" json:"project_id"`
	ItemID        uint      `gorm:"index;not null" json:"item_id"`
	ProductID     uint      `gorm:"index;not null" json:"product_id"`
	Product       *Product  `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	Quantity      int       `gorm:"type:int;not null;comment:本次出库数量" json:"quantity"`
	WarehouseType int       `gorm:"type:tinyint;not null;comment:2门店 3云仓" json:"warehouse_type"`
	ShipDate      time.Time `gorm:"type:date;not null;index" json:"ship_date"`
	TrackingNo    string    `gorm:"type:varchar(100);not null;default:''" json:"tracking_no"`
	Remark        string    `gorm:"type:varchar(200);not null;default:''" json:"remark"`
	AllowNegative bool      `gorm:"not null;default:false;comment:是否确认过负库存出库" json:"allow_negative"`
	OperatorID    *uint     `gorm:"index" json:"operator_id"`
	OperatorName  string    `gorm:"type:varchar(100);not null;default:''" json:"operator_name"`
	CreatedAt     time.Time `json:"created_at"`
}

// EngineeringExpense 工程费用（v1.1 新增）
//
// 隔离声明（代码评审一级拦截项）：
//   - 本表与 accounts / financial_logs / account_id=7（经营开销）**零关联**；
//   - pay_method 仅为备查信息，录入时不需要、也不允许扣减任何账户余额；
//   - 用户明确要求"工程开销与日常经营开销分开"，两边重复记账会导致成本双算。
type EngineeringExpense struct {
	ID        uint                `gorm:"primaryKey" json:"id"`
	ProjectID *uint               `gorm:"index;comment:所属项目(NULL=不分摊的工程公共费用)" json:"project_id"`
	Project   *EngineeringProject `gorm:"foreignKey:ProjectID" json:"project,omitempty"`

	Category    int       `gorm:"type:tinyint;not null;index;comment:1安装人工 2运输物流 3辅材耗材 4现场施工 5差旅招待 6外包服务 7其他" json:"category"`
	Amount      float64   `gorm:"type:decimal(12,2);not null;comment:费用金额(恒为正数)" json:"amount"`
	ExpenseDate time.Time `gorm:"type:date;not null;index;comment:发生日期" json:"expense_date"`

	PayStatus int        `gorm:"type:tinyint;not null;default:1;index;comment:1已付 0未付(挂账)" json:"pay_status"`
	PayMethod int        `gorm:"type:tinyint;not null;default:0;comment:0未指定 1微信 2支付宝 3公户 4现金 5其他(仅备查,不动余额)" json:"pay_method"`
	PaidDate  *time.Time `gorm:"type:date;comment:付款日期" json:"paid_date"`
	Payee     string     `gorm:"type:varchar(100);not null;default:'';comment:收款方/供应商" json:"payee"`
	BillNo    string     `gorm:"type:varchar(50);not null;default:'';comment:票据/凭证号" json:"bill_no"`
	Remark    string     `gorm:"type:varchar(200);not null;default:''" json:"remark"`

	OperatorID      *uint   `gorm:"index" json:"operator_id"`
	OperatorName    string  `gorm:"type:varchar(100);not null;default:''" json:"operator_name"`
	ClientRequestID *string `gorm:"type:varchar(64);uniqueIndex;comment:幂等键" json:"client_request_id,omitempty"`

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"` // 软删除：费用是净利依据，删除必须可追溯
}
```

> **关于 `ProjectID` 可空**：GORM 自动迁移可能生成外键约束，但 SQL 外键对 `NULL` 值不做校验，因此公共费用不受影响。索引单独建在 `project_id` 上（`idx_engineering_expenses_project_id`），台账按项目筛选用得到；另建 `idx_engineering_expenses_date`、`idx_engineering_expenses_category`、`idx_engineering_expenses_pay_status` 支撑台账的三个常用筛选。

### 2.2.1 为什么 `expense_total` 要冗余到主表

| 方案 | 优点 | 缺点 | 结论 |
| --- | --- | --- | --- |
| 不冗余，每次 `SUM(expenses)` | 无一致性风险 | 列表页 20 行 = 20 次子查询（或一次 JOIN 聚合），且净利率排序要走临时表；汇总卡要再扫一遍 | 排除 |
| **冗余 `expense_total` + 增删改同事务重算** | 列表/排序/汇总都是单表聚合；与 `contract_total`、`cost_total`、`received_amount` 的现有做法一致 | 需要一个对账 SQL 兜底 | **采用** |

重算逻辑与 `contract_total` 完全同款：费用任何写操作，事务内 `SELECT SUM(amount) FROM engineering_expenses WHERE project_id = ? AND deleted_at IS NULL` 后写回，再由对账 SQL ⑥ 每日兜底。

### 2.3 关键设计取舍

| 取舍 | 决定 | 理由 |
| --- | --- | --- |
| 行金额是否落库 | **不落**，实时算 `quantity × unit_price` | 避免"单价改了行金额没改"的双源；行数少（<100），计算无压力 |
| 项目级金额是否冗余 | **落** `items_total / contract_total / cost_total` | 列表与汇总卡要按合同额排序、求和，必须能走索引与聚合，不能每行展开算 |
| 节点/项目已收金额 | **落** `received_amount` | 用于原子条件更新防并发超收（见 6.2），且避免列表 N+1 |
| 回款记录 | **只增不改**，冲销写负数 | 与现有"退款红冲"思路一致，账目可追溯 |
| 编号唯一 | `project_no` 单列唯一（不带 `deleted_at`） | MySQL 唯一索引对 NULL 不去重，`(project_no, deleted_at)` 会失效；编号删除后不释放，符合单据编号习惯 |
| 子表软删除 | **不用** `gorm.DeletedAt`，整表替换 | 清单/节点是"整表提交"，物理删除旧行 + 插入新行最简单；历史靠 `operation_histories` 快照追溯 |
| 金额类型 | Go `float64` + DB `decimal(12,2)` | 与全库一致；所有写入前 `roundMoney()` 归一到 2 位小数（见 4.1） |
| **费用是否与回款同表** | **不同表**：回款 `engineering_receipts`（钱进）、费用 `engineering_expenses`（钱出） | 同一表用正负号区分会让所有聚合 SQL 都要带符号判断，且"节点已收"的原子更新逻辑无法复用；分开后各自演进互不影响 |
| **费用是否支持负数/冲销** | **不支持**。只能编辑或删除（软删） | 费用是内部记账，不是对客户的账；回款的负数冲销是为了对冲已发生的银行流水，费用没有这个概念。删错重录 + 审计快照足够 |
| **费用是否分摊到项目** | **公共费用不摊**（`project_id` 为 NULL） | 摊派规则本身就是争议点；摊完每个项目净利都不再是"真实花的钱" |

### 2.4 迁移策略

1. `config/db.go` 的 `AutoMigrate` 追加 **6 个**模型 —— 只做**新增表**，不动任何既有表结构（`engineering_projects.expense_total` 是给本次新建的表加列，不属于"改既有表"）；
2. `inventory_logs.log_type` 新增取值 `engineering` 属于**数据值扩展**，无需 DDL；
3. 上线顺序：后端先发（建表 + 接口）→ 前端再发（页面 + 菜单）。此顺序下前端未发布时接口已可用但无入口，对现有功能零影响；
4. 回滚：删前端菜单即"功能下线"，后端表保留不影响任何既有查询；如需彻底回滚，**6 张**新表可直接 drop（无外部依赖）。

---

## 3. 状态机

### 3.1 转移矩阵（唯一事实来源，硬编码在 `engineering_service.go`）

| 当前 \ 目标 | 1 跟进中 | 2 已签约 | 3 供货中 | 4 已验收 | 5 已结清 | 9 已作废 |
| --- | --- | --- | --- | --- | --- | --- |
| 1 跟进中 | — | ✅ sign | ✗ | ✗ | ✗ | ✅ void |
| 2 已签约 | ✅ revert | — | ✅ ship(自动) | ✗ | ✗ | ✅ void |
| 3 供货中 | ✗ | ✅ revert | — | ✅ accept | ✗ | ✅ void |
| 4 已验收 | ✗ | ✗ | ✗ | — | ✅ settle(自动) | ✅ void |
| 5 已结清 | ✗ | ✗ | ✗ | ✗ | — | ✗ |
| 9 已作废 | ✗ | ✗ | ✗ | ✗ | ✗ | — |

**进入条件（全部满足才允许）**

| 目标状态 | 条件 |
| --- | --- |
| 2 已签约 | 清单非空；`contract_total > 0`；节点数 ≥1；`Σ节点计划金额 == contract_total`（容差 0.01）；`sign_date` 非空 |
| 3 供货中 | 至少 1 条出库记录（由出库接口自动触发，不单独开放） |
| 4 已验收 | `Σ item.shipped_qty == Σ item.quantity`；`accept_date` 非空 |
| 5 已结清 | `received_amount >= contract_total`（自动触发） |
| 9 已作废 | `void_reason` 长度 ≥ 5；状态 ≠ 5 |

**回退额外约束**：`3 → 2` 仅当该项目的出库记录数为 0（有出库就不能退，防止状态与库存事实背离）。

### 3.2 实现

```go
// services/engineering_service.go
type StatusAction string

const (
	ActionSign   StatusAction = "sign"
	ActionAccept StatusAction = "accept"
	ActionRevert StatusAction = "revert"
	ActionVoid   StatusAction = "void"
)

// allowedTransition 返回 false 时，controller 回 400 + 明确原因
func checkTransition(project *EngineeringProject, action StatusAction) error

// applyAutoStatus 在事务内调用，处理"出库→供货中""收齐→已结清"两条自动规则
func applyAutoStatusOnShipment(tx *gorm.DB, projectID uint) error
func applyAutoStatusOnReceipt(tx *gorm.DB, projectID uint) error
```

自动推进一律用**条件更新**，保证并发下只生效一次且幂等：

```sql
-- 出库后：2 → 3
UPDATE engineering_projects SET status = 3, updated_at = NOW()
 WHERE id = ? AND status = 2 AND deleted_at IS NULL;

-- 收款后：2/3/4 → 5（结清时间只在首次生效）
UPDATE engineering_projects SET status = 5, settled_at = NOW(), updated_at = NOW()
 WHERE id = ? AND status IN (2,3,4) AND received_amount >= contract_total AND deleted_at IS NULL;
```

`RowsAffected == 0` 时视为"状态已推进/不满足"，**不报错**（幂等）。

---

## 4. 金额算法

### 4.1 精度处理（统一入口，禁止散落）

```go
// services/engineering_service.go
func roundMoney(v float64) float64 { return math.Round(v*100) / 100 }

const moneyEpsilon = 0.005

func moneyEqual(a, b float64) bool { return math.Abs(a-b) < moneyEpsilon }
```

- 所有落库金额、所有返回前端的计算金额，一律 `roundMoney()`；
- 所有金额**比较**（节点合计 = 合同额、实收 = 应收）一律 `moneyEqual`，不用 `==`。

### 4.2 合同总额重算（后端唯一权威）

```
items_total    = Σ round(quantity × unit_price)
cost_total     = Σ round(quantity × unit_cost)
contract_total = round(items_total + install_fee + freight_fee + other_fee − discount_amount)
expense_total  = Σ round(amount)   [engineering_expenses WHERE project_id = ? AND deleted_at IS NULL]
ordered_qty    = Σ quantity
```

**触发时机**：保存清单、修改费用项、创建项目（费用项默认为 0）、**费用增删改**。重算后写回 `engineering_projects`，前端不做权威计算（前端只做展示预览）。

> 注意 `expense_total` 只统计 `project_id = 当前项目` 的费用，**公共费用（project_id IS NULL）不参与**——这是"项目净利不含公共费用"这一口径在代码里的落点，写错这一行，所有项目的净利都会偏。建议在 service 层单独封装 `recalcExpenseTotal(tx, projectID)`，禁止在 controller 里手写 SUM。

### 4.3 合同额 = 节点合计 的三道强制点

| # | 位置 | 行为 |
| --- | --- | --- |
| 1 | `PUT /items`（改清单导致合同额变化） | 事务内重算合同额后，若 `status >= 2` 且新合同额 ≠ 节点合计 → **整体回滚**，返回 409 + 差额 + `需要同时调整付款节点` |
| 2 | `PUT /payment-terms` | 保存后若 `status >= 2` 且节点合计 ≠ 合同额 → 整体回滚，409 + 差额 |
| 3 | `PUT /projects/:id`（改安装费/运输费/折让） | 同 1：`status >= 2` 时若导致合同额变化而不平 → 回滚 409 |
| 4 | `sign` 动作 | 最终把关：未不平才允许签约 |

> 这条规则的效果：**已签约项目在数据库中任何时刻都满足 `contract_total == Σ terms.planned_amount`**，不会出现"合同改了、节点没改"的脏状态。前端需要在改清单/费用时提示用户"合同额发生变化，请同步调整付款节点"。

### 4.4 逾期判定（派生，不落库）

```sql
-- 单个项目的逾期金额与最大逾期天数
SELECT COALESCE(SUM(planned_amount - received_amount), 0) AS overdue_amount,
       COALESCE(MAX(DATEDIFF(CURDATE(), planned_date)), 0) AS overdue_days
  FROM engineering_payment_terms
 WHERE project_id = ?
   AND planned_amount > received_amount
   AND planned_date IS NOT NULL
   AND planned_date < CURDATE();
```

列表页用一次 `LEFT JOIN` 聚合子查询批量算，禁止逐行查询（N+1）。

### 4.5 盈利口径（v1.1 改为两层，必须在 UI 与文档同时声明）

```
设备毛利  gross_profit = round(contract_total − cost_total)
项目净利  net_profit   = round(contract_total − cost_total − expense_total)
设备毛利率 gross_margin = gross_profit / contract_total × 100
净利率     net_margin   = net_profit   / contract_total × 100
```

| 口径 | 用途 | 说明 |
| --- | --- | --- |
| **设备毛利** | 看"卖货赚多少"，与零售毛利同算法 | 只减进货成本，不减任何支出 |
| **项目净利** | **老板看的那个数**：这个工程到底赚了多少 | 再减本项目实际发生的工程费用 |

**不含**（必须写进 UI 说明）：
1. 安装费、运输费在合同额里是**收入项**（向甲方收的钱），不是成本项；
2. **公共费用**（不分摊）不摊入任何项目净利，只在「工程开销台账」与「本年工程开销」体现；
3. 公司管理费用（房租、水电、行政）不摊——这是公司层面成本，与零售一致。

> 这套口径与现有零售毛利逻辑保持一致（零售也只算进货成本，不摊管理费）。差异在于工程多了**实际支出**这一层，因为工程的钱花得太集中，不扣掉根本看不出真实盈亏。

### 4.6 费用汇总口径（台账页与汇总卡，禁止各处自算）

筛选条件是**动态**的（项目/类别/日期/付款状态/关键词任意组合），所以用 GORM 条件构造，不写死占位符 SQL。条件用**闭包**封装，供两条聚合查询各调一次（GORM 的链式 `Where` 在 `Scan` 之后复用会带上次的 Statement，直接复用同一个 `q` 是本项目最容易踩的坑）：

```go
// services/engineering_service.go
applyFilters := func(db *gorm.DB, f ExpenseFilter) *gorm.DB {
	db = db.Where("deleted_at IS NULL")
	switch {
	case f.OnlyPublic:  db = db.Where("project_id IS NULL")
	case f.ProjectID > 0: db = db.Where("project_id = ?", f.ProjectID)
	}
	if len(f.Categories) > 0     { db = db.Where("category IN ?", f.Categories) }
	if !f.DateStart.IsZero()     { db = db.Where("expense_date >= ?", f.DateStart) }
	if !f.DateEnd.IsZero()       { db = db.Where("expense_date <= ?", f.DateEnd) }
	if f.PayStatus != nil        { db = db.Where("pay_status = ?", *f.PayStatus) }
	if f.Keyword != "" {
		like := "%" + f.Keyword + "%"
		db = db.Where("payee LIKE ? OR remark LIKE ?", like, like)
	}
	return db
}

// ① 汇总（4 个数字 + 笔数），一次往返
type expenseSummaryRow struct {
	Total, Paid, Unpaid, YearTotal float64
	Cnt                            int64
}
var s expenseSummaryRow
if err := applyFilters(tx.Model(&EngineeringExpense{}), f).
	Select(`COALESCE(SUM(amount),0) AS total,
	        COALESCE(SUM(CASE WHEN pay_status = 1 THEN amount ELSE 0 END),0) AS paid,
	        COALESCE(SUM(CASE WHEN pay_status = 0 THEN amount ELSE 0 END),0) AS unpaid,
	        COALESCE(SUM(CASE WHEN YEAR(expense_date) = YEAR(CURDATE()) THEN amount ELSE 0 END),0) AS year_total,
	        COUNT(*) AS cnt`).
	Scan(&s).Error; err != nil {
	return err
}

// ② 类别分布（重新构造查询，不复用上面那条）
type categoryRow struct {
	Category int
	Amount   float64
}
var rows []categoryRow
applyFilters(tx.Model(&EngineeringExpense{}), f).
	Select("category, SUM(amount) AS amount").
	Group("category").Scan(&rows)
```

- 汇总卡的「本年工程开销」**不带任何筛选**（固定口径），用 `ExpenseFilter{}` 空筛选单独查一次；
- `by_category` 补齐 7 项（SQL 只返回有金额的类别，缺的由 service 补 0），保证前端拿到的恒为 7 项；
- 所有聚合都在 SQL 里完成，**禁止**把全部费用拉到内存里 `reduce` —— 费用条数会随年份线性增长，拉全量迟早卡。

---

## 5. 事务与并发

### 5.1 写操作事务清单

| 操作 | 事务内涉及 | 隔离要求 |
| --- | --- | --- |
| 创建项目 | projects(insert) | 无需行锁；唯一索引兜底编号冲突 |
| 保存清单 | project_items(delete+insert) + projects(update 金额) | 锁项目行 `FOR UPDATE` |
| 保存节点 | payment_terms(delete+insert) + projects(校验合同额) | 锁项目行 `FOR UPDATE` |
| 登记回款 | receipts(insert) + terms(update 已收) + projects(update 已收) + 自动结清 | 条件更新（见 5.2-③） |
| 冲销回款 | receipts(insert 负数) + terms(update) + projects(update) + 状态回退判定 | 条件更新 |
| 出库 | shipments(insert) + products(update 库存) + inventory_logs(insert) + project_items(update 已发) + projects(update 已发/状态) | 商品行 `FOR UPDATE` |
| **新增费用（项目内）** | **expenses(insert) + projects(update expense_total)** | **锁项目行 `FOR UPDATE`** |
| **编辑费用** | **expenses(update) + projects(update expense_total)** | **锁项目行（若所属项目变更，需锁新旧两个项目，按 ID 升序锁避免死锁）** |
| **删除费用** | **expenses(soft delete) + projects(update expense_total)** | **锁项目行** |
| **新增/编辑/删除公共费用**（`project_id = NULL`） | **expenses 单表操作，不碰 projects** | 无需行锁（无共享计数字段） |
| 状态流转 | projects(update) | 条件更新 `WHERE status = 期望前态` |
| 作废 | projects(update) + operation_histories | 条件更新 |
| 删除 | projects(soft delete) | 条件更新 |

### 5.2 并发场景逐条处置

| # | 场景 | 风险 | 手段 |
| --- | --- | --- | --- |
| ① | 两人同时保存同一项目的清单 | 后写覆盖前者，金额与节点失配 | 事务开头 `SELECT * FROM engineering_projects WHERE id = ? FOR UPDATE`，串行化同项目写操作 |
| ② | 同时保存清单与节点 | 一个改合同额、一个改节点，互相看不见 | 同上：两者都先锁项目行，天然串行 |
| ③ | 两人同时登记同一节点回款 | 双倍入账、节点超收 | **条件更新**（原子）：<br>`UPDATE engineering_payment_terms SET received_amount = received_amount + ? WHERE id = ? AND received_amount + ? <= planned_amount`<br>再 `UPDATE engineering_projects SET received_amount = received_amount + ? WHERE id = ? AND received_amount + ? <= contract_total`<br>任一 `RowsAffected == 0` → 回滚 + 409 |
| ④ | 同一笔回款重复提交（双击/重试） | 重复入账 | `client_request_id` 唯一索引；命中重复键 → 返回 409 `ErrDuplicated`，前端提示"该笔回款已登记" |
| ⑤ | 同时出库同一商品 | 库存超发、`before_qty` 错乱 | `SELECT ... FOR UPDATE` 锁商品行 → 算 before/after → `UPDATE products SET store_stock = store_stock - ?` → 写流水 |
| ⑥ | 出库与零售开单同时扣同一商品 | 库存为负且未确认 | 零售走"条件更新 `WHERE store_stock >= ?`"，会失败并提示库存不足；工程若走负库存分支则显式允许（`allow_negative=true` 且有审计） |
| ⑦ | 两人同时登记最后一笔回款 | 重复置结清、`settled_at` 被覆盖 | 条件更新 `WHERE status IN (2,3,4)`，第二次 `RowsAffected=0` 静默返回 |
| ⑧ | 出库与作废同时发生 | 已作废项目仍出库 | 出库事务内先 `SELECT status FOR UPDATE`，`status=9` 直接 400 |
| ⑨ | 编号并发生成 | 重号 | 唯一索引 + 生成后重试（最多 3 次，每次序号 +1）；3 次失败返回 500 |
| ⑩ | **两人同时改同一项目的费用，或费用与清单同时保存** | **`expense_total` 互相覆盖（A 算完写回，B 用旧快照覆盖），净利错** | **两条路径都先 `SELECT ... FOR UPDATE` 锁项目行；`expense_total` 必须在事务内重新 `SUM` 后写回，禁止用"旧值 + 本次金额"增量累加** |
| ⑪ | **费用所属项目被并发变更（A 把它从项目 1 改到项目 2，B 正在改项目 1 的清单）** | **项目 1 的 `expense_total` 少算、项目 2 多算** | **按 `id` 升序对涉及的两个项目行加锁（固定顺序避免死锁），两个项目的 `expense_total` 都重算** |

### 5.3 幂等实现

| 接口 | 幂等键 | 冲突响应 |
| --- | --- | --- |
| `POST /projects` | body `client_request_id` | 409 `ErrDuplicated`，`data` 带上已存在项目 ID |
| `POST /projects/:id/receipts` | body `client_request_id` | 409 `ErrDuplicated` |
| `POST /projects/:id/shipments` | 不引入（出库有数量上限校验，重复提交会被"超订购量"拦截） | 400 |
| **`POST /expenses`** | **body `client_request_id`** | **409 `ErrDuplicated`；费用没有天然的"上限"校验，双击必重复，幂等键是唯一防线** |

### 5.4 对账巡检 SQL（上线后每天跑，写入运维手册）

```sql
-- ① 项目已收 = 回款记录合计（应为 0 行）
SELECT p.id, p.received_amount, COALESCE(SUM(r.amount),0) AS ledger
  FROM engineering_projects p LEFT JOIN engineering_receipts r ON r.project_id = p.id
 WHERE p.deleted_at IS NULL GROUP BY p.id
HAVING ABS(p.received_amount - ledger) > 0.01;

-- ② 节点已收 = 该节点回款合计（应为 0 行）
SELECT t.id, t.received_amount, COALESCE(SUM(r.amount),0) AS ledger
  FROM engineering_payment_terms t LEFT JOIN engineering_receipts r ON r.term_id = t.id
 GROUP BY t.id HAVING ABS(t.received_amount - ledger) > 0.01;

-- ③ 已签约及以上：合同额 = 节点合计（应为 0 行）
SELECT p.id, p.contract_total, COALESCE(SUM(t.planned_amount),0) AS terms
  FROM engineering_projects p LEFT JOIN engineering_payment_terms t ON t.project_id = p.id
 WHERE p.deleted_at IS NULL AND p.status IN (2,3,4,5) GROUP BY p.id
HAVING ABS(p.contract_total - terms) > 0.01;

-- ④ 清单已发 = 出库记录合计（应为 0 行）
SELECT i.id, i.shipped_qty, COALESCE(SUM(s.quantity),0) AS shipped
  FROM engineering_project_items i LEFT JOIN engineering_shipments s ON s.item_id = i.id
 GROUP BY i.id HAVING i.shipped_qty <> shipped;

-- ⑤ 项目已发 = 清单已发合计（应为 0 行）
SELECT p.id, p.shipped_qty, COALESCE(SUM(i.shipped_qty),0)
  FROM engineering_projects p LEFT JOIN engineering_project_items i ON i.project_id = p.id
 WHERE p.deleted_at IS NULL GROUP BY p.id HAVING p.shipped_qty <> COALESCE(SUM(i.shipped_qty),0);

-- ⑥ 项目费用合计(expense_total) = 本项目费用记录合计（应为 0 行）★ v1.1 新增，直接影响净利
SELECT p.id, p.project_no, p.expense_total, COALESCE(SUM(e.amount),0) AS ledger
  FROM engineering_projects p
  LEFT JOIN engineering_expenses e ON e.project_id = p.id AND e.deleted_at IS NULL
 WHERE p.deleted_at IS NULL
 GROUP BY p.id HAVING ABS(p.expense_total - ledger) > 0.01;

-- ⑦ 费用金额必须为正（应为 0 行）★ v1.1 新增，防止负数把净利抬高
SELECT id, project_id, amount, category FROM engineering_expenses
 WHERE deleted_at IS NULL AND amount <= 0;

-- ⑧ 公共费用不得挂到已作废项目（应为 0 行）★ v1.1 新增，作废后新增应被拦截
SELECT p.id, p.project_no, p.status, COUNT(e.id) AS cnt
  FROM engineering_projects p JOIN engineering_expenses e ON e.project_id = p.id AND e.deleted_at IS NULL
 WHERE p.status = 9 GROUP BY p.id;
```

**巡检执行**：⑥⑦⑧ 与 ①②③④⑤ 一并写入 `scripts/check-engineering-consistency.sql`，部署后手工跑一次。⑥ 若有结果，说明 `expense_total` 重算漏了某条路径（编辑/删除/项目变更），**此时项目净利已不可信**，需重新全量重算：

```sql
-- 全量修复（谨慎执行，执行前先备份）
UPDATE engineering_projects p
  SET p.expense_total = (SELECT COALESCE(SUM(amount),0) FROM engineering_expenses e
                          WHERE e.project_id = p.id AND e.deleted_at IS NULL);
```

---

## 6. 库存集成

### 6.1 出库扣减实现（复用现有风格）

```go
// 事务内，逐行处理
var product models.Product
if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, item.ProductID).Error; err != nil { ... }

beforeQty := product.StoreStock
if warehouseType == 2 { beforeQty = product.StoreStock } else { beforeQty = product.CloudStock }

if beforeQty < qty && !req.AllowNegative {
    return 409, fmt.Sprintf("商品 %s 库存不足（当前 %d 台），如需继续请确认负库存出库", product.Name, beforeQty)
}

field := "store_stock"; if warehouseType == 3 { field = "cloud_stock" }
if err := tx.Model(&models.Product{}).Where("id = ?", product.ID).
    UpdateColumn(field, gorm.Expr(field+" - ?", qty)).Error; err != nil { ... }

if err := WriteInventoryLog(tx, product.ID, warehouseType, "engineering",
    beforeQty, -qty, item.UnitCost, project.ProjectNo,
    fmt.Sprintf("工程出库：%s", project.Name), operator); err != nil { ... }
```

- `WriteInventoryLog` **直接复用**（`controllers/inventory_log_controller.go:49`），不新增写入函数；
- `related_no` 存 `project_no`（`GC-...`），与现有 `order_no` 并存，互不冲突；
- `unit_cost` 传清单行的成本快照（不用商品当前 `avg_cost`），保证流水成本与项目毛利同源。

### 6.2 `engineering` 类型的 4 处联动（漏一处就显示异常）

| # | 位置 | 改动 |
| --- | --- | --- |
| 1 | `src/views/InventoryLog.vue` 筛选下拉 | 新增 `<el-option label="工程出库" value="engineering" />` |
| 2 | `src/views/InventoryLog.vue` 类型映射 map（约 508 行） | `'engineering': { label: '工程出库', type: 'primary' }` |
| 3 | `controllers/inventory_log_controller.go` `GetInventoryLogs` / `GetGlobalInventoryLogs` | 无白名单校验，**无需改**（确认过：直接 `Where("log_type = ?")` 透传） |
| 4 | `controllers/historical_cost_repair_controller.go` 的成本修复范围 | **不改**：`log_type IN ('purchase','stocktake')` 是"零成本入库修复"专用，工程出库是负向流水，不应纳入 |

### 6.3 负库存策略

- 默认拦截；前端必须勾选「我确认允许负库存出库」→ 传 `allow_negative: true`；
- 落 `engineering_shipments.allow_negative = 1`，并在 `inventory_logs.remark` 追加「（负库存出库，已确认）」；
- 不做自动补货、不联动采购。

---

## 7. 审计接入

### 7.1 `operationRoutes` 追加（12 条）

```go
"POST /api/admin/engineering/projects":                          {title: "新建工程项目", tables: []string{"engineering_projects"}},
"PUT /api/admin/engineering/projects/:id":                       {title: "修改工程项目", tables: []string{"engineering_projects"}},
"DELETE /api/admin/engineering/projects/:id":                    {title: "删除工程项目", tables: []string{"engineering_projects"}},
"PUT /api/admin/engineering/projects/:id/status":                {title: "工程状态流转", tables: []string{"engineering_projects"}},
"PUT /api/admin/engineering/projects/:id/items":                 {title: "修改工程设备清单", tables: []string{"engineering_projects", "engineering_project_items"}},
"PUT /api/admin/engineering/projects/:id/payment-terms":         {title: "修改工程付款节点", tables: []string{"engineering_projects", "engineering_payment_terms"}},
"POST /api/admin/engineering/projects/:id/receipts":             {title: "登记工程回款", tables: []string{"engineering_projects", "engineering_payment_terms", "engineering_receipts"}},
"POST /api/admin/engineering/projects/:id/receipts/:rid/reverse": {title: "冲销工程回款", tables: []string{"engineering_projects", "engineering_payment_terms", "engineering_receipts"}},
"POST /api/admin/engineering/projects/:id/shipments":            {title: "工程出库", tables: []string{"engineering_projects", "engineering_project_items", "engineering_shipments", "products", "inventory_logs"}},
"POST /api/admin/engineering/expenses":                          {title: "新增工程费用", tables: []string{"engineering_expenses", "engineering_projects"}},
"PUT /api/admin/engineering/expenses/:eid":                      {title: "修改工程费用", tables: []string{"engineering_expenses", "engineering_projects"}},
"DELETE /api/admin/engineering/expenses/:eid":                   {title: "删除工程费用", tables: []string{"engineering_expenses", "engineering_projects"}},
```

### 7.2 `pathParamTable` 追加

```go
case "id":
    // ... 既有分支
    if strings.HasPrefix(path, "/api/admin/engineering/projects/") { return "engineering_projects" }
case "rid":
    if strings.Contains(path, "/engineering/projects/") { return "engineering_receipts" }
case "eid":                                    // v1.1 新增
    if strings.Contains(path, "/engineering/expenses/") { return "engineering_expenses" }
```

> 路径参数刻意用 `:eid` 而不是 `:id`：现有 `case "id"` 分支已被多个模块占用，再往里塞 `expenses` 判断会让这个分支继续膨胀；新增独立 case 更安全，也避免 `/engineering/expenses/5` 被误判成 `engineering_projects`。

> 若不加这两条：新建类的操作能靠 `includeNew` 捕获，但**更新类操作（改状态、作废、冲销）会抓不到旧值**，历史页显示空 diff。这是必须改的点。

### 7.3 审计边界（明确不支持"一键撤销"）

现有操作历史的撤销能力依赖更复杂的反向逻辑。工程模块**不支持撤销按钮**，只做留痕（before/after 快照 + 操作人 + 时间）。理由：撤销工程出库需要反向加库存并回退多表状态，风险高于收益；真实场景下有出库/回款的项目应走"作废 + 人工补录"，而不是撤销。

---

## 8. 权限与安全

- 所有工程接口挂在 `auth.Group("/admin/engineering")` 下并 `Use(middlewares.AdminAuthRequired())` —— 员工端 `pos_token` 一律 403；
- `AdminAuthRequired` 已存在，直接复用，不新写中间件；
- 前端路由不做额外守卫（现有管理端布局已依赖 `erp_token`），但菜单项只在管理端布局出现；
- SQL 注入：全部走 GORM 参数绑定；聚合查询用 `?` 占位符，禁止 `fmt.Sprintf` 拼 SQL。

---

## 9. 性能

| 关注点 | 措施 |
| --- | --- |
| 列表查询 | 单条 SQL：projects + `LEFT JOIN OwnerEmployee` + 逾期聚合子查询；索引 `idx_eng_status`、`idx_eng_updated`、`idx_eng_sign_date` |
| 详情查询 | 5 条子表查询（items/terms/receipts/shipments/**expenses**），各自按 `project_id` 索引；总行数预计 < 200 |
| 汇总卡 | 一条 SQL 多聚合（`SUM(CASE WHEN ...)`），与列表同参数，独立接口便于前端并行请求 |
| 大项目 | 清单 > 200 行时前端表格启用虚拟滚动（Element Plus `el-table-v2`）——一期按普通表格，超过 200 行再优化（记录为已知限制） |
| **净利率排序** | `ORDER BY (contract_total - cost_total - expense_total) / NULLIF(contract_total,0)` 是表达式排序，**走不了索引**；项目量 < 1000 时可接受（全表扫描 1000 行约 2ms）。若将来项目数破万，再把 `net_profit` 落成物理列 + 索引 |
| 索引冗余风险 | **6 张**表新增索引均不影响既有表 |

---

## 10. 观测

- **日志**：每个写操作 `log.Printf("[engineering] %s project=%s user=%s ...")`，便于对账回溯；
- **巡检**：5.4 的 **8 条** SQL 建议做成 `scripts/check-engineering-consistency.sql`，每次部署后手工跑一次（不接入定时任务，避免增加运维面）；
- **错误日志**：事务回滚时打印失败原因与项目号，不打印金额明细（避免日志膨胀）。

---

## 11. 风险清单

| # | 风险 | 影响 | 缓解 |
| --- | --- | --- | --- |
| R1 | 改清单导致合同额与节点失配 | 数据自相矛盾 | 4.3 的四道强制点 + 巡检 SQL ③ |
| R2 | `received_amount` 冗余字段与流水不一致 | 待回款显示错 | 全部走条件更新（原子），禁止 `SUM` 后覆盖写；巡检 ①② 兜底 |
| R3 | 负库存出库被滥用 | 库存账面失真 | 必须显式确认 + 流水留痕 + 出库明细可查 |
| R4 | 工程出库流水混入成本修复逻辑 | 影响既有成本修复 | 明确不改 `historical_cost_repair` 的 `log_type` 白名单 |
| R5 | 操作历史文件改动引入回归 | 影响全站审计 | 只追加分支，不改既有 case；改完跑 `operation_history_controller_test.go` |
| R6 | 前端把工程数据混进零售统计 | 口径污染 | 后端接口不提供跨模块统计；前端不在看板引用工程接口 |
| R7 | 项目编号并发重号 | 建档失败 | 唯一索引 + 3 次重试 |
| **R8** | **老板把同一笔支出既录「工程费用」又录「经营开销」** | **成本双算，两边毛利都失真；且账户余额与实际不符** | ① 经营开销录入页顶部加提示，指向「工程 → 工程开销」；② 工程费用页副标题明写"与日常经营开销分开记账"；③ 付款方式字段标注"仅备查，不改变任何账户余额"；④ 实施培训时口头强调一次 |
| **R9** | **`expense_total` 重算漏路径（编辑改金额、删除、费用换项目）** | **项目净利长期错，且页面看不出来** | 全部重算收口到 `recalcExpenseTotal(tx, projectID)` 单一函数；对账 SQL ⑥ 每日兜底；单测覆盖三条路径 |
| **R10** | **公共费用被误当成某项目成本** | **项目净利偏低，老板决策被误导** | 台账按项目筛选时排除 NULL；列表用灰色 chip 标识；净利计算 SQL 显式带 `project_id = ?`（NULL 天然不匹配） |

---

## 12. 测试方案

### 12.1 单元测试（`engineering_controller_test.go`，跑在 `erp_audit_test` 库）

| # | 用例 | 断言 |
| --- | --- | --- |
| 1 | 金额重算（3 行清单 + 费用项 + 折让） | `contract_total` 精确等于手算值 |
| 2 | 节点合计 ≠ 合同额时签约 | 400 + `msg` 含差额 |
| 3 | 节点合计 = 合同额时签约 | 200 + `status=2` + `sign_date` 落库 |
| 4 | 已签约后改清单导致不平 | 409 + 事务回滚（清单未变） |
| 5 | 回款超额登记 | 409 + 节点 `received_amount` 未变 |
| 6 | 并发两笔回款（goroutine × 2，合计超应收） | 只有一笔成功，另一笔 409 |
| 7 | 收齐全款 | 自动 `status=5` + `settled_at` 非空 |
| 8 | 出库超订购量 | 400 |
| 9 | 出库库存不足且未确认 | 409 + 库存未变 |
| 10 | 出库库存不足但确认 | 200 + 库存为负 + 流水含"负库存" |
| 11 | 出库后状态 | `2 → 3` |
| 12 | 全部发完 + 验收 | `status=4` |
| 13 | 非法流转（1 → 4） | 400 |
| 14 | 3 → 2 回退（有出库） | 400 |
| 15 | 作废原因 < 5 字 | 400 |
| 16 | 员工 token 访问任一工程接口 | 403 |
| 17 | 重复 `client_request_id` 建项目 | 409 + 只建 1 条 |
| 18 | 删除已签约项目 | 400 |
| 19 | 冲销回款 | 节点与项目已收同步减少，状态可能回退 |
| 20 | 巡检 SQL ①②③ 在正常数据下返回 0 行 | 通过 |
| **21** | **新增项目费用 ¥3,000** | **`project_id`、`amount` 落库；项目 `expense_total` = 3000；净利减少 3000** |
| **22** | **编辑该费用改为 ¥3,500** | **`expense_total` = 3500（不是 6500，验证不是增量累加）** |
| **23** | **删除该费用** | **`expense_total` = 0；记录 `deleted_at` 非空；净利回原值** |
| **24** | **新增公共费用（`project_id` 为空）** | **任何项目 `expense_total` 不变；台账合计增加** |
| **25** | **把项目 1 的费用改挂到项目 2** | **项目 1 `expense_total` 减、项目 2 增，两个项目都正确** |
| **26** | **给已作废项目新增费用** | **400，费用未落库** |
| **27** | **金额 0 或负数** | **400** |
| **28** | **类别为「其他」但备注为空** | **400** |
| **29** | **并发：两个 goroutine 同时新增费用到同一项目** | **`expense_total` = 两笔之和（不丢一笔），巡检 SQL ⑥ 返回 0 行** |
| **30** | **重复 `client_request_id` 新增费用** | **409，只落 1 条** |
| **31** | **巡检 SQL ⑥ 在正常数据下返回 0 行** | **通过** |

### 12.2 集成自检（交付前必做，对应"先自检再交付"）

1. `go build ./...` 通过；
2. `go test ./controllers/ -run Engineering` 全绿；
3. 既有测试不回归：`go test ./controllers/ ./middlewares/ ./services/` 全绿（重点看 `operation_history_controller_test.go`）；
4. 前端 `npm run build` 通过；
5. 手工走查：建档 → 清单 → 节点 → 签约 → 回款 → 出库 → 验收 → 结清 → 作废，各一次；
6. **费用链路走查：录项目费用 → 改金额 → 删除 → 录公共费用 → 台账按类别筛选 → 核对项目净利与「本年工程开销」**；
7. **巡检 8 条 SQL 全 0 行**；
8. 回归确认：零售开单、国补管理、经营看板、库存流水页数据不变；
9. **`financial_logs` 与 `accounts` 表在整条工程链路操作前后，行数与余额完全不变（用 SQL 前后对比截图留档）**。

---

## 13. 实施顺序（供任务卡拆分）

| 批次 | 内容 | 依赖 | 可并行 |
| --- | --- | --- | --- |
| B1 | 模型（6 张表）+ AutoMigrate + 路由骨架 + 权限拦截 | — | 否（地基） |
| B2 | service：金额重算（含 `recalcExpenseTotal`）/ 状态机 / 校验 / 净利计算（纯函数 + 单测） | B1 | 与 B3 并行 |
| B3 | 列表 + 汇总接口（含净利率、本年工程开销） | B1 | 与 B2 并行 |
| B4 | 创建 / 详情（聚合 expenses）/ 更新 / 删除 / 编号生成 | B2 | 否 |
| B5 | 清单整表保存 + 节点整表保存 + 合同额一致性 | B4 | 与 B8 并行 |
| B6 | 回款登记 / 冲销 / 自动结清 | B5 | 与 B7、B8 并行 |
| B7 | 出库 + 库存扣减 + 流水 + 自动供货中 | B5 | 与 B6、B8 并行 |
| **B8** | **费用后端：台账查询 / 新增 / 编辑 / 删除 + `expense_total` 同事务重算** | **B4** | **与 B5/B6/B7 并行** |
| B9 | 前端：api + 列表页 + 详情页（5 Tab）+ 台账页 + 弹层 | B3~B8（可先对 mock） | 否 |
| B10 | 跨模块联动：库存流水类型、操作历史注册、菜单路由（3 条，台账在 `:id` 前）、经营开销页提示 | B1 | 与 B9 并行 |
| B11 | 测试补齐 + 全量自检 + 巡检 8 条 SQL | 全部 | 否 |

> **B8 为什么能提前并行**：费用只依赖"项目存在"这一个前提（B4），与清单、节点、回款、出库全无耦合。把它拉出来单独做，可以让前端在收到费用接口后先跑通「费用 Tab」和「台账页」，不必等回款链路做完。这是 v1.1 相比 v1.0 唯一新增的并行机会，工期增量可控在 1 个批次内。

> **已拆成可并行任务卡**：上表 11 个批次已细化为 16 张自包含任务卡（T01~T16，6 个波次，含文件所有权矩阵与冻结的组件/service 契约），见 `docs/tasks/2026-10-29-engineering-module-task-cards.md`（TASK-ENG-001）。开发按任务卡执行，本文档作为背景依据。

---

## 14. 一期明确不做（防止范围蔓延）

与 SRS 第 2.3 节一致，技术侧特别强调三条：

1. **不写 `orders` / `order_items` / `financial_logs` / `accounts`** —— 代码评审硬性拦截；
2. **不提供工程数据的 AI 工具接口** —— `ai_agent_data.go` 白名单不加工程表；
3. **不做工程提成** —— 不产生 `commission_amount`，不进工资条、不进离职结算；
4. **不做费用冲销/负数费用**（v1.1 补充）—— 费用错了就编辑或删除，不引入负号体系；
5. **不做公共费用自动分摊**（v1.1 补充）—— 二期再议，一期只做"不摊"；
6. **不做费用导出 Excel**（v1.1 补充）—— 台账页能看能筛即可。

---

**确认方式**：回复"确认"即进入第四阶段（并行开发任务卡）。若需调整表结构、并发策略或实施顺序，请指明小节编号。
