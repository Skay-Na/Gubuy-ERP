package models

// SystemSetting 系统级开关/配置项（key-value）。
// 读取对所有已登录身份（管理员 + 员工）开放，修改仅限管理员。
type SystemSetting struct {
	ID        uint   `gorm:"primaryKey" json:"id"`
	SettingKey string `gorm:"uniqueIndex;size:64;not null;comment:设置键" json:"setting_key"`
	Value     string `gorm:"size:255;not null;comment:设置值" json:"value"`
	Remark    string `gorm:"size:255;comment:说明" json:"remark"`
}

// 系统设置键名常量
const (
	// SettingOrderBackdateEnabled 允许员工端开单时补录开单时间（漏录单场景）。
	// true：员工端结算面板出现时间选择，可把订单归属到过去时间；false/缺省：员工端只能开当天。
	SettingOrderBackdateEnabled = "order_backdate_enabled"
)

// IsTruthyValue 判断设置值是否为开启态。
func IsTruthyValue(v string) bool {
	return v == "true" || v == "1"
}
