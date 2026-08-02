package controllers

import (
	"encoding/json"
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// PaymentBreakdownItem 订单收款分摊明细项
type PaymentBreakdownItem struct {
	PaymentMethod int     `json:"payment_method"`
	AccountID     uint    `json:"account_id"`
	Amount        float64 `json:"amount"`
}

func encodePaymentBreakdown(items []PaymentBreakdownItem) string {
	data, err := json.Marshal(items)
	if err != nil {
		return ""
	}
	return string(data)
}

// getOrderPaymentBreakdown 解析订单收款账户分摊：优先用 payment_breakdown JSON，
// 否则回退到单账户 (order.AccountID)。
func getOrderPaymentBreakdown(tx *gorm.DB, order models.Order, fallbackAmount float64) ([]PaymentBreakdownItem, error) {
	if order.PaymentBreakdown != "" {
		var splits []PaymentBreakdownItem
		if err := json.Unmarshal([]byte(order.PaymentBreakdown), &splits); err == nil && len(splits) > 0 {
			return splits, nil
		}
	}
	if order.AccountID == nil {
		return nil, fmt.Errorf("订单 %s 既无 payment_breakdown 也无 account_id", order.OrderNo)
	}
	return []PaymentBreakdownItem{{
		PaymentMethod: order.PaymentMethod,
		AccountID:     *order.AccountID,
		Amount:        fallbackAmount,
	}}, nil
}

// OrderItemRequest 购物车单个商品项
type OrderItemRequest struct {
	ProductID uint    `json:"product_id" binding:"required"`
	Quantity  int     `json:"quantity" binding:"required,gt=0"`
	UnitPrice float64 `json:"unit_price"`
	IsGift    bool    `json:"is_gift"`
}

// CreateOrderRequest 提交开单请求体
type CreateOrderRequest struct {
	Items         []OrderItemRequest `json:"items" binding:"required,dive"`
	IsSubsidy     bool               `json:"is_subsidy"`
	DepositAmount float64            `json:"deposit_amount"`
	CustomerName  string             `json:"customer_name"`
	CustomerPhone string             `json:"customer_phone"`
	CustomerAddress string           `json:"customer_address"`
	EmployeeID    *uint              `json:"employee_id"`
	ReferrerName  string             `json:"referrer_name"`
	ReferralFee   float64            `json:"referral_fee"`
	// 支付方式: 1-支付宝, 2-微信, 3-公户
	PaymentMethod int `json:"payment_method" binding:"required"`
	// 发货方式: 1-门店自提, 2-主仓发货, 3-云仓代发
	DeliveryMethod int `json:"delivery_method" binding:"required,oneof=1 2 3"`
	BypassPriceCheck bool `json:"bypass_price_check"`
	// 新增：以旧换新相关字段
	TradeInAmount float64 `json:"trade_in_amount"`
	TradeInType   string  `json:"trade_in_type"`
	TradeInRemark string  `json:"trade_in_remark"`
}

// CreateOrder 提交开单接口
func CreateOrder(c *gin.Context) {
	var req CreateOrderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "请求参数格式错误: " + err.Error(),
		})
		return
	}

	// 1. 开启事务
	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 2. 锁定并验证账户
	var account models.Account
	if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&account, req.PaymentMethod).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "收款账户不存在，请检查支付方式",
		})
		return
	}

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
			WriteInventoryLog(tx, product.ID, req.DeliveryMethod, "sale", beforeQty, changeQty, product.LatestCost, "", "销售开单")
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
			UnitCost:          product.LatestCost,
			Quantity:          item.Quantity,
			CloudDeductionQty: cloudDeductionQty,
			IsGift:            item.IsGift,
		})
	}

	// 4. 计算国补金额
	// 规则：15% 补贴，最高 1500 (即总价超过 10000 时封顶)
	var subsidyAmount float64
	if req.IsSubsidy {
		if totalAmount <= 10000 {
			subsidyAmount = totalAmount * 0.15
		} else {
			subsidyAmount = 1500
		}
	}

	actualPayAmount := totalAmount - subsidyAmount - req.TradeInAmount
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

	accountID := uint(req.PaymentMethod)

	// 5.5 计算默认业务提成金额
	var commissionAmount float64
	if req.EmployeeID != nil {
		var employee models.Employee
		if err := tx.First(&employee, *req.EmployeeID).Error; err == nil {
			commissionAmount = actualPayAmount * employee.CommissionRate
		}
	}

	// 6. 生成订单号并落库
	orderNo := fmt.Sprintf("ORD%d", time.Now().UnixNano()/1e6)
	order := models.Order{
		OrderNo:          orderNo,
		CustomerName:     req.CustomerName,
		CustomerPhone:    req.CustomerPhone,
		CustomerAddress:  req.CustomerAddress,
		TotalAmount:      totalAmount,
		SubsidyAmount:    subsidyAmount,
		ActualPayAmount:  actualPayAmount,
		DepositAmount:    req.DepositAmount,
		PaymentStatus:    paymentStatus,
		SubsidyStatus:    subsidyStatus,
		PaymentMethod:    req.PaymentMethod,
		DeliveryMethod:   req.DeliveryMethod,
		OrderStatus:      1, // 正常
		AccountID:        &accountID,
		EmployeeID:       req.EmployeeID,
		ReferrerName:     req.ReferrerName,
		ReferralFee:      req.ReferralFee,
		CommissionAmount: commissionAmount,
		TradeInAmount:    req.TradeInAmount,
		TradeInType:      req.TradeInType,
		TradeInRemark:    req.TradeInRemark,
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
	receiptAmount := req.DepositAmount
	category := "预付定金"
	if paymentStatus == 2 {
		// 全款：收取实际应付金额
		receiptAmount = actualPayAmount
		category = "销售货款"
	}
	if err := tx.Model(&account).UpdateColumn("balance", gorm.Expr("balance + ?", receiptAmount)).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "账户余额更新失败",
		})
		return
	}

	// 获取更新后的账户余额
	var updatedAccount models.Account
	tx.First(&updatedAccount, accountID)
	logRemark := fmt.Sprintf("订单 %s 收款", orderNo)
	if order.TradeInAmount > 0 {
		logRemark = fmt.Sprintf("订单 %s 收款 (含以旧换新折抵 ¥%.2f)", orderNo, order.TradeInAmount)
	}
	if err := WriteFinancialLog(tx, accountID, 1, category, receiptAmount, updatedAccount.Balance, orderNo, logRemark); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "财务流水写入失败"})
		return
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

	// 10. 提交事务
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "事务提交失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":     200,
		"msg":      "下单成功",
		"order_no": orderNo,
		"data":     order,
	})
}

// GetOrders 获取所有订单列表
func GetOrders(c *gin.Context) {
	keyword := c.Query("keyword")
	subsidyStatus := c.Query("subsidy_status")
	paymentStatus := c.Query("payment_status")
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	query := config.DB.Preload("OrderItems.Product").Preload("OrderItems.DeliveryPerson").Preload("OrderItems.Installer").Preload("Employee").Preload("Account").Preload("Installer").Preload("DeliveryPerson").Order("created_at desc")
	if keyword != "" {
		k := "%" + keyword + "%"
		query = query.Where(
			"order_no LIKE ? OR customer_name LIKE ? OR customer_phone LIKE ? OR id IN (SELECT order_id FROM order_items JOIN products ON order_items.product_id = products.id LEFT JOIN categories ON products.category_id = categories.id WHERE products.name LIKE ? OR categories.name LIKE ?)",
			k, k, k, k, k,
		)
	}
	if subsidyStatus != "" {
		query = query.Where("subsidy_status = ?", subsidyStatus)
	}
	if paymentStatus != "" {
		query = query.Where("payment_status = ?", paymentStatus)
	}
	if startDate != "" {
		query = query.Where("created_at >= ?", startDate)
	}
	if endDate != "" {
		query = query.Where("created_at <= ?", endDate)
	}

	var orders []models.Order
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
	TargetStatus int `json:"target_status" binding:"required,oneof=2 3"`
}

// UpdateSubsidyStatus 更新国补状态（提交资料 / 核销回款）
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

	// 2. 如果状态变更为 "已回款(3)"，则自动将款项打入公户
	if req.TargetStatus == 3 && order.SubsidyAmount > 0 {
		var publicAcc models.Account
		// 寻找名为 "公户" 的账户
		if err := tx.Where("name = ?", "公户").First(&publicAcc).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "未找到名为「公户」的账户，无法完成资金核销"})
			return
		}

		// 增加余额
		if err := tx.Model(&publicAcc).UpdateColumn("balance", gorm.Expr("balance + ?", order.SubsidyAmount)).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "公户余额更新失败"})
			return
		}

		// 写入财务流水
		var updatedAcc models.Account
		tx.First(&updatedAcc, publicAcc.ID)
		if err := WriteFinancialLog(tx, publicAcc.ID, 1, "国补回款", order.SubsidyAmount, updatedAcc.Balance, order.OrderNo, fmt.Sprintf("订单 %s 政府补贴款项回流", order.OrderNo)); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "财务流水写入失败"})
			return
		}
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
	// 最终全款支付方式: 1-支付宝, 2-微信, 3-公户
	FinalPaymentMethod int `json:"final_payment_method" binding:"required"`
}

// PayOrderBalance 结清尾款 —— "退定再结全额"模式
// 第一步：从原收款账户退还定金（记录支出）
// 第二步：向新选账户收取全款（actual_pay_amount）
// 第三步：更新订单状态
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

	var order models.Order
	if err := config.DB.Set("gorm:query_option", "FOR UPDATE").First(&order, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "订单不存在",
		})
		return
	}

	// 如果已经是已结全款 (2)
	if order.PaymentStatus == 2 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "订单已结清",
		})
		return
	}

	// 如果订单已取消
	if order.OrderStatus == 2 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "订单已取消，无法操作",
		})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// ========== 第一步：定金对冲退款（写入冲销配对日志，排除于总收支汇总）==========
	if order.DepositAmount > 0 {
		depositSplits, err := getOrderPaymentBreakdown(tx, order, order.DepositAmount)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询原账户失败"})
			return
		}

		for _, split := range depositSplits {
			// 1a. 锁定原账户
			var oldAccount models.Account
			if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&oldAccount, split.AccountID).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{
					"code": 500,
					"msg":  fmt.Sprintf("定金账户(ID %d)查询失败，无法完成结算", split.AccountID),
				})
				return
			}
			if oldAccount.Balance < split.Amount {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{
					"code": 400,
					"msg":  fmt.Sprintf("账户「%s」余额 ¥%.2f 不足以退还定金 ¥%.2f，无法完成结算，请先补充账户余额或调整收款账户", oldAccount.Name, oldAccount.Balance, split.Amount),
				})
				return
			}

			// 1b. 查找该订单/账户对应的原定金流水（必须是未被配对的）
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

			// 1c. 扣减原账户余额
			if err := tx.Model(&models.Account{}).Where("id = ?", split.AccountID).
				UpdateColumn("balance", gorm.Expr("balance - ?", split.Amount)).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{
					"code": 500,
					"msg":  "原账户定金退还失败",
				})
				return
			}

			// 1d. 用 WriteReversalLog：自动把原 depositLog 标记 reversal_pair_id = depositLog.ID，
			// 并写入冲销日志共享同一 pair_id
			var updatedAccount models.Account
			tx.First(&updatedAccount, split.AccountID)
			if err := WriteReversalLog(tx,
				depositLog.ID,
				split.AccountID,
				2, // flow_type=支出
				"退还定金",
				split.Amount,
				updatedAccount.Balance,
				order.OrderNo,
				fmt.Sprintf("订单 %s 结算前退还定金 ¥%.2f", order.OrderNo, split.Amount),
			); err != nil {
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
	finalAccountID := uint(req.FinalPaymentMethod)

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
		"deposit_amount": 0,                      // 定金已退，账面清零
		"account_id":     finalAccountID,         // 关联到最终收款账户
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
		"msg":  fmt.Sprintf("结算完成：已退还定金 ¥%.2f，已收取全款 ¥%.2f", order.DepositAmount, order.ActualPayAmount),
	})
}

// cancelOrderWithOptions 取消订单共享实现
// actionName: 在响应消息与流水备注中使用的动词（"取消"/"红冲"）
// allowFulfilled: 为 true 时跳过 IsDelivered/IsInstalled 校验（用于红冲已结清订单）
func cancelOrderWithOptions(c *gin.Context, actionName string, allowFulfilled bool) {
	id := c.Param("id")

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1. 查询订单（锁定行，加载 OrderItems）
	var order models.Order
	if err := tx.Set("gorm:query_option", "FOR UPDATE").Preload("OrderItems").First(&order, id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "订单不存在",
		})
		return
	}

	// 2. 检查订单状态
	if order.OrderStatus == 2 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "订单已取消，请勿重复操作",
		})
		return
	}

	// 2.5 检查生命周期状态：已送货或已安装商品，普通取消不允许；
	// 红冲路径 (allowFulfilled=true) 跳过该校验
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

	// 3. 库存回滚：将每个商品的库存加回去 (基于订单当时的发货方式回滚)
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
				tx.First(&product, item.ProductID)
				WriteInventoryLog(tx, item.ProductID, 3, "cancel", product.CloudStock-item.CloudDeductionQty, item.CloudDeductionQty, product.LatestCost, order.OrderNo, "订单取消云仓库存回滚")
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
		tx.First(&product, item.ProductID)
		beforeQty := 0
		warehouseType := order.DeliveryMethod
		if warehouseType == 1 {
			beforeQty = product.StoreStock - item.Quantity
		} else if warehouseType == 2 {
			beforeQty = product.MainStock - item.Quantity
		} else if warehouseType == 3 {
			beforeQty = product.CloudStock - item.Quantity
		}

		WriteInventoryLog(tx, item.ProductID, warehouseType, "cancel", beforeQty, item.Quantity, product.LatestCost, order.OrderNo, "订单取消库存回滚")
	}

	// 4. 退款处理：根据支付状态决定退款金额
	// 已结全款(2)：退还 actual_pay_amount（因为结算后 deposit_amount 已清零）
	// 仅付定金(1)：退还 deposit_amount
	var refundAmount float64
	if order.PaymentStatus == 2 {
		refundAmount = order.ActualPayAmount
	} else {
		refundAmount = order.DepositAmount
	}

	if refundAmount > 0 {
		// 按订单收款账户分摊逐笔回退；支持单账户 / 多账户混合支付
		refundSplits, err := getOrderPaymentBreakdown(tx, order, refundAmount)
		if err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询订单收款账户失败"})
			return
		}

		for _, split := range refundSplits {
			// 校验原收款账户余额是否足够退款
			var account models.Account
			if err := tx.Set("gorm:query_option", "FOR UPDATE").First(&account, split.AccountID).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询退款账户失败"})
				return
			}
			if account.Balance < split.Amount {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": fmt.Sprintf("退款账户「%s」余额不足，无法%s订单", account.Name, actionName)})
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

			// 写入退款流水：若找到对应的"销售货款"收款流水，使用 WriteReversalLog 配对
			// 配对后该笔销售 / 退款在 dashboard 中按 WHERE reversal_pair_id IS NULL 排除
			var updatedAccount models.Account
			tx.First(&updatedAccount, split.AccountID)

			var collectLog models.FinancialLog
			if err := tx.Where("related_no = ? AND account_id = ? AND category = ? AND reversal_pair_id IS NULL",
				order.OrderNo, split.AccountID, "销售货款").
				Order("id DESC").
				First(&collectLog).Error; err != nil {
				// 找不到原 collect log 时 (例如仅付定金的订单、或老数据无销售货款流水)，
				// 回退为普通退款日志，不参与配对，保持向后兼容
				if err := WriteFinancialLog(tx, split.AccountID, 2, "订单退款",
					split.Amount, updatedAccount.Balance, order.OrderNo,
					fmt.Sprintf("订单 %s %s，退款 ¥%.2f", order.OrderNo, actionName, split.Amount)); err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "退款流水写入失败"})
					return
				}
			} else {
				if err := WriteReversalLog(tx, collectLog.ID,
					split.AccountID, 2, "订单退款", split.Amount,
					updatedAccount.Balance, order.OrderNo,
					fmt.Sprintf("订单 %s %s，反向退款 ¥%.2f", order.OrderNo, actionName, split.Amount),
				); err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "退款配对流水写入失败"})
					return
				}
			}
		}
	}

	// 5. 更新订单状态为已取消
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
		"msg":  fmt.Sprintf("订单已%s，库存已回滚，已退还 ¥%.2f", actionName, refundAmount),
	})
}

// CancelOrder 普通取消订单接口（不允许已发货 / 已安装）
func CancelOrder(c *gin.Context) {
	cancelOrderWithOptions(c, "取消", false)
}

// ReversalCancelOrder 红冲取消订单接口（允许已发货 / 已安装，配合 WriteReversalLog 用于已结清订单的反向对账）
func ReversalCancelOrder(c *gin.Context) {
	cancelOrderWithOptions(c, "红冲", true)
}

// UpdateDeliveryStatusRequest 确认送货请求体
type UpdateDeliveryStatusRequest struct {
	DeliveryPersonID uint       `json:"delivery_person_id" binding:"required"`
	DeliveryFee      float64    `json:"delivery_fee"`
	DeliveryTime     *time.Time `json:"delivery_time"`
}

// UpdateDeliveryStatus 确认送货接口
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

	var order models.Order
	if err := config.DB.Set("gorm:query_option", "FOR UPDATE").First(&order, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "订单不存在",
		})
		return
	}

	if order.OrderStatus == 2 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "订单已取消，无法操作",
		})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

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
	InstallerID      uint       `json:"installer_id" binding:"required"`
	InstallTime      *time.Time `json:"install_time"`
	InstallationFee  float64    `json:"installation_fee"`
}

// UpdateInstallStatus 确认安装接口
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

	var order models.Order
	if err := config.DB.Set("gorm:query_option", "FOR UPDATE").First(&order, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "订单不存在",
		})
		return
	}

	if order.OrderStatus == 2 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "订单已取消，无法操作",
		})
		return
	}

	if !order.IsDelivered {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "订单尚未送货，请先进行送货操作",
		})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	installTime := time.Now()
	if req.InstallTime != nil {
		installTime = *req.InstallTime
	}

	updates := map[string]interface{}{
		"is_installed":       true,
		"installer_id":       req.InstallerID,
		"install_time":       installTime,
		"installation_fee":   req.InstallationFee,
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
// UpdateCommissionRequest 更新提成金额请求体
type UpdateCommissionRequest struct {
	CommissionAmount float64 `json:"commission_amount" binding:"min=0"`
}

// UpdateOrderCommission 手动调整订单提成金额
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

// UpdateItemDeliveryStatus 确认单品送货接口
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

	var order models.Order
	if err := config.DB.Set("gorm:query_option", "FOR UPDATE").First(&order, item.OrderID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "关联订单不存在",
		})
		return
	}

	if order.OrderStatus == 2 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "订单已取消，无法操作",
		})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

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

	var order models.Order
	if err := config.DB.Set("gorm:query_option", "FOR UPDATE").First(&order, item.OrderID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "关联订单不存在",
		})
		return
	}

	if order.OrderStatus == 2 {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "订单已取消，无法操作",
		})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

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

