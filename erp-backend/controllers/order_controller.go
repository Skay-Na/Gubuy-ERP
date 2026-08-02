package controllers

import (
	"bytes"
	"encoding/json"
	"erp-backend/config"
	"erp-backend/models"
	"erp-backend/utils"
	"errors"
	"fmt"
	"io"
	"math"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var errDailyOrderSequenceExhausted = errors.New("daily order sequence exhausted")

// GenerateOrderNo 生成订单号 (yyyymmdd + 4位当日顺序号)
func GenerateOrderNo(now time.Time, dailySeq int) string {
	return fmt.Sprintf("%s%04d", now.Format("20060102"), dailySeq)
}

// orderCostSnapshot returns the cost basis used for new sales. Average cost is
// the calibrated moving-cost value; legacy products without it retain their
// latest purchase-cost fallback.
func orderCostSnapshot(product models.Product) float64 {
	if product.AvgCost > 0 {
		return product.AvgCost
	}
	return product.LatestCost
}

func nextDailyOrderSequence(tx *gorm.DB, now time.Time) (int, error) {
	businessDate := now.Format("2006-01-02")

	result := tx.Exec(`
		INSERT INTO order_daily_counters (business_date, current_seq, created_at, updated_at)
		VALUES (?, 1, ?, ?)
		ON DUPLICATE KEY UPDATE
			current_seq = LAST_INSERT_ID(current_seq + 1),
			updated_at = VALUES(updated_at)
	`, businessDate, now, now)
	if result.Error != nil {
		return 0, result.Error
	}

	if result.RowsAffected == 1 {
		return 1, nil
	}

	var seq int
	if err := tx.Raw("SELECT LAST_INSERT_ID()").Scan(&seq).Error; err != nil {
		return 0, err
	}
	if seq > 9999 {
		return 0, errDailyOrderSequenceExhausted
	}
	return seq, nil
}

// OrderItemRequest 购物车单个商品项
type OrderItemRequest struct {
	ProductID uint    `json:"product_id" binding:"required"`
	Quantity  int     `json:"quantity" binding:"required,gt=0"`
	UnitPrice float64 `json:"unit_price"`
	IsGift    bool    `json:"is_gift"`
}

type PaymentSplitRequest struct {
	PaymentMethod int     `json:"payment_method" binding:"required,oneof=1 2 3 5"`
	AccountID     *uint   `json:"account_id"`
	Amount        float64 `json:"amount" binding:"required,gt=0"`
}

// paymentMethodName 返回支付方式编号对应的中文账户名（用于按名称查找账户）。
// 返回空字符串表示该编号未配置名称，将回退到按 ID 查找。
func paymentMethodName(method int) string {
	switch method {
	case 1:
		return "支付宝"
	case 2:
		return "微信"
	case 3:
		return "公户"
	case 5:
		return "现金"
	}
	return ""
}

type PaymentBreakdownItem struct {
	PaymentMethod int     `json:"payment_method"`
	AccountID     uint    `json:"account_id"`
	Amount        float64 `json:"amount"`
}

// CreateOrderRequest 提交开单请求体
type CreateOrderRequest struct {
	Items           []OrderItemRequest `json:"items" binding:"required,dive"`
	IsSubsidy       bool               `json:"is_subsidy"`
	DepositAmount   float64            `json:"deposit_amount"`
	CustomerName    string             `json:"customer_name"`
	CustomerPhone   string             `json:"customer_phone"`
	CustomerAddress string             `json:"customer_address"`
	EmployeeID      *uint              `json:"employee_id"`
	ReferrerName    string             `json:"referrer_name"`
	ReferralFee     float64            `json:"referral_fee"`
	NoCommission    bool               `json:"no_commission"`
	// 支付方式: 1-支付宝, 2-微信, 3-公户, 5-现金
	PaymentMethod int `json:"payment_method" binding:"required"`
	// 新增：收款账户ID（解耦支付方式与账户）
	AccountID *uint `json:"account_id"`
	// 混合支付拆分；为空时沿用 payment_method/account_id 单一支付
	PaymentSplits []PaymentSplitRequest `json:"payment_splits"`
	// 发货方式: 1-门店自提, 2-主仓发货, 3-云仓代发
	DeliveryMethod   int  `json:"delivery_method" binding:"required,oneof=1 2 3"`
	BypassPriceCheck bool `json:"bypass_price_check"`
	// 新增：以旧换新相关字段
	TradeInAmount  float64 `json:"trade_in_amount"`
	TradeInType    string  `json:"trade_in_type"`
	TradeInRemark  string  `json:"trade_in_remark"`
	DiscountAmount float64 `json:"discount_amount"` // 新增：手动抹零优惠
	// 新增：订单类型 (1-普通订单, 2-商会订单)
	OrderType int `json:"order_type"`
}

// orderDeliveryToWarehouse 将订单的发货方式 (delivery_method) 转换为库存流水的仓库类型
// (warehouse_type)。两者编号语义不同：delivery_method 中 1=门店自提, 2=主仓发货, 3=云仓代发；
// warehouse_type 中 1=主仓, 2=门店, 3=云仓, 4=样机。直接混用会导致所有订单/红冲库存流水被错误归类。
func orderDeliveryToWarehouse(deliveryMethod int) int {
	switch deliveryMethod {
	case 1:
		return 2
	case 2:
		return 1
	case 3:
		return 3
	}
	return deliveryMethod
}

func resolvePaymentAccount(tx *gorm.DB, paymentMethod int, accountID *uint) (models.Account, error) {
	var account models.Account
	var err error
	if accountID != nil {
		err = tx.Set("gorm:query_option", "FOR UPDATE").First(&account, *accountID).Error
		return account, err
	}

	var name string
	switch paymentMethod {
	case 1:
		name = "支付宝"
	case 2:
		name = "微信"
	case 3:
		name = "公户"
	case 5:
		name = "现金"
	}
	if name != "" {
		err = tx.Set("gorm:query_option", "FOR UPDATE").Where("name = ?", name).First(&account).Error
	}
	if err != nil || name == "" {
		err = tx.Set("gorm:query_option", "FOR UPDATE").First(&account, paymentMethod).Error
	}
	return account, err
}

func normalizePaymentSplits(tx *gorm.DB, req CreateOrderRequest, receiptAmount float64) ([]PaymentBreakdownItem, error) {
	if len(req.PaymentSplits) == 0 {
		if req.PaymentMethod == 4 {
			return nil, fmt.Errorf("请选择至少一个混合支付账户并填写金额")
		}
		account, err := resolvePaymentAccount(tx, req.PaymentMethod, req.AccountID)
		if err != nil {
			return nil, err
		}
		return []PaymentBreakdownItem{{
			PaymentMethod: req.PaymentMethod,
			AccountID:     account.ID,
			Amount:        receiptAmount,
		}}, nil
	}

	var total float64
	splits := make([]PaymentBreakdownItem, 0, len(req.PaymentSplits))
	for _, split := range req.PaymentSplits {
		account, err := resolvePaymentAccount(tx, split.PaymentMethod, split.AccountID)
		if err != nil {
			return nil, err
		}
		total += split.Amount
		splits = append(splits, PaymentBreakdownItem{
			PaymentMethod: split.PaymentMethod,
			AccountID:     account.ID,
			Amount:        split.Amount,
		})
	}
	if math.Abs(total-receiptAmount) > 0.01 {
		return nil, fmt.Errorf("混合支付金额合计 ¥%.2f 与本次应收 ¥%.2f 不一致", total, receiptAmount)
	}
	return splits, nil
}

func resolveDefaultExpenseAccount(tx *gorm.DB) (models.Account, error) {
	var account models.Account
	if err := tx.Set("gorm:query_option", "FOR UPDATE").Where("is_default_expense = ?", true).First(&account).Error; err == nil {
		return account, nil
	}
	return account, tx.Set("gorm:query_option", "FOR UPDATE").First(&account).Error
}

func createTradeInExpense(tx *gorm.DB, orderNo string, req CreateOrderRequest, occurredAt time.Time) error {
	if req.TradeInAmount <= 0 {
		return nil
	}

	account, err := resolveDefaultExpenseAccount(tx)
	if err != nil {
		return fmt.Errorf("系统暂无可用支出账户，请先在财务账户中设置默认支出账户")
	}

	if err := tx.Model(&account).UpdateColumn("balance", gorm.Expr("balance - ?", req.TradeInAmount)).Error; err != nil {
		return fmt.Errorf("以旧换新支出账户扣款失败")
	}

	remarkParts := []string{fmt.Sprintf("订单 %s 以旧换新付款给客户", orderNo)}
	if strings.TrimSpace(req.TradeInType) != "" {
		remarkParts = append(remarkParts, "旧件品类: "+strings.TrimSpace(req.TradeInType))
	}
	if strings.TrimSpace(req.TradeInRemark) != "" {
		remarkParts = append(remarkParts, "备注: "+strings.TrimSpace(req.TradeInRemark))
	}
	remark := strings.Join(remarkParts, "；")

	expense := models.Expense{
		ExpenseType: "以旧换新",
		Amount:      req.TradeInAmount,
		Remark:      remark,
		AccountID:   &account.ID,
		RelatedNo:   orderNo,
		CreatedAt:   occurredAt,
	}
	if err := tx.Create(&expense).Error; err != nil {
		return fmt.Errorf("以旧换新支出记录创建失败")
	}

	var updatedAccount models.Account
	tx.First(&updatedAccount, account.ID)
	if err := WriteFinancialLog(tx, account.ID, 2, "以旧换新", req.TradeInAmount, updatedAccount.Balance, orderNo, remark, occurredAt); err != nil {
		return fmt.Errorf("以旧换新财务流水写入失败")
	}

	return nil
}

func encodePaymentBreakdown(splits []PaymentBreakdownItem) string {
	data, err := json.Marshal(splits)
	if err != nil {
		return ""
	}
	return string(data)
}

func getOrderPaymentBreakdown(tx *gorm.DB, order models.Order, fallbackAmount float64) ([]PaymentBreakdownItem, error) {
	if order.PaymentBreakdown != "" {
		var splits []PaymentBreakdownItem
		if err := json.Unmarshal([]byte(order.PaymentBreakdown), &splits); err == nil && len(splits) > 0 {
			return splits, nil
		}
	}
	if order.AccountID == nil {
		return nil, nil
	}
	var account models.Account
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&account, *order.AccountID).Error; err != nil {
		return nil, err
	}
	return []PaymentBreakdownItem{{
		PaymentMethod: order.PaymentMethod,
		AccountID:     account.ID,
		Amount:        fallbackAmount,
	}}, nil
}

// CreateOrder 提交开单接口
// @Summary 创建订单
// @Description 创建订单
// @Tags Orders
// @Accept json
// @Produce json
// @Param body body CreateOrderRequest true "订单参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_orders
// @Router /api/orders [post]
func CreateOrder(c *gin.Context) {
	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "请求参数格式错误: " + err.Error(),
		})
		return
	}
	if len(req.Items) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "请至少添加一个商品",
		})
		return
	}
	if employee, isEmployeeToken, err := currentEmployeeScope(c); isEmployeeToken {
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "员工身份无效，请重新登录"})
			return
		}
		// 员工端 token 以服务端识别出的员工为准，避免前端篡改 employee_id 影响提成/订单归属。
		req.EmployeeID = &employee.ID
	}

	// 1. 开启事务
	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1.5 提前生成订单号（用于关联流水）
	orderTime := time.Now()
	dailySeq, err := nextDailyOrderSequence(tx, orderTime)
	if err != nil {
		tx.Rollback()
		if errors.Is(err, errDailyOrderSequenceExhausted) {
			c.JSON(http.StatusBadRequest, gin.H{
				"code": 400,
				"msg":  "今日订单号已达上限，请联系管理员处理",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "订单号生成失败",
		})
		return
	}
	orderNo := GenerateOrderNo(orderTime, dailySeq)

	var totalAmount float64
	orderItems := make([]models.OrderItem, 0)

	// 3. 校验库存并计算总价
	for _, item := range req.Items {
		var product models.Product
		if err := tx.First(&product, item.ProductID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusNotFound, gin.H{
				"code": 404,
				"msg":  fmt.Sprintf("商品 ID %d 不存在", item.ProductID),
			})
			return
		}

		// 根据发货方式检查对应库存
		cloudDeductionQty := 0
		if req.DeliveryMethod == 1 { // 门店自提
			if product.StoreStock < item.Quantity {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{
					"code": 400,
					"msg":  fmt.Sprintf("商品 %s 门店库存不足 (当前: %d)", product.Name, product.StoreStock),
				})
				return
			}
			result := tx.Model(&product).Where("store_stock >= ?", item.Quantity).UpdateColumn("store_stock", gorm.Expr("store_stock - ?", item.Quantity))
			if result.Error != nil || result.RowsAffected == 0 {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "库存扣减失败，因并发导致库存不足"})
				return
			}
		} else if req.DeliveryMethod == 2 { // 主仓发货
			if product.MainStock < item.Quantity {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{
					"code": 400,
					"msg":  fmt.Sprintf("商品 %s 主仓库存不足 (当前: %d)", product.Name, product.MainStock),
				})
				return
			}
			result := tx.Model(&product).Where("main_stock >= ?", item.Quantity).UpdateColumn("main_stock", gorm.Expr("main_stock - ?", item.Quantity))
			if result.Error != nil || result.RowsAffected == 0 {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "库存扣减失败，因并发导致库存不足"})
				return
			}
		} else if req.DeliveryMethod == 3 { // 云仓代发
			if product.CloudStock <= 0 && !product.SupportCloud {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{
					"code": 400,
					"msg":  fmt.Sprintf("商品 %s 不支持云仓代发且无云仓自有库存", product.Name),
				})
				return
			}
			// 如果有云仓库存，优先扣减
			if product.CloudStock > 0 {
				cloudDeductionQty = item.Quantity
				if product.CloudStock < item.Quantity {
					cloudDeductionQty = product.CloudStock
				}
				result := tx.Model(&product).Where("cloud_stock >= ?", cloudDeductionQty).UpdateColumn("cloud_stock", gorm.Expr("cloud_stock - ?", cloudDeductionQty))
				if result.Error != nil || result.RowsAffected == 0 {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "云仓库存扣减失败"})
					return
				}
			}
		}

		// 记录库存流水
		beforeQty := 0
		changeQty := -item.Quantity
		shouldWriteLog := true
		if req.DeliveryMethod == 1 {
			beforeQty = product.StoreStock
		} else if req.DeliveryMethod == 2 {
			beforeQty = product.MainStock
		} else if req.DeliveryMethod == 3 {
			beforeQty = product.CloudStock
			changeQty = -cloudDeductionQty
			// 如果没有从自有的云仓库存中扣除数量（直接由云仓提供网上代发），则不写入我们自己资产的流水
			if cloudDeductionQty == 0 {
				shouldWriteLog = false
			}
		}
		if shouldWriteLog {
			WriteInventoryLog(tx, product.ID, orderDeliveryToWarehouse(req.DeliveryMethod), "sale", beforeQty, changeQty, orderCostSnapshot(product), orderNo, "销售开单")
		}

		unitPrice := item.UnitPrice
		if item.IsGift {
			unitPrice = 0
		} else {
			// 核心防御：成交价不能低于移动平均成本（除非 AvgCost 为 0，即新商品未入库，或者管理员手动跳过校验）
			if !req.BypassPriceCheck && product.AvgCost > 0 && unitPrice < product.AvgCost {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{
					"code": 400,
					"msg":  fmt.Sprintf("商品 %s 成交价(¥%.2f)低于成本价(¥%.2f)，请联系管理员授权", product.Name, unitPrice, product.AvgCost),
				})
				return
			}
		}

		totalAmount += unitPrice * float64(item.Quantity)
		orderItems = append(orderItems, models.OrderItem{
			ProductID:         item.ProductID,
			UnitPrice:         unitPrice,
			UnitCost:          orderCostSnapshot(product),
			Quantity:          item.Quantity,
			CloudDeductionQty: cloudDeductionQty,
			IsGift:            item.IsGift,
		})
	}

	// 4. 计算国补金额
	// 国补按成交价 15% 计算，成交价超过 10000 时封顶 1500。
	var subsidyAmount float64
	baseAmount := totalAmount
	if req.IsSubsidy {
		baseForSubsidy := baseAmount

		// 如果是混合支付，国补基数应使用填入公户的金额
		if req.PaymentMethod == 4 {
			var publicAmount float64
			for _, split := range req.PaymentSplits {
				if split.PaymentMethod == 3 {
					publicAmount += split.Amount
				}
			}
			if publicAmount > 0 {
				baseForSubsidy = publicAmount
			}
		}

		subsidyAmount = baseForSubsidy * 0.15
		if subsidyAmount > 1500 {
			subsidyAmount = 1500
		}
		subsidyAmount = math.Round(subsidyAmount*100) / 100
	}

	actualPayAmount := baseAmount - subsidyAmount - req.TradeInAmount - req.DiscountAmount
	if actualPayAmount < 0 {
		actualPayAmount = 0
	}

	if req.DepositAmount > actualPayAmount {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "定金不能大于应付总额",
		})
		return
	}

	// 5. 判定订单状态
	paymentStatus := 2 // 默认已结全款
	if req.DepositAmount > 0 {
		paymentStatus = 1 // 仅付定金
	}

	subsidyStatus := 0 // 无国补
	if req.IsSubsidy {
		subsidyStatus = 1 // 待提交资料
	}

	receiptAmount := req.DepositAmount
	if paymentStatus == 2 {
		receiptAmount = actualPayAmount
	}
	paymentSplits, err := normalizePaymentSplits(tx, req, receiptAmount)
	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "收款账户或金额错误: " + err.Error(),
		})
		return
	}
	accountID := paymentSplits[0].AccountID
	orderPaymentMethod := req.PaymentMethod
	if req.PaymentMethod == 4 || len(paymentSplits) > 1 {
		orderPaymentMethod = 4
	}

	// 5.5 计算默认业务提成金额
	var commissionAmount float64
	if req.EmployeeID != nil && !req.NoCommission {
		var employee models.Employee
		if err := tx.First(&employee, *req.EmployeeID).Error; err == nil {
			commissionBase := actualPayAmount
			if req.IsSubsidy {
				commissionBase = baseAmount
			}
			commissionAmount = commissionBase * employee.CommissionRate
		}
	}
	if req.NoCommission && req.ReferrerName == "" {
		req.ReferrerName = "老板客户"
	}

	// 5.6 计算真实的国补待回款金额
	var subsidyPendingAmount float64
	if req.IsSubsidy {
		subsidyPendingAmount = subsidyAmount
	}

	// 6. 落库订单

	if req.OrderType == 0 {
		req.OrderType = 1
	}

	pinyinStr, pinyinInitial := utils.GetPinyin(req.CustomerName)

	order := models.Order{
		OrderNo:               orderNo,
		CustomerName:          req.CustomerName,
		CustomerPinyin:        pinyinStr,
		CustomerPinyinInitial: pinyinInitial,
		CustomerPhone:         req.CustomerPhone,
		CustomerAddress:       req.CustomerAddress,
		TotalAmount:           totalAmount,
		SubsidyAmount:         subsidyAmount,
		SubsidyPendingAmount:  subsidyPendingAmount,
		ActualPayAmount:       actualPayAmount,
		DepositAmount:         req.DepositAmount,
		DiscountAmount:        req.DiscountAmount,
		PaymentStatus:         paymentStatus,
		SubsidyStatus:         subsidyStatus,
		PaymentMethod:         orderPaymentMethod,
		PaymentBreakdown:      encodePaymentBreakdown(paymentSplits),
		DeliveryMethod:        req.DeliveryMethod,
		OrderStatus:           1, // 正常
		AccountID:             &accountID,
		EmployeeID:            req.EmployeeID,
		ReferrerName:          req.ReferrerName,
		ReferralFee:           req.ReferralFee,
		CommissionAmount:      commissionAmount,
		TradeInAmount:         req.TradeInAmount,
		TradeInType:           req.TradeInType,
		TradeInRemark:         req.TradeInRemark,
		OrderType:             req.OrderType,
	}

	if err := tx.Create(&order).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "订单创建失败",
		})
		return
	}

	// 7. 实时更新账户余额（收款金额 = 定金 or 全款）
	category := "预付定金"
	if paymentStatus == 2 {
		category = "销售货款"
	}
	for _, split := range paymentSplits {
		if err := tx.Model(&models.Account{}).Where("id = ?", split.AccountID).UpdateColumn("balance", gorm.Expr("balance + ?", split.Amount)).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"code": 500,
				"msg":  "账户余额更新失败",
			})
			return
		}

		var updatedAccount models.Account
		tx.Set("gorm:query_option", "FOR UPDATE").First(&updatedAccount, split.AccountID)
		logRemark := fmt.Sprintf("订单 %s 收款", orderNo)
		if len(paymentSplits) > 1 {
			logRemark = fmt.Sprintf("订单 %s 混合支付收款 ¥%.2f", orderNo, split.Amount)
		}
		if order.OrderType == 2 {
			logRemark = fmt.Sprintf("%s (商会订单)", logRemark)
		}
		if err := WriteFinancialLog(tx, split.AccountID, 1, category, split.Amount, updatedAccount.Balance, orderNo, logRemark); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "财务流水写入失败"})
			return
		}
	}

	// 8. 若有推荐人，不再在此预先写入开支，留作结平尾款或完成安装后核发

	// 9. 批量插入订单明细
	for i := range orderItems {
		orderItems[i].OrderID = order.ID
	}
	if err := tx.Create(&orderItems).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "订单明细保存失败",
		})
		return
	}

	if err := createTradeInExpense(tx, orderNo, req, orderTime); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
		return
	}

	var financialLogs []models.FinancialLog
	if err := tx.Where("related_no = ?", orderNo).Find(&financialLogs).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询财务流水失败: " + err.Error()})
		return
	}

	var inventoryLogs []models.InventoryLog
	if err := tx.Where("related_no = ?", orderNo).Find(&inventoryLogs).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询库存流水失败: " + err.Error()})
		return
	}

	// 10. 提交事务
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "事务提交失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":     200,
		"msg":      "下单成功",
		"order_no": orderNo,
		"data": gin.H{
			"order":          order,
			"items":          orderItems,
			"financial_logs": financialLogs,
			"inventory_logs": inventoryLogs,
		},
	})
}

func applyOrderFilters(query *gorm.DB, keyword, subsidyStatus, paymentStatus, fulfillmentStatus, orderStatus, startDate, endDate, categoryID string) *gorm.DB {
	if keyword != "" {
		k := "%" + keyword + "%"
		conditions := []string{
			"order_no LIKE ? OR customer_name LIKE ? OR customer_phone LIKE ? OR customer_address LIKE ? OR referrer_name LIKE ? OR trade_in_type LIKE ? OR trade_in_remark LIKE ? OR employee_id IN (SELECT id FROM employees WHERE name LIKE ?) OR installer_id IN (SELECT id FROM employees WHERE name LIKE ?) OR delivery_person_id IN (SELECT id FROM employees WHERE name LIKE ?) OR id IN (SELECT order_id FROM order_items JOIN products ON order_items.product_id = products.id LEFT JOIN categories ON products.category_id = categories.id WHERE products.name LIKE ? OR categories.name LIKE ?)",
		}
		args := []interface{}{k, k, k, k, k, k, k, k, k, k, k, k}
		if isSubsidyKeyword(keyword) {
			conditions = append(conditions, "(subsidy_amount > 0 OR subsidy_status != 0)")
		}
		if isTradeInKeyword(keyword) {
			conditions = append(conditions, "trade_in_amount > 0")
		}
		query = query.Where(
			strings.Join(conditions, " OR "),
			args...,
		)
	}
	if subsidyStatus != "" {
		query = query.Where("subsidy_status = ?", subsidyStatus)
	}
	if paymentStatus != "" {
		query = query.Where("payment_status = ?", paymentStatus)
	}
	if orderStatus != "" {
		query = query.Where("order_status = ?", orderStatus)
	}
	if fulfillmentStatus != "" {
		orderItems := func() *gorm.DB {
			return query.Session(&gorm.Session{NewDB: true}).Table("order_items")
		}
		switch fulfillmentStatus {
		case "pending_delivery":
			query = query.Where(
				"delivery_method != ? AND id IN (?)",
				1,
				orderItems().
					Select("DISTINCT order_id").
					Where("is_delivered = ?", false),
			)
		case "pending_install":
			query = query.Where(
				"delivery_method != ? AND id IN (?)",
				1,
				orderItems().
					Select("DISTINCT order_id").
					Where("is_delivered = ? AND is_installed = ?", true, false),
			)
		case "installed":
			query = query.Where("delivery_method != ? AND is_installed = ?", 1, true)
		case "partial":
			touchedItems := orderItems().
				Select("DISTINCT order_id").
				Where("is_delivered = ? OR is_installed = ?", true, true)
			incompleteItems := orderItems().
				Select("DISTINCT order_id").
				Where("is_delivered = ? OR is_installed = ?", false, false)
			query = query.Where("delivery_method != ? AND id IN (?) AND id IN (?)", 1, touchedItems, incompleteItems)
		case "pickup":
			query = query.Where("delivery_method = ?", 1)
		}
	}
	if startDate != "" {
		query = query.Where("created_at >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("created_at <= ?", endDate)
	}
	if categoryID != "" {
		categoryOrders := config.DB.Table("order_items").
			Select("DISTINCT order_items.order_id").
			Joins("JOIN products ON order_items.product_id = products.id").
			Where("products.category_id = ?", categoryID)
		query = query.Where("id IN (?)", categoryOrders)
	}
	return query
}

func isSubsidyKeyword(keyword string) bool {
	normalized := strings.ToLower(strings.TrimSpace(keyword))
	for _, term := range []string{"国补", "国家补贴", "政府补贴", "补贴", "subsidy"} {
		if strings.Contains(normalized, term) {
			return true
		}
	}
	return false
}

func isTradeInKeyword(keyword string) bool {
	normalized := strings.ToLower(strings.TrimSpace(keyword))
	for _, term := range []string{"以旧换新", "换新", "trade in", "trade-in", "tradein"} {
		if strings.Contains(normalized, term) {
			return true
		}
	}
	return false
}

// GetOrders 获取所有订单列表
// @Summary 获取订单列表
// @Description 获取订单列表
// @Tags Orders
// @Accept json
// @Produce json
// @Param keyword query string false "模糊搜索"
// @Param subsidy_status query string false "国补状态"
// @Param payment_status query string false "支付状态"
// @Param fulfillment_status query string false "配送与安装状态"
// @Param order_status query string false "订单状态"
// @Param start_date query string false "开始日期"
// @Param end_date query string false "结束日期"
// @Param category_id query int false "分类ID"
// @Param page query int false "页码"
// @Param page_size query int false "每页数量"
// @Param fields query string false "字段控制(如light)"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_orders
// @Router /api/orders [get]
func GetOrders(c *gin.Context) {
	keyword := c.Query("keyword")
	subsidyStatus := c.Query("subsidy_status")
	paymentStatus := c.Query("payment_status")
	fulfillmentStatus := c.Query("fulfillment_status")
	orderStatus := c.Query("order_status")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	categoryID := c.Query("category_id")
	pageStr := c.Query("page")
	pageSizeStr := c.Query("page_size")
	fields := c.Query("fields")

	query := config.DB.Order("created_at desc")
	if employee, isEmployeeToken, err := currentEmployeeScope(c); isEmployeeToken {
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{"code": 401, "msg": "员工身份无效，请重新登录"})
			return
		}
		query = query.Where("employee_id = ?", employee.ID)
	}
	if fields != "" && fields != "light" {
		query = query.Select(strings.Split(fields, ","))
	} else if fields != "light" {
		query = query.Preload("OrderItems.Product").Preload("OrderItems.Product.Category").Preload("OrderItems.DeliveryPerson").Preload("OrderItems.Installer").Preload("Employee").Preload("Account").Preload("Installer").Preload("DeliveryPerson")
	} else {
		// light模式只加载最基本的员工信息
		query = query.Preload("Employee")
	}

	query = applyOrderFilters(query, keyword, subsidyStatus, paymentStatus, fulfillmentStatus, orderStatus, startDate, endDate, categoryID)

	var orders []models.Order

	if pageStr != "" {
		page, _ := strconv.Atoi(pageStr)
		if page <= 0 {
			page = 1
		}
		pageSize, _ := strconv.Atoi(pageSizeStr)
		if pageSize <= 0 {
			pageSize = 15
		}

		var total int64
		if err := query.Model(&models.Order{}).Count(&total).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
			return
		}

		offset := (page - 1) * pageSize
		if err := query.Offset(offset).Limit(pageSize).Find(&orders).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": err.Error()})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"code":  200,
			"msg":   "查询成功",
			"data":  orders,
			"total": total,
		})
		return
	}

	if err := query.Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "订单查询失败: " + err.Error(),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "查询成功",
		"data": orders,
	})
}

// UpdateSubsidyRequest 更新国补状态请求体
type UpdateSubsidyRequest struct {
	TargetStatus int `json:"target_status" binding:"required,oneof=2"`
}

// UpdateSubsidyStatus 更新国补状态（提交资料）
// @Summary 更新订单国补状态
// @Description 更新订单国补状态
// @Tags Orders
// @Accept json
// @Produce json
// @Param body body UpdateSubsidyRequest true "国补状态参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param id path int true "路径资源ID"
// @ID put_api_orders_id_subsidy
// @Router /api/orders/{id}/subsidy [put]
func UpdateSubsidyStatus(c *gin.Context) {
	id := c.Param("id")

	var req UpdateSubsidyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "参数错误: " + err.Error(),
		})
		return
	}

	var order models.Order
	if err := config.DB.First(&order, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "订单不存在",
		})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1. 更新订单国补状态
	if err := tx.Model(&order).Update("subsidy_status", req.TargetStatus).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新状态失败: " + err.Error()})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "事务提交失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "状态更新成功",
	})
}

// PayBalanceRequest 结清尾款请求体
type PayBalanceRequest struct {
	// 最终全款支付方式: 1-支付宝, 2-微信, 3-公户, 5-现金
	FinalPaymentMethod int `json:"final_payment_method" binding:"required"`
}

// PayOrderBalance 结清尾款 —— "退定再结全额"模式
// 第一步：从原收款账户退还定金（记录支出）
// 第二步：向新选账户收取全款（actual_pay_amount）
// 第三步：更新订单状态
// @Summary 支付订单尾款
// @Description 支付订单尾款
// @Tags Orders
// @Accept json
// @Produce json
// @Param body body PayBalanceRequest true "尾款支付参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param id path int true "路径资源ID"
// @ID put_api_orders_id_balance
// @Router /api/orders/{id}/balance [put]
func PayOrderBalance(c *gin.Context) {
	id := c.Param("id")

	var req PayBalanceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "请选择全款支付方式",
		})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var order models.Order
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&order, id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "订单不存在",
		})
		return
	}

	// 如果已经是已结全款 (2)
	if order.PaymentStatus == 2 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "订单已结清",
		})
		return
	}

	// 如果订单已取消
	if order.OrderStatus == 2 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "订单已取消，无法操作",
		})
		return
	}

	originalDepositAmount := order.DepositAmount

	// ========== 第一步：定金对冲退款 ==========
	if order.DepositAmount > 0 {
		depositSplits, err := getOrderPaymentBreakdown(tx, order, order.DepositAmount)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询原账户失败"})
			return
		}

		for _, split := range depositSplits {
			var depositLog models.FinancialLog
			if err := tx.Where("related_no = ? AND account_id = ? AND category = ? AND reversal_pair_id IS NULL",
				order.OrderNo, split.AccountID, "预付定金").
				Order("id DESC").
				First(&depositLog).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{
					"code": 500,
					"msg":  fmt.Sprintf("未找到订单 %s 账户 %d 的原定金流水，无法完成冲销对账", order.OrderNo, split.AccountID),
				})
				return
			}

			var refundAccount models.Account
			if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&refundAccount, split.AccountID).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{
					"code": 500,
					"msg":  fmt.Sprintf("定金账户(ID %d)查询失败，无法完成结算", split.AccountID),
				})
				return
			}
			if refundAccount.Balance < split.Amount {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{
					"code": 400,
					"msg":  fmt.Sprintf("账户「%s」余额 ¥%.2f 不足以退还定金 ¥%.2f，无法完成结算，请先补充账户余额或调整收款账户", refundAccount.Name, refundAccount.Balance, split.Amount),
				})
				return
			}
			if err := tx.Model(&models.Account{}).Where("id = ?", split.AccountID).
				UpdateColumn("balance", gorm.Expr("balance - ?", split.Amount)).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{
					"code": 500,
					"msg":  "原账户定金退还失败",
				})
				return
			}

			var oldAccount models.Account
			if err := tx.First(&oldAccount, split.AccountID).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取退款后账户余额失败"})
				return
			}
			if err := WriteReversalLog(tx, depositLog.ID, split.AccountID, 2, "退还定金", split.Amount, oldAccount.Balance, order.OrderNo, fmt.Sprintf("订单 %s 结算前退还定金 ¥%.2f", order.OrderNo, split.Amount)); err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{
					"code": 500,
					"msg":  "定金退还记录写入失败",
				})
				return
			}
		}
	}

	// ========== 第二步：收取全款 ==========
	var finalAccountID uint
	var finalAcc models.Account
	var accErr error
	var name string
	switch req.FinalPaymentMethod {
	case 1:
		name = "支付宝"
	case 2:
		name = "微信"
	case 3:
		name = "公户"
	case 5:
		name = "现金"
	}
	if name != "" {
		accErr = tx.Set("gorm:query_option", "FOR UPDATE").Where("name = ?", name).First(&finalAcc).Error
	}
	if accErr != nil || name == "" {
		accErr = tx.Set("gorm:query_option", "FOR UPDATE").First(&finalAcc, req.FinalPaymentMethod).Error
	}
	if accErr != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "结算账户不存在，请检查配置",
		})
		return
	}
	finalAccountID = finalAcc.ID

	// 2a. 锁定新账户并增加全款
	if err := tx.Model(&models.Account{}).Where("id = ?", finalAccountID).
		UpdateColumn("balance", gorm.Expr("balance + ?", order.ActualPayAmount)).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "全款收取失败，目标账户不存在或更新失败",
		})
		return
	}

	// 2b. 插入"全款收入"流水记录
	var newAccount models.Account
	tx.First(&newAccount, finalAccountID)
	if err := WriteFinancialLog(tx, finalAccountID, 1, "销售货款", order.ActualPayAmount, newAccount.Balance, order.OrderNo, fmt.Sprintf("订单 %s 收取全款 ¥%.2f", order.OrderNo, order.ActualPayAmount)); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "全款收入记录写入失败",
		})
		return
	}

	// ========== 第三步：更新订单 ==========
	updates := map[string]interface{}{
		"payment_status": 2,                      // 已结全款
		"payment_method": req.FinalPaymentMethod, // 最终支付方式
		"payment_breakdown": encodePaymentBreakdown([]PaymentBreakdownItem{{
			PaymentMethod: req.FinalPaymentMethod,
			AccountID:     finalAccountID,
			Amount:        order.ActualPayAmount,
		}}),
		"deposit_amount": 0,              // 定金已退，账面清零
		"account_id":     finalAccountID, // 关联到最终收款账户
	}
	// 使用 map 方式的 Updates 会包含零值字段，无需额外 Select
	if err := tx.Model(&order).Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "订单状态更新失败: " + err.Error(),
		})
		return
	}

	// 触发推荐人提成下发检查 (结平尾款可能是由于后续操作，也可能此时已经安装完毕)
	order.PaymentStatus = 2
	order.AccountID = &finalAccountID
	if err := checkAndPayReferralFee(tx, &order); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "第三方提成下发失败"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "事务提交失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  fmt.Sprintf("结算完成：已退还定金 ¥%.2f，已收取全款 ¥%.2f", originalDepositAmount, order.ActualPayAmount),
	})
}

// CancelOrder 取消订单接口
// @Summary 取消订单
// @Description 取消订单
// @Tags Orders
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param id path int true "路径资源ID"
// @ID put_api_orders_id_cancel
// @Router /api/orders/{id}/cancel [put]
func CancelOrder(c *gin.Context) {
	cancelOrderWithOptions(c, false)
}

// ReversalCancelOrder 已履约订单红冲接口
// @Summary 撤销取消订单
// @Description 撤销取消订单
// @Tags Orders
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param id path int true "路径资源ID"
// @ID put_api_orders_id_reversal_cancel
// @Router /api/orders/{id}/reversal-cancel [put]
func ReversalCancelOrder(c *gin.Context) {
	username, _ := c.Get("username")
	if name, ok := username.(string); ok && strings.HasPrefix(name, "emp_") {
		c.JSON(http.StatusForbidden, gin.H{
			"code": 403,
			"msg":  "员工账号无权执行订单红冲",
		})
		return
	}

	cancelOrderWithOptions(c, true)
}

func cancelOrderWithOptions(c *gin.Context, allowFulfilled bool) {
	id := c.Param("id")
	actionName := "取消"
	inventoryRemark := "订单取消库存回滚"
	cloudInventoryRemark := "订单取消云仓库存回滚"
	financialCategory := "订单取消退款"
	if allowFulfilled {
		actionName = "红冲"
		inventoryRemark = "订单红冲库存回滚"
		cloudInventoryRemark = "订单红冲云仓库存回滚"
		financialCategory = "订单红冲"
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var order models.Order
	if err := tx.Set("gorm:query_option", "FOR UPDATE").Preload("OrderItems").First(&order, id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "订单不存在",
		})
		return
	}

	if order.OrderStatus == 2 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "订单已取消，请勿重复操作",
		})
		return
	}

	if !allowFulfilled {
		for _, item := range order.OrderItems {
			if item.IsDelivered || item.IsInstalled {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{
					"code": 400,
					"msg":  "订单中已有商品完成了配送送货或上门安装，无法直接取消订单",
				})
				return
			}
		}
	}

	for _, item := range order.OrderItems {
		var updateField string
		if order.DeliveryMethod == 1 {
			updateField = "store_stock"
		} else if order.DeliveryMethod == 2 {
			updateField = "main_stock"
		} else if order.DeliveryMethod == 3 {
			// 如果是云仓代发，需要回滚当时扣除的云仓自有库存
			if item.CloudDeductionQty > 0 {
				if err := tx.Model(&models.Product{}).Where("id = ?", item.ProductID).
					UpdateColumn("cloud_stock", gorm.Expr("cloud_stock + ?", item.CloudDeductionQty)).Error; err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{
						"code": 500,
						"msg":  fmt.Sprintf("商品 ID %d 云仓库存回滚失败", item.ProductID),
					})
					return
				}
				// 记录云仓库存回滚流水
				var product models.Product
				if err := tx.First(&product, item.ProductID).Error; err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询商品库存失败"})
					return
				}
				if err := WriteInventoryLog(tx, item.ProductID, 3, "cancel", product.CloudStock-item.CloudDeductionQty, item.CloudDeductionQty, product.LatestCost, order.OrderNo, cloudInventoryRemark); err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "库存流水写入失败"})
					return
				}
			}
			continue
		}
		if err := tx.Model(&models.Product{}).Where("id = ?", item.ProductID).
			UpdateColumn(updateField, gorm.Expr(updateField+" + ?", item.Quantity)).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"code": 500,
				"msg":  fmt.Sprintf("商品 ID %d 库存回滚失败", item.ProductID),
			})
			return
		}

		// 记录库存回滚流水
		var product models.Product
		if err := tx.First(&product, item.ProductID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询商品库存失败"})
			return
		}
		beforeQty := 0
		warehouseType := orderDeliveryToWarehouse(order.DeliveryMethod)
		if warehouseType == 1 {
			beforeQty = product.MainStock - item.Quantity
		} else if warehouseType == 2 {
			beforeQty = product.StoreStock - item.Quantity
		} else if warehouseType == 3 {
			beforeQty = product.CloudStock - item.Quantity
		}

		if err := WriteInventoryLog(tx, item.ProductID, warehouseType, "cancel", beforeQty, item.Quantity, product.LatestCost, order.OrderNo, inventoryRemark); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "库存流水写入失败"})
			return
		}
	}

	var refundAmount float64
	if order.PaymentStatus == 2 {
		refundAmount = order.ActualPayAmount
	} else {
		refundAmount = order.DepositAmount
	}

	if refundAmount > 0 {
		refundSplits, err := getOrderPaymentBreakdown(tx, order, refundAmount)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询退款账户失败"})
			return
		}

		for _, split := range refundSplits {
			// 校验原收款账户余额是否足够退款
			var targetAccount models.Account
			if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&targetAccount, split.AccountID).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询退款账户失败"})
				return
			}
			if targetAccount.Balance < split.Amount {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "退款账户余额不足，无法取消订单"})
				return
			}

			// 从账户扣除已收款项 (退款)
			if err := tx.Model(&models.Account{}).Where("id = ?", split.AccountID).
				UpdateColumn("balance", gorm.Expr("balance - ?", split.Amount)).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{
					"code": 500,
					"msg":  "退款账户余额扣除失败",
				})
				return
			}

			var updatedAccount models.Account
			if err := tx.First(&updatedAccount, split.AccountID).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取退款后账户余额失败"})
				return
			}

			var collectLog models.FinancialLog
			collectLogErr := tx.Where("related_no = ? AND account_id = ? AND category = ? AND reversal_pair_id IS NULL",
				order.OrderNo, split.AccountID, "销售货款").
				Order("id DESC").
				First(&collectLog).Error
			if collectLogErr != nil {
				collectLogErr = tx.Where("related_no = ? AND account_id = ? AND category = ? AND reversal_pair_id IS NULL",
					order.OrderNo, split.AccountID, "预付定金").
					Order("id DESC").
					First(&collectLog).Error
			}

			if collectLogErr == nil {
				if err := WriteReversalLog(tx, collectLog.ID, split.AccountID, 2, financialCategory, split.Amount, updatedAccount.Balance, order.OrderNo, fmt.Sprintf("订单 %s %s，反向退款 ¥%.2f", order.OrderNo, actionName, split.Amount)); err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "退款配对流水写入失败"})
					return
				}
			} else if err := WriteFinancialLog(tx, split.AccountID, 2, financialCategory, split.Amount, updatedAccount.Balance, order.OrderNo, fmt.Sprintf("订单 %s %s，反向退款 ¥%.2f", order.OrderNo, actionName, split.Amount)); err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "退款记录写入失败"})
				return
			}
		}
	}

	if err := tx.Model(&order).Update("order_status", 2).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "订单状态更新失败",
		})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "事务提交失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  fmt.Sprintf("订单已%s，库存已回滚，已反向退款 ¥%.2f，请重新录入完整正确订单", actionName, refundAmount),
	})
}

// UpdateDeliveryStatusRequest 确认送货请求体
type UpdateDeliveryStatusRequest struct {
	DeliveryPersonID uint       `json:"delivery_person_id" binding:"required"`
	DeliveryFee      float64    `json:"delivery_fee"`
	DeliveryTime     *time.Time `json:"delivery_time"`
}

// UpdateDeliveryStatus 确认送货接口
// @Summary 更新订单发货状态
// @Description 更新订单发货状态
// @Tags Orders
// @Accept json
// @Produce json
// @Param body body UpdateDeliveryStatusRequest true "送货参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param id path int true "路径资源ID"
// @ID put_api_orders_id_deliver
// @Router /api/orders/{id}/deliver [put]
func UpdateDeliveryStatus(c *gin.Context) {
	id := c.Param("id")

	var req UpdateDeliveryStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "参数错误: " + err.Error(),
		})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var order models.Order
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&order, id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "订单不存在",
		})
		return
	}

	if order.OrderStatus == 2 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "订单已取消，无法操作",
		})
		return
	}

	deliveryTime := time.Now()
	if req.DeliveryTime != nil {
		deliveryTime = *req.DeliveryTime
	}

	updates := map[string]interface{}{
		"is_delivered":       true,
		"delivery_person_id": req.DeliveryPersonID,
		"delivery_fee":       req.DeliveryFee,
		"delivery_time":      deliveryTime,
	}

	if err := tx.Model(&order).Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "状态更新失败",
		})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "事务提交失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "送货确认成功",
	})
}

// UpdateInstallStatusRequest 确认安装请求体
type UpdateInstallStatusRequest struct {
	InstallerID     uint       `json:"installer_id" binding:"required"`
	InstallTime     *time.Time `json:"install_time"`
	InstallationFee float64    `json:"installation_fee"`
}

// UpdateInstallStatus 确认安装接口
// @Summary 更新订单安装状态
// @Description 更新订单安装状态
// @Tags Orders
// @Accept json
// @Produce json
// @Param body body UpdateInstallStatusRequest true "安装参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param id path int true "路径资源ID"
// @ID put_api_orders_id_install
// @Router /api/orders/{id}/install [put]
func UpdateInstallStatus(c *gin.Context) {
	id := c.Param("id")

	var req UpdateInstallStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "参数错误: " + err.Error(),
		})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var order models.Order
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&order, id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "订单不存在",
		})
		return
	}

	if order.OrderStatus == 2 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "订单已取消，无法操作",
		})
		return
	}

	if !order.IsDelivered {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "订单尚未送货，请先进行送货操作",
		})
		return
	}

	installTime := time.Now()
	if req.InstallTime != nil {
		installTime = *req.InstallTime
	}

	updates := map[string]interface{}{
		"is_installed":     true,
		"installer_id":     req.InstallerID,
		"install_time":     installTime,
		"installation_fee": req.InstallationFee,
	}

	if err := tx.Model(&order).Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "状态更新失败",
		})
		return
	}

	order.IsInstalled = true
	// 判断是否需要下发推荐人提成 (安装由于可能是结清全款之后的事情，因此也作为触发点)
	if err := checkAndPayReferralFee(tx, &order); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "第三方提成下发失败"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "事务提交失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "确认安装成功",
	})
}

// checkAndPayReferralFee 检查订单是否已结清尾款且已安装，若是则下发推荐人提成
func checkAndPayReferralFee(tx *gorm.DB, order *models.Order) error {
	if order.ReferralFee > 0 && !order.IsReferralFeePaid && order.PaymentStatus == 2 && order.IsInstalled {
		accountID := uint(3) // 默认兜底从公户
		if order.AccountID != nil {
			accountID = *order.AccountID
		}

		if err := tx.Model(&models.Account{}).Where("id = ?", accountID).UpdateColumn("balance", gorm.Expr("balance - ?", order.ReferralFee)).Error; err != nil {
			return err
		}

		var updatedAcc models.Account
		tx.First(&updatedAcc, accountID)
		if err := WriteFinancialLog(tx, accountID, 2, "第三方奖励", order.ReferralFee, updatedAcc.Balance, order.OrderNo, fmt.Sprintf("订单 %s 完结核算(款齐装完)：推荐人(%s)奖励", order.OrderNo, order.ReferrerName)); err != nil {
			return err
		}

		if err := tx.Model(order).UpdateColumn("is_referral_fee_paid", true).Error; err != nil {
			return err
		}
		order.IsReferralFeePaid = true
	}
	return nil
}

var errReferralFeeAlreadyPaid = errors.New("referral fee already paid")

func validateReferralFeeUpdate(order *models.Order, referralFee float64) error {
	if order.IsReferralFeePaid {
		return errReferralFeeAlreadyPaid
	}
	return nil
}

// UpdateCommissionRequest 更新提成金额请求体
type UpdateCommissionRequest struct {
	CommissionAmount float64 `json:"commission_amount" binding:"min=0"`
}

// UpdateReferralFeeRequest 更新推荐人提成金额请求体
type UpdateReferralFeeRequest struct {
	ReferralFee *float64 `json:"referral_fee" binding:"required,gte=0"`
}

// UpdateOrderCommission 手动调整订单提成金额
// @Summary 更新订单提成
// @Description 更新订单提成
// @Tags Orders
// @Accept json
// @Produce json
// @Param body body UpdateCommissionRequest true "提成参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param id path int true "路径资源ID"
// @ID put_api_orders_id_commission
// @Router /api/orders/{id}/commission [put]
func UpdateOrderCommission(c *gin.Context) {
	id := c.Param("id")
	var req UpdateCommissionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	var order models.Order
	if err := config.DB.First(&order, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "订单不存在"})
		return
	}

	// 核心防御：提成金额不能超过订单实付金额的 30%
	maxCommission := order.ActualPayAmount * 0.3
	if req.CommissionAmount > maxCommission {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  fmt.Sprintf("提成金额超出合理范围(上限 ¥%.2f)", maxCommission),
		})
		return
	}

	if err := config.DB.Model(&models.Order{}).Where("id = ?", id).Update("commission_amount", req.CommissionAmount).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新提成失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "提成调整成功"})
}

// UpdateOrderReferralFee 手动调整订单推荐人提成金额
// @Summary 更新订单推荐人提成
// @Description 仅管理员可修改未发放的订单推荐人提成
// @Tags Orders
// @Accept json
// @Produce json
// @Param body body UpdateReferralFeeRequest true "推荐人提成参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param id path int true "路径资源ID"
// @ID put_api_orders_id_referral_fee
// @Router /api/orders/{id}/referral-fee [put]
func UpdateOrderReferralFee(c *gin.Context) {
	id := c.Param("id")
	var req UpdateReferralFeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	var order models.Order
	if err := config.DB.First(&order, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "订单不存在"})
		return
	}
	if err := validateReferralFeeUpdate(&order, *req.ReferralFee); err != nil {
		if errors.Is(err, errReferralFeeAlreadyPaid) {
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "推荐人提成已发放，不能直接修改，请先进行财务冲正"})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}

	if err := config.DB.Model(&order).Update("referral_fee", *req.ReferralFee).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新推荐人提成失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "推荐人提成调整成功"})
}

// UpdateItemDeliveryStatus 确认单品送货接口
// @Summary 更新订单项发货状态
// @Description 更新订单项发货状态
// @Tags Order-items
// @Accept json
// @Produce json
// @Param body body UpdateDeliveryStatusRequest true "单品送货参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param item_id path int true "路径资源ID"
// @ID put_api_order_items_item_id_deliver
// @Router /api/order-items/{item_id}/deliver [put]
func UpdateItemDeliveryStatus(c *gin.Context) {
	itemID := c.Param("item_id")

	var req UpdateDeliveryStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "参数错误: " + err.Error(),
		})
		return
	}

	var item models.OrderItem
	if err := config.DB.First(&item, itemID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "商品明细不存在",
		})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var order models.Order
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&order, item.OrderID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "关联订单不存在",
		})
		return
	}

	if order.OrderStatus == 2 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "订单已取消，无法操作",
		})
		return
	}

	deliveryTime := time.Now()
	if req.DeliveryTime != nil {
		deliveryTime = *req.DeliveryTime
	}

	// 1. 更新单品送货状态
	itemUpdates := map[string]interface{}{
		"is_delivered":       true,
		"delivery_person_id": req.DeliveryPersonID,
		"delivery_fee":       req.DeliveryFee,
		"delivery_time":      deliveryTime,
	}
	if err := tx.Model(&item).Updates(itemUpdates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "商品送货状态更新失败"})
		return
	}

	// 2. 重新聚合父订单配送状态与送货费用总和
	var items []models.OrderItem
	if err := tx.Where("order_id = ?", order.ID).Find(&items).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询商品明细失败"})
		return
	}

	allDelivered := true
	totalDeliveryFee := 0.0
	for _, it := range items {
		if it.ID == item.ID {
			totalDeliveryFee += req.DeliveryFee
		} else {
			if !it.IsDelivered {
				allDelivered = false
			}
			totalDeliveryFee += it.DeliveryFee
		}
	}

	orderUpdates := map[string]interface{}{
		"is_delivered": allDelivered,
		"delivery_fee": totalDeliveryFee,
	}
	if order.DeliveryPersonID == nil {
		orderUpdates["delivery_person_id"] = req.DeliveryPersonID
	}
	if order.DeliveryTime == nil {
		orderUpdates["delivery_time"] = deliveryTime
	}

	if err := tx.Model(&order).Updates(orderUpdates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "订单状态聚合失败"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "事务提交失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "商品送货确认成功",
	})
}

// UpdateItemInstallStatus 确认单品安装接口
// @Summary 更新订单项安装状态
// @Description 更新订单项安装状态
// @Tags Order-items
// @Accept json
// @Produce json
// @Param body body UpdateInstallStatusRequest true "单品安装参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param item_id path int true "路径资源ID"
// @ID put_api_order_items_item_id_install
// @Router /api/order-items/{item_id}/install [put]
func UpdateItemInstallStatus(c *gin.Context) {
	itemID := c.Param("item_id")

	var req UpdateInstallStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "参数错误: " + err.Error(),
		})
		return
	}

	var item models.OrderItem
	if err := config.DB.First(&item, itemID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "商品明细不存在",
		})
		return
	}

	if !item.IsDelivered {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "该商品尚未送货，请先送货！",
		})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var order models.Order
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&order, item.OrderID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "关联订单不存在",
		})
		return
	}

	if order.OrderStatus == 2 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "订单已取消，无法操作",
		})
		return
	}

	installTime := time.Now()
	if req.InstallTime != nil {
		installTime = *req.InstallTime
	}

	// 1. 更新单品安装状态
	itemUpdates := map[string]interface{}{
		"is_installed":     true,
		"installer_id":     req.InstallerID,
		"installation_fee": req.InstallationFee,
		"install_time":     installTime,
	}
	if err := tx.Model(&item).Updates(itemUpdates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "商品安装状态更新失败"})
		return
	}

	// 2. 重新聚合父订单安装状态与费用
	var items []models.OrderItem
	if err := tx.Where("order_id = ?", order.ID).Find(&items).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询商品明细失败"})
		return
	}

	allInstalled := true
	totalInstallationFee := 0.0
	for _, it := range items {
		if it.ID == item.ID {
			totalInstallationFee += req.InstallationFee
		} else {
			if !it.IsInstalled {
				allInstalled = false
			}
			totalInstallationFee += it.InstallationFee
		}
	}

	orderUpdates := map[string]interface{}{
		"is_installed":     allInstalled,
		"installation_fee": totalInstallationFee,
	}
	if order.InstallerID == nil {
		orderUpdates["installer_id"] = req.InstallerID
	}
	if order.InstallTime == nil {
		orderUpdates["install_time"] = installTime
	}

	if err := tx.Model(&order).Updates(orderUpdates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "订单状态聚合失败"})
		return
	}

	order.IsInstalled = allInstalled
	if err := checkAndPayReferralFee(tx, &order); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "第三方提成下发失败"})
		return
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "事务提交失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "商品安装确认成功",
	})
}

// ExportOrders 导出所有满足条件的订单
// @Summary 导出订单数据
// @Description 导出订单数据
// @Tags Orders
// @Produce application/octet-stream
// @Param keyword query string false "模糊搜索"
// @Param subsidy_status query int false "国补状态"
// @Param payment_status query int false "支付状态"
// @Param fulfillment_status query string false "配送与安装状态"
// @Param order_status query int false "订单状态"
// @Param start_date query string false "开始日期"
// @Param end_date query string false "结束日期"
// @Param category_id query int false "分类ID"
// @Success 200 {file} binary "Excel 文件"
// @Security ApiKeyAuth
// @ID get_api_orders_export
// @Router /api/orders/export [get]
func ExportOrders(c *gin.Context) {
	keyword := c.Query("keyword")
	subsidyStatus := c.Query("subsidy_status")
	paymentStatus := c.Query("payment_status")
	fulfillmentStatus := c.Query("fulfillment_status")
	orderStatus := c.Query("order_status")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")
	categoryID := c.Query("category_id")

	query := config.DB.Preload("OrderItems.Product").Preload("OrderItems.Product.Category").Preload("OrderItems.DeliveryPerson").Preload("OrderItems.Installer").Preload("Employee").Preload("Account").Preload("Installer").Preload("DeliveryPerson").Order("created_at desc")
	query = applyOrderFilters(query, keyword, subsidyStatus, paymentStatus, fulfillmentStatus, orderStatus, startDate, endDate, categoryID)

	var orders []models.Order
	if err := query.Find(&orders).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询订单失败"})
		return
	}

	headers := []string{"订单号", "客户姓名", "联系电话", "客户地址", "商品明细", "订单总额", "国补金额", "实付金额", "已付定金", "抹零优惠", "货品成本", "履约成本", "其他成本", "总成本", "毛利", "毛利率", "业务提成", "渠道提成", "支付状态", "订单状态", "收款账户", "开单人", "开单时间"}
	var data [][]interface{}
	for _, o := range orders {
		payStatus := "仅付定金"
		if o.PaymentStatus == 2 {
			payStatus = "已结全款"
		}
		ordStatus := "正常"
		if o.OrderStatus == 2 {
			ordStatus = "已取消"
		}
		accName := "-"
		if o.Account != nil {
			accName = o.Account.Name
		}
		empName := "-"
		if o.Employee != nil {
			empName = o.Employee.Name
		}

		var productDetails []string
		var productCost float64
		var fulfillmentCost float64
		for _, item := range o.OrderItems {
			productName := "未知商品"
			if item.Product != nil {
				productName = item.Product.Name
			}
			lineAmount := item.UnitPrice * float64(item.Quantity)
			lineCost := item.UnitCost * float64(item.Quantity)
			lineProfit := lineAmount - lineCost
			productDetails = append(productDetails, fmt.Sprintf("%s x%d 单价¥%.2f 成本¥%.2f 毛利¥%.2f", productName, item.Quantity, item.UnitPrice, lineCost, lineProfit))
			productCost += lineCost
			fulfillmentCost += item.DeliveryFee + item.InstallationFee
		}
		otherCost := o.ReferralFee + o.TradeInAmount
		totalCost := productCost + fulfillmentCost + otherCost
		grossProfit := o.TotalAmount - totalCost
		grossMargin := 0.0
		if o.TotalAmount > 0 {
			grossMargin = grossProfit / o.TotalAmount
		}

		row := []interface{}{
			o.OrderNo,
			o.CustomerName,
			o.CustomerPhone,
			o.CustomerAddress,
			strings.Join(productDetails, "\n"),
			o.TotalAmount,
			o.SubsidyAmount,
			o.ActualPayAmount,
			o.DepositAmount,
			o.DiscountAmount,
			productCost,
			fulfillmentCost,
			otherCost,
			totalCost,
			grossProfit,
			fmt.Sprintf("%.2f%%", grossMargin*100),
			o.CommissionAmount,
			o.ReferralFee,
			payStatus,
			ordStatus,
			accName,
			empName,
			o.CreatedAt.Format("2006-01-02 15:04:05"),
		}
		data = append(data, row)
	}

	utils.ExportToExcel(c, "订单明细", headers, data)
}

// AIOrderItemRequest AI友好的购物车单个商品项
type AIOrderItemRequest struct {
	Keyword   string  `json:"keyword" binding:"required"`
	Quantity  int     `json:"quantity" binding:"required,gt=0"`
	UnitPrice float64 `json:"unit_price"`
}

// AIOrderRequest AI友好的创建订单请求体
type AIOrderRequest struct {
	CustomerName   string               `json:"customer_name"`
	CustomerPhone  string               `json:"customer_phone"`
	Items          []AIOrderItemRequest `json:"items" binding:"required,dive"`
	PaymentAccount string               `json:"payment_account"`
	DeliveryMethod string               `json:"delivery_method"`
	Referrer       string               `json:"referrer"`
}

// CreateAIOrder AI友好的提交开单接口
// @Summary AI创建订单
// @Description 接受自然语言特征的订单信息并创建订单
// @Tags Orders
// @Accept json
// @Produce json
// @Param body body AIOrderRequest true "订单参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_ai_orders
// @Router /api/ai/orders [post]
func CreateAIOrder(c *gin.Context) {
	var req AIOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "请求参数格式错误: " + err.Error(),
		})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var actualReq CreateOrderRequest
	actualReq.CustomerName = req.CustomerName
	actualReq.CustomerPhone = req.CustomerPhone
	actualReq.ReferrerName = req.Referrer

	// 解析支付账户
	var account models.Account
	var paymentMethod int
	if req.PaymentAccount != "" {
		if err := tx.Where("name LIKE ?", "%"+req.PaymentAccount+"%").First(&account).Error; err == nil {
			actualReq.AccountID = &account.ID
			if strings.Contains(account.Name, "微信") {
				paymentMethod = 2
			} else if strings.Contains(account.Name, "支付宝") {
				paymentMethod = 1
			} else if strings.Contains(account.Name, "公户") || strings.Contains(account.Name, "对公") {
				paymentMethod = 3
			} else if strings.Contains(account.Name, "现金") {
				paymentMethod = 5
			} else {
				paymentMethod = 1
			}
		}
	}
	if paymentMethod == 0 {
		paymentMethod = 2 // 默认微信
	}
	actualReq.PaymentMethod = paymentMethod

	// 解析发货方式
	if strings.Contains(req.DeliveryMethod, "自提") {
		actualReq.DeliveryMethod = 1
	} else if strings.Contains(req.DeliveryMethod, "云仓") || strings.Contains(req.DeliveryMethod, "代发") {
		actualReq.DeliveryMethod = 3
	} else {
		actualReq.DeliveryMethod = 2 // 默认主仓
	}

	// 匹配商品
	for _, item := range req.Items {
		var products []models.Product
		if err := tx.Where("name LIKE ? OR sku LIKE ?", "%"+item.Keyword+"%", "%"+item.Keyword+"%").Find(&products).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询商品失败: " + err.Error()})
			return
		}

		if len(products) == 0 {
			tx.Rollback()
			c.JSON(http.StatusNotFound, gin.H{
				"code": 404,
				"msg":  fmt.Sprintf("未找到包含关键字 '%s' 的商品", item.Keyword),
			})
			return
		} else if len(products) > 1 {
			tx.Rollback()
			var candidates []map[string]interface{}
			for _, p := range products {
				candidates = append(candidates, map[string]interface{}{
					"id":          p.ID,
					"name":        p.Name,
					"store_stock": p.StoreStock,
					"main_stock":  p.MainStock,
					"cloud_stock": p.CloudStock,
				})
			}
			c.JSON(http.StatusConflict, gin.H{
				"code":       409,
				"msg":        fmt.Sprintf("商品匹配多个，请明确选择: '%s'", item.Keyword),
				"candidates": candidates,
			})
			return
		}

		actualReq.Items = append(actualReq.Items, OrderItemRequest{
			ProductID: products[0].ID,
			Quantity:  item.Quantity,
			UnitPrice: item.UnitPrice,
		})
	}

	if actualReq.ReferrerName == "" {
		actualReq.ReferrerName = "老板客户"
		actualReq.NoCommission = true
	}

	tx.Rollback() // 结束只读事务

	// Encode to byte array
	jsonBytes, err := json.Marshal(actualReq)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "内部编码错误"})
		return
	}

	// 替换 Request Body 并调用正常 CreateOrder
	c.Request.Body = io.NopCloser(bytes.NewBuffer(jsonBytes))
	CreateOrder(c)
}
