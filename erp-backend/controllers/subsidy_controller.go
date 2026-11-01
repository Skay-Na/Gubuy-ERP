package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"errors"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// errSubsidyDuplicated 核销幂等命中：请求已提交过，事务回滚并返回已有记录。
var errSubsidyDuplicated = errors.New("该请求已提交过，返回已有核销记录，未重复核销")

type CreateSubsidySettlementRequest struct {
	Amount    float64 `json:"amount" binding:"required,gt=0"`
	AccountID uint    `json:"account_id" binding:"required"`
	Remark    string  `json:"remark"`
}

// CreateSubsidySettlement 录入国补回款记录
// @Summary 创建国补结算单
// @Description 创建国补结算单
// @Tags Subsidy
// @Accept json
// @Produce json
// @Param body body CreateSubsidySettlementRequest true "国补结算参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_subsidy_settle
// @Router /api/subsidy/settle [post]
func CreateSubsidySettlement(c *gin.Context) {
	var input CreateSubsidySettlementRequest

	if err := c.ShouldBindJSON(&input); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 1. 增加账户余额 (使用行级锁)
	var account models.Account
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, input.AccountID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "账户查询失败"})
		return
	}

	if err := tx.Model(&account).UpdateColumn("balance", gorm.Expr("balance + ?", input.Amount)).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "更新账户余额失败"})
		return
	}

	// 2. 创建回款记录
	settlement := models.SubsidySettlement{
		Amount:    input.Amount,
		AccountID: input.AccountID,
		Remark:    input.Remark,
	}
	if err := tx.Create(&settlement).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "创建回款记录失败"})
		return
	}

	// 3. 写入财务流水
	if err := WriteFinancialLog(tx, input.AccountID, 1, "国补回款", input.Amount, account.Balance+input.Amount, fmt.Sprintf("SUB-%d", settlement.ID), input.Remark); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "写入财务流水失败"})
		return
	}

	tx.Commit()
	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "success", "data": settlement})
}

// GetSubsidySettlements 获取国补回款历史
// @Summary 获取国补结算列表
// @Description 获取国补结算列表（附每笔回款池已核销金额）
// @Tags Subsidy
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_subsidy_settlements
// @Router /api/subsidy/settlements [get]
func GetSubsidySettlements(c *gin.Context) {
	var list []models.SubsidySettlement
	if err := config.DB.Preload("Account").Order("created_at desc").Find(&list).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "获取记录失败"})
		return
	}

	// 统计每笔回款池已核销金额，供前端展示剩余可核销额度
	allocatedBySettlement := map[uint]float64{}
	var allocatedRows []struct {
		SettlementID uint
		Total        float64
	}
	if err := config.DB.Model(&models.SubsidyReceipt{}).
		Select("settlement_id, COALESCE(SUM(allocated_amt), 0) AS total").
		Group("settlement_id").Scan(&allocatedRows).Error; err == nil {
		for _, row := range allocatedRows {
			allocatedBySettlement[row.SettlementID] = row.Total
		}
	}

	result := make([]gin.H, 0, len(list))
	for _, settlement := range list {
		result = append(result, gin.H{
			"id":            settlement.ID,
			"amount":        settlement.Amount,
			"account_id":    settlement.AccountID,
			"account":       settlement.Account,
			"remark":        settlement.Remark,
			"created_at":    settlement.CreatedAt,
			"allocated_amt": math.Round(allocatedBySettlement[settlement.ID]*100) / 100,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": result})
}

// roundSubsidyAmount 金额统一保留两位小数，避免浮点累积误差。
func roundSubsidyAmount(value float64) float64 {
	return math.Round(value*100) / 100
}

// subsidyAmountEqual 金额比较容差（分）
const subsidyAmountEpsilon = 0.005

// subsidyIdempotencyKey 同批核销每行派生唯一幂等键（<base>-<orderID>）。
// client_request_id 列是单列唯一索引，同一请求的多笔核销不能共用同一个键；
// 以订单 ID 派生后缀保证批内唯一，base 超长时截断防溢出 varchar(64)。
func subsidyIdempotencyKey(base string, orderID uint) string {
	if len(base) > 50 {
		base = base[:50]
	}
	return fmt.Sprintf("%s-%d", base, orderID)
}

type SubsidyAllocationItem struct {
	OrderID uint    `json:"order_id" binding:"required"`
	Amount  float64 `json:"amount" binding:"required,gt=0"`
}

type SubsidyAllocateRequest struct {
	SettlementID    uint                    `json:"settlement_id" binding:"required"`
	Allocations     []SubsidyAllocationItem `json:"allocations" binding:"required,min=1,dive"`
	ClientRequestID string                  `json:"client_request_id"`
}

// AllocateSubsidyReceipt 国补回款逐单核销
// @Summary 核销国补回款到订单
// @Description 把一笔回款池资金分配到若干国补订单；单事务行锁校验，收满自动置 subsidy_status=3。
// @Tags Subsidy
// @Accept json
// @Produce json
// @Param body body SubsidyAllocateRequest true "核销参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_subsidy_allocate
// @Router /api/subsidy/allocate [post]
func AllocateSubsidyReceipt(c *gin.Context) {
	var req SubsidyAllocateRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}

	// 合并重复订单并规整金额；订单 ID 排序保证后面按稳定顺序加锁（防死锁）
	allocByOrder := map[uint]float64{}
	orderIDs := make([]uint, 0, len(req.Allocations))
	totalAlloc := 0.0
	for _, item := range req.Allocations {
		amount := roundSubsidyAmount(item.Amount)
		if amount <= 0 {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "单笔核销金额必须大于 0"})
			return
		}
		if _, exists := allocByOrder[item.OrderID]; !exists {
			orderIDs = append(orderIDs, item.OrderID)
		}
		allocByOrder[item.OrderID] += amount
		totalAlloc += amount
	}
	totalAlloc = roundSubsidyAmount(totalAlloc)
	sort.Slice(orderIDs, func(i, j int) bool { return orderIDs[i] < orderIDs[j] })

	created := make([]models.SubsidyReceipt, 0, len(orderIDs))
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		// 幂等：请求携带 client_request_id 时，事务内先按派生键（首单）查询，
		// 已存在核销记录则直接返回（duplicated:true），不重复核销。
		// 每行实际写入派生键 <key>-<orderID>（见 subsidyIdempotencyKey），避免单列唯一索引冲突。
		req.ClientRequestID = strings.TrimSpace(req.ClientRequestID)
		if req.ClientRequestID != "" {
			var existing models.SubsidyReceipt
			firstKey := subsidyIdempotencyKey(req.ClientRequestID, orderIDs[0])
			idempotencyErr := tx.Where("client_request_id = ?", firstKey).First(&existing).Error
			if idempotencyErr == nil {
				created = append(created, existing)
				return errSubsidyDuplicated
			}
			if !errors.Is(idempotencyErr, gorm.ErrRecordNotFound) {
				return fmt.Errorf("幂等键查询失败")
			}
		}

		// a. 行锁读回款结算单
		var settlement models.SubsidySettlement
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&settlement, req.SettlementID).Error; err != nil {
			return fmt.Errorf("回款记录不存在")
		}

		// b. 校验回款池余额：结算单金额 - 已分配 SUM(allocated_amt) >= 本次总分配额
		var allocated float64
		if err := tx.Model(&models.SubsidyReceipt{}).
			Where("settlement_id = ?", settlement.ID).
			Select("COALESCE(SUM(allocated_amt), 0)").Scan(&allocated).Error; err != nil {
			return fmt.Errorf("统计回款池已核销金额失败")
		}
		poolRemaining := roundSubsidyAmount(settlement.Amount - allocated)
		if totalAlloc > poolRemaining+subsidyAmountEpsilon {
			return fmt.Errorf("回款池余额不足：本笔回款 ¥%.2f 仅剩可核销 ¥%.2f", settlement.Amount, poolRemaining)
		}

		// c. 行锁逐单校验
		type allocPlan struct {
			order           models.Order
			amount          float64
			remainingBefore float64
		}
		plans := make([]allocPlan, 0, len(orderIDs))
		for _, orderID := range orderIDs {
			var order models.Order
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, orderID).Error; err != nil {
				return fmt.Errorf("订单 ID %d 不存在", orderID)
			}
			if order.OrderStatus != 1 {
				return fmt.Errorf("订单 %s 已取消，不能核销", order.OrderNo)
			}
			remainingBefore := roundSubsidyAmount(order.SubsidyPendingAmount - order.SubsidyReceivedAmt)
			if remainingBefore <= subsidyAmountEpsilon {
				return fmt.Errorf("订单 %s 没有待回款金额，不能核销", order.OrderNo)
			}
			if allocByOrder[orderID] > remainingBefore+subsidyAmountEpsilon {
				return fmt.Errorf("订单 %s 核销超额：待回款 ¥%.2f，本次申请 ¥%.2f", order.OrderNo, remainingBefore, allocByOrder[orderID])
			}
			plans = append(plans, allocPlan{order: order, amount: allocByOrder[orderID], remainingBefore: remainingBefore})
		}

		// d. 写核销记录并累加订单已回款；收满置 subsidy_status=3（只能由核销写入）
		operator := c.GetString("username")
		now := time.Now()
		for _, plan := range plans {
			receipt := models.SubsidyReceipt{
				SettlementID:    settlement.ID,
				OrderID:         plan.order.ID,
				OrderNo:         plan.order.OrderNo,
				AllocatedAmt:    plan.amount,
				RemainingBefore: plan.remainingBefore,
				CreatedBy:       operator,
				CreatedAt:       now,
			}
			if req.ClientRequestID != "" {
				derived := subsidyIdempotencyKey(req.ClientRequestID, plan.order.ID)
				receipt.ClientRequestID = &derived
			}
			if err := tx.Create(&receipt).Error; err != nil {
				return fmt.Errorf("写入核销记录失败")
			}
			newReceived := roundSubsidyAmount(plan.order.SubsidyReceivedAmt + plan.amount)
			updates := map[string]interface{}{
				"subsidy_received_amt": newReceived,
			}
			if newReceived >= plan.order.SubsidyPendingAmount-subsidyAmountEpsilon {
				updates["subsidy_status"] = 3
			}
			if err := tx.Model(&models.Order{}).Where("id = ?", plan.order.ID).Updates(updates).Error; err != nil {
				return fmt.Errorf("更新订单 %s 已回款金额失败", plan.order.OrderNo)
			}
			created = append(created, receipt)
		}
		return nil
	})
	if err != nil {
		if errors.Is(err, errSubsidyDuplicated) {
			c.JSON(http.StatusOK, gin.H{
				"code":       200,
				"msg":        err.Error(),
				"duplicated": true,
				"data":       created,
			})
			return
		}
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  fmt.Sprintf("核销成功，共分配 %d 笔订单，金额 ¥%.2f", len(created), totalAlloc),
		"data": created,
	})
}

// SubsidyBatchReceiveRequest 批量到账请求：勾选若干国补订单一次性标记到账。
type SubsidyBatchReceiveRequest struct {
	OrderIDs        []uint `json:"order_ids" binding:"required,min=1,dive"`
	AccountID       uint   `json:"account_id" binding:"required"`
	Remark          string `json:"remark"`
	ClientRequestID string `json:"client_request_id"`
}

// BatchReceiveSubsidy 国补批量到账
// @Summary 国补订单批量到账勾选
// @Description 勾选多笔国补订单一次性完成到账闭环：单事务内按各单待回额合计录入回款池（资金入账+财务流水），再逐单全额核销（收满自动置 subsidy_status=3、待回归零）。幂等键防重复提交。
// @Tags Subsidy
// @Accept json
// @Produce json
// @Param body body SubsidyBatchReceiveRequest true "批量到账参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID post_api_subsidy_batch_receive
// @Router /api/subsidy/batch-receive [post]
func BatchReceiveSubsidy(c *gin.Context) {
	var req SubsidyBatchReceiveRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}

	// 去重 + 排序：保证加锁顺序稳定（防死锁），批内重复勾选只算一次
	seen := map[uint]struct{}{}
	orderIDs := make([]uint, 0, len(req.OrderIDs))
	for _, id := range req.OrderIDs {
		if id == 0 {
			continue
		}
		if _, exists := seen[id]; !exists {
			seen[id] = struct{}{}
			orderIDs = append(orderIDs, id)
		}
	}
	if len(orderIDs) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "请至少勾选一笔订单"})
		return
	}
	sort.Slice(orderIDs, func(i, j int) bool { return orderIDs[i] < orderIDs[j] })

	type batchPlan struct {
		order     models.Order
		amount    float64
		remBefore float64
	}
	var (
		settlement models.SubsidySettlement
		total      float64
	)
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		// 幂等：client_request_id 已有核销记录则拒绝重复提交（查首单派生键）
		req.ClientRequestID = strings.TrimSpace(req.ClientRequestID)
		if req.ClientRequestID != "" {
			var existing models.SubsidyReceipt
			firstKey := subsidyIdempotencyKey(req.ClientRequestID, orderIDs[0])
			idempotencyErr := tx.Where("client_request_id = ?", firstKey).First(&existing).Error
			if idempotencyErr == nil {
				return fmt.Errorf("该请求已提交过，请勿重复操作")
			}
			if !errors.Is(idempotencyErr, gorm.ErrRecordNotFound) {
				return fmt.Errorf("幂等键查询失败")
			}
		}

		// a. 行锁逐单校验并汇总待回金额
		plans := make([]batchPlan, 0, len(orderIDs))
		for _, orderID := range orderIDs {
			var order models.Order
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&order, orderID).Error; err != nil {
				return fmt.Errorf("订单 ID %d 不存在", orderID)
			}
			if order.OrderStatus != 1 {
				return fmt.Errorf("订单 %s 已取消，不能标记到账", order.OrderNo)
			}
			if order.SubsidyAmount <= 0 {
				return fmt.Errorf("订单 %s 不是国补订单", order.OrderNo)
			}
			remaining := roundSubsidyAmount(order.SubsidyPendingAmount - order.SubsidyReceivedAmt)
			if remaining <= subsidyAmountEpsilon {
				return fmt.Errorf("订单 %s 待回款已核销完毕，无需重复到账", order.OrderNo)
			}
			plans = append(plans, batchPlan{order: order, amount: remaining, remBefore: remaining})
			total += remaining
		}
		total = roundSubsidyAmount(total)
		if total <= subsidyAmountEpsilon {
			return fmt.Errorf("选中订单待回金额合计为 0")
		}

		// b. 行锁收款账户并增加余额（与录入回款池同口径：政府补贴部分真实入账）
		var account models.Account
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&account, req.AccountID).Error; err != nil {
			return fmt.Errorf("收款账户不存在")
		}
		if err := tx.Model(&account).UpdateColumn("balance", gorm.Expr("balance + ?", total)).Error; err != nil {
			return fmt.Errorf("更新账户余额失败")
		}

		// c. 创建回款池记录（本批到账金额）
		settlement = models.SubsidySettlement{
			Amount:    total,
			AccountID: req.AccountID,
			Remark:    req.Remark,
		}
		if err := tx.Create(&settlement).Error; err != nil {
			return fmt.Errorf("创建回款记录失败")
		}

		// d. 写财务流水
		if err := WriteFinancialLog(tx, req.AccountID, 1, "国补回款", total, account.Balance+total, fmt.Sprintf("SUB-%d", settlement.ID), req.Remark); err != nil {
			return fmt.Errorf("写入财务流水失败")
		}

		// e. 逐单全额核销：收满置 subsidy_status=3（只能由核销流程写入）
		operator := c.GetString("username")
		now := time.Now()
		for _, plan := range plans {
			receipt := models.SubsidyReceipt{
				SettlementID:    settlement.ID,
				OrderID:         plan.order.ID,
				OrderNo:         plan.order.OrderNo,
				AllocatedAmt:    plan.amount,
				RemainingBefore: plan.remBefore,
				CreatedBy:       operator,
				CreatedAt:       now,
			}
			if req.ClientRequestID != "" {
				derived := subsidyIdempotencyKey(req.ClientRequestID, plan.order.ID)
				receipt.ClientRequestID = &derived
			}
			if err := tx.Create(&receipt).Error; err != nil {
				return fmt.Errorf("订单 %s 写入核销记录失败", plan.order.OrderNo)
			}
			newReceived := roundSubsidyAmount(plan.order.SubsidyReceivedAmt + plan.amount)
			if err := tx.Model(&models.Order{}).Where("id = ?", plan.order.ID).Updates(map[string]interface{}{
				"subsidy_received_amt": newReceived,
				"subsidy_status":       3,
			}).Error; err != nil {
				return fmt.Errorf("订单 %s 标记已回款失败", plan.order.OrderNo)
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  fmt.Sprintf("批量到账成功：%d 笔订单共 ¥%.2f 已入账并核销", len(orderIDs), total),
		"data": gin.H{
			"settlement_id": settlement.ID,
			"order_count":   len(orderIDs),
			"total_amount":  total,
		},
	})
}

// GetSubsidyReceipts 核销明细（含客户姓名，供"已核销"分组展示）
// @Summary 获取国补核销明细
// @Description 获取国补核销明细
// @Tags Subsidy
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_subsidy_receipts
// @Router /api/subsidy/receipts [get]
func GetSubsidyReceipts(c *gin.Context) {
	var receipts []models.SubsidyReceipt
	if err := config.DB.Order("created_at desc, id desc").Find(&receipts).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "获取核销明细失败"})
		return
	}

	orderIDs := make([]uint, 0, len(receipts))
	seen := map[uint]struct{}{}
	for _, receipt := range receipts {
		if _, exists := seen[receipt.OrderID]; !exists {
			seen[receipt.OrderID] = struct{}{}
			orderIDs = append(orderIDs, receipt.OrderID)
		}
	}
	customers := map[uint]string{}
	if len(orderIDs) > 0 {
		var orders []models.Order
		if err := config.DB.Select("id, customer_name").Where("id IN ?", orderIDs).Find(&orders).Error; err == nil {
			for _, order := range orders {
				customers[order.ID] = order.CustomerName
			}
		}
	}

	result := make([]gin.H, 0, len(receipts))
	for _, receipt := range receipts {
		result = append(result, gin.H{
			"id":               receipt.ID,
			"settlement_id":    receipt.SettlementID,
			"order_id":         receipt.OrderID,
			"order_no":         receipt.OrderNo,
			"customer_name":    customers[receipt.OrderID],
			"allocated_amt":    receipt.AllocatedAmt,
			"remaining_before": receipt.RemainingBefore,
			"created_by":       receipt.CreatedBy,
			"created_at":       receipt.CreatedAt,
		})
	}
	c.JSON(http.StatusOK, gin.H{"code": 200, "data": result})
}

// GetSubsidySummary 获取国补概览数据
// @Summary 获取国补数据总览
// @Description 获取国补数据总览
// @Tags Subsidy
// @Accept json
// @Produce json
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_subsidy_summary
// @Router /api/subsidy/summary [get]
func GetSubsidySummary(c *gin.Context) {
	overview, err := readSubsidyOverview(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "获取国补概览失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"data": gin.H{
			"total_generated": overview.TotalGenerated,
			"total_received":  overview.TotalReceived,
			"pending_balance": overview.PendingBalance,
		},
	})
}
