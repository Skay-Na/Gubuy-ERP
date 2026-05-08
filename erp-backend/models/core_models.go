package models

import (
	"time"

	"gorm.io/gorm"
)

// Account 资金账户表
type Account struct {
	ID        uint           `gorm:"primaryKey;comment:主键ID" json:"id"`
	Name      string         `gorm:"type:varchar(100);not null;uniqueIndex;comment:账户名称（支付宝/微信/公户）" json:"name"`
	Balance   float64        `gorm:"type:decimal(12,2);not null;default:0;comment:当前余额" json:"balance"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"` // 软删除保护，防止误删导致历史账单断层
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
	ID            uint      `gorm:"primaryKey;comment:主键ID" json:"id"`
	FromAccountID uint      `gorm:"not null;comment:转出账户ID" json:"from_account_id"`
	ToAccountID   uint      `gorm:"not null;comment:转入账户ID" json:"to_account_id"`
	Amount        float64   `gorm:"type:decimal(12,2);not null;comment:划拨金额" json:"amount"`
	Remark        string    `gorm:"type:text;comment:备注说明" json:"remark"`
	FromAccount   *Account  `gorm:"foreignKey:FromAccountID" json:"from_account,omitempty"`
	ToAccount     *Account  `gorm:"foreignKey:ToAccountID" json:"to_account,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// Category 商品分类表
type Category struct {
	ID        uint           `gorm:"primaryKey;comment:主键ID" json:"id"`
	Name      string         `gorm:"type:varchar(100);not null;uniqueIndex;comment:分类名称" json:"name"`
	SortOrder int            `gorm:"type:int;default:0;comment:排序权重" json:"sort_order"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

// Product 商品表
type Product struct {
	ID           uint           `gorm:"primaryKey;comment:主键ID" json:"id"`
	Name         string         `gorm:"type:varchar(255);not null;comment:商品名称" json:"name"`
	SKU          string         `gorm:"type:varchar(100);uniqueIndex;not null;comment:唯一编码" json:"sku"`
	LatestCost   float64        `gorm:"type:decimal(10,2);not null;default:0;comment:最新进价" json:"latest_cost"`
	MarginRate   float64        `gorm:"type:decimal(5,2);not null;default:0.1;comment:固定毛利率" json:"margin_rate"`
	MainStock    int            `gorm:"type:int;not null;default:0;comment:主仓库存" json:"main_stock"`
	StoreStock   int            `gorm:"type:int;not null;default:0;comment:门店可用库存" json:"store_stock"`
	SampleStock  int            `gorm:"type:int;not null;default:0;comment:门店样机数量" json:"sample_stock"`
	CloudStock   int            `gorm:"type:int;not null;default:0;comment:云仓自有库存" json:"cloud_stock"`
	SupportCloud bool           `gorm:"not null;default:false;comment:是否支持云仓代发" json:"support_cloud"`
	CategoryID   uint           `gorm:"index;comment:所属分类ID" json:"category_id"`
	Category     *Category      `gorm:"foreignKey:CategoryID" json:"category,omitempty"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

// Employee 员工表
type Employee struct {
	ID             uint           `gorm:"primaryKey;comment:主键ID" json:"id"`
	Name           string         `gorm:"type:varchar(100);not null;comment:员工姓名" json:"name"`
	Phone          string         `gorm:"type:varchar(20);comment:联系电话" json:"phone"`
	EmpNo          string         `gorm:"type:varchar(50);uniqueIndex;not null;comment:工号" json:"emp_no"`
	BaseSalary     float64        `gorm:"type:decimal(10,2);not null;default:0;comment:基本底薪" json:"base_salary"`
	CommissionRate float64        `gorm:"type:decimal(5,4);not null;default:0;comment:提成比例" json:"commission_rate"`
	PinCode        string         `gorm:"type:varchar(10);comment:终端登录PIN码" json:"pin_code"`
	EntryDate      *time.Time     `gorm:"type:date;comment:入职时间" json:"entry_date"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
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

// Order 销售订单主表
type Order struct {
	ID              uint    `gorm:"primaryKey;comment:主键ID" json:"id"`
	OrderNo         string  `gorm:"type:varchar(50);uniqueIndex;not null;comment:唯一订单号" json:"order_no"`
	CustomerName    string  `gorm:"type:varchar(100);comment:客户姓名" json:"customer_name"`
	CustomerPhone   string  `gorm:"type:varchar(20);comment:客户手机号" json:"customer_phone"`
	CustomerAddress string  `gorm:"type:varchar(255);comment:客户详细地址" json:"customer_address"`
	TotalAmount     float64 `gorm:"type:decimal(10,2);not null;comment:商品总金额" json:"total_amount"`
	SubsidyAmount   float64 `gorm:"type:decimal(10,2);not null;default:0;comment:国补金额" json:"subsidy_amount"`
	ActualPayAmount float64 `gorm:"type:decimal(10,2);not null;comment:客户实际应付" json:"actual_pay_amount"`
	DepositAmount   float64 `gorm:"type:decimal(10,2);not null;default:0;comment:已付定金" json:"deposit_amount"`
	PaymentStatus   int     `gorm:"type:tinyint;not null;default:1;comment:支付状态：1-仅付定金, 2-已结全款" json:"payment_status"`
	SubsidyStatus   int     `gorm:"type:tinyint;not null;default:0;comment:国补资料状态：0-无国补, 1-待提交, 2-已提交, 3-已回款" json:"subsidy_status"`
	// 新增：安装相关字段
	IsInstalled       bool       `gorm:"default:false;comment:是否已安装" json:"is_installed"`
	InstallerID       *uint      `gorm:"index;comment:安装人ID" json:"installer_id"`
	InstallTime       *time.Time `json:"install_time"`
	Installer         *Employee  `gorm:"foreignKey:InstallerID" json:"installer,omitempty"`
	DeliveryPersonID  *uint      `gorm:"index;comment:送货人ID" json:"delivery_person_id"`
	DeliveryPerson    *Employee  `gorm:"foreignKey:DeliveryPersonID" json:"delivery_person,omitempty"`
	// 新增：支付方式 (1-支付宝, 2-微信, 3-公户)
	PaymentMethod int `gorm:"type:tinyint;not null;default:1;comment:支付方式：1-支付宝, 2-微信, 3-公户" json:"payment_method"`
	// 新增：订单状态 (1-正常, 2-已取消)
	OrderStatus int `gorm:"type:tinyint;not null;default:1;comment:订单状态：1-正常, 2-已取消" json:"order_status"`
	// 新增：关联账户
	AccountID         *uint       `gorm:"index;comment:收款账户ID" json:"account_id"`
	Account           *Account    `gorm:"foreignKey:AccountID" json:"account,omitempty"`
	OrderItems        []OrderItem `gorm:"foreignKey:OrderID" json:"order_items"`
	EmployeeID        *uint       `gorm:"index;comment:开单员工ID" json:"employee_id"`
	Employee          *Employee   `gorm:"foreignKey:EmployeeID" json:"employee,omitempty"`
	ReferrerName      string      `gorm:"type:varchar(100);comment:推荐人姓名" json:"referrer_name"`
	ReferralFee       float64     `gorm:"type:decimal(10,2);not null;default:0;comment:第三方带单提成金额" json:"referral_fee"`
	IsReferralFeePaid bool        `gorm:"not null;default:false;comment:推荐提成是否已发" json:"is_referral_fee_paid"`
	// 新增：发货/提货方式 (1-门店自提, 2-主仓发货, 3-云仓代发)
	DeliveryMethod int            `gorm:"type:tinyint;not null;default:1;comment:发货方式：1-门店自提, 2-主仓发货, 3-云仓代发" json:"delivery_method"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
	DeletedAt      gorm.DeletedAt `gorm:"index" json:"-"`
}

// OrderItem 订单明细表
type OrderItem struct {
	ID                uint      `gorm:"primaryKey;comment:主键ID" json:"id"`
	OrderID           uint      `gorm:"not null;index;comment:关联 Order 的 ID" json:"order_id"`
	ProductID         uint      `gorm:"not null;index;comment:关联 Product 的 ID" json:"product_id"`
	UnitPrice         float64   `gorm:"type:decimal(10,2);not null;comment:实际成交单价" json:"unit_price"`
	UnitCost          float64   `gorm:"type:decimal(10,2);not null;default:0;comment:下单时进价快照" json:"unit_cost"`
	Quantity          int       `gorm:"type:int;not null;default:1;comment:购买数量" json:"quantity"`
	CloudDeductionQty int       `gorm:"type:int;not null;default:0;comment:从云仓自有库存扣除的数量" json:"cloud_deduction_qty"`
	IsGift            bool      `gorm:"type:tinyint(1);not null;default:0;comment:是否为赠品" json:"is_gift"`
	Product           *Product  `gorm:"foreignKey:ProductID" json:"product,omitempty"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

// Expense 门店日常支出表
type Expense struct {
	ID          uint    `gorm:"primaryKey;comment:主键ID" json:"id"`
	ExpenseType string  `gorm:"type:varchar(100);not null;comment:支出类型：房租/水电/送货费等" json:"expense_type"`
	Amount      float64 `gorm:"type:decimal(10,2);not null;comment:支出金额" json:"amount"`
	Remark      string  `gorm:"type:text;comment:备注说明" json:"remark"`
	// 新增：关联账户
	AccountID *uint     `gorm:"index;comment:支出账户ID" json:"account_id"`
	Account   *Account  `gorm:"foreignKey:AccountID" json:"account,omitempty"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

// FinancialLog 全口径财务流水表
type FinancialLog struct {
	ID           uint      `gorm:"primaryKey;comment:主键ID" json:"id"`
	AccountID    uint      `gorm:"not null;index;comment:关联资金账户ID" json:"account_id"`
	Account      *Account  `gorm:"foreignKey:AccountID" json:"account,omitempty"`
	FlowType     int       `gorm:"type:tinyint;not null;comment:资金流向" json:"flow_type"`
	Category     string    `gorm:"type:varchar(50);not null;comment:细分分类" json:"category"`
	Amount       float64   `gorm:"type:decimal(10,2);not null;comment:变动金额(绝对值)" json:"amount"`
	BalanceAfter float64   `gorm:"type:decimal(10,2);not null;comment:变动后该账户余额快照" json:"balance_after"`
	RelatedNo    string    `gorm:"type:varchar(100);comment:关联单号(订单号等)" json:"related_no"`
	Remark       string    `gorm:"type:varchar(255);comment:备注说明" json:"remark"`
	CreatedAt    time.Time `json:"created_at"`
}

// InboundRecord 入库记录表
type InboundRecord struct {
	ID        uint    `gorm:"primaryKey;comment:主键ID" json:"id"`
	ProductID uint    `gorm:"not null;index;comment:关联商品ID" json:"product_id"`
	Quantity  int     `gorm:"not null;comment:入库数量" json:"quantity"`
	UnitCost  float64 `gorm:"type:decimal(10,2);not null;comment:入库单价" json:"unit_cost"`
	TotalCost float64 `gorm:"type:decimal(10,2);not null;comment:本次入库总成本" json:"total_cost"`
	// 新增：关联账户
	AccountID *uint     `gorm:"index;comment:支出账户ID" json:"account_id"`
	Account   *Account  `gorm:"foreignKey:AccountID" json:"account,omitempty"`
	CreatedAt time.Time `json:"created_at"`
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
	WarehouseType int       `gorm:"type:tinyint;not null;comment:操作仓库：1-主仓, 2-门店, 3-云仓, 4-样机" json:"warehouse_type"`
	LogType       string    `gorm:"type:varchar(20);not null;comment:记录类型：purchase(入库), sale(销售), transfer(调拨), stocktake(盘点), cancel(取消退库)" json:"log_type"`
	BeforeQty     int       `gorm:"not null;comment:变动前库存快照" json:"before_qty"`
	ChangeQty     int       `gorm:"not null;comment:变动数量(正数增加，负数扣减)" json:"change_qty"`
	AfterQty      int       `gorm:"not null;comment:变动后库存快照" json:"after_qty"`
	RelatedNo     string    `gorm:"type:varchar(100);comment:关联业务单号" json:"related_no"`
	Remark        string    `gorm:"type:varchar(255);comment:操作备注" json:"remark"`
	CreatedAt     time.Time `json:"created_at"`
}
