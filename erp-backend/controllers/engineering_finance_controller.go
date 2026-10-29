package controllers

// 工程模块 · 清单 / 付款节点 / 回款控制器
//
// 归属任务卡：
//   - T08：SaveEngineeringItems / SaveEngineeringTerms（合同额与节点合计一致性双闸门）
//   - T09：CreateEngineeringReceipt / ReverseEngineeringReceipt（原子条件更新防超收）
//
// 铁律：本文件只写 engineering_* 表；不回写任何资金账户或 orders。

import (
	"erp-backend/config"
	"erp-backend/models"
	"erp-backend/services"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// engItemsAgg 清单汇总聚合
type engItemsAgg struct {
	ItemsTotal float64 `gorm:"column:items_total"`
	CostTotal  float64 `gorm:"column:cost_total"`
	OrderedQty int     `gorm:"column:ordered_qty"`
	ShippedQty int     `gorm:"column:shipped_qty"`
}

// engTermsAgg 节点汇总聚合
type engTermsAgg struct {
	Total    float64 `gorm:"column:total"`
	Received float64 `gorm:"column:received"`
}

// engLoadProjectForWrite 事务内锁项目行（所有写操作先锁主档，天然串行）
func engLoadProjectForWrite(tx *gorm.DB, id uint) (*models.EngineeringProject, error) {
	var p models.EngineeringProject
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&p, id).Error; err != nil {
		return nil, err
	}
	return &p, nil
}

// engAggItems 从库中重新汇总清单（不信任客户端传来的金额）
func engAggItems(tx *gorm.DB, projectID uint) (engItemsAgg, error) {
	var agg engItemsAgg
	err := tx.Model(&models.EngineeringProjectItem{}).
		Select(`COALESCE(SUM(ROUND(quantity * unit_price, 2)), 0) AS items_total,
		        COALESCE(SUM(ROUND(quantity * unit_cost, 2)), 0) AS cost_total,
		        COALESCE(SUM(quantity), 0) AS ordered_qty,
		        COALESCE(SUM(shipped_qty), 0) AS shipped_qty`).
		Where("project_id = ?", projectID).Scan(&agg).Error
	return agg, err
}

// engAggTerms 节点计划/实收合计
func engAggTerms(tx *gorm.DB, projectID uint) (engTermsAgg, error) {
	var agg engTermsAgg
	err := tx.Model(&models.EngineeringPaymentTerm{}).
		Select(`COALESCE(SUM(planned_amount), 0) AS total, COALESCE(SUM(received_amount), 0) AS received`).
		Where("project_id = ?", projectID).Scan(&agg).Error
	return agg, err
}

// ---------------------------------------------------------------------------
// T08-a 清单整表保存
// ---------------------------------------------------------------------------

type engItemInput struct {
	ID        uint    `json:"id"`
	ProductID uint    `json:"product_id"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
	UnitCost  float64 `json:"unit_cost"`
	Remark    string  `json:"remark"`
}

type engItemsSaveReq struct {
	Items []engItemInput `json:"items"`
}

// SaveEngineeringItems 整表保存清单 PUT /api/admin/engineering/projects/:id/items（T08）
//
// 全量替换语义：id=0 新增，id>0 保留，未出现的删除。
// 合同额变化时，已签约及以后必须先与节点合计相等，否则整笔回滚（第三道强制点之一）。
// @Summary 整表保存设备清单
// @Tags 工程管理
// @Accept json
// @Produce json
// @Param id path int true "项目 ID"
// @Param body body engItemsSaveReq true "清单行数组（已签约后合计须等于合同额）"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID save_engineering_items
// @Router /api/admin/engineering/projects/{id}/items [put]
func SaveEngineeringItems(c *gin.Context) {
	id, ok := engIDParam(c, "id")
	if !ok {
		return
	}
	var req engItemsSaveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		FailBadRequest(c, "参数格式错误："+err.Error())
		return
	}

	var result gin.H
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		p, err := engLoadProjectForWrite(tx, id)
		if err != nil {
			return err
		}
		if p.Status == services.EngStatusVoided {
			return errors.New("项目已作废，不可修改设备清单")
		}
		if p.Status == services.EngStatusSettled {
			return errors.New("项目已结清，不可修改设备清单")
		}

		var existing []models.EngineeringProjectItem
		if err := tx.Where("project_id = ?", id).Find(&existing).Error; err != nil {
			return err
		}
		existingByID := make(map[uint]models.EngineeringProjectItem, len(existing))
		for _, it := range existing {
			existingByID[it.ID] = it
		}

		// 商品存在性批量校验（一次查询，避免 N+1）
		productIDs := make([]uint, 0, len(req.Items))
		for _, it := range req.Items {
			if it.ProductID == 0 {
				return errors.New("请选择商品")
			}
			productIDs = append(productIDs, it.ProductID)
		}
		if len(productIDs) > 0 {
			var cnt int64
			if err := tx.Model(&models.Product{}).Where("id IN ?", productIDs).Count(&cnt).Error; err != nil {
				return err
			}
			if int(cnt) < len(uniqueUint(productIDs)) {
				return errors.New("清单中存在已删除或不存在的商品，请刷新后重试")
			}
		}

		keep := map[uint]bool{}
		for idx, it := range req.Items {
			if it.Quantity < 1 {
				return fmt.Errorf("第 %d 行数量必须大于 0", idx+1)
			}
			if it.UnitPrice <= 0 {
				return fmt.Errorf("第 %d 行工程单价必须大于 0", idx+1)
			}
			if it.UnitCost < 0 {
				return fmt.Errorf("第 %d 行成本单价不能为负数", idx+1)
			}
			if it.ID > 0 {
				old, exists := existingByID[it.ID]
				if !exists {
					return fmt.Errorf("第 %d 行的清单记录不存在，请刷新后重试", idx+1)
				}
				if old.ShippedQty > 0 && old.ProductID != it.ProductID {
					return errors.New("已出库的商品不可替换")
				}
				if it.Quantity < old.ShippedQty {
					return fmt.Errorf("第 %d 行数量不能小于已出库数量 %d", idx+1, old.ShippedQty)
				}
				keep[it.ID] = true
			}
		}

		// 未出现在本次提交中的行：有出库记录的不能删（否则库存与清单背离）
		for _, old := range existing {
			if keep[old.ID] {
				continue
			}
			if old.ShippedQty > 0 {
				return fmt.Errorf("商品行「%d」已有 %d 台出库记录，不可删除", old.ID, old.ShippedQty)
			}
			if err := tx.Delete(&models.EngineeringProjectItem{}, old.ID).Error; err != nil {
				return err
			}
		}

		for idx, it := range req.Items {
			remark := strings.TrimSpace(it.Remark)
			if engRuneLen(remark) > 200 {
				return fmt.Errorf("第 %d 行备注不能超过 200 字", idx+1)
			}
			if it.ID > 0 {
				if err := tx.Model(&models.EngineeringProjectItem{}).Where("id = ?", it.ID).
					Updates(map[string]interface{}{
						"product_id": it.ProductID,
						"quantity":   it.Quantity,
						"unit_price": services.RoundMoney(it.UnitPrice),
						"unit_cost":  services.RoundMoney(it.UnitCost),
						"remark":     remark,
						"sort_order": idx,
					}).Error; err != nil {
					return err
				}
				continue
			}
			row := models.EngineeringProjectItem{
				ProjectID: id, ProductID: it.ProductID, Quantity: it.Quantity,
				UnitPrice: services.RoundMoney(it.UnitPrice), UnitCost: services.RoundMoney(it.UnitCost),
				Remark: remark, SortOrder: idx,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}

		agg, err := engAggItems(tx, id)
		if err != nil {
			return err
		}
		if p.Status >= services.EngStatusSigned && agg.OrderedQty == 0 {
			return errors.New("已签约项目不可清空设备清单")
		}

		newContract := services.CalcContractTotal(agg.ItemsTotal, p.InstallFee, p.FreightFee, p.OtherFee, p.DiscountAmount)
		termsAgg, err := engAggTerms(tx, id)
		if err != nil {
			return err
		}
		balanced := services.MoneyEqual(newContract, termsAgg.Total)
		if services.EngAmountLocked(p.Status) && !balanced {
			return engConflict(fmt.Sprintf("合同总额调整为 %s，与付款节点合计相差 %s，请同时调整付款节点",
				services.FormatMoneyCN(newContract),
				services.FormatMoneyCN(services.RoundMoney(newContract-termsAgg.Total))))
		}

		if err := tx.Model(&models.EngineeringProject{}).Where("id = ?", id).Updates(map[string]interface{}{
			"items_total":    services.RoundMoney(agg.ItemsTotal),
			"cost_total":     services.RoundMoney(agg.CostTotal),
			"contract_total": newContract,
			"ordered_qty":    agg.OrderedQty,
			"shipped_qty":    agg.ShippedQty,
		}).Error; err != nil {
			return err
		}

		profit := services.CalcProfitability(newContract, agg.CostTotal, p.ExpenseTotal)
		pending := services.RoundMoney(newContract - p.ReceivedAmount)
		if pending < 0 {
			pending = 0
		}
		result = gin.H{
			"items_total":    services.RoundMoney(agg.ItemsTotal),
			"cost_total":     services.RoundMoney(agg.CostTotal),
			"contract_total": newContract,
			"ordered_qty":    agg.OrderedQty,
			"shipped_qty":    agg.ShippedQty,
			"gross_profit":   profit.GrossProfit,
			"margin_rate":    profit.GrossMarginRate,
			"net_profit":     profit.NetProfit,
			"pending_amount": pending,
			"terms_total":    services.RoundMoney(termsAgg.Total),
			"balanced":       balanced,
		}
		return nil
	})

	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			FailNotFound(c, "项目不存在")
		case engIsConflictErr(err):
			FailDuplicated(c, err.Error())
		default:
			FailBadRequest(c, err.Error())
		}
		return
	}
	OKWithMsg(c, "设备清单已保存", result)
}

// uniqueUint 去重（校验商品存在性时避免重复计数）
func uniqueUint(in []uint) []uint {
	seen := make(map[uint]struct{}, len(in))
	out := make([]uint, 0, len(in))
	for _, v := range in {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

// ---------------------------------------------------------------------------
// T08-b 付款节点整表保存
// ---------------------------------------------------------------------------

type engTermInput struct {
	ID            uint     `json:"id"`
	Name          string   `json:"name"`
	Ratio         *float64 `json:"ratio"`
	PlannedAmount float64  `json:"planned_amount"`
	PlannedDate   *string  `json:"planned_date"`
}

type engTermsSaveReq struct {
	Terms []engTermInput `json:"terms"`
}

// SaveEngineeringTerms 整表保存付款节点 PUT /api/admin/engineering/projects/:id/payment-terms（T08）
// @Summary 整表保存付款节点
// @Tags 工程管理
// @Accept json
// @Produce json
// @Param id path int true "项目 ID"
// @Param body body engTermsSaveReq true "节点数组（合计须等于合同额）"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID save_engineering_terms
// @Router /api/admin/engineering/projects/{id}/payment-terms [put]
func SaveEngineeringTerms(c *gin.Context) {
	id, ok := engIDParam(c, "id")
	if !ok {
		return
	}
	var req engTermsSaveReq
	if err := c.ShouldBindJSON(&req); err != nil {
		FailBadRequest(c, "参数格式错误："+err.Error())
		return
	}

	var result gin.H
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		p, err := engLoadProjectForWrite(tx, id)
		if err != nil {
			return err
		}
		if p.Status == services.EngStatusVoided {
			return errors.New("项目已作废，不可修改付款节点")
		}
		if p.Status == services.EngStatusSettled {
			return errors.New("项目已结清，不可修改付款节点")
		}

		var existing []models.EngineeringPaymentTerm
		if err := tx.Where("project_id = ?", id).Find(&existing).Error; err != nil {
			return err
		}
		existingByID := make(map[uint]models.EngineeringPaymentTerm, len(existing))
		for _, t := range existing {
			existingByID[t.ID] = t
		}

		keep := map[uint]bool{}
		for idx, in := range req.Terms {
			name := strings.TrimSpace(in.Name)
			if engRuneLen(name) == 0 || engRuneLen(name) > 50 {
				return fmt.Errorf("第 %d 个节点名称需在 1~50 字之间", idx+1)
			}
			if services.RoundMoney(in.PlannedAmount) <= 0 {
				return fmt.Errorf("第 %d 个节点计划金额必须大于 0", idx+1)
			}
			if in.Ratio != nil && (*in.Ratio < 0 || *in.Ratio > 100) {
				return fmt.Errorf("第 %d 个节点比例需在 0~100 之间", idx+1)
			}
			if in.ID > 0 {
				old, exists := existingByID[in.ID]
				if !exists {
					return fmt.Errorf("第 %d 个节点不存在，请刷新后重试", idx+1)
				}
				if services.RoundMoney(in.PlannedAmount) < services.RoundMoney(old.ReceivedAmount) {
					return fmt.Errorf("节点「%s」计划金额不能小于已收金额 %s", old.Name,
						services.FormatMoneyCN(old.ReceivedAmount))
				}
				keep[in.ID] = true
			}
		}

		for _, old := range existing {
			if keep[old.ID] {
				continue
			}
			if services.RoundMoney(old.ReceivedAmount) > 0 {
				return fmt.Errorf("节点「%s」已有回款记录，不可删除", old.Name)
			}
			if err := tx.Delete(&models.EngineeringPaymentTerm{}, old.ID).Error; err != nil {
				return err
			}
		}

		for idx, in := range req.Terms {
			plannedDate, err := services.ParseBusinessDate(derefString(in.PlannedDate))
			if err != nil {
				return fmt.Errorf("第 %d 个节点计划回款日格式错误，应为 YYYY-MM-DD", idx+1)
			}
			var ratio interface{}
			if in.Ratio != nil {
				ratio = services.RoundMoney(*in.Ratio)
			}
			if in.ID > 0 {
				if err := tx.Model(&models.EngineeringPaymentTerm{}).Where("id = ?", in.ID).
					Updates(map[string]interface{}{
						"name":           strings.TrimSpace(in.Name),
						"ratio":          ratio,
						"planned_amount": services.RoundMoney(in.PlannedAmount),
						"planned_date":   plannedDate,
						"sort_order":     idx,
					}).Error; err != nil {
					return err
				}
				continue
			}
			row := models.EngineeringPaymentTerm{
				ProjectID: id, Name: strings.TrimSpace(in.Name),
				PlannedAmount: services.RoundMoney(in.PlannedAmount),
				PlannedDate:   plannedDate, SortOrder: idx,
			}
			if in.Ratio != nil {
				r := services.RoundMoney(*in.Ratio)
				row.Ratio = &r
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}

		termsAgg, err := engAggTerms(tx, id)
		if err != nil {
			return err
		}
		balanced := services.MoneyEqual(termsAgg.Total, p.ContractTotal)
		diff := services.RoundMoney(termsAgg.Total - p.ContractTotal)
		if services.EngAmountLocked(p.Status) && !balanced {
			return engConflict(fmt.Sprintf("付款节点合计 %s 与合同总额 %s 相差 %s",
				services.FormatMoneyCN(termsAgg.Total), services.FormatMoneyCN(p.ContractTotal),
				services.FormatMoneyCN(absMoney(diff))))
		}

		var saved []models.EngineeringPaymentTerm
		if err := tx.Where("project_id = ?", id).
			Order("planned_date IS NULL, planned_date ASC, sort_order ASC, id ASC").Find(&saved).Error; err != nil {
			return err
		}
		today := engToday()
		termsDTO := make([]gin.H, 0, len(saved))
		for _, t := range saved {
			planned := services.RoundMoney(t.PlannedAmount)
			received := services.RoundMoney(t.ReceivedAmount)
			pend := services.RoundMoney(planned - received)
			if pend < 0 {
				pend = 0
			}
			status, label, overdueDays := "unpaid", "未收", 0
			switch {
			case pend <= 0:
				status, label = "paid", "已收"
			case t.PlannedDate != nil && t.PlannedDate.Before(today):
				status, label = "overdue", "逾期"
				overdueDays = int(today.Sub(*t.PlannedDate).Hours() / 24)
			case received > 0:
				status, label = "partial", "部分收"
			}
			var ratio interface{}
			if t.Ratio != nil {
				ratio = services.RoundMoney(*t.Ratio)
			}
			termsDTO = append(termsDTO, gin.H{
				"id": t.ID, "name": t.Name, "ratio": ratio,
				"planned_amount": planned, "planned_date": engDatePtr(t.PlannedDate),
				"received_amount": received, "pending_amount": pend,
				"status": status, "status_label": label, "overdue_days": overdueDays,
			})
		}

		result = gin.H{
			"terms_total":    services.RoundMoney(termsAgg.Total),
			"contract_total": services.RoundMoney(p.ContractTotal),
			"balanced":       balanced,
			"diff":           diff,
			"terms":          termsDTO,
		}
		return nil
	})

	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			FailNotFound(c, "项目不存在")
		case engIsConflictErr(err):
			FailDuplicated(c, err.Error())
		default:
			FailBadRequest(c, err.Error())
		}
		return
	}
	OKWithMsg(c, "付款节点已保存", result)
}

func absMoney(v float64) float64 {
	if v < 0 {
		return -v
	}
	return v
}

// ---------------------------------------------------------------------------
// T09 回款登记与冲销
// ---------------------------------------------------------------------------

type engReceiptCreateReq struct {
	TermID          uint    `json:"term_id"`
	Amount          float64 `json:"amount"`
	PaymentDate     string  `json:"payment_date"`
	Method          int     `json:"method"`
	BillNo          string  `json:"bill_no"`
	Remark          string  `json:"remark"`
	ClientRequestID *string `json:"client_request_id"`
}

// CreateEngineeringReceipt 登记回款 POST /api/admin/engineering/projects/:id/receipts（T09）
//
// 并发保护：节点用"条件更新"（已收 + 本次 ≤ 计划）而非先读后写，
// 第二个人会收到冲突提示，不会出现同节点双倍入账。
// @Summary 登记工程回款
// @Tags 工程管理
// @Accept json
// @Produce json
// @Param id path int true "项目 ID"
// @Param body body engReceiptCreateReq true "回款参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID create_engineering_receipt
// @Router /api/admin/engineering/projects/{id}/receipts [post]
func CreateEngineeringReceipt(c *gin.Context) {
	id, ok := engIDParam(c, "id")
	if !ok {
		return
	}
	var req engReceiptCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		FailBadRequest(c, "参数格式错误："+err.Error())
		return
	}
	if req.Amount <= 0 {
		FailBadRequest(c, "回款金额必须大于 0")
		return
	}
	if req.Method < 1 || req.Method > 5 {
		FailBadRequest(c, "请选择收款方式")
		return
	}
	paymentDate, err := services.ParseBusinessDate(req.PaymentDate)
	if err != nil || paymentDate == nil {
		FailBadRequest(c, "到账日期格式错误，应为 YYYY-MM-DD")
		return
	}
	if paymentDate.After(engToday()) {
		FailBadRequest(c, "到账日期不能晚于今天")
		return
	}
	if engRuneLen(req.BillNo) > 50 || engRuneLen(req.Remark) > 200 {
		FailBadRequest(c, "票据号或备注过长")
		return
	}

	// 幂等
	if req.ClientRequestID != nil && strings.TrimSpace(*req.ClientRequestID) != "" {
		var cnt int64
		if err := config.DB.Model(&models.EngineeringReceipt{}).
			Where("client_request_id = ?", *req.ClientRequestID).Count(&cnt).Error; err != nil {
			FailInternal(c, "幂等校验失败："+err.Error())
			return
		}
		if cnt > 0 {
			FailDuplicated(c, "该笔回款已登记")
			return
		}
	}

	operator := InventoryOperatorFromContext(c)
	amount := services.RoundMoney(req.Amount)

	var result gin.H
	err = config.DB.Transaction(func(tx *gorm.DB) error {
		p, err := engLoadProjectForWrite(tx, id)
		if err != nil {
			return err
		}
		if p.Status == services.EngStatusVoided {
			return errors.New("项目已作废，不可登记回款")
		}

		var term models.EngineeringPaymentTerm
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&term, req.TermID).Error; err != nil {
			return errors.New("付款节点不存在")
		}
		if term.ProjectID != id {
			return errors.New("付款节点不属于该项目")
		}
		if err := services.CheckReceiptAmount(term.PlannedAmount, term.ReceivedAmount, amount); err != nil {
			return err
		}

		// 条件更新：并发下第二个请求 RowsAffected=0
		res := tx.Model(&models.EngineeringPaymentTerm{}).
			Where("id = ? AND received_amount + ? <= planned_amount + 0.004", term.ID, amount).
			UpdateColumn("received_amount", gorm.Expr("received_amount + ?", amount))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return engConflict("该节点应收已变化，请刷新后重试")
		}

		receipt := models.EngineeringReceipt{
			ProjectID: id, TermID: term.ID, Amount: amount, PaymentDate: *paymentDate,
			Method: req.Method, BillNo: strings.TrimSpace(req.BillNo), Remark: strings.TrimSpace(req.Remark),
			OperatorID: operator.EmployeeID, OperatorName: operator.Name,
			ClientRequestID: req.ClientRequestID,
		}
		if err := tx.Create(&receipt).Error; err != nil {
			if engIsDuplicateErr(err) {
				return engConflict("该笔回款已登记")
			}
			return err
		}

		// 项目已收以回款流水为准重算（不累加，避免历史脏数据滚雪球）
		var sum struct {
			Received float64 `gorm:"column:received"`
		}
		if err := tx.Model(&models.EngineeringReceipt{}).
			Select("COALESCE(SUM(amount), 0) AS received").
			Where("project_id = ?", id).Scan(&sum).Error; err != nil {
			return err
		}
		projectReceived := services.RoundMoney(sum.Received)
		if err := tx.Model(&models.EngineeringProject{}).Where("id = ?", id).
			UpdateColumn("received_amount", projectReceived).Error; err != nil {
			return err
		}

		// 收齐自动结清（条件更新，幂等：已结清时 RowsAffected=0 不报错）
		// 只认「已验收」：SRS §4.1 状态表里 5 已结清只能由 4 已验收自动触发。
		// 已签约/供货中即使客户先打全款也不跳级 —— 必须先出库、验收，把 accept_date 落下，
		// 否则会出现「没发货没验收就已结清」的状态。
		settled := false
		if p.ContractTotal > 0 && projectReceived >= services.RoundMoney(p.ContractTotal)-0.004 {
			now := time.Now()
			res := tx.Model(&models.EngineeringProject{}).
				Where("id = ? AND status = ?", id, services.EngStatusAccepted).
				Updates(map[string]interface{}{"status": services.EngStatusSettled, "settled_at": &now})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected > 0 {
				settled = true
			}
		}

		finalStatus := p.Status
		if settled {
			finalStatus = services.EngStatusSettled
		}
		termReceived := services.RoundMoney(term.ReceivedAmount + amount)
		termPending := services.RoundMoney(term.PlannedAmount - termReceived)
		if termPending < 0 {
			termPending = 0
		}
		projectPending := services.RoundMoney(p.ContractTotal - projectReceived)
		if projectPending < 0 {
			projectPending = 0
		}

		result = gin.H{
			"receipt_id":           receipt.ID,
			"project_received":     projectReceived,
			"project_pending":      projectPending,
			"term_received":        termReceived,
			"term_pending":         termPending,
			"project_status":       finalStatus,
			"project_status_label": services.EngStatusLabel(finalStatus),
			"settled":              settled,
		}
		return nil
	})

	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			FailNotFound(c, "项目不存在")
		case engIsConflictErr(err):
			FailDuplicated(c, err.Error())
		default:
			FailBadRequest(c, err.Error())
		}
		return
	}
	OKWithMsg(c, "回款已登记", result)
}

type engReceiptReverseReq struct {
	Reason          string  `json:"reason"`
	PaymentDate     *string `json:"payment_date"`
	ClientRequestID *string `json:"client_request_id"`
}

// ReverseEngineeringReceipt 冲销回款 POST /api/admin/engineering/projects/:id/receipts/:rid/reverse（T09）
// @Summary 冲销工程回款
// @Tags 工程管理
// @Accept json
// @Produce json
// @Param id path int true "项目 ID"
// @Param rid path int true "被冲销的回款记录 ID"
// @Param body body engReceiptReverseReq true "冲销参数"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID reverse_engineering_receipt
// @Router /api/admin/engineering/projects/{id}/receipts/{rid}/reverse [post]
func ReverseEngineeringReceipt(c *gin.Context) {
	id, ok := engIDParam(c, "id")
	if !ok {
		return
	}
	rid, ok := engIDParam(c, "rid")
	if !ok {
		return
	}
	var req engReceiptReverseReq
	if err := c.ShouldBindJSON(&req); err != nil {
		FailBadRequest(c, "参数格式错误："+err.Error())
		return
	}
	if engRuneLen(strings.TrimSpace(req.Reason)) < 5 {
		FailBadRequest(c, "冲销原因至少填 5 个字，便于日后追溯")
		return
	}
	paymentDate := engToday()
	if req.PaymentDate != nil {
		pd, err := services.ParseBusinessDate(*req.PaymentDate)
		if err != nil || pd == nil {
			FailBadRequest(c, "冲销日期格式错误，应为 YYYY-MM-DD")
			return
		}
		if pd.After(engToday()) {
			FailBadRequest(c, "冲销日期不能晚于今天")
			return
		}
		paymentDate = *pd
	}

	operator := InventoryOperatorFromContext(c)

	var result gin.H
	err := config.DB.Transaction(func(tx *gorm.DB) error {
		p, err := engLoadProjectForWrite(tx, id)
		if err != nil {
			return err
		}
		if p.Status == services.EngStatusVoided {
			return errors.New("项目已作废，不可冲销回款")
		}

		var origin models.EngineeringReceipt
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&origin, rid).Error; err != nil {
			return errors.New("回款记录不存在")
		}
		if origin.ProjectID != id {
			return errors.New("回款记录不属于该项目")
		}
		if origin.Amount < 0 {
			return errors.New("负数记录不可冲销")
		}
		var reversedCnt int64
		if err := tx.Model(&models.EngineeringReceipt{}).
			Where("reversed_receipt_id = ?", origin.ID).Count(&reversedCnt).Error; err != nil {
			return err
		}
		if reversedCnt > 0 {
			return errors.New("该记录已冲销，请勿重复操作")
		}

		var term models.EngineeringPaymentTerm
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&term, origin.TermID).Error; err != nil {
			return errors.New("付款节点不存在")
		}
		if err := services.CheckReverseAmount(term.ReceivedAmount, origin.Amount); err != nil {
			return err
		}

		res := tx.Model(&models.EngineeringPaymentTerm{}).
			Where("id = ? AND received_amount >= ?", term.ID, origin.Amount).
			UpdateColumn("received_amount", gorm.Expr("received_amount - ?", origin.Amount))
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return engConflict("该节点应收已变化，请刷新后重试")
		}

		reversal := models.EngineeringReceipt{
			ProjectID: id, TermID: origin.TermID, Amount: -services.RoundMoney(origin.Amount),
			PaymentDate: paymentDate, Method: origin.Method, BillNo: origin.BillNo,
			Remark: "冲销：" + strings.TrimSpace(req.Reason), ReversedReceiptID: &origin.ID,
			OperatorID: operator.EmployeeID, OperatorName: operator.Name,
			ClientRequestID: req.ClientRequestID,
		}
		if err := tx.Create(&reversal).Error; err != nil {
			if engIsDuplicateErr(err) {
				return engConflict("该笔冲销已登记")
			}
			return err
		}

		var sum struct {
			Received float64 `gorm:"column:received"`
		}
		if err := tx.Model(&models.EngineeringReceipt{}).
			Select("COALESCE(SUM(amount), 0) AS received").
			Where("project_id = ?", id).Scan(&sum).Error; err != nil {
			return err
		}
		projectReceived := services.RoundMoney(sum.Received)
		if err := tx.Model(&models.EngineeringProject{}).Where("id = ?", id).
			UpdateColumn("received_amount", projectReceived).Error; err != nil {
			return err
		}

		// 已结清但因冲销不再收齐 → 按验收/发货情况回退状态
		finalStatus := p.Status
		if p.Status == services.EngStatusSettled && projectReceived < services.RoundMoney(p.ContractTotal)-0.004 {
			target := services.EngStatusSigned
			if p.AcceptDate != nil {
				target = services.EngStatusAccepted
			} else {
				var shipments int64
				if err := tx.Model(&models.EngineeringShipment{}).Where("project_id = ?", id).Count(&shipments).Error; err != nil {
					return err
				}
				if shipments > 0 {
					target = services.EngStatusSupplying
				}
			}
			if err := tx.Model(&models.EngineeringProject{}).Where("id = ?", id).
				Updates(map[string]interface{}{"status": target, "settled_at": nil}).Error; err != nil {
				return err
			}
			finalStatus = target
		}

		projectPending := services.RoundMoney(p.ContractTotal - projectReceived)
		if projectPending < 0 {
			projectPending = 0
		}
		result = gin.H{
			"receipt_id":           reversal.ID,
			"amount":               reversal.Amount,
			"project_received":     projectReceived,
			"project_pending":      projectPending,
			"project_status":       finalStatus,
			"project_status_label": services.EngStatusLabel(finalStatus),
		}
		return nil
	})

	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			FailNotFound(c, "项目不存在")
		case engIsConflictErr(err):
			FailDuplicated(c, err.Error())
		default:
			FailBadRequest(c, err.Error())
		}
		return
	}
	OKWithMsg(c, "回款已冲销", result)
}
