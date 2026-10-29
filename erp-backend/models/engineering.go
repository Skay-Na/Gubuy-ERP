package models

import (
	"time"

	"gorm.io/gorm"
)

// 工程模块（engineering）数据模型。
//
// 设计基线：docs/tech/2026-10-29-engineering-module-tech-design.md §2.2
//
// 三条隔离声明（代码评审一级拦截项）：
//  1. 本文件所有表与 orders / order_items 零关联：工程业务独立成表，不并入 OrderType；
//  2. 本文件所有表与 accounts / financial_logs 零关联：工程回款与费用都不改任何账户余额；
//  3. engineering_expenses 与经营开销（account_id=7）零关联：工程开销独立成账，禁止两边重复记。

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

	SignDate   *time.Time `gorm:"type:date;index;comment:签约日期" json:"sign_date"`
	AcceptDate *time.Time `gorm:"type:date;comment:验收日期" json:"accept_date"`
	SettledAt  *time.Time `gorm:"comment:结清时间" json:"settled_at"`

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
	ID         uint      `gorm:"primaryKey" json:"id"`
	ProjectID  uint      `gorm:"index;not null" json:"project_id"`
	ProductID  uint      `gorm:"index;not null" json:"product_id"`
	Product    *Product  `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	Quantity   int       `gorm:"type:int;not null;comment:订购数量" json:"quantity"`
	UnitPrice  float64   `gorm:"type:decimal(12,2);not null;comment:工程单价" json:"unit_price"`
	UnitCost   float64   `gorm:"type:decimal(12,2);not null;default:0;comment:成本单价快照" json:"unit_cost"`
	ShippedQty int       `gorm:"type:int;not null;default:0;comment:已出库数量" json:"shipped_qty"`
	Remark     string    `gorm:"type:varchar(200);not null;default:''" json:"remark"`
	SortOrder  int       `gorm:"type:int;not null;default:0" json:"sort_order"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
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
	ID        uint                    `gorm:"primaryKey" json:"id"`
	ProjectID uint                    `gorm:"index;not null" json:"project_id"`
	TermID    uint                    `gorm:"index;not null" json:"term_id"`
	Term      *EngineeringPaymentTerm `gorm:"foreignKey:TermID" json:"term,omitempty"`

	Amount      float64   `gorm:"type:decimal(12,2);not null;comment:金额(冲销为负)" json:"amount"`
	PaymentDate time.Time `gorm:"type:date;index;not null;comment:到账日期" json:"payment_date"`
	Method      int       `gorm:"type:tinyint;not null;default:1;comment:1银行转账 2承兑汇票 3现金 4支票 5其他" json:"method"`
	BillNo      string    `gorm:"type:varchar(50);not null;default:'';comment:票据号/流水号" json:"bill_no"`
	Remark      string    `gorm:"type:varchar(200);not null;default:''" json:"remark"`

	ReversedReceiptID *uint `gorm:"index;comment:被冲销的原记录ID" json:"reversed_receipt_id"`

	OperatorID      *uint   `gorm:"index" json:"operator_id"`
	OperatorName    string  `gorm:"type:varchar(100);not null;default:''" json:"operator_name"`
	ClientRequestID *string `gorm:"type:varchar(64);uniqueIndex;comment:幂等键" json:"client_request_id,omitempty"`

	CreatedAt time.Time `json:"created_at"`
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

	OperatorID   *uint  `gorm:"index" json:"operator_id"`
	OperatorName string `gorm:"type:varchar(100);not null;default:''" json:"operator_name"`

	CreatedAt time.Time `json:"created_at"`
}

// EngineeringExpense 工程费用
//
// 隔离声明（代码评审一级拦截项）：
//   - 本表与 accounts / financial_logs / account_id=7（经营开销）零关联；
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
