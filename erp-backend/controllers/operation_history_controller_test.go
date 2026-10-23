package controllers

import (
	"strings"
	"testing"
	"time"

	"erp-backend/models"
)

func TestDescribeOperationHistory(t *testing.T) {
	tests := []struct {
		name              string
		record            models.OperationHistory
		employeesByID     map[uint]string
		employeesByNumber map[string]string
		want              []string
		wantNot           []string
	}{
		{
			name: "attendance check-in uses employee name and clock time",
			record: models.OperationHistory{
				Username:  "emp_003",
				Title:     "员工签到",
				CreatedAt: time.Date(2026, 10, 23, 8, 50, 0, 0, time.Local),
				Changes:   `[{"table":"daily_attendances","id":1,"after":{"id":1,"employee_id":7,"date":"2026-10-23","check_in":"2026-10-23T08:50:00+08:00"}}]`,
			},
			employeesByID:     map[uint]string{7: "张三"},
			employeesByNumber: map[string]string{"003": "张三"},
			want:              []string{"张三", "2026年10月23日 08:50", "上班签到"},
			wantNot:           []string{"emp_003", "daily_attendances", "check_in"},
		},
		{
			name: "new order includes customer due and purchased items",
			record: models.OperationHistory{
				Username:  "admin",
				Title:     "创建销售订单",
				CreatedAt: time.Date(2026, 10, 23, 9, 0, 0, 0, time.Local),
				Changes:   `[{"table":"orders","id":9,"after":{"id":9,"order_no":"D202610230001","customer_name":"陈先生","actual_pay_amount":2380,"deposit_amount":500}},{"table":"order_items","id":2,"after":{"id":2,"product_id":11,"quantity":1,"is_gift":false}},{"table":"products","id":11,"after":{"id":11,"name":"冰箱","store_stock":4}}]`,
			},
			want:    []string{"订单 D202610230001", "客户 陈先生", "客户应付 ¥2380.00", "商品冰箱 ×1"},
			wantNot: []string{"order_items", "product_id"},
		},
		{
			name: "stock change shows product and warehouse counts",
			record: models.OperationHistory{
				Username:  "admin",
				Title:     "商品库存调拨",
				CreatedAt: time.Date(2026, 10, 23, 10, 0, 0, 0, time.Local),
				Changes:   `[{"table":"products","id":11,"before":{"id":11,"name":"冰箱","store_stock":5,"cloud_stock":1},"after":{"id":11,"name":"冰箱","store_stock":3,"cloud_stock":3}}]`,
			},
			want:    []string{"商品「冰箱」", "门店库存 5→3", "云仓库存 1→3"},
			wantNot: []string{"products", "store_stock"},
		},
		{
			name: "item delivery update uses familiar status labels",
			record: models.OperationHistory{
				Username:  "admin",
				Title:     "更新商品配送状态",
				CreatedAt: time.Date(2026, 10, 23, 10, 30, 0, 0, time.Local),
				Changes:   `[{"table":"order_items","id":2,"before":{"id":2,"product_id":11,"is_delivered":false},"after":{"id":2,"product_id":11,"is_delivered":true}},{"table":"products","id":11,"before":{"id":11,"name":"冰箱","store_stock":3},"after":{"id":11,"name":"冰箱","store_stock":3}}]`,
			},
			want:    []string{"商品「冰箱」", "配送状态 待配送→已配送"},
			wantNot: []string{"is_delivered", "product_id"},
		},
		{
			name: "expense shows category amount and note",
			record: models.OperationHistory{
				Username:  "admin",
				Title:     "新增支出记录",
				CreatedAt: time.Date(2026, 10, 23, 11, 0, 0, 0, time.Local),
				Changes:   `[{"table":"expenses","id":8,"after":{"id":8,"expense_type":"房租","amount":3000,"remark":"十月门店租金"}}]`,
			},
			want:    []string{"房租 ¥3000.00", "备注：十月门店租金"},
			wantNot: []string{"expenses", "amount"},
		},
		{
			name: "offboarding detail distinguishes payable snapshot from payment",
			record: models.OperationHistory{
				Username:  "admin",
				Title:     "发起员工离职交接",
				CreatedAt: time.Date(2026, 10, 23, 12, 0, 0, 0, time.Local),
				Changes:   `[{"table":"employee_offboardings","id":3,"after":{"id":3,"employee_name":"李四","emp_no":"E003","last_working_date":"2026-10-31T00:00:00+08:00","final_payable_snapshot":4800,"status":"in_progress"}}]`,
			},
			want:    []string{"李四", "最后工作日 2026年10月31日", "应结算金额快照 ¥4800.00"},
			wantNot: []string{"已支付", "employee_offboardings"},
		},
	}

	for _, testCase := range tests {
		t.Run(testCase.name, func(t *testing.T) {
			got := describeOperationHistory(testCase.record, testCase.employeesByID, testCase.employeesByNumber)
			for _, fragment := range testCase.want {
				if !strings.Contains(got, fragment) {
					t.Errorf("detail %q does not contain %q", got, fragment)
				}
			}
			for _, fragment := range testCase.wantNot {
				if strings.Contains(got, fragment) {
					t.Errorf("detail %q unexpectedly contains %q", got, fragment)
				}
			}
		})
	}
}
