package controllers

import (
	"encoding/json"
	"erp-backend/config"
	"erp-backend/models"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func TestCountsTowardEmployeePerformance(t *testing.T) {
	tests := []struct {
		name         string
		referrerName string
		want         bool
	}{
		{name: "normal employee order", referrerName: "", want: true},
		{name: "third party referral", referrerName: "张三", want: true},
		{name: "boss customer", referrerName: "老板客户", want: false},
		{name: "boss keyword variant", referrerName: "老板带单", want: false},
		{name: "boss customer suffix", referrerName: "老板客户-老张", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := CountsTowardEmployeePerformance(tt.referrerName); got != tt.want {
				t.Fatalf("CountsTowardEmployeePerformance(%q) = %v, want %v", tt.referrerName, got, tt.want)
			}
		})
	}
}

func TestAddOrderToCommissionReport(t *testing.T) {
	employeeID := uint(7)
	tests := []struct {
		name           string
		order          models.Order
		wantSales      float64
		wantCommission float64
	}{
		{
			name: "normal order counts sales and commission",
			order: models.Order{
				OrderType:        1,
				EmployeeID:       &employeeID,
				CommissionAmount: 10,
				OrderItems:       []models.OrderItem{{UnitPrice: 1000, Quantity: 1}},
			},
			wantSales:      1000,
			wantCommission: 10,
		},
		{
			name: "chamber order is handled by the shared pool",
			order: models.Order{
				OrderType:        2,
				EmployeeID:       &employeeID,
				CommissionAmount: 20,
				OrderItems:       []models.OrderItem{{UnitPrice: 1000, Quantity: 1}},
			},
		},
		{
			name: "boss customer is excluded",
			order: models.Order{
				OrderType:        1,
				EmployeeID:       &employeeID,
				ReferrerName:     "老板客户",
				CommissionAmount: 20,
				OrderItems:       []models.OrderItem{{UnitPrice: 1000, Quantity: 1}},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			report := &CommissionReportItem{}
			addOrderToCommissionReport(tt.order, map[uint]*CommissionReportItem{employeeID: report})
			if report.TotalSales != tt.wantSales || report.TotalCommission != tt.wantCommission {
				t.Fatalf("sales = %.2f, commission = %.2f; want sales = %.2f, commission = %.2f",
					report.TotalSales, report.TotalCommission, tt.wantSales, tt.wantCommission)
			}
		})
	}
}

func TestCommissionProductModels(t *testing.T) {
	order := models.Order{OrderItems: []models.OrderItem{
		{Product: &models.Product{Name: "空调 KFR-35"}},
		{Product: &models.Product{Name: "空调 KFR-35"}},
		{Product: &models.Product{Name: "冰箱 BCD-500"}},
		{Product: nil},
	}}

	got := commissionProductModels(order)
	want := []string{"空调 KFR-35", "冰箱 BCD-500"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("commissionProductModels() = %v, want %v", got, want)
	}
}

func TestChamberCommissionPoolCents(t *testing.T) {
	tests := []struct {
		name  string
		order models.Order
		want  int64
	}{
		{
			name:  "uses persisted pool for new orders",
			order: models.Order{TotalAmount: 1000, CommissionAmount: 6},
			want:  600,
		},
		{
			name:  "falls back for legacy orders",
			order: models.Order{TotalAmount: 1000},
			want:  1000,
		},
		{
			name: "keeps intentional zero for excluded products",
			order: models.Order{
				TotalAmount: 1000,
				OrderItems:  []models.OrderItem{{NoCommission: true}},
			},
			want: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := chamberCommissionPoolCents(tt.order); got != tt.want {
				t.Fatalf("chamberCommissionPoolCents() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestAddChamberOrderToCommissionReport(t *testing.T) {
	orderDate := time.Date(2026, 9, 10, 12, 0, 0, 0, time.Local)
	employees := []models.Employee{
		{ID: 1, Name: "甲", Role: "sales", CreatedAt: orderDate.AddDate(0, 0, -30)},
		{ID: 2, Name: "乙", Role: "sales", CreatedAt: orderDate.AddDate(0, 0, -30)},
		{ID: 3, Name: "丙", Role: "sales", CreatedAt: orderDate.AddDate(0, 0, -30), DeletedAt: gorm.DeletedAt{Time: orderDate.AddDate(0, 0, 1), Valid: true}},
		{ID: 4, Name: "外包", IsOutsourced: true, CreatedAt: orderDate.AddDate(0, 0, -30)},
		{ID: 5, Name: "管理员", Role: "admin", CreatedAt: orderDate.AddDate(0, 0, -30)},
		{ID: 6, Name: "老板", CreatedAt: orderDate.AddDate(0, 0, -30)},
		{ID: 7, Name: "尚未入职", CreatedAt: orderDate.AddDate(0, 0, -30), EntryDate: timePointer(orderDate.AddDate(0, 0, 1))},
		{ID: 8, Name: "已经离职", CreatedAt: orderDate.AddDate(0, 0, -30), DeletedAt: gorm.DeletedAt{Time: orderDate.AddDate(0, 0, -1), Valid: true}},
		{ID: 9, Name: "后加入", CreatedAt: orderDate.AddDate(0, 0, 1)},
	}
	reports := make(map[uint]*CommissionReportItem)
	for _, employee := range employees {
		reports[employee.ID] = &CommissionReportItem{}
	}

	order := models.Order{
		ID: 42, OrderNo: "SH-42", OrderType: 2, TotalAmount: 100.01,
		CommissionAmount: 0, CreatedAt: orderDate,
	}
	addChamberOrderToCommissionReport(order, employees, reports)

	want := map[uint]float64{1: 0.34, 2: 0.33, 3: 0.33}
	var totalCents int64
	for _, employee := range employees {
		report := reports[employee.ID]
		if report.ChamberCommission != want[employee.ID] || report.TotalCommission != want[employee.ID] {
			t.Errorf("employee %d received %.2f, want %.2f", employee.ID, report.ChamberCommission, want[employee.ID])
		}
		if employee.ID <= 3 {
			if len(report.CommissionDetails) != 1 || report.CommissionDetails[0].ParticipantCount != 3 || report.CommissionDetails[0].CommissionPool != 1 {
				t.Errorf("employee %d has incorrect chamber detail: %+v", employee.ID, report.CommissionDetails)
			}
		} else if len(report.CommissionDetails) != 0 {
			t.Errorf("ineligible employee %d received order detail", employee.ID)
		}
		totalCents += moneyCents(report.ChamberCommission)
	}
	if totalCents != chamberCommissionCents(order.TotalAmount) {
		t.Fatalf("allocated %d cents, want pool %d", totalCents, chamberCommissionCents(order.TotalAmount))
	}
}

func timePointer(value time.Time) *time.Time { return &value }

// getCommissionReportForMonth 通过 GET /api/employees/commission-report 拉取指定月报表。
func getCommissionReportForMonth(t *testing.T, month string) []CommissionReportItem {
	t.Helper()
	response := httptest.NewRecorder()
	context, _ := gin.CreateTestContext(response)
	context.Request = httptest.NewRequest(http.MethodGet, "/api/employees/commission-report?month="+month, nil)
	GetCommissionReport(context)
	if response.Code != http.StatusOK {
		t.Fatalf("report status = %d, body = %s", response.Code, response.Body.String())
	}
	var body struct {
		Data []CommissionReportItem `json:"data"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode report: %v", err)
	}
	return body.Data
}

// TestGetCommissionReportIncludesChamberOrderWithoutEmployee 商会订单分成起计口径：
// 2026-10 之前的商会订单不再计入提成（chamberCommissionStart，用户 2026-10-26 定稿），
// 2026-10 起的商会订单仍在在职正式员工间平分，且已删除（离职）员工不进报表。
func TestGetCommissionReportIncludesChamberOrderWithoutEmployee(t *testing.T) {
	setupAudit(t)
	// 报表会按"当月在职正式员工"分摊商会池，先清掉历史用例残留员工保证分母确定。
	if err := config.DB.Unscoped().Where("is_outsourced = ?", false).Delete(&models.Employee{}).Error; err != nil {
		t.Fatalf("清理残留员工失败: %v", err)
	}

	entryDate := time.Date(2000, 1, 1, 0, 0, 0, 0, time.Local)
	employees := []models.Employee{
		{Name: "商会测试员工甲", EmpNo: "CHAMBER-A-" + strconv.FormatInt(time.Now().UnixNano(), 10), Role: "sales", EntryDate: &entryDate, BaseSalary: 1000},
		{Name: "商会测试员工乙", EmpNo: "CHAMBER-B-" + strconv.FormatInt(time.Now().UnixNano(), 10), Role: "sales", EntryDate: &entryDate, BaseSalary: 1000},
		{Name: "已离职测试员工", EmpNo: "CHAMBER-DELETED-" + strconv.FormatInt(time.Now().UnixNano(), 10), Role: "sales", EntryDate: &entryDate, BaseSalary: 1000},
	}
	for index := range employees {
		if err := config.DB.Create(&employees[index]).Error; err != nil {
			t.Fatalf("create employee: %v", err)
		}
	}
	suffix := strconv.FormatInt(time.Now().UnixNano(), 10)
	legacyOrder := models.Order{
		OrderNo:   "PAYROLL-CHAMBER-OLD-" + suffix,
		OrderType: 2, OrderStatus: 1, PaymentStatus: 2,
		TotalAmount: 100, ActualPayAmount: 100,
		CreatedAt: time.Date(2000, 1, 10, 12, 0, 0, 0, time.Local),
	}
	if err := config.DB.Create(&legacyOrder).Error; err != nil {
		t.Fatalf("create legacy order: %v", err)
	}
	activeOrder := models.Order{
		OrderNo:   "PAYROLL-CHAMBER-NEW-" + suffix,
		OrderType: 2, OrderStatus: 1, PaymentStatus: 2,
		TotalAmount: 100, ActualPayAmount: 100,
		CreatedAt: time.Date(2026, 10, 10, 12, 0, 0, 0, time.Local),
	}
	if err := config.DB.Create(&activeOrder).Error; err != nil {
		t.Fatalf("create active order: %v", err)
	}
	if err := config.DB.Delete(&employees[2]).Error; err != nil {
		t.Fatalf("delete former employee: %v", err)
	}
	t.Cleanup(func() {
		config.DB.Unscoped().Delete(&legacyOrder)
		config.DB.Unscoped().Delete(&activeOrder)
		for _, employee := range employees {
			config.DB.Unscoped().Delete(&employee)
		}
	})

	// 2000-01 的商会订单早于 chamberCommissionStart：不计任何分成。
	for _, row := range getCommissionReportForMonth(t, "2000-01") {
		if row.ChamberCommission != 0 || row.TotalCommission != 0 || len(row.CommissionDetails) != 0 {
			t.Errorf("legacy chamber order must not pay commission: %+v", row)
		}
	}

	// 2026-10 起的商会订单：在职两名各分 0.33/0.34（池 100×1%=1 元）。
	// 已删除员工下单日仍在职 → 参与分摊但不进报表，与既有口径一致。
	deletedSeen := false
	for _, row := range getCommissionReportForMonth(t, "2026-10") {
		if row.EmployeeID == employees[2].ID {
			deletedSeen = true
			continue
		}
		if row.ChamberCommission != 0.33 && row.ChamberCommission != 0.34 ||
			row.TotalCommission != row.ChamberCommission ||
			len(row.CommissionDetails) != 1 || row.CommissionDetails[0].OrderNo != activeOrder.OrderNo {
			t.Errorf("employee %d has incorrect chamber commission: %+v", row.EmployeeID, row)
		}
	}
	if deletedSeen {
		t.Fatalf("deleted employee %d must not appear in payroll", employees[2].ID)
	}
}
