package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"erp-backend/utils"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// WriteFinancialLog 在事务中写入一条财务流水记录（公用辅助函数）
// tx: 数据库事务
// accountID: 资金账户ID
// flowType: 1-收入, 2-支出, 3-内部调拨, 4-余额校准
// category: 业务分类(如: 销售货款, 采购货款, 盘亏损耗, 工资等)
// amount: 变动金额(绝对值)
// balanceAfter: 变动后账户余额快照
// relatedNo: 关联单号
// remark: 备注
func WriteFinancialLog(tx *gorm.DB, accountID uint, flowType int, category string, amount float64, balanceAfter float64, relatedNo string, remark string, createdAt ...time.Time) error {
	log := models.FinancialLog{
		AccountID:    accountID,
		FlowType:     flowType,
		Category:     category,
		Amount:       amount,
		BalanceAfter: balanceAfter,
		RelatedNo:    relatedNo,
		Remark:       remark,
	}
	if len(createdAt) > 0 && !createdAt[0].IsZero() {
		log.CreatedAt = createdAt[0]
	}
	return tx.Create(&log).Error
}

// GetFinancialLogs 查询全口径财务流水
// GET /api/finance/logs?account_id=&flow_type=&category=&keyword=&page=&page_size=
// @Summary 获取财务流水列表
// @Description 获取财务流水列表
// @Tags Finance
// @Accept json
// @Param account_id query int false "账户ID"
// @Param flow_type query int false "流水类型 1-收入 2-支出 3-内部 4-校准"
// @Param category query string false "分类"
// @Param keyword query string false "分类、关联单号或备注关键字"
// @Param start_date query string false "开始日期"
// @Param end_date query string false "结束日期"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID get_api_finance_logs
// @Router /api/finance/logs [get]
func GetFinancialLogs(c *gin.Context) {
	accountID := c.Query("account_id")
	flowType := c.Query("flow_type")
	category := c.Query("category")
	sourceType := c.Query("source_type")
	keyword := strings.TrimSpace(c.Query("keyword"))
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	query := config.DB.
		Preload("Account").
		Order("created_at desc")

	if accountID != "" {
		query = query.Where("account_id = ?", accountID)
	}
	if flowType != "" {
		query = query.Where("flow_type = ?", flowType)
	}
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if sourceType != "" {
		query = query.Where("source_type = ?", sourceType)
	}
	if keyword != "" {
		likeKeyword := "%" + keyword + "%"
		query = query.Where(
			"(category LIKE ? OR counterparty LIKE ? OR related_no LIKE ? OR remark LIKE ?)",
			likeKeyword,
			likeKeyword,
			likeKeyword,
			likeKeyword,
		)
	}
	if startDate != "" {
		query = query.Where("created_at >= ?", startDate)
	}
	if endDate != "" {
		endTime, err := time.ParseInLocation("2006-01-02", endDate, time.Local)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "结束日期格式无效"})
			return
		}
		query = query.Where("created_at < ?", endTime.AddDate(0, 0, 1))
	}

	var total int64
	query.Model(&models.FinancialLog{}).Count(&total)

	page := 1
	if pageValue, err := strconv.Atoi(c.DefaultQuery("page", "1")); err == nil && pageValue > 0 {
		page = pageValue
	}
	pageSize := 20
	if sizeValue, err := strconv.Atoi(c.DefaultQuery("page_size", "20")); err == nil && sizeValue > 0 {
		pageSize = min(sizeValue, 100)
	}
	offset := (page - 1) * pageSize

	var logs []models.FinancialLog
	if err := query.Offset(offset).Limit(pageSize).Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询财务流水失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code":  200,
		"total": total,
		"data":  logs,
	})
}

// WriteReversalLog 写入一条冲销配对日志。
// 与 WriteFinancialLog 不同：必须传入 depositLogID 作为 pair_id；同时把原日志的 reversal_pair_id 也置为同一 ID。
// 同一 pair_id 的两条日志会在 dashboard / stats 汇总时被 WHERE reversal_pair_id IS NULL 排除。
func WriteReversalLog(tx *gorm.DB, depositLogID uint, accountID uint, flowType int, category string, amount float64, balanceAfter float64, relatedNo string, remark string) error {
	// 1) 把原日志标记为"已被冲销"，pair_id 自配对 = depositLogID
	if err := tx.Model(&models.FinancialLog{}).
		Where("id = ?", depositLogID).
		Update("reversal_pair_id", depositLogID).Error; err != nil {
		return err
	}
	// 2) 写入冲销日志，共享同一 pair_id
	return tx.Create(&models.FinancialLog{
		AccountID:      accountID,
		FlowType:       flowType,
		Category:       category,
		Amount:         amount,
		BalanceAfter:   balanceAfter,
		RelatedNo:      relatedNo,
		Remark:         remark,
		ReversalPairID: &depositLogID,
	}).Error
}

// ExportFinancialLogs 导出财务流水
// @Summary 导出财务流水
// @Description 导出财务流水
// @Tags Finance
// @Produce application/octet-stream
// @Param account_id query int false "账户ID"
// @Param flow_type query int false "流水类型"
// @Param category query string false "分类"
// @Param keyword query string false "分类、关联单号或备注关键字"
// @Param start_date query string false "开始日期"
// @Param end_date query string false "结束日期"
// @Success 200 {file} binary "Excel 文件"
// @Security ApiKeyAuth
// @ID get_api_finance_logs_export
// @Router /api/finance/logs/export [get]
func ExportFinancialLogs(c *gin.Context) {
	accountID := c.Query("account_id")
	flowType := c.Query("flow_type")
	category := c.Query("category")
	sourceType := c.Query("source_type")
	keyword := strings.TrimSpace(c.Query("keyword"))
	startDate := c.Query("start_date")
	endDate := c.Query("end_date")

	query := config.DB.Preload("Account").Order("created_at desc")

	if accountID != "" {
		query = query.Where("account_id = ?", accountID)
	}
	if flowType != "" {
		query = query.Where("flow_type = ?", flowType)
	}
	if category != "" {
		query = query.Where("category = ?", category)
	}
	if sourceType != "" {
		query = query.Where("source_type = ?", sourceType)
	}
	if keyword != "" {
		likeKeyword := "%" + keyword + "%"
		query = query.Where(
			"(category LIKE ? OR counterparty LIKE ? OR related_no LIKE ? OR remark LIKE ?)",
			likeKeyword,
			likeKeyword,
			likeKeyword,
			likeKeyword,
		)
	}
	if startDate != "" {
		query = query.Where("created_at >= ?", startDate)
	}
	if endDate != "" {
		endTime, err := time.ParseInLocation("2006-01-02", endDate, time.Local)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "结束日期格式无效"})
			return
		}
		query = query.Where("created_at < ?", endTime.AddDate(0, 0, 1))
	}

	var logs []models.FinancialLog
	if err := query.Find(&logs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "查询流水失败"})
		return
	}

	headers := []string{"流水ID", "账户名称", "流向", "业务分类", "变动金额", "变动后余额", "往来对象", "关联单号", "备注", "来源", "记录时间"}
	var data [][]interface{}
	for _, l := range logs {
		accName := "-"
		if l.Account != nil {
			accName = l.Account.Name
		}
		flow := "收入"
		if l.FlowType == 2 {
			flow = "支出"
		} else if l.FlowType == 3 {
			flow = "内部调拨"
		} else if l.FlowType == 4 {
			flow = "系统校准"
		}

		row := []interface{}{
			l.ID,
			accName,
			flow,
			l.Category,
			l.Amount,
			l.BalanceAfter,
			l.Counterparty,
			l.RelatedNo,
			l.Remark,
			l.SourceType,
			l.CreatedAt.Format("2006-01-02 15:04:05"),
		}
		data = append(data, row)
	}

	utils.ExportToExcel(c, "财务流水", headers, data)
}
