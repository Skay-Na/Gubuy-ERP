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
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
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

func cancellationRestockCost(item models.OrderItem, product models.Product) float64 {
	if item.UnitCost > 0 {
		return item.UnitCost
	}
	// Legacy zero-cost orders can be reversed, but the returned stock is valued
	// at the best current product cost without rewriting the historical order.
	return orderCostSnapshot(product)
}

// commissionableOrderAmount applies order-level reductions proportionally to
// commissionable products. Subsidized orders retain the original sales amount
// as their commission basis, matching the existing commission rule.
func commissionableOrderAmount(
	totalAmount float64,
	actualPayAmount float64,
	commissionableSales float64,
	isSubsidy bool,
) float64 {
	if totalAmount <= 0 || commissionableSales <= 0 {
		return 0
	}

	commissionBase := actualPayAmount
	if isSubsidy {
		commissionBase = totalAmount
	}
	return commissionBase * math.Min(commissionableSales, totalAmount) / totalAmount
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

// paymentMethodName 返回支付方式编号对应的规范收款账户名（与 seedAccounts 初始化的账户一致）。
// 返回空字符串表示该编号未配置名称，调用方应返回明确错误而不是回退查询。
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
	AIImportID      uint               `json:"ai_import_id"`
	AgentActionID   uint               `json:"agent_action_id"`
	Items           []OrderItemRequest `json:"items" binding:"required,dive"`
	IsSubsidy       bool               `json:"is_subsidy"`
	DepositAmount   float64            `json:"deposit_amount"`
	CustomerName    string             `json:"customer_name"`
	CustomerPhone   string             `json:"customer_phone"`
	CustomerAddress string             `json:"customer_address"`
	EmployeeID      *uint              `json:"employee_id"`
	ReferrerName    string             `json:"referrer_name"`
	NoCommission    bool               `json:"no_commission"`
	// 支付方式: 1-支付宝, 2-微信, 3-公户, 5-现金
	PaymentMethod int `json:"payment_method" binding:"required"`
	// 新增：收款账户ID（解耦支付方式与账户）
	AccountID *uint `json:"account_id"`
	// 混合支付拆分；为空时沿用 payment_method/account_id 单一支付
	PaymentSplits []PaymentSplitRequest `json:"payment_splits"`
	// 发货方式: 1-门店自提, 2-历史主仓发货, 3-历史云仓代发, 4-云仓发货
	DeliveryMethod   int  `json:"delivery_method" binding:"required,oneof=1 2 3 4"`
	BypassPriceCheck bool `json:"bypass_price_check"`
	// 新增：以旧换新相关字段
	TradeInAmount    float64 `json:"trade_in_amount"`
	TradeInAccountID *uint   `json:"trade_in_account_id"`
	TradeInType      string  `json:"trade_in_type"`
	TradeInRemark    string  `json:"trade_in_remark"`
	DiscountAmount   float64 `json:"discount_amount"` // 新增：手动抹零优惠
	// 新增：订单类型 (1-普通订单, 2-商会订单)
	OrderType int `json:"order_type"`
	// 客户端幂等键（可选）：同一键重复提交时直接返回已有订单，防止双击重复开单
	ClientRequestID string `json:"client_request_id" binding:"max=64"`
}

// orderDeliveryToWarehouse 将发货方式转换为库存流水的仓库类型。
func orderDeliveryToWarehouse(deliveryMethod int) int {
	switch deliveryMethod {
	case 1:
		return 2
	case 2:
		return 1
	case 3, 4:
		return 3
	}
	return deliveryMethod
}

func resolvePaymentAccount(tx *gorm.DB, paymentMethod int, accountID *uint) (models.Account, error) {
	return lookupPaymentAccount(tx, paymentMethod, accountID, true)
}

// lookupPaymentAccount 按显式账户 ID 或支付方式编号解析收款账户。
// 统一入口：CreateOrder 与 PayOrderBalance 共用，避免两处解析行为漂移。
// 解析规则：
//  1. accountID 非空时按账户主键精确查找；
//  2. 否则按支付方式编号映射到规范账户名（支付宝/微信/公户/现金）精确匹配；
//  3. 查不到一律返回明确错误，绝不允许把"支付方式编号"当账户主键回退查询
//     （旧逻辑在账户表 id 与方式编号错位时会把钱记到错误账户，属于假账隐患）。
func lookupPaymentAccount(tx *gorm.DB, paymentMethod int, accountID *uint, lock bool) (models.Account, error) {
	query := tx
	if lock {
		query = query.Clauses(clause.Locking{Strength: "UPDATE"})
	}

	var account models.Account
	var err error
	if accountID != nil {
		err = query.First(&account, *accountID).Error
		if err != nil {
			return account, fmt.Errorf("收款账户(ID %d)不存在: %w", *accountID, err)
		}
		return account, nil
	}

	name := paymentMethodName(paymentMethod)
	if name == "" {
		return account, fmt.Errorf("不支持的支付方式编号 %d", paymentMethod)
	}
	err = query.Where("name = ?", name).First(&account).Error
	if err != nil {
		return account, fmt.Errorf("未找到与支付方式「%s」对应的收款账户，请检查账户配置", name)
	}
	return account, nil
}

func lockOrderAccounts(tx *gorm.DB, req CreateOrderRequest) error {
	requests := req.PaymentSplits
	if len(requests) == 0 {
		if req.PaymentMethod == 4 {
			return nil
		}
		requests = []PaymentSplitRequest{{PaymentMethod: req.PaymentMethod, AccountID: req.AccountID}}
	}
	accountIDs := make(map[uint]struct{}, len(requests)+1)
	for _, split := range requests {
		account, err := lookupPaymentAccount(tx, split.PaymentMethod, split.AccountID, false)
		if err != nil {
			return err
		}
		accountIDs[account.ID] = struct{}{}
	}
	if req.TradeInAmount > 0 {
		if req.TradeInAccountID == nil || *req.TradeInAccountID == 0 {
			return fmt.Errorf("以旧换新必须选择实际付款账户")
		}
		accountIDs[*req.TradeInAccountID] = struct{}{}
	}
	ids := make([]uint, 0, len(accountIDs))
	for id := range accountIDs {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	for _, id := range ids {
		var account models.Account
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, id).Error; err != nil {
			return err
		}
	}
	return nil
}

func normalizePaymentSplits(tx *gorm.DB, req CreateOrderRequest, receiptAmount, subsidyAmount float64) ([]PaymentBreakdownItem, error) {
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

		// 国补混付时，公户填写的是 POS 原始刷卡金额（含国补）。
		// 国补部分不会实际到账公户，订单收款、账户余额和财务流水必须记录净到账。
		netAmount := split.Amount
		if req.IsSubsidy && req.PaymentMethod == 4 && req.DepositAmount == 0 && split.PaymentMethod == 3 {
			netAmount -= subsidyAmount
		}
		if netAmount < 0 {
			return nil, fmt.Errorf("公户原始金额不足以扣除国补 ¥%.2f", subsidyAmount)
		}

		total += netAmount
		splits = append(splits, PaymentBreakdownItem{
			PaymentMethod: split.PaymentMethod,
			AccountID:     account.ID,
			Amount:        netAmount,
		})
	}
	if math.Abs(total-receiptAmount) > 0.01 {
		return nil, fmt.Errorf("混合支付净到账金额合计 ¥%.2f 与本次应收 ¥%.2f 不一致", total, receiptAmount)
	}
	return splits, nil
}

func createTradeInExpense(tx *gorm.DB, orderNo string, req CreateOrderRequest, occurredAt time.Time) error {
	if req.TradeInAmount <= 0 {
		return nil
	}
	if req.TradeInAccountID == nil || *req.TradeInAccountID == 0 {
		return fmt.Errorf("以旧换新必须选择实际付款账户")
	}

	var account models.Account
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, *req.TradeInAccountID).Error; err != nil {
		return fmt.Errorf("查询以旧换新付款账户: %w", err)
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
	if err := tx.First(&updatedAccount, account.ID).Error; err != nil {
		return fmt.Errorf("读取以旧换新账户余额: %w", err)
	}
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
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, *order.AccountID).Error; err != nil {
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
	if req.BypassPriceCheck && strings.HasPrefix(c.GetString("username"), "emp_") {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "低于成本开单需要管理员授权"})
		return
	}
	if req.TradeInAmount > 0 && (req.TradeInAccountID == nil || *req.TradeInAccountID == 0) {
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "以旧换新必须选择实际付款账户",
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
	operator := InventoryOperatorFromContext(c)

	// 1. 开启事务
	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()
	if err := lockAIImport(tx, req.AIImportID, "order", c.GetString("username")); err != nil {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": err.Error()})
		return
	}
	if err := lockAgentAction(tx, req.AgentActionID, c.GetString("username"), "order"); err != nil {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": err.Error()})
		return
	}

	// 1.2 开单幂等：请求携带 client_request_id 时，事务内先按该键查询，
	// 已存在订单则直接返回（duplicated:true），不重复扣库存、不重复收款。
	req.ClientRequestID = strings.TrimSpace(req.ClientRequestID)
	if req.ClientRequestID != "" {
		var existing models.Order
		idempotencyErr := tx.Where("client_request_id = ?", req.ClientRequestID).First(&existing).Error
		if idempotencyErr == nil {
			tx.Rollback()
			var existingItems []models.OrderItem
			config.DB.Where("order_id = ?", existing.ID).Find(&existingItems)
			c.JSON(http.StatusOK, gin.H{
				"code":       200,
				"msg":        "该请求已提交过，返回已有订单，未重复创建",
				"order_no":   existing.OrderNo,
				"duplicated": true,
				"data": gin.H{
					"order": existing,
					"items": existingItems,
				},
			})
			return
		}
		if !errors.Is(idempotencyErr, gorm.ErrRecordNotFound) {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "幂等键查询失败"})
			return
		}
	}

	// 1.5 提前生成订单号（用于关联流水）
	orderTime := time.Now()
	if req.EmployeeID != nil && *req.EmployeeID > 0 {
		var assignedEmployee models.Employee
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&assignedEmployee, *req.EmployeeID).Error; err != nil || !assignedEmployee.CanUseEmployeeAccess(orderTime) {
			tx.Rollback()
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "开单员工已离职或超过最后工作日，不能继续归属新订单"})
			return
		}
	}
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
	var commissionableSales float64
	orderItems := make([]models.OrderItem, 0)

	// Lock product rows in a stable order so concurrent multi-item orders cannot
	// acquire the same rows in opposite orders and deadlock.
	productIDs := make([]uint, 0, len(req.Items))
	seenProductIDs := make(map[uint]struct{}, len(req.Items))
	for _, item := range req.Items {
		if _, exists := seenProductIDs[item.ProductID]; exists {
			continue
		}
		seenProductIDs[item.ProductID] = struct{}{}
		productIDs = append(productIDs, item.ProductID)
	}
	sort.Slice(productIDs, func(i, j int) bool { return productIDs[i] < productIDs[j] })
	for _, productID := range productIDs {
		var product models.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, productID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": fmt.Sprintf("商品 ID %d 不存在", productID)})
			return
		}
	}

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
		unitCost := orderCostSnapshot(product)
		if unitCost <= 0 {
			tx.Rollback()
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": fmt.Sprintf("商品 %s 尚未建立有效成本，请先补录商品平均成本或入库成本", product.Name)})
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
		} else if req.DeliveryMethod == 2 { // 历史订单兼容：主仓发货
			if product.MainStock < item.Quantity {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": fmt.Sprintf("商品 %s 主仓库存不足 (当前: %d)", product.Name, product.MainStock)})
				return
			}
			result := tx.Model(&product).Where("main_stock >= ?", item.Quantity).UpdateColumn("main_stock", gorm.Expr("main_stock - ?", item.Quantity))
			if result.Error != nil || result.RowsAffected == 0 {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "库存扣减失败，因并发导致库存不足"})
				return
			}
		} else if req.DeliveryMethod == 3 { // 历史订单兼容：仅云仓代发
			if product.CloudStock > 0 {
				cloudDeductionQty = item.Quantity
				if product.CloudStock < cloudDeductionQty {
					cloudDeductionQty = product.CloudStock
				}
				result := tx.Model(&product).Where("cloud_stock >= ?", cloudDeductionQty).UpdateColumn("cloud_stock", gorm.Expr("cloud_stock - ?", cloudDeductionQty))
				if result.Error != nil || result.RowsAffected == 0 {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "云仓库存扣减失败"})
					return
				}
			}
		} else if req.DeliveryMethod == 4 { // 优先扣减云仓实物库存，不足部分由云端代发且不设库存上限
			if product.CloudStock > 0 {
				cloudDeductionQty = item.Quantity
				if product.CloudStock < cloudDeductionQty {
					cloudDeductionQty = product.CloudStock
				}
				result := tx.Model(&product).Where("cloud_stock >= ?", cloudDeductionQty).UpdateColumn("cloud_stock", gorm.Expr("cloud_stock - ?", cloudDeductionQty))
				if result.Error != nil || result.RowsAffected == 0 {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "云仓库存扣减失败，因并发导致库存不足"})
					return
				}
			}
		}

		// 记录库存流水
		beforeQty := 0
		changeQty := -item.Quantity
		if req.DeliveryMethod == 1 {
			beforeQty = product.StoreStock
			if err := WriteInventoryLog(tx, product.ID, 2, "sale", beforeQty, changeQty, unitCost, orderNo, "销售开单", operator); err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "库存流水写入失败"})
				return
			}
		}
		if req.DeliveryMethod == 2 {
			if err := WriteInventoryLog(tx, product.ID, 1, "sale", product.MainStock, -item.Quantity, unitCost, orderNo, "销售开单（历史主仓）", operator); err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "库存流水写入失败"})
				return
			}
		}
		if (req.DeliveryMethod == 3 || req.DeliveryMethod == 4) && cloudDeductionQty > 0 {
			if err := WriteInventoryLog(tx, product.ID, 3, "sale", product.CloudStock, -cloudDeductionQty, unitCost, orderNo, "销售开单：扣减云仓自有库存", operator); err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "库存流水写入失败"})
				return
			}
		}
		unitPrice := item.UnitPrice
		if item.IsGift {
			unitPrice = 0
		} else {
			// 成交底价使用与订单成本快照相同的成本口径。
			if !req.BypassPriceCheck && unitPrice < unitCost {
				tx.Rollback()
				c.JSON(http.StatusBadRequest, gin.H{
					"code": 400,
					"msg":  fmt.Sprintf("商品 %s 成交价(¥%.2f)低于成本价(¥%.2f)，请联系管理员授权", product.Name, unitPrice, unitCost),
				})
				return
			}
		}

		totalAmount += unitPrice * float64(item.Quantity)
		if !product.NoCommission {
			commissionableSales += unitPrice * float64(item.Quantity)
		}
		orderItems = append(orderItems, models.OrderItem{
			ProductID:         item.ProductID,
			UnitPrice:         unitPrice,
			UnitCost:          unitCost,
			Quantity:          item.Quantity,
			CloudDeductionQty: cloudDeductionQty,
			IsGift:            item.IsGift,
			NoCommission:      product.NoCommission,
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
	if err := lockOrderAccounts(tx, req); err != nil {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "收款或以旧换新账户错误：" + err.Error()})
		return
	}
	paymentSplits, err := normalizePaymentSplits(tx, req, receiptAmount, subsidyAmount)
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
	if req.OrderType == 2 {
		commissionAmount = float64(chamberCommissionCents(commissionableSales)) / 100
	} else if req.EmployeeID != nil && !req.NoCommission {
		var employee models.Employee
		if err := tx.First(&employee, *req.EmployeeID).Error; err == nil {
			commissionBase := commissionableOrderAmount(
				totalAmount,
				actualPayAmount,
				commissionableSales,
				req.IsSubsidy,
			)
			commissionAmount = float64(moneyCents(commissionBase*employee.CommissionRate)) / 100
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
		CommissionAmount:      commissionAmount,
		TradeInAmount:         req.TradeInAmount,
		TradeInType:           req.TradeInType,
		TradeInRemark:         req.TradeInRemark,
		OrderType:             req.OrderType,
	}
	if req.ClientRequestID != "" {
		order.ClientRequestID = &req.ClientRequestID
	}

	if err := tx.Create(&order).Error; err != nil {
		// 并发场景下两个同幂等键请求可能同时通过预查询，唯一索引兜底：
		// 命中重复键时回滚并返回已存在订单。
		if req.ClientRequestID != "" && strings.Contains(err.Error(), "Duplicate entry") {
			tx.Rollback()
			var existing models.Order
			if lookupErr := config.DB.Where("client_request_id = ?", req.ClientRequestID).First(&existing).Error; lookupErr == nil {
				var existingItems []models.OrderItem
				config.DB.Where("order_id = ?", existing.ID).Find(&existingItems)
				c.JSON(http.StatusOK, gin.H{
					"code":       200,
					"msg":        "该请求已提交过，返回已有订单，未重复创建",
					"order_no":   existing.OrderNo,
					"duplicated": true,
					"data": gin.H{
						"order": existing,
						"items": existingItems,
					},
				})
				return
			}
		}
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
		if err := tx.First(&updatedAccount, split.AccountID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "读取收款后账户余额失败"})
			return
		}
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
	if err := attachOrderToActiveOffboarding(tx, order, order.EmployeeID); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "订单交接任务登记失败，开单已回滚"})
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

	if err := completeAIImport(tx, req.AIImportID, order.ID); err != nil {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": err.Error()})
		return
	}
	if err := completeAgentAction(tx, req.AgentActionID, order.ID); err != nil {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": err.Error()})
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

func applyOrderFilters(query *gorm.DB, keyword, subsidyStatus, paymentStatus, fulfillmentStatus, orderStatus, startDate, endDate, categoryID, deletedEmployeeFollowup string) *gorm.DB {
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
		case "todo":
			// 待办聚合：待结尾款 OR 国补待提交 OR 待送货 OR 待安装（与前端原口径一致）
			query = query.Where(
				"(payment_status = ? OR subsidy_status = ?"+
					" OR (delivery_method != ? AND id IN (SELECT DISTINCT order_id FROM order_items WHERE is_delivered = ?))"+
					" OR (delivery_method != ? AND id IN (SELECT DISTINCT order_id FROM order_items WHERE is_delivered = ? AND is_installed = ?)))",
				1, 1, 1, false, 1, true, false,
			)
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
			Where("products.category_id = ? OR products.category_id IN (SELECT id FROM categories WHERE parent_id = ? AND deleted_at IS NULL)", categoryID, categoryID)
		query = query.Where("id IN (?)", categoryOrders)
	}
	if deletedEmployeeFollowup == "1" {
		query = query.Where(`
			employee_id IN (SELECT id FROM employees WHERE deleted_at IS NOT NULL)
			AND order_status = ?
			AND (
				payment_status = ?
				OR subsidy_status IN (?, ?)
				OR (
					delivery_method <> ?
					AND id IN (SELECT order_id FROM order_items WHERE is_delivered = ?)
				)
				OR (
					delivery_method <> ?
					AND id IN (SELECT order_id FROM order_items WHERE is_delivered = ? AND is_installed = ?)
				)
			)`, 1, 1, 1, 2, 1, false, 1, true, false)
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
// @Param deleted_employee_followup query string false "员工档案已软删除且仍待跟进的订单（1 开启）"
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
	deletedEmployeeFollowup := c.Query("deleted_employee_followup")
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

	query = applyOrderFilters(query, keyword, subsidyStatus, paymentStatus, fulfillmentStatus, orderStatus, startDate, endDate, categoryID, deletedEmployeeFollowup)

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
	// 目标状态: 1-待提交资料, 2-已提交资料（0/3 不可通过此接口设置）
	TargetStatus int `json:"target_status" binding:"required,oneof=1 2"`
}

// subsidyStatusLabel 国补状态的中文描述（用于错误提示与日志）。
func subsidyStatusLabel(status int) string {
	switch status {
	case 0:
		return "无国补"
	case 1:
		return "待提交资料"
	case 2:
		return "已提交资料"
	case 3:
		return "已回款"
	}
	return "未知状态"
}

// validateSubsidyTransition 校验国补状态跃迁合法性：
// 只允许单向前进 0→1→2（0→2 允许，业务上存在"无国补资料直接已提交"），
// 禁止任何回退；3（已回款）只能由国补核销流程写入，本接口一律拒绝。
func validateSubsidyTransition(current, target int) error {
	if target == 3 {
		return fmt.Errorf("已回款状态由国补核销流程写入，不能通过此接口修改")
	}
	if target <= current {
		return fmt.Errorf("国补状态不允许回退或重复设置（当前「%s」，目标「%s」）",
			subsidyStatusLabel(current), subsidyStatusLabel(target))
	}
	return nil
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

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 读取移入事务内并对订单行加锁，防止并发提交资料时状态校验失效。
	var order models.Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "订单不存在",
		})
		return
	}

	// 1. 状态机校验：单向跃迁，禁止回退与跳到已回款
	if err := validateSubsidyTransition(order.SubsidyStatus, req.TargetStatus); err != nil {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  err.Error(),
		})
		return
	}

	// 2. 更新订单国补状态
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
		"msg":  fmt.Sprintf("国补状态已更新为「%s」", subsidyStatusLabel(req.TargetStatus)),
	})
}

// PayBalanceRequest 结清尾款请求体
type PayBalanceRequest struct {
	// 最终全款支付方式: 1-支付宝, 2-微信, 3-公户, 5-现金
	FinalPaymentMethod int `json:"final_payment_method" binding:"required,oneof=1 2 3 5"`
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
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, id).Error; err != nil {
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
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&refundAccount, split.AccountID).Error; err != nil {
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
	// 与开单共用同一个账户解析函数：只按支付方式映射的规范账户名查找，
	// 禁止把支付方式编号当账户主键回退查询。
	finalAcc, accErr := resolvePaymentAccount(tx, req.FinalPaymentMethod, nil)
	if accErr != nil {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{
			"code": 400,
			"msg":  "结算账户错误：" + accErr.Error(),
		})
		return
	}
	finalAccountID := finalAcc.ID

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

	order.PaymentStatus = 2
	order.AccountID = &finalAccountID

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
	operator := InventoryOperatorFromContext(c)

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var order models.Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Preload("OrderItems").First(&order, id).Error; err != nil {
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
	productIDs := make([]uint, 0, len(order.OrderItems))
	seenProductIDs := make(map[uint]struct{}, len(order.OrderItems))
	for _, item := range order.OrderItems {
		if _, exists := seenProductIDs[item.ProductID]; exists {
			continue
		}
		seenProductIDs[item.ProductID] = struct{}{}
		productIDs = append(productIDs, item.ProductID)
	}
	sort.Slice(productIDs, func(i, j int) bool { return productIDs[i] < productIDs[j] })
	uncostedProducts := make(map[uint]struct{})
	for _, productID := range productIDs {
		var product models.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, productID).Error; err != nil {
			tx.Rollback()
			c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": fmt.Sprintf("关联商品 ID %d 不存在", productID)})
			return
		}
		if orderCostSnapshot(product) <= 0 {
			uncostedProducts[productID] = struct{}{}
		} else if err := ensureProductCostBaseline(tx, product); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "建立商品成本基准失败"})
			return
		}
	}

	for _, item := range order.OrderItems {
		var updateField string
		if order.DeliveryMethod == 1 {
			updateField = "store_stock"
		} else if order.DeliveryMethod == 2 {
			// 历史主仓订单红冲后将数量归入统一后的云仓。
			updateField = "cloud_stock"
		} else if order.DeliveryMethod == 3 || order.DeliveryMethod == 4 {
			if item.StoreDeductionQty > 0 {
				if err := tx.Model(&models.Product{}).Where("id = ?", item.ProductID).
					UpdateColumn("store_stock", gorm.Expr("store_stock + ?", item.StoreDeductionQty)).Error; err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{
						"code": 500,
						"msg":  fmt.Sprintf("商品 ID %d 门店库存回滚失败", item.ProductID),
					})
					return
				}
				var product models.Product
				if err := tx.First(&product, item.ProductID).Error; err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询商品库存失败"})
					return
				}
				if err := WriteInventoryLog(tx, item.ProductID, 2, "cancel", product.StoreStock-item.StoreDeductionQty, item.StoreDeductionQty, cancellationRestockCost(item, product), order.OrderNo, "云仓发货单取消：门店库存回滚", operator); err != nil {
					tx.Rollback()
					c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "库存流水写入失败"})
					return
				}
			}

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
				if err := WriteInventoryLog(tx, item.ProductID, 3, "cancel", product.CloudStock-item.CloudDeductionQty, item.CloudDeductionQty, cancellationRestockCost(item, product), order.OrderNo, cloudInventoryRemark, operator); err != nil {
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

		if err := WriteInventoryLog(tx, item.ProductID, warehouseType, "cancel", beforeQty, item.Quantity, cancellationRestockCost(item, product), order.OrderNo, inventoryRemark, operator); err != nil {
			tx.Rollback()
			c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "库存流水写入失败"})
			return
		}
	}
	for _, productID := range productIDs {
		if _, costUnknown := uncostedProducts[productID]; costUnknown {
			continue
		}
		if err := RecalculateSingleProductCosts(tx, productID); err != nil {
			tx.Rollback()
			c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": fmt.Sprintf("商品 ID %d 成本重算失败：%s", productID, err.Error())})
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
		sort.Slice(refundSplits, func(i, j int) bool { return refundSplits[i].AccountID < refundSplits[j].AccountID })

		for _, split := range refundSplits {
			// 校验原收款账户余额是否足够退款
			var targetAccount models.Account
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&targetAccount, split.AccountID).Error; err != nil {
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
	alertType := "order_cancelled"
	if allowFulfilled {
		alertType = "order_reversed"
	}
	if err := PostExitOrderChangeAlerts(tx, order, alertType, refundAmount, nil, time.Now()); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "离职后订单告警写入失败，订单操作已回滚"})
		return
	}
	if err := resolvePendingFulfillmentAlert(tx, order.ID); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新离职订单履约告警失败，订单操作已回滚"})
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
	DeliveryFee      float64    `json:"delivery_fee" binding:"gte=0"`
	DeliveryTime     *time.Time `json:"delivery_time"`
}

type UpdateItemDeliveryFeeRequest struct {
	DeliveryFee *float64 `json:"delivery_fee" binding:"required,gte=0"`
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
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, id).Error; err != nil {
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
	if err := validateActiveEmployeeAssignment(tx, req.DeliveryPersonID, time.Now()); err != nil {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": err.Error()})
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
	order.IsDelivered = true
	if err := resolvePendingFulfillmentAlert(tx, order.ID); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新离职订单履约告警失败"})
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
	InstallationFee float64    `json:"installation_fee" binding:"gte=0"`
}

type UpdateItemInstallationFeeRequest struct {
	InstallationFee *float64 `json:"installation_fee" binding:"required,gte=0"`
}

// UpdateItemInstallationFee 修改订单项安装费
// @Summary 修改订单项安装费
// @Description 修改订单项安装费
// @Tags Orders
// @Param item_id path int true "item_id"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID put_api_order_items_item_id_installation_fee
// @Router /api/order-items/{item_id}/installation-fee [put]
// UpdateItemInstallationFee 更新单个订单商品的安装费，并同步订单汇总费用。
func UpdateItemInstallationFee(c *gin.Context) {
	itemID := c.Param("item_id")
	var req UpdateItemInstallationFeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "安装费用必须大于等于 0"})
		return
	}

	var item models.OrderItem
	if err := config.DB.First(&item, itemID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "商品明细不存在"})
		return
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "开启事务失败"})
		return
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var order models.Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, item.OrderID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "关联订单不存在"})
		return
	}
	if order.OrderStatus == 2 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "订单已取消，无法调整安装费"})
		return
	}

	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&item, itemID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "商品明细不存在"})
		return
	}
	if err := tx.Model(&item).Update("installation_fee", *req.InstallationFee).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新商品安装费失败"})
		return
	}

	var totalInstallationFee float64
	if err := tx.Model(&models.OrderItem{}).
		Where("order_id = ?", order.ID).
		Select("COALESCE(SUM(installation_fee), 0)").
		Scan(&totalInstallationFee).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "计算订单安装费失败"})
		return
	}
	if err := tx.Model(&order).Update("installation_fee", totalInstallationFee).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新订单安装费合计失败"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "保存安装费失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "安装费已更新"})
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
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, id).Error; err != nil {
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
	if err := validateActiveEmployeeAssignment(tx, req.InstallerID, time.Now()); err != nil {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": err.Error()})
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
	if err := resolvePendingFulfillmentAlert(tx, order.ID); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新离职订单履约告警失败"})
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

var errReferralFeeAlreadyPaid = errors.New("referral fee already paid")

// validateReferralFeeUpdate prevents changing referral details after the reward has been paid.
func validateReferralFeeUpdate(order *models.Order, _ float64) error {
	if order.IsReferralFeePaid {
		return errReferralFeeAlreadyPaid
	}
	return nil
}

// UpdateCommissionRequest 更新提成金额请求体
type UpdateCommissionRequest struct {
	CommissionAmount float64 `json:"commission_amount" binding:"min=0"`
}

// UpdateOrderReferrerRequest 更新订单推荐人/渠道请求体
type UpdateOrderReferrerRequest struct {
	ReferrerName *string `json:"referrer_name" binding:"omitempty,max=100"`
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
	if order.OrderType == 2 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "商会订单提成固定为订单金额的 1%，并自动平分，不能手动调整"})
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

// UpdateOrderReferrer 更新订单推荐人/渠道信息
// @Summary 更新订单推荐人/渠道信息
// @Description 仅管理员可修改尚未记账支出的订单推荐人/渠道信息
// @Tags Orders
// @Accept json
// @Produce json
// @Param body body UpdateOrderReferrerRequest true "推荐人/渠道参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @Param id path int true "路径资源ID"
// @ID put_api_orders_id_referrer
// @Router /api/orders/{id}/referrer [put]
func UpdateOrderReferrer(c *gin.Context) {
	id := c.Param("id")
	var req UpdateOrderReferrerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误"})
		return
	}
	if req.ReferrerName == nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "推荐人/渠道不能为空"})
		return
	}

	var order models.Order
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, id).Error; err != nil {
			return err
		}
		if err := validateReferralFeeUpdate(&order, order.ReferralFee); err != nil {
			return err
		}
		var previousPayments int64
		if err := tx.Model(&models.FinancialLog{}).
			Where("related_no = ? AND category IN ? AND flow_type = ? AND reversal_pair_id IS NULL", order.OrderNo, []string{"渠道奖励", "第三方奖励"}, 2).
			Count(&previousPayments).Error; err != nil {
			return err
		}
		if previousPayments > 0 {
			return errReferralFeeAlreadyPaid
		}
		return tx.Model(&order).Update("referrer_name", strings.TrimSpace(*req.ReferrerName)).Error
	})
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "订单不存在"})
		return
	}
	if errors.Is(err, errReferralFeeAlreadyPaid) {
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": "渠道奖励已记账支出，不能直接修改推荐人/渠道"})
		return
	}
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新推荐人/渠道失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "推荐人/渠道更新成功"})
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
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, item.OrderID).Error; err != nil {
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
	if err := validateActiveEmployeeAssignment(tx, req.DeliveryPersonID, time.Now()); err != nil {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": err.Error()})
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
	order.IsDelivered = allDelivered
	if err := resolvePendingFulfillmentAlert(tx, order.ID); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新离职订单履约告警失败"})
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

// UpdateItemDeliveryFee 修改订单项送货费
// @Summary 修改订单项送货费
// @Description 修改订单项送货费
// @Tags Orders
// @Param item_id path int true "item_id"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID put_api_order_items_item_id_delivery_fee
// @Router /api/order-items/{item_id}/delivery-fee [put]
// UpdateItemDeliveryFee 更新单个订单商品的送货费，并同步订单汇总费用。
func UpdateItemDeliveryFee(c *gin.Context) {
	itemID := c.Param("item_id")
	var req UpdateItemDeliveryFeeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "送货费用必须大于等于 0"})
		return
	}

	var item models.OrderItem
	if err := config.DB.First(&item, itemID).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "商品明细不存在"})
		return
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "开启事务失败"})
		return
	}
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	var order models.Order
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, item.OrderID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "关联订单不存在"})
		return
	}
	if order.OrderStatus == 2 {
		tx.Rollback()
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "订单已取消，无法调整送货费"})
		return
	}

	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&item, itemID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "商品明细不存在"})
		return
	}
	if err := tx.Model(&item).Update("delivery_fee", *req.DeliveryFee).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新商品送货费失败"})
		return
	}

	var totalDeliveryFee float64
	if err := tx.Model(&models.OrderItem{}).
		Where("order_id = ?", order.ID).
		Select("COALESCE(SUM(delivery_fee), 0)").
		Scan(&totalDeliveryFee).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "计算订单送货费失败"})
		return
	}
	if err := tx.Model(&order).Update("delivery_fee", totalDeliveryFee).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新订单送货费合计失败"})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "保存送货费失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "送货费已更新"})
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
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, item.OrderID).Error; err != nil {
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
	if err := validateActiveEmployeeAssignment(tx, req.InstallerID, time.Now()); err != nil {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": err.Error()})
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
	if err := resolvePendingFulfillmentAlert(tx, order.ID); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新离职订单履约告警失败"})
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
// @Param deleted_employee_followup query string false "员工档案已软删除且仍待跟进的订单（1 开启）"
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
	deletedEmployeeFollowup := c.Query("deleted_employee_followup")

	query := config.DB.Preload("OrderItems.Product").Preload("OrderItems.Product.Category").Preload("OrderItems.DeliveryPerson").Preload("OrderItems.Installer").Preload("Employee").Preload("Account").Preload("Installer").Preload("DeliveryPerson").Order("created_at desc")
	query = applyOrderFilters(query, keyword, subsidyStatus, paymentStatus, fulfillmentStatus, orderStatus, startDate, endDate, categoryID, deletedEmployeeFollowup)

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
		actualReq.DeliveryMethod = 4
	} else {
		actualReq.DeliveryMethod = 4 // 默认云仓发货
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
