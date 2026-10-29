package controllers

// 工程模块 · 出库控制器
//
// 归属任务卡：T10（扣库存 + 写 inventory_logs(log_type=engineering) + 自动供货中）
//
// 铁律：
//  1. 库存扣减复用既有写法（FOR UPDATE 锁商品行 + 条件更新 + WriteInventoryLog），
//     不另造一套，否则与零售开单的库存口径分叉；
//  2. 只扣库存与写流水，不动任何资金账户（工程出库不是销售出库，不产生收款）。

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

type engShipmentLine struct {
	ItemID   uint `json:"item_id"`
	Quantity int  `json:"quantity"`
}

type engShipmentReq struct {
	WarehouseType int               `json:"warehouse_type"`
	ShipDate      string            `json:"ship_date"`
	TrackingNo    string            `json:"tracking_no"`
	Remark        string            `json:"remark"`
	AllowNegative bool              `json:"allow_negative"`
	Lines         []engShipmentLine `json:"lines"`
}

// CreateEngineeringShipment 工程出库 POST /api/admin/engineering/projects/:id/shipments（T10）
//
// 一次请求可含多行，全部在同一事务内：任一商品库存不足则整单回滚。
// @Summary 工程出库（扣减库存并写库存流水）
// @Tags 工程管理
// @Accept json
// @Produce json
// @Param id path int true "项目 ID"
// @Param body body engShipmentReq true "出库参数（allow_negative 允许负库存）"
// @Success 200 {object} APIResponse "成功响应"
// @Security ApiKeyAuth
// @ID create_engineering_shipment
// @Router /api/admin/engineering/projects/{id}/shipments [post]
func CreateEngineeringShipment(c *gin.Context) {
	id, ok := engIDParam(c, "id")
	if !ok {
		return
	}
	var req engShipmentReq
	if err := c.ShouldBindJSON(&req); err != nil {
		FailBadRequest(c, "参数格式错误："+err.Error())
		return
	}
	if req.WarehouseType != 2 && req.WarehouseType != 3 {
		FailBadRequest(c, "请选择出库仓库（门店或云仓）")
		return
	}
	if len(req.Lines) == 0 {
		FailBadRequest(c, "请至少选择一行出库商品")
		return
	}
	shipDate, err := services.ParseBusinessDate(req.ShipDate)
	if err != nil || shipDate == nil {
		FailBadRequest(c, "出库日期格式错误，应为 YYYY-MM-DD")
		return
	}
	if shipDate.After(engToday()) {
		FailBadRequest(c, "出库日期不能晚于今天")
		return
	}
	if engRuneLen(req.TrackingNo) > 100 || engRuneLen(req.Remark) > 200 {
		FailBadRequest(c, "物流单号或备注过长")
		return
	}

	// 同一行重复提交时先合并，避免两次扣减判断错位
	merged := map[uint]int{}
	order := make([]uint, 0, len(req.Lines))
	for _, l := range req.Lines {
		if l.ItemID == 0 {
			FailBadRequest(c, "出库行缺少商品")
			return
		}
		if l.Quantity < 1 {
			FailBadRequest(c, "出库数量必须大于 0")
			return
		}
		if _, exists := merged[l.ItemID]; !exists {
			order = append(order, l.ItemID)
		}
		merged[l.ItemID] += l.Quantity
	}

	operator := InventoryOperatorFromContext(c)

	var result gin.H
	err = config.DB.Transaction(func(tx *gorm.DB) error {
		p, err := engLoadProjectForWrite(tx, id)
		if err != nil {
			return err
		}
		if p.Status == services.EngStatusVoided {
			return errors.New("项目已作废，不可出库")
		}
		if p.Status == services.EngStatusFollowing {
			return errors.New("项目尚未签约，不可出库")
		}

		totalQty := 0
		for _, itemID := range order {
			qty := merged[itemID]

			var item models.EngineeringProjectItem
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&item, itemID).Error; err != nil {
				return fmt.Errorf("出库行不存在（ID %d），请刷新后重试", itemID)
			}
			if item.ProjectID != id {
				return errors.New("出库行不属于该项目")
			}
			if err := services.CheckShipmentQty(item.Quantity, item.ShippedQty, qty); err != nil {
				return err
			}

			// 锁商品行拿准确的前库存（同时串行化同商品的并发出库）
			var product models.Product
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, item.ProductID).Error; err != nil {
				return errors.New("商品不存在或已删除，无法出库")
			}
			beforeQty := product.StoreStock
			stockField := "store_stock"
			if req.WarehouseType == 3 {
				beforeQty = product.CloudStock
				stockField = "cloud_stock"
			}
			if beforeQty < qty && !req.AllowNegative {
				return fmt.Errorf("商品「%s」%s库存不足（当前 %d 台），如需继续请确认负库存出库",
					product.Name, services.EngWarehouseLabel(req.WarehouseType), beforeQty)
			}

			if err := tx.Model(&models.Product{}).Where("id = ?", product.ID).
				UpdateColumn(stockField, gorm.Expr(stockField+" - ?", qty)).Error; err != nil {
				return err
			}

			if err := WriteInventoryLog(tx, product.ID, req.WarehouseType, "engineering",
				beforeQty, -qty, item.UnitCost, p.ProjectNo,
				fmt.Sprintf("工程出库：%s（%s）", p.Name, p.ProjectNo), operator); err != nil {
				return err
			}

			res := tx.Model(&models.EngineeringProjectItem{}).
				Where("id = ? AND shipped_qty + ? <= quantity", item.ID, qty).
				UpdateColumn("shipped_qty", gorm.Expr("shipped_qty + ?", qty))
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected == 0 {
				return engConflict("该行待发数量已变化，请刷新后重试")
			}

			shipment := models.EngineeringShipment{
				ProjectID: id, ItemID: item.ID, ProductID: item.ProductID, Quantity: qty,
				WarehouseType: req.WarehouseType, ShipDate: *shipDate,
				TrackingNo: strings.TrimSpace(req.TrackingNo), Remark: strings.TrimSpace(req.Remark),
				AllowNegative: req.AllowNegative,
				OperatorID:    operator.EmployeeID, OperatorName: operator.Name,
			}
			if err := tx.Create(&shipment).Error; err != nil {
				return err
			}
			totalQty += qty
		}

		// 项目已发台数以清单行为准重算
		agg, err := engAggItems(tx, id)
		if err != nil {
			return err
		}
		updates := map[string]interface{}{"shipped_qty": agg.ShippedQty}
		finalStatus := p.Status
		// 首次出库自动推进 2 → 3（条件更新，幂等）
		if p.Status == services.EngStatusSigned {
			updates["status"] = services.EngStatusSupplying
			finalStatus = services.EngStatusSupplying
		}
		if err := tx.Model(&models.EngineeringProject{}).Where("id = ?", id).Updates(updates).Error; err != nil {
			return err
		}

		shippedRatio := 0
		if agg.OrderedQty > 0 {
			shippedRatio = int(services.RoundMoney(float64(agg.ShippedQty) / float64(agg.OrderedQty) * 100))
			if shippedRatio > 100 {
				shippedRatio = 100
			}
		}
		result = gin.H{
			"shipment_count":       len(order),
			"total_qty":            totalQty,
			"project_status":       finalStatus,
			"project_status_label": services.EngStatusLabel(finalStatus),
			"shipped_qty":          agg.ShippedQty,
			"ordered_qty":          agg.OrderedQty,
			"shipped_ratio":        shippedRatio,
			"ship_date":            shipDate.Format("2006-01-02"),
			"created_at":           time.Now(),
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
	OKWithMsg(c, "出库已登记", result)
}
