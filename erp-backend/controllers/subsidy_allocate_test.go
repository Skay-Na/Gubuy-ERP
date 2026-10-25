package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"erp-backend/config"
	"erp-backend/middlewares"
	"erp-backend/models"

	"github.com/gin-gonic/gin"
)

// allocateRouter 独立测试路由：挂上 OperationHistoryMiddleware，
// 让核销动作真实走一遍操作历史 capture，供撤销用例复用。
var (
	allocateRouterOnce sync.Once
	allocateRouter     *gin.Engine
)

func setupSubsidyAllocateTest(t *testing.T) {
	t.Helper()
	setupAudit(t)
	allocateRouterOnce.Do(func() {
		gin.SetMode(gin.TestMode)
		allocateRouter = gin.New()
		allocateRouter.POST("/api/subsidy/allocate", middlewares.OperationHistoryMiddleware(), AllocateSubsidyReceipt)
	})
}

// seedAllocateFixture 直建一笔国补订单 + 一笔回款池，返回 (orderID, settlementID)。
func seedAllocateFixture(t *testing.T, pending float64, pool float64) (uint, uint) {
	t.Helper()
	orderNo := time.Now().Format("20060102150405") + fmt.Sprintf("%06d", time.Now().UnixNano()%1000000)
	order := models.Order{
		OrderNo:             orderNo,
		OrderStatus:         1,
		TotalAmount:         10000,
		SubsidyAmount:       pending,
		SubsidyPendingAmount: pending,
		SubsidyStatus:       2, // 已提交资料，等待回款
		PaymentStatus:       2,
	}
	if err := config.DB.Create(&order).Error; err != nil {
		t.Fatalf("创建测试订单失败: %v", err)
	}
	account := models.Account{Name: "国补核销测试账户"}
	if err := config.DB.Where("name = ?", account.Name).First(&account).Error; err != nil {
		if err := config.DB.Create(&account).Error; err != nil {
			t.Fatalf("创建测试账户失败: %v", err)
		}
	}
	settlement := models.SubsidySettlement{Amount: pool, AccountID: account.ID}
	if err := config.DB.Create(&settlement).Error; err != nil {
		t.Fatalf("创建测试回款失败: %v", err)
	}
	return order.ID, settlement.ID
}

func postAllocate(body gin.H) *httptest.ResponseRecorder {
	jsonValue, _ := json.Marshal(body)
	req, _ := http.NewRequest("POST", "/api/subsidy/allocate", bytes.NewBuffer(jsonValue))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	allocateRouter.ServeHTTP(w, req)
	return w
}

// readAllocateOrder 读取订单的核销相关字段
func readAllocateOrder(t *testing.T, id uint) models.Order {
	t.Helper()
	var order models.Order
	if err := config.DB.First(&order, id).Error; err != nil {
		t.Fatalf("读取订单失败: %v", err)
	}
	return order
}

// TestAllocateSubsidyReceiptSuccess 正常核销：收满自动置 subsidy_status=3。
func TestAllocateSubsidyReceiptSuccess(t *testing.T) {
	setupSubsidyAllocateTest(t)
	orderID, settlementID := seedAllocateFixture(t, 100, 100)

	w := postAllocate(gin.H{
		"settlement_id": settlementID,
		"allocations":   []gin.H{{"order_id": orderID, "amount": 100}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("核销应成功: %d %s", w.Code, w.Body.String())
	}

	order := readAllocateOrder(t, orderID)
	if order.SubsidyReceivedAmt != 100 {
		t.Fatalf("已回款金额应为 100，实际 %.2f", order.SubsidyReceivedAmt)
	}
	if order.SubsidyStatus != 3 {
		t.Fatalf("收满后状态应为 3（已回款），实际 %d", order.SubsidyStatus)
	}
	var count int64
	config.DB.Model(&models.SubsidyReceipt{}).Where("order_id = ? AND settlement_id = ?", orderID, settlementID).Count(&count)
	if count != 1 {
		t.Fatalf("应写入 1 条核销记录，实际 %d", count)
	}
	var receipt models.SubsidyReceipt
	config.DB.Where("order_id = ?", orderID).First(&receipt)
	if receipt.RemainingBefore != 100 {
		t.Fatalf("核销前待回款快照应为 100，实际 %.2f", receipt.RemainingBefore)
	}
}

// TestAllocateSubsidyReceiptRejectsOverallocate 超额拒绝：核销额 > 订单剩余待回款。
func TestAllocateSubsidyReceiptRejectsOverallocate(t *testing.T) {
	setupSubsidyAllocateTest(t)
	orderID, settlementID := seedAllocateFixture(t, 100, 200)

	w := postAllocate(gin.H{
		"settlement_id": settlementID,
		"allocations":   []gin.H{{"order_id": orderID, "amount": 100.01}},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("超额核销应返回 400: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Msg string `json:"msg"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !strings.Contains(resp.Msg, "超额") {
		t.Fatalf("错误信息应说明核销超额，实际: %s", resp.Msg)
	}

	order := readAllocateOrder(t, orderID)
	if order.SubsidyReceivedAmt != 0 || order.SubsidyStatus != 2 {
		t.Fatalf("被拒绝后订单状态不应变化: received=%.2f status=%d", order.SubsidyReceivedAmt, order.SubsidyStatus)
	}
}

// TestAllocateSubsidyReceiptRejectsPoolShortage 池余额不足拒绝。
func TestAllocateSubsidyReceiptRejectsPoolShortage(t *testing.T) {
	setupSubsidyAllocateTest(t)
	orderID, settlementID := seedAllocateFixture(t, 100, 50)

	w := postAllocate(gin.H{
		"settlement_id": settlementID,
		"allocations":   []gin.H{{"order_id": orderID, "amount": 60}},
	})
	if w.Code != http.StatusBadRequest {
		t.Fatalf("池余额不足应返回 400: %d %s", w.Code, w.Body.String())
	}
	var resp struct {
		Msg string `json:"msg"`
	}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if !strings.Contains(resp.Msg, "不足") {
		t.Fatalf("错误信息应说明池余额不足，实际: %s", resp.Msg)
	}
	if got := readAllocateOrder(t, orderID); got.SubsidyReceivedAmt != 0 {
		t.Fatalf("被拒绝后订单不应入账，实际 %.2f", got.SubsidyReceivedAmt)
	}
}

// TestAllocateSubsidyReceiptUndoRestoresOrder 核销后撤销：回退 received/status 并删核销记录。
func TestAllocateSubsidyReceiptUndoRestoresOrder(t *testing.T) {
	setupSubsidyAllocateTest(t)
	orderID, settlementID := seedAllocateFixture(t, 100, 100)

	w := postAllocate(gin.H{
		"settlement_id": settlementID,
		"allocations":   []gin.H{{"order_id": orderID, "amount": 100}},
	})
	if w.Code != http.StatusOK {
		t.Fatalf("前置：核销应成功: %d %s", w.Code, w.Body.String())
	}
	if got := readAllocateOrder(t, orderID); got.SubsidyStatus != 3 {
		t.Fatalf("前置：核销后状态应为 3，实际 %d", got.SubsidyStatus)
	}

	// 找到本次核销产生的操作历史（标题：核销国补回款）
	var record models.OperationHistory
	if err := config.DB.Where("title = ?", "核销国补回款").Order("id desc").First(&record).Error; err != nil {
		t.Fatalf("应生成核销操作历史: %v", err)
	}
	if record.UndoneAt != nil {
		t.Fatal("新产生的核销历史不应已处于撤销状态")
	}

	// 通过通用撤销接口回退（同事务恢复 orders 行并删除 subsidy_receipts 行）
	undoW := httptest.NewRecorder()
	undoCtx, _ := gin.CreateTestContext(undoW)
	undoCtx.Request = httptest.NewRequest("POST", "/api/operations/"+itoa(record.ID)+"/undo", nil)
	undoCtx.Params = gin.Params{{Key: "id", Value: itoa(record.ID)}}
	UndoOperation(undoCtx)
	if undoW.Code != http.StatusOK {
		t.Fatalf("撤销应成功: %d %s", undoW.Code, undoW.Body.String())
	}

	order := readAllocateOrder(t, orderID)
	if order.SubsidyReceivedAmt != 0 {
		t.Fatalf("撤销后已回款金额应回退为 0，实际 %.2f", order.SubsidyReceivedAmt)
	}
	if order.SubsidyStatus != 2 {
		t.Fatalf("撤销后国补状态应回退为 2，实际 %d", order.SubsidyStatus)
	}
	var count int64
	config.DB.Unscoped().Model(&models.SubsidyReceipt{}).Where("order_id = ?", orderID).Count(&count)
	if count != 0 {
		t.Fatalf("撤销后核销记录应被删除，实际剩余 %d 条", count)
	}
}
