package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

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
	EmployeeID    *uint              `json:"employee_id"`
	ReferrerName  string             `json:"referrer_name"`
	ReferralFee   float64            `json:"referral_fee"`
	// 支付方式: 1-支付宝, 2-微信, 3-公户
	PaymentMethod int `json:"payment_method" binding:"required"`
	// 发货方式: 1-门店自提, 2-主仓发货, 3-云仓代发
	DeliveryMethod int `json:"delivery_method" binding:"required,oneof=1 2 3"`
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

		unitPrice := item.UnitPrice
		if item.IsGift {
			unitPrice = 0
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

	actualPayAmount := totalAmount - subsidyAmount

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

	// 6. 生成订单号并落库
	orderNo := fmt.Sprintf("ORD%d", time.Now().UnixNano()/1e6)
	order := models.Order{
		OrderNo:         orderNo,
		CustomerName:    req.CustomerName,
		CustomerPhone:   req.CustomerPhone,
		TotalAmount:     totalAmount,
		SubsidyAmount:   subsidyAmount,
		ActualPayAmount: actualPayAmount,
		DepositAmount:   req.DepositAmount,
		PaymentStatus:   paymentStatus,
		SubsidyStatus:   subsidyStatus,
		PaymentMethod:   req.PaymentMethod,
		DeliveryMethod:  req.DeliveryMethod,
		OrderStatus:     1, // 正常
		AccountID:       &accountID,
		EmployeeID:      req.EmployeeID,
		ReferrerName:    req.ReferrerName,
		ReferralFee:     req.ReferralFee,
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
	if err := WriteFinancialLog(tx, accountID, 1, category, receiptAmount, updatedAccount.Balance, orderNo, fmt.Sprintf("订单 %s 收款", orderNo)); err != nil {
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

	query := config.DB.Preload("OrderItems.Product").Preload("Employee").Preload("Account").Preload("Installer").Order("created_at desc")
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
	if err := config.DB.First(&order, id).Error; err != nil {
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

	// ========== 第一步：定金对冲退款 ==========
	if order.DepositAmount > 0 && order.AccountID != nil {
		// 1a. 从原账户扣除定金
		if err := tx.Model(&models.Account{}).Where("id = ?", *order.AccountID).
			UpdateColumn("balance", gorm.Expr("balance - ?", order.DepositAmount)).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"code": 500,
				"msg":  "原账户定金退还失败",
			})
			return
		}

		// 1b. 插入"定金退还"流水记录
		var oldAccount models.Account
		tx.First(&oldAccount, *order.AccountID)
		if err := WriteFinancialLog(tx, *order.AccountID, 2, "退还定金", order.DepositAmount, oldAccount.Balance, order.OrderNo, fmt.Sprintf("订单 %s 结算前退还定金 ¥%.2f", order.OrderNo, order.DepositAmount)); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"code": 500,
				"msg":  "定金退还记录写入失败",
			})
			return
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

// CancelOrder 取消订单接口
func CancelOrder(c *gin.Context) {
	id := c.Param("id")

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1. 查询订单（加载 OrderItems）
	var order models.Order
	if err := tx.Preload("OrderItems").First(&order, id).Error; err != nil {
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

	if refundAmount > 0 && order.AccountID != nil {
		// 从账户扣除已收款项 (退款)
		if err := tx.Model(&models.Account{}).Where("id = ?", *order.AccountID).
			UpdateColumn("balance", gorm.Expr("balance - ?", refundAmount)).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"code": 500,
				"msg":  "退款账户余额扣除失败",
			})
			return
		}

		// 插入一条"订单取消退款"的流水记录
		var updatedAccount models.Account
		tx.First(&updatedAccount, *order.AccountID)
		if err := WriteFinancialLog(tx, *order.AccountID, 2, "订单退款", refundAmount, updatedAccount.Balance, order.OrderNo, fmt.Sprintf("订单 %s 取消，退款 ¥%.2f", order.OrderNo, refundAmount)); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{
				"code": 500,
				"msg":  "退款记录写入失败",
			})
			return
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
		"msg":  fmt.Sprintf("订单已取消，库存已回滚，已退还 ¥%.2f", refundAmount),
	})
}

// UpdateInstallStatusRequest 确认安装请求体
type UpdateInstallStatusRequest struct {
	InstallerID uint `json:"installer_id" binding:"required"`
}

// UpdateInstallStatus 确认安装接口
func UpdateInstallStatus(c *gin.Context) {
	id := c.Param("id")

	var req UpdateInstallStatusRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "请选择安装师傅",
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

	now := time.Now()
	updates := map[string]interface{}{
		"is_installed": true,
		"installer_id": req.InstallerID,
		"install_time": now,
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
