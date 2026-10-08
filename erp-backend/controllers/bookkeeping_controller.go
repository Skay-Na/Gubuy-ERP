package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	bookkeepingIncome  = 1
	bookkeepingExpense = 2
)

var (
	errInvalidBookkeepingType   = errors.New("记账类型仅支持收入或支出")
	errInvalidBookkeepingAmount = errors.New("记账金额必须大于 0")
	errMissingBookkeepingFields = errors.New("账户和分类不能为空")
	errOrderCommissionForbidden = errors.New("商会订单不能通过记账增加员工提成")
	errOrderCommissionTooHigh   = errors.New("员工提成累计金额超过订单实付金额的 30%")
	errOrderCostRequiresExpense = errors.New("订单成本类记账必须选择支出")
)

var bookkeepingOrderFields = map[string]string{
	"渠道奖励": "referral_fee",
	"员工提成": "commission_amount",
	"安装费":  "installation_fee",
	"运输配送": "delivery_fee",
}

// CreateBookkeepingEntryRequest 手工记账请求体。
type CreateBookkeepingEntryRequest struct {
	AIImportID   uint    `json:"ai_import_id"`
	EntryType    int     `json:"entry_type" binding:"required"`
	Category     string  `json:"category" binding:"required,max=50"`
	Amount       float64 `json:"amount" binding:"required,gt=0"`
	AccountID    uint    `json:"account_id" binding:"required"`
	OrderID      uint    `json:"order_id"`
	Counterparty string  `json:"counterparty" binding:"max=100"`
	RelatedNo    string  `json:"related_no" binding:"max=100"`
	Remark       string  `json:"remark" binding:"max=255"`
	OccurredAt   string  `json:"occurred_at"`
}

// BookkeepingOrderOption 记账页面的订单联想选项。
type BookkeepingOrderOption struct {
	ID              uint      `json:"id"`
	OrderNo         string    `json:"order_no"`
	CustomerName    string    `json:"customer_name"`
	CustomerPhone   string    `json:"customer_phone"`
	ActualPayAmount float64   `json:"actual_pay_amount"`
	OrderType       int       `json:"order_type"`
	CreatedAt       time.Time `json:"created_at"`
}

func validateBookkeepingEntry(req *CreateBookkeepingEntryRequest) error {
	req.Category = strings.TrimSpace(req.Category)
	req.Counterparty = strings.TrimSpace(req.Counterparty)
	req.RelatedNo = strings.TrimSpace(req.RelatedNo)
	req.Remark = strings.TrimSpace(req.Remark)

	if req.EntryType != bookkeepingIncome && req.EntryType != bookkeepingExpense {
		return errInvalidBookkeepingType
	}
	if req.Amount <= 0 {
		return errInvalidBookkeepingAmount
	}
	if req.AccountID == 0 || req.Category == "" {
		return errMissingBookkeepingFields
	}
	return nil
}

func parseBookkeepingOccurredAt(value string, fallback time.Time) (time.Time, error) {
	if strings.TrimSpace(value) == "" {
		return fallback, nil
	}

	occurredAt, err := time.ParseInLocation("2006-01-02 15:04:05", value, time.Local)
	if err != nil {
		return time.Time{}, fmt.Errorf("解析发生时间: %w", err)
	}
	return occurredAt, nil
}

func bookkeepingOrderField(req CreateBookkeepingEntryRequest, order models.Order) (string, error) {
	field := bookkeepingOrderFields[req.Category]
	if field == "" {
		return "", nil
	}
	if req.EntryType != bookkeepingExpense {
		return "", errOrderCostRequiresExpense
	}
	if field != "commission_amount" {
		return field, nil
	}
	if order.OrderType == 2 {
		return "", errOrderCommissionForbidden
	}
	if order.CommissionAmount+req.Amount > order.ActualPayAmount*0.3 {
		return "", errOrderCommissionTooHigh
	}
	return field, nil
}

func resolveBookkeepingOrder(
	tx *gorm.DB,
	req *CreateBookkeepingEntryRequest,
) (*models.Order, error) {
	if req.OrderID == 0 && req.RelatedNo == "" {
		return nil, nil
	}

	query := tx.Clauses(clause.Locking{Strength: "UPDATE"})
	var order models.Order
	var err error
	if req.OrderID != 0 {
		err = query.First(&order, req.OrderID).Error
	} else {
		err = query.Where("order_no = ?", req.RelatedNo).First(&order).Error
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if req.OrderID != 0 {
			return nil, errors.New("关联订单不存在")
		}
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("查询关联订单: %w", err)
	}
	if order.OrderStatus == 2 {
		return nil, errors.New("已取消订单不能关联记账")
	}

	req.OrderID = order.ID
	req.RelatedNo = order.OrderNo
	if req.Counterparty == "" {
		req.Counterparty = order.CustomerName
	}
	return &order, nil
}

// SearchBookkeepingOrders 模糊搜索可关联的正常订单。
// @Summary 搜索记账关联订单
// @Description 按订单号、客户姓名、手机号或客户拼音搜索正常订单
// @Tags Bookkeeping
// @Produce json
// @Param q query string false "订单号、客户姓名、手机号或拼音"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_bookkeeping_orders
// @Router /api/bookkeeping/orders [get]
func SearchBookkeepingOrders(c *gin.Context) {
	keyword := strings.TrimSpace(c.Query("q"))
	query := config.DB.Model(&models.Order{}).
		Where("order_status = ?", 1).
		Order("created_at DESC").
		Limit(20)
	if keyword != "" {
		likeKeyword := "%" + keyword + "%"
		query = query.Where(
			"order_no LIKE ? OR customer_name LIKE ? OR customer_phone LIKE ? OR customer_pinyin LIKE ? OR customer_pinyin_initial LIKE ?",
			likeKeyword,
			likeKeyword,
			likeKeyword,
			likeKeyword,
			likeKeyword,
		)
	}

	options := []BookkeepingOrderOption{}
	if err := query.Find(&options).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "订单搜索失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": options})
}

// CreateBookkeepingEntry 创建一笔手工收入或支出，并原子更新账户余额与财务流水。
// @Summary 新增手工记账
// @Description 创建收入或支出记录，并同步更新资金账户余额
// @Tags Bookkeeping
// @Accept json
// @Produce json
// @Param body body CreateBookkeepingEntryRequest true "记账参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_bookkeeping_entries
// @Router /api/bookkeeping/entries [post]
func CreateBookkeepingEntry(c *gin.Context) {
	var req CreateBookkeepingEntryRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "记账参数不完整"})
		return
	}
	if req.AIImportID != 0 && strings.HasPrefix(c.GetString("username"), "emp_") {
		c.JSON(http.StatusForbidden, gin.H{"code": 403, "msg": "AI 导入需要管理员权限"})
		return
	}
	if err := validateBookkeepingEntry(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	occurredAt, err := parseBookkeepingOccurredAt(req.OccurredAt, time.Now())
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "发生时间格式无效"})
		return
	}

	tx := config.DB.Begin()
	if tx.Error != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "记账事务启动失败"})
		return
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			tx.Rollback()
			panic(recovered)
		}
	}()
	if err := lockAIImport(tx, req.AIImportID, "expense", c.GetString("username")); err != nil {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": err.Error()})
		return
	}

	entry, linkedOrder, err := postBookkeepingEntry(tx, req, occurredAt)
	if err != nil {
		tx.Rollback()
		status, message := bookkeepingHTTPError(err)
		c.JSON(status, gin.H{"code": status, "msg": message})
		return
	}
	if err := completeAIImport(tx, req.AIImportID, entry.ID); err != nil {
		tx.Rollback()
		c.JSON(http.StatusConflict, gin.H{"code": 409, "msg": err.Error()})
		return
	}
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "记账事务提交失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "记账成功",
		"data": gin.H{
			"entry":        entry,
			"linked_order": linkedOrder,
		},
	})
}
