package models

import (
	"time"

	"gorm.io/gorm"
)

// Account 资金账户表
type Account struct {
	ID               uint           `gorm:"primaryKey;comment:主键ID" json:"id"`
	Name             string         `gorm:"type:varchar(100);not null;uniqueIndex;comment:账户名称（支付宝/微信/公户）" json:"name"`
	Balance          float64        `gorm:"type:decimal(12,2);not null;default:0;comment:当前余额" json:"balance"`
	IsDefaultExpense bool           `gorm:"not null;default:false;comment:是否为默认支出账户" json:"is_default_expense"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"` // 软删除保护，防止误删导致历史账单断层
}

// AdminUser 管理员账户
type AdminUser struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	Username  string    `gorm:"type:varchar(100);uniqueIndex;not null" json:"username"`
	Password  string    `gorm:"type:varchar(255);not null" json:"-"`
	UpdatedAt time.Time `json:"updated_at"`
}

// TransferRecord 账户划拨记录
type TransferRecord struct {
	ID            uint     `gorm:"primaryKey;comment:主键ID" json:"id"`
	FromAccountID uint     `gorm:"not null;comment:转出账户ID" json:"from_account_id"`
	ToAccountID   uint     `gorm:"not null;comment:转入账户ID" json:"to_account_id"`
	Amount        float64  `gorm:"type:decimal(12,2);not null;comment:划拨金额" json:"amount"`
	Remark        string   `gorm:"type:text;comment:备注说明" json:"remark"`
	FromAccount   *Account `gorm:"foreignKey:FromAccountID" json:"from_account,omitempty"`
	ToAccount     *Account `gorm:"foreignKey:ToAccountID" json:"to_account,omitempty"`
	// 客户端幂等键：资金划拨可选携带，防重复划拨
	ClientRequestID *string   `gorm:"type:varchar(64);uniqueIndex:uniq_transfer_records_client_request_id;comment:客户端幂等键(防重复划拨)" json:"client_request_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// Category 商品分类表
type Category struct {
	ID        uint           `gorm:"primaryKey;comment:主键ID" json:"id"`
	Name      string         `gorm:"type:varchar(100);not null;index;comment:分类名称" json:"name"`
	ParentID  *uint          `gorm:"index;comment:父分类ID（NULL=一级大分类）" json:"parent_id"`
	SortOrder int            `gorm:"type:int;default:0;comment:排序权重" json:"sort_order"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// Product 商品表
type Product struct {
	ID           uint    `gorm:"primaryKey;comment:主键ID" json:"id"`
	Name         string  `gorm:"type:varchar(255);not null;comment:商品名称" json:"name"`
	Remark       *string `gorm:"column:sku;type:varchar(255);comment:商品备注" json:"remark"`
	LatestCost   float64 `gorm:"type:decimal(10,2);not null;default:0;comment:最新进价" json:"latest_cost"`
	AvgCost      float64 `gorm:"type:decimal(10,2);not null;default:0;comment:平均成本" json:"avg_cost"`
	MarginRate   float64 `gorm:"type:decimal(5,2);not null;default:0.1;comment:固定毛利率" json:"margin_rate"`
	MainStock    int     `gorm:"type:int;not null;default:0;comment:已停用主仓库存（历史兼容字段）" json:"main_stock"`
	StoreStock   int     `gorm:"type:int;not null;default:0;comment:门店可用库存" json:"store_stock"`
	SampleStock  int     `gorm:"type:int;not null;default:0;comment:门店样机数量" json:"sample_stock"`
	CloudStock   int     `gorm:"type:int;not null;default:0;comment:云仓自有库存" json:"cloud_stock"`
	SupportCloud bool    `gorm:"not null;default:true;comment:历史兼容字段，所有商品均可云仓代发" json:"-"`
	NoCommission bool    `gorm:"not null;default:false;comment:销售该商品是否不计提成" json:"no_commission"`
	// 0=跟随员工提成比例，>0=每台固定提成金额（不参与订单折扣分摊）
	CommissionFixedAmount float64        `gorm:"type:decimal(10,2);not null;default:0;comment:每台固定提成金额(0=跟随员工比例)" json:"commission_fixed_amount"`
	SpecialPrice          float64        `gorm:"type:decimal(10,2);default:0;comment:特价金额" json:"special_price"`
	SpecialPriceStart     *time.Time     `gorm:"comment:特价开始时间" json:"special_price_start"`
	SpecialPriceEnd       *time.Time     `gorm:"comment:特价结束时间" json:"special_price_end"`
	CategoryID            uint           `gorm:"index;comment:所属分类ID" json:"category_id"`
	Category              *Category      `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
	CreatedAt             time.Time      `json:"created_at"`
	UpdatedAt             time.Time      `json:"updated_at"`
	DeletedAt             gorm.DeletedAt `gorm:"index" json:"-"`
}

// Employee 员工表
type Employee struct {
	ID               uint           `gorm:"primaryKey;comment:主键ID" json:"id"`
	Name             string         `gorm:"type:varchar(100);not null;comment:员工姓名" json:"name"`
	Phone            string         `gorm:"type:varchar(20);comment:联系电话" json:"phone"`
	EmpNo            string         `gorm:"type:varchar(50);uniqueIndex;not null;comment:工号" json:"emp_no"`
	BaseSalary       float64        `gorm:"type:decimal(10,2);not null;default:0;comment:基本底薪" json:"base_salary"`
	CommissionRate   float64        `gorm:"type:decimal(5,4);not null;default:0;comment:提成比例" json:"commission_rate"`
	PinCode          string         `gorm:"type:varchar(10);comment:终端登录PIN码" json:"pin_code"`
	Role             string         `gorm:"type:varchar(50);default:'sales';comment:角色分类(admin/sales/installer等)" json:"role"`
	Pinyin           string         `gorm:"type:varchar(255);comment:名字全拼" json:"pinyin"`
	PinyinInitial    string         `gorm:"type:varchar(100);comment:名字首字母" json:"pinyin_initial"`
	EntryDate        *time.Time     `gorm:"type:date;comment:入职时间" json:"entry_date"`
	EmploymentStatus string         `gorm:"type:varchar(20);not null;default:'active';index;comment:任职状态 active/offboarding/departed" json:"employment_status"`
	LastWorkingDate  *time.Time     `gorm:"type:date;index;comment:最后工作日" json:"last_working_date,omitempty"`
	IsOutsourced     bool           `gorm:"default:false;comment:是否为外包人员" json:"is_outsourced"`
	OutsourcedType   string         `gorm:"type:varchar(20);default:'';comment:外包类型：install(安装), delivery(送货)" json:"outsourced_type"`
	CreatedAt        time.Time      `json:"created_at"`
	UpdatedAt        time.Time      `json:"updated_at"`
	DeletedAt        gorm.DeletedAt `gorm:"index" json:"-"`
}

// DailyAttendance 每日打卡表
type DailyAttendance struct {
	ID         uint       `gorm:"primaryKey;comment:主键ID" json:"id"`
	EmployeeID uint       `gorm:"index;not null;comment:员工ID" json:"employee_id"`
	Date       string     `gorm:"type:varchar(10);index;not null;comment:日期 YYYY-MM-DD" json:"date"`
	CheckIn    *time.Time `json:"check_in"`
	CheckOut   *time.Time `json:"check_out"`
	Status     int        `gorm:"type:tinyint;default:1;comment:状态：1-正常, 2-迟到, 3-早退, 4-缺勤" json:"status"`
	Remark     string     `gorm:"type:varchar(255)" json:"remark"`
	CreatedAt  time.Time  `json:"created_at"`
	UpdatedAt  time.Time  `json:"updated_at"`
}

// MonthlyAttendance 员工月度考勤表
type MonthlyAttendance struct {
	ID         uint      `gorm:"primaryKey" json:"id"`
	EmployeeID uint      `gorm:"index" json:"employee_id"`
	Month      string    `gorm:"type:varchar(7);index;comment:月份 YYYY-MM" json:"month"`
	LeaveDays  int       `gorm:"default:0;comment:请假天数" json:"leave_days"`
	ActualDays int       `gorm:"default:0;comment:实际出勤天数" json:"actual_days"`
	LateCount  int       `gorm:"default:0;comment:迟到次数" json:"late_count"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// EmployeeOffboarding stores the handover case and immutable final-payroll snapshot.
type EmployeeOffboarding struct {
	ID                         uint       `gorm:"primaryKey" json:"id"`
	EmployeeID                 uint       `gorm:"not null;index" json:"employee_id"`
	EmployeeName               string     `gorm:"type:varchar(100);not null" json:"employee_name"`
	EmpNo                      string     `gorm:"type:varchar(50);not null;index" json:"emp_no"`
	LastWorkingDate            time.Time  `gorm:"type:date;not null;index" json:"last_working_date"`
	SuccessorEmployeeID        *uint      `gorm:"index" json:"successor_employee_id,omitempty"`
	Status                     string     `gorm:"type:varchar(20);not null;default:'in_progress';index" json:"status"`
	HolidayDays                int        `gorm:"not null;default:0" json:"holiday_days"`
	CalendarDays               int        `gorm:"not null;default:0" json:"calendar_days"`
	SignedInDays               int        `gorm:"not null;default:0" json:"signed_in_days"`
	BaseSalarySnapshot         float64    `gorm:"type:decimal(12,2);not null;default:0" json:"base_salary_snapshot"`
	CalculatedBaseSalary       float64    `gorm:"type:decimal(12,2);not null;default:0" json:"calculated_base_salary"`
	PersonalCommissionSnapshot float64    `gorm:"type:decimal(12,2);not null;default:0" json:"personal_commission_snapshot"`
	ChamberCommissionSnapshot  float64    `gorm:"type:decimal(12,2);not null;default:0" json:"chamber_commission_snapshot"`
	CommissionSnapshot         float64    `gorm:"type:decimal(12,2);not null;default:0" json:"commission_snapshot"`
	CommissionDetailsSnapshot  string     `gorm:"type:longtext" json:"-"`
	FinalPayableSnapshot       float64    `gorm:"type:decimal(12,2);not null;default:0" json:"final_payable_snapshot"`
	SettlementMonth            string     `gorm:"type:char(7);not null;default:''" json:"settlement_month"`
	SettlementConfirmedAt      *time.Time `json:"settlement_confirmed_at,omitempty"`
	SettlementConfirmedBy      string     `gorm:"type:varchar(100)" json:"settlement_confirmed_by,omitempty"`
	CreatedBy                  string     `gorm:"type:varchar(100);not null" json:"created_by"`
	CompletedAt                *time.Time `json:"completed_at,omitempty"`
	CompletedBy                string     `gorm:"type:varchar(100)" json:"completed_by,omitempty"`
	Notes                      string     `gorm:"type:text" json:"notes,omitempty"`
	CreatedAt                  time.Time  `json:"created_at"`
	UpdatedAt                  time.Time  `json:"updated_at"`
}

// EmployeeHandoverTask is a separately accepted checklist item or order follow-up.
type EmployeeHandoverTask struct {
	ID                 uint       `gorm:"primaryKey" json:"id"`
	OffboardingID      uint       `gorm:"not null;index;uniqueIndex:idx_offboarding_task_key" json:"offboarding_id"`
	EmployeeID         uint       `gorm:"not null;index" json:"employee_id"`
	OrderID            *uint      `gorm:"index" json:"order_id,omitempty"`
	TaskKey            string     `gorm:"type:varchar(120);not null;uniqueIndex:idx_offboarding_task_key" json:"task_key"`
	TaskType           string     `gorm:"type:varchar(40);not null;index" json:"task_type"`
	Title              string     `gorm:"type:varchar(160);not null" json:"title"`
	AssigneeEmployeeID *uint      `gorm:"index" json:"assignee_employee_id,omitempty"`
	Status             string     `gorm:"type:varchar(20);not null;default:'pending';index" json:"status"`
	Notes              string     `gorm:"type:text" json:"notes,omitempty"`
	CompletedBy        string     `gorm:"type:varchar(100)" json:"completed_by,omitempty"`
	CompletedAt        *time.Time `json:"completed_at,omitempty"`
	CreatedAt          time.Time  `json:"created_at"`
	UpdatedAt          time.Time  `json:"updated_at"`
}

// EmployeeOffboardingAlert keeps post-exit order changes visible until reviewed.
type EmployeeOffboardingAlert struct {
	ID               uint       `gorm:"primaryKey" json:"id"`
	OffboardingID    uint       `gorm:"not null;index" json:"offboarding_id"`
	EmployeeID       uint       `gorm:"not null;index" json:"employee_id"`
	OrderID          uint       `gorm:"not null;index" json:"order_id"`
	EmployeeName     string     `gorm:"type:varchar(100);not null" json:"employee_name"`
	FinancialLogID   *uint      `gorm:"index" json:"financial_log_id,omitempty"`
	SourceKey        string     `gorm:"type:varchar(140);not null;uniqueIndex" json:"source_key"`
	AlertType        string     `gorm:"type:varchar(40);not null;index" json:"alert_type"`
	Severity         string     `gorm:"type:varchar(20);not null;default:'high'" json:"severity"`
	Status           string     `gorm:"type:varchar(20);not null;default:'open';index" json:"status"`
	OrderNo          string     `gorm:"type:varchar(50);not null" json:"order_no"`
	RefundAmount     float64    `gorm:"type:decimal(12,2);not null;default:0" json:"refund_amount"`
	CommissionAmount float64    `gorm:"type:decimal(12,2);not null;default:0" json:"commission_amount"`
	Message          string     `gorm:"type:varchar(500);not null" json:"message"`
	OccurredAt       time.Time  `gorm:"index" json:"occurred_at"`
	ResolvedBy       string     `gorm:"type:varchar(100)" json:"resolved_by,omitempty"`
	ResolvedAt       *time.Time `json:"resolved_at,omitempty"`
	Resolution       string     `gorm:"type:text" json:"resolution,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// OrderDailyCounter 每日订单序号计数器
type OrderDailyCounter struct {
	ID           uint      `gorm:"primaryKey" json:"id"`
	BusinessDate string    `gorm:"type:char(10);uniqueIndex;not null;comment:业务日期 YYYY-MM-DD" json:"business_date"`
	CurrentSeq   int       `gorm:"not null;default:0;comment:当日当前序号" json:"current_seq"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Order 销售订单主表
type Order struct {
	ID                    uint    `gorm:"primaryKey;comment:主键ID" json:"id"`
	OrderNo               string  `gorm:"type:varchar(50);uniqueIndex;not null;comment:唯一订单号" json:"order_no"`
	CustomerName          string  `gorm:"type:varchar(100);comment:客户姓名" json:"customer_name"`
	CustomerPinyin        string  `gorm:"type:varchar(255);comment:客户姓名全拼" json:"customer_pinyin"`
	CustomerPinyinInitial string  `gorm:"type:varchar(100);comment:客户姓名首字母" json:"customer_pinyin_initial"`
	CustomerPhone         string  `gorm:"type:varchar(20);comment:客户手机号" json:"customer_phone"`
	CustomerAddress       string  `gorm:"type:varchar(255);comment:客户详细地址" json:"customer_address"`
	TotalAmount           float64 `gorm:"type:decimal(10,2);not null;comment:商品总金额" json:"total_amount"`
	SubsidyAmount         float64 `gorm:"type:decimal(10,2);not null;default:0;comment:国补金额" json:"subsidy_amount"`
	SubsidyPendingAmount  float64 `gorm:"type:decimal(10,2);not null;default:0;comment:实际国补待回款金额(反推原价)" json:"subsidy_pending_amount"`
	SubsidyReceivedAmt    float64 `gorm:"type:decimal(10,2);not null;default:0;comment:已核销国补回款金额(累计)；received>=pending 即已回款" json:"subsidy_received_amt"`
	ActualPayAmount       float64 `gorm:"type:decimal(10,2);not null;comment:客户实际应付" json:"actual_pay_amount"`
	DepositAmount         float64 `gorm:"type:decimal(10,2);not null;default:0;comment:已付定金" json:"deposit_amount"`
	DiscountAmount        float64 `gorm:"type:decimal(10,2);not null;default:0;comment:手动抹零优惠" json:"discount_amount"`
	PaymentStatus         int     `gorm:"type:tinyint;not null;default:1;comment:支付状态：1-仅付定金, 2-已结全款" json:"payment_status"`
	SubsidyStatus         int     `gorm:"type:tinyint;not null;default:0;comment:国补资料状态：0-无国补, 1-待提交, 2-已提交, 3-已回款" json:"subsidy_status"`
	// 新增：安装相关字段
	IsDelivered      bool       `gorm:"default:false;comment:是否已送货" json:"is_delivered"`
	DeliveryTime     *time.Time `json:"delivery_time"`
	IsInstalled      bool       `gorm:"default:false;comment:是否已安装" json:"is_installed"`
	InstallerID      *uint      `gorm:"index;comment:安装人ID" json:"installer_id"`
	InstallTime      *time.Time `json:"install_time"`
	InstallationFee  float64    `gorm:"type:decimal(10,2);not null;default:0;comment:实际安装费用" json:"installation_fee"`
	DeliveryFee      float64    `gorm:"type:decimal(10,2);not null;default:0;comment:实际送货费用" json:"delivery_fee"`
	Installer        *Employee  `gorm:"foreignKey:InstallerID" json:"installer,omitempty"`
	DeliveryPersonID *uint      `gorm:"index;comment:送货人ID" json:"delivery_person_id"`
	DeliveryPerson   *Employee  `gorm:"foreignKey:DeliveryPersonID" json:"delivery_person,omitempty"`
	// 新增：支付方式 (1-支付宝, 2-微信, 3-公户, 4-混合支付, 5-现金)
	PaymentMethod    int    `gorm:"type:tinyint;not null;default:1;comment:支付方式：1-支付宝, 2-微信, 3-公户, 4-混合支付, 5-现金" json:"payment_method"`
	PaymentBreakdown string `gorm:"type:text;comment:支付拆分明细JSON" json:"payment_breakdown"`
	// 新增：订单状态 (1-正常, 2-已取消)
	OrderStatus int `gorm:"type:tinyint;not null;default:1;comment:订单状态：1-正常, 2-已取消" json:"order_status"`
	// 新增：订单类型 (1-普通订单, 2-商会订单)
	OrderType int `gorm:"type:tinyint;not null;default:1;comment:订单类型：1-普通订单, 2-商会订单" json:"order_type"`
	// 客户端幂等键：前端每次清空购物车/进入开单页生成 UUID，防止双击重复开单。
	// 指针类型保证未传时落库为 NULL（MySQL 唯一索引允许多个 NULL），历史数据无需迁移。
	ClientRequestID *string `gorm:"type:varchar(64);uniqueIndex:uniq_orders_client_request_id;comment:客户端幂等键(防重复开单)" json:"client_request_id,omitempty"`
	// 新增：关联账户
	AccountID         *uint       `gorm:"index;comment:收款账户ID" json:"account_id"`
	Account           *Account    `gorm:"foreignKey:AccountID" json:"account,omitempty"`
	OrderItems        []OrderItem `gorm:"foreignKey:OrderID" json:"order_items"`
	EmployeeID        *uint       `gorm:"index;comment:开单员工ID" json:"employee_id"`
	Employee          *Employee   `gorm:"foreignKey:EmployeeID" json:"employee,omitempty"`
	ReferrerName      string      `gorm:"type:varchar(100);comment:推荐人姓名" json:"referrer_name"`
	ReferralFee       float64     `gorm:"type:decimal(10,2);not null;default:0;comment:第三方带单提成金额" json:"referral_fee"`
	IsReferralFeePaid bool        `gorm:"not null;default:false;comment:推荐提成是否已发" json:"is_referral_fee_paid"`
	CommissionAmount  float64     `gorm:"type:decimal(10,2);not null;default:0;comment:业务提成金额" json:"commission_amount"`
	// 发货/提货方式：1-门店自提，2/3-历史方式，4-云仓发货。
	DeliveryMethod int `gorm:"type:tinyint;not null;default:1;comment:发货方式：1-门店自提, 2/3-历史方式, 4-云仓发货" json:"delivery_method"`
	// 新增：以旧换新相关字段
	TradeInAmount float64        `gorm:"type:decimal(10,2);not null;default:0;comment:以旧换新折抵金额" json:"trade_in_amount"`
	TradeInType   string         `gorm:"type:varchar(100);comment:以旧换新旧件品类" json:"trade_in_type"`
	TradeInRemark string         `gorm:"type:varchar(255);comment:以旧换新备注描述" json:"trade_in_remark"`
	CreatedAt     time.Time      `json:"created_at"`
	UpdatedAt     time.Time      `json:"updated_at"`
	DeletedAt     gorm.DeletedAt `gorm:"index" json:"-"`
}

// OrderItem 订单明细表
type OrderItem struct {
	ID                uint     `gorm:"primaryKey;comment:主键ID" json:"id"`
	OrderID           uint     `gorm:"not null;index;comment:关联 Order 的 ID" json:"order_id"`
	ProductID         uint     `gorm:"not null;index;comment:关联 Product 的 ID" json:"product_id"`
	UnitPrice         float64  `gorm:"type:decimal(10,2);not null;comment:实际成交单价" json:"unit_price"`
	UnitCost          float64  `gorm:"type:decimal(10,2);not null;default:0;comment:下单时进价快照" json:"unit_cost"`
	Quantity          int      `gorm:"type:int;not null;default:1;comment:购买数量" json:"quantity"`
	StoreDeductionQty int      `gorm:"type:int;not null;default:0;comment:从门店可用库存扣除的数量" json:"store_deduction_qty"`
	CloudDeductionQty int      `gorm:"type:int;not null;default:0;comment:从云仓自有库存扣除的数量" json:"cloud_deduction_qty"`
	IsGift            bool     `gorm:"type:tinyint(1);not null;default:0;comment:是否为赠品" json:"is_gift"`
	NoCommission      bool     `gorm:"not null;default:false;comment:下单时商品是否不计提成快照" json:"no_commission"`
	Product           *Product `gorm:"foreignKey:ProductID" json:"product,omitempty"`

	// 新增：单品级配送追踪
	IsDelivered      bool       `gorm:"not null;default:false;comment:是否已送货" json:"is_delivered"`
	DeliveryTime     *time.Time `gorm:"comment:送货时间" json:"delivery_time"`
	DeliveryPersonID *uint      `gorm:"index;comment:送货人员ID" json:"delivery_person_id"`
	DeliveryPerson   *Employee  `gorm:"foreignKey:DeliveryPersonID" json:"delivery_person,omitempty"`
	DeliveryFee      float64    `gorm:"type:decimal(10,2);not null;default:0;comment:单品送货费" json:"delivery_fee"`

	// 新增：单品级安装追踪
	IsInstalled     bool       `gorm:"not null;default:false;comment:是否已安装" json:"is_installed"`
	InstallTime     *time.Time `gorm:"comment:安装时间" json:"install_time"`
	InstallerID     *uint      `gorm:"index;comment:安装师傅ID" json:"installer_id"`
	Installer       *Employee  `gorm:"foreignKey:InstallerID" json:"installer,omitempty"`
	InstallationFee float64    `gorm:"type:decimal(10,2);not null;default:0;comment:单品安装费" json:"installation_fee"`

	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// Expense 门店日常支出表
type Expense struct {
	ID          uint    `gorm:"primaryKey;comment:主键ID" json:"id"`
	ExpenseType string  `gorm:"type:varchar(100);not null;comment:支出类型：房租/水电/送货费等" json:"expense_type"`
	Amount      float64 `gorm:"type:decimal(10,2);not null;comment:支出金额" json:"amount"`
	Remark      string  `gorm:"type:text;comment:备注说明" json:"remark"`
	// 新增：关联账户
	AccountID *uint    `gorm:"index;comment:支出账户ID" json:"account_id"`
	Account   *Account `gorm:"foreignKey:AccountID" json:"account,omitempty"`
	RelatedNo string   `gorm:"type:varchar(100);comment:关联人员或单号" json:"related_no"`
	// 客户端幂等键：可选；携带时用于防重复提交（AutoMigrate 自动加列，历史数据为 NULL 不受影响）
	ClientRequestID *string   `gorm:"type:varchar(64);uniqueIndex:uniq_expenses_client_request_id;comment:客户端幂等键(防重复提交)" json:"client_request_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// FinancialLog 全口径财务流水表
type FinancialLog struct {
	ID             uint     `gorm:"primaryKey;comment:主键ID" json:"id"`
	AccountID      uint     `gorm:"not null;index;comment:关联资金账户ID" json:"account_id"`
	Account        *Account `gorm:"foreignKey:AccountID" json:"account,omitempty"`
	FlowType       int      `gorm:"type:tinyint;not null;comment:资金流向" json:"flow_type"`
	Category       string   `gorm:"type:varchar(50);not null;comment:细分分类" json:"category"`
	Amount         float64  `gorm:"type:decimal(10,2);not null;comment:变动金额(绝对值)" json:"amount"`
	BalanceAfter   float64  `gorm:"type:decimal(10,2);not null;comment:变动后该账户余额快照" json:"balance_after"`
	RelatedNo      string   `gorm:"type:varchar(100);comment:关联单号(订单号等)" json:"related_no"`
	Remark         string   `gorm:"type:varchar(255);comment:备注说明" json:"remark"`
	Counterparty   string   `gorm:"type:varchar(100);comment:往来对象" json:"counterparty"`
	SourceType     string   `gorm:"type:varchar(30);not null;default:'system';index;comment:来源类型 system/manual" json:"source_type"`
	ReversalPairID *uint    `gorm:"index;comment:冲销配对ID；配对日志(定金+退定金 / 销售+退款)汇总时排除" json:"reversal_pair_id,omitempty"`
	// 客户端幂等键：手工记账可选携带，防重复记账
	ClientRequestID *string   `gorm:"type:varchar(64);uniqueIndex:uniq_financial_logs_client_request_id;comment:客户端幂等键(防重复记账)" json:"client_request_id,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
}

// InboundRecord 入库记录表
type InboundRecord struct {
	ID             uint    `gorm:"primaryKey;comment:主键ID" json:"id"`
	ProductID      uint    `gorm:"not null;index;comment:关联商品ID" json:"product_id"`
	Quantity       int     `gorm:"not null;comment:入库数量" json:"quantity"`
	UnitCost       float64 `gorm:"type:decimal(10,2);not null;comment:入库单价" json:"unit_cost"`
	TotalCost      float64 `gorm:"type:decimal(10,2);not null;comment:本次入库总成本" json:"total_cost"`
	SettlementType string  `gorm:"type:varchar(30);not null;default:'cash_purchase';comment:结算方式 cash_purchase/sale_settlement" json:"settlement_type"`
	// 新增：关联账户
	AccountID *uint     `gorm:"index;comment:支出账户ID" json:"account_id"`
	Account   *Account  `gorm:"foreignKey:AccountID" json:"account,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// PendingInbound 待确认入库申请表
type PendingInbound struct {
	ID                  uint      `gorm:"primaryKey;comment:主键ID" json:"id"`
	ProductID           uint      `gorm:"not null;index;comment:关联商品ID" json:"product_id"`
	Product             *Product  `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	WarehouseType       int       `gorm:"type:tinyint;not null;comment:仓库类型：1-主仓, 2-门店, 3-云仓" json:"warehouse_type"`
	Quantity            int       `gorm:"not null;comment:申请入库数量" json:"quantity"`
	SubmitterEmployeeID *uint     `gorm:"index;comment:提交员工ID" json:"submitter_employee_id,omitempty"`
	SubmitterName       string    `gorm:"type:varchar(50);not null;comment:提交人姓名" json:"submitter_name"`
	Remark              string    `gorm:"type:varchar(255);comment:备注说明" json:"remark"`
	Status              string    `gorm:"type:varchar(20);not null;default:'pending';comment:状态: pending/approved/rejected" json:"status"`
	RejectionReason     string    `gorm:"type:varchar(255);comment:拒绝原因" json:"rejection_reason,omitempty"`
	CreatedAt           time.Time `json:"created_at"`
	UpdatedAt           time.Time `json:"updated_at"`
}

// PendingStocktake stores employee counts that need administrator review before stock changes.
type PendingStocktake struct {
	ID                   uint       `gorm:"primaryKey" json:"id"`
	StocktakeBatchItemID *uint      `gorm:"index" json:"stocktake_batch_item_id,omitempty"`
	ProductID            uint       `gorm:"not null;index" json:"product_id"`
	Product              *Product   `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	WarehouseType        int        `gorm:"type:tinyint;not null" json:"warehouse_type"`
	ExpectedStock        int        `gorm:"not null" json:"expected_stock"`
	ActualStock          int        `gorm:"not null" json:"actual_stock"`
	Difference           int        `gorm:"not null" json:"difference"`
	StocktakeType        int        `gorm:"type:tinyint;not null;default:1" json:"stocktake_type"`
	Remark               string     `gorm:"type:varchar(255);not null" json:"remark"`
	OperatorID           *uint      `gorm:"index" json:"operator_id,omitempty"`
	OperatorName         string     `gorm:"type:varchar(100);not null" json:"operator_name"`
	Status               string     `gorm:"type:varchar(20);not null;default:'pending';index" json:"status"`
	ReviewRemark         string     `gorm:"type:varchar(255)" json:"review_remark,omitempty"`
	ReviewedBy           string     `gorm:"type:varchar(100)" json:"reviewed_by,omitempty"`
	ReviewedAt           *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt            time.Time  `json:"created_at"`
	UpdatedAt            time.Time  `json:"updated_at"`
}

// StocktakeBatch is a blind-count task created before employees count physical stock.
type StocktakeBatch struct {
	ID            uint                 `gorm:"primaryKey" json:"id"`
	BatchNo       string               `gorm:"type:varchar(40);uniqueIndex;not null" json:"batch_no"`
	WarehouseType int                  `gorm:"type:tinyint;not null;index" json:"warehouse_type"`
	ScopeLabel    string               `gorm:"type:varchar(255);not null" json:"scope_label"`
	OperatorID    *uint                `gorm:"index" json:"operator_id,omitempty"`
	OperatorName  string               `gorm:"type:varchar(100);not null" json:"operator_name"`
	StocktakeType int                  `gorm:"type:tinyint;not null;default:1" json:"stocktake_type"`
	PrintCount    int                  `gorm:"not null;default:0" json:"print_count"`
	PrintedAt     *time.Time           `json:"printed_at,omitempty"`
	Status        string               `gorm:"type:varchar(20);not null;default:'in_progress';index" json:"status"`
	CreatedAt     time.Time            `json:"created_at"`
	UpdatedAt     time.Time            `json:"updated_at"`
	Items         []StocktakeBatchItem `gorm:"foreignKey:BatchID" json:"items,omitempty"`
}

// StocktakeBatchItem stores a hidden book-stock snapshot and the employee's physical count.
type StocktakeBatchItem struct {
	ID                 uint      `gorm:"primaryKey" json:"id"`
	BatchID            uint      `gorm:"not null;index" json:"batch_id"`
	ProductID          uint      `gorm:"not null;index" json:"product_id"`
	ProductName        string    `gorm:"type:varchar(255);not null" json:"product_name"`
	ProductRemark      string    `gorm:"type:varchar(255)" json:"product_remark"`
	ExpectedStock      int       `gorm:"not null" json:"-"`
	ActualStock        *int      `json:"actual_stock,omitempty"`
	Remark             string    `gorm:"type:varchar(255)" json:"remark"`
	Difference         int       `gorm:"not null;default:0" json:"difference"`
	PendingStocktakeID *uint     `gorm:"index" json:"pending_stocktake_id,omitempty"`
	Status             string    `gorm:"type:varchar(20);not null;default:'uncounted';index" json:"status"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// StocktakeRecord 盘点记录表
type StocktakeRecord struct {
	ID            uint `gorm:"primaryKey;comment:主键ID" json:"id"`
	ProductID     uint `gorm:"not null;index;comment:关联商品ID" json:"product_id"`
	WarehouseType int  `gorm:"type:tinyint;not null;comment:仓库类型：1-主仓, 2-门店, 3-云仓" json:"warehouse_type"`
	// StocktakeType: 1-正常损耗(盘亏计入财务), 2-业务调整(跨店销售/移库，仅修正库存)
	StocktakeType int       `gorm:"type:tinyint;not null;default:1;comment:盘点性质：1-正常损耗, 2-业务调整" json:"stocktake_type"`
	BeforeStock   int       `gorm:"not null;comment:盘点前库存" json:"before_stock"`
	AfterStock    int       `gorm:"not null;comment:盘点后(实际)库存" json:"after_stock"`
	Difference    int       `gorm:"not null;comment:差异数量(后-前)" json:"difference"`
	Remark        string    `gorm:"type:varchar(255);comment:转账备注" json:"remark"`
	CreatedAt     time.Time `json:"created_at"`
}

// InventoryLog 库存流水审计表
type InventoryLog struct {
	ID            uint      `gorm:"primaryKey;comment:主键ID" json:"id"`
	ProductID     uint      `gorm:"not null;index;comment:关联商品ID" json:"product_id"`
	Product       *Product  `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	OperatorID    *uint     `gorm:"index;comment:员工操作人ID" json:"operator_id,omitempty"`
	OperatorName  string    `gorm:"type:varchar(100);not null;default:'';comment:操作人姓名快照" json:"operator_name"`
	WarehouseType int       `gorm:"type:tinyint;not null;comment:操作仓库：1-主仓, 2-门店, 3-云仓, 4-样机" json:"warehouse_type"`
	LogType       string    `gorm:"type:varchar(20);not null;comment:记录类型：purchase(入库), sale(销售), transfer(调拨), stocktake(盘点), cancel(取消退库)" json:"log_type"`
	BeforeQty     int       `gorm:"not null;comment:变动前库存快照" json:"before_qty"`
	ChangeQty     int       `gorm:"not null;comment:变动数量(正数增加，负数扣减)" json:"change_qty"`
	AfterQty      int       `gorm:"not null;comment:变动后库存快照" json:"after_qty"`
	RelatedNo     string    `gorm:"type:varchar(100);comment:关联业务单号" json:"related_no"`
	UnitCost      float64   `gorm:"type:decimal(10,2);not null;default:0;comment:单价快照" json:"unit_cost"`
	Remark        string    `gorm:"type:varchar(255);comment:操作备注" json:"remark"`
	CreatedAt     time.Time `json:"created_at"`
}

// HistoricalCostRepair 历史零成本修复审计记录；修复流水或商品成本快照，不调整库存数量或账户余额。
type HistoricalCostRepair struct {
	ID                 uint      `gorm:"primaryKey;comment:主键ID" json:"id"`
	InventoryLogID     *uint     `gorm:"index;comment:被修复的历史库存流水；直接修复商品档案时为空" json:"inventory_log_id,omitempty"`
	OrderItemID        *uint     `gorm:"index;comment:被逐笔修复的订单明细" json:"order_item_id,omitempty"`
	ProductID          uint      `gorm:"not null;index;comment:商品ID" json:"product_id"`
	OrderNo            string    `gorm:"type:varchar(100);not null;default:'';comment:订单成本修复对应的订单号" json:"order_no,omitempty"`
	UnitCost           float64   `gorm:"type:decimal(10,2);not null;comment:补录单价" json:"unit_cost"`
	PreviousUnitCost   float64   `gorm:"type:decimal(10,2);not null;default:0;comment:校准前平均成本" json:"previous_unit_cost"`
	StockQty           int       `gorm:"not null;default:0;comment:成本校准时的总库存基准" json:"stock_qty"`
	CostBaseline       bool      `gorm:"not null;default:false;comment:是否为成本重算基准事件" json:"cost_baseline"`
	RepairedOrderCount int64     `gorm:"not null;default:0;comment:回写订单成本数" json:"repaired_order_count"`
	RepairedSaleCount  int64     `gorm:"not null;default:0;comment:回写销售流水数" json:"repaired_sale_count"`
	Remark             string    `gorm:"type:varchar(255);comment:修复说明" json:"remark"`
	CreatedAt          time.Time `json:"created_at"`
}

// SubsidySettlement 国补回款记录表
type SubsidySettlement struct {
	ID        uint      `gorm:"primaryKey;comment:主键ID" json:"id"`
	Amount    float64   `gorm:"type:decimal(12,2);not null;comment:回款金额" json:"amount"`
	AccountID uint      `gorm:"not null;index;comment:收款账户ID" json:"account_id"`
	Account   *Account  `gorm:"foreignKey:AccountID" json:"account,omitempty"`
	Remark    string    `gorm:"type:varchar(255);comment:备注说明" json:"remark"`
	CreatedAt time.Time `json:"created_at"`
}
