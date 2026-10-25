package models

import "time"

// PayrollConfirmation 工资发放确认记录：薪资结算抽屉里"确认本月已发放"的台账。
// 唯一索引 uk_employee_month(employee_id, month) 天然防止同月同员工重复确认。
// 确认时同步在经营开销账户（accounts.id=7）记一笔"工资"支出流水（FinancialLog），
// 不动任何真实资金账户（支付宝/微信/公户/现金）。
type PayrollConfirmation struct {
	ID         uint      `gorm:"primaryKey;comment:主键ID" json:"id"`
	EmployeeID uint      `gorm:"not null;uniqueIndex:uk_employee_month,priority:1;index;comment:员工ID" json:"employee_id"`
	Month      string    `gorm:"type:char(7);not null;uniqueIndex:uk_employee_month,priority:2;comment:工资月份 YYYY-MM" json:"month"`
	Amount     float64   `gorm:"type:decimal(10,2);not null;comment:实发工资" json:"amount"`
	CreatedBy  string    `gorm:"type:varchar(100);not null;default:'';comment:确认操作人" json:"created_by"`
	CreatedAt  time.Time `json:"created_at"`
}
