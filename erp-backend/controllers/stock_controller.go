package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// TransferRequest 调拨请求
type TransferRequest struct {
	Action   string `json:"action" binding:"required,oneof=main_to_store store_to_sample sample_to_store main_to_cloud cloud_to_main"`
	Quantity int    `json:"quantity" binding:"required,gt=0"`
}

// TransferProduct 处理商品调拨
func TransferProduct(c *gin.Context) {
	id := c.Param("id")
	var req TransferRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 加锁商品行（FOR UPDATE），防止并发调拨/下单时库存计算竞争
	var product models.Product
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "商品不存在"})
		return
	}

	var err error
	switch req.Action {
	case "main_to_store":
		if product.MainStock < req.Quantity {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "主仓库存不足"})
			return
		}
		err = tx.Model(&product).Updates(map[string]interface{}{
			"main_stock":  gorm.Expr("main_stock - ?", req.Quantity),
			"store_stock": gorm.Expr("store_stock + ?", req.Quantity),
		}).Error
	case "store_to_sample":
		if product.StoreStock < req.Quantity {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "门店库存不足"})
			return
		}
		err = tx.Model(&product).Updates(map[string]interface{}{
			"store_stock":  gorm.Expr("store_stock - ?", req.Quantity),
			"sample_stock": gorm.Expr("sample_stock + ?", req.Quantity),
		}).Error
	case "sample_to_store":
		if product.SampleStock < req.Quantity {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "样机数量不足"})
			return
		}
		err = tx.Model(&product).Updates(map[string]interface{}{
			"sample_stock": gorm.Expr("sample_stock - ?", req.Quantity),
			"store_stock":  gorm.Expr("store_stock + ?", req.Quantity),
		}).Error
	case "main_to_cloud":
		if product.MainStock < req.Quantity {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "主仓库存不足"})
			return
		}
		err = tx.Model(&product).Updates(map[string]interface{}{
			"main_stock":  gorm.Expr("main_stock - ?", req.Quantity),
			"cloud_stock": gorm.Expr("cloud_stock + ?", req.Quantity),
		}).Error
	case "cloud_to_main":
		if product.CloudStock < req.Quantity {
			tx.Rollback()
			c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "云仓库存不足"})
			return
		}
		err = tx.Model(&product).Updates(map[string]interface{}{
			"cloud_stock": gorm.Expr("cloud_stock - ?", req.Quantity),
			"main_stock":  gorm.Expr("main_stock + ?", req.Quantity),
		}).Error
	}

	if err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "调拨失败"})
		return
	}

	// 记录库存流水 (双向流水)
	sourceWarehouse := 0
	destWarehouse := 0
	sourceRemark := ""
	destRemark := ""

	switch req.Action {
	case "main_to_store":
		sourceWarehouse, destWarehouse = 1, 2
		sourceRemark, destRemark = "调拨出库 -> 门店", "调拨入库 <- 主仓"
	case "store_to_sample":
		sourceWarehouse, destWarehouse = 2, 4
		sourceRemark, destRemark = "调拨出库 -> 样机", "调拨入库 <- 门店"
	case "sample_to_store":
		sourceWarehouse, destWarehouse = 4, 2
		sourceRemark, destRemark = "调拨出库 -> 门店", "调拨入库 <- 样机"
	case "main_to_cloud":
		sourceWarehouse, destWarehouse = 1, 3
		sourceRemark, destRemark = "调拨出库 -> 云仓", "调拨入库 <- 主仓"
	case "cloud_to_main":
		sourceWarehouse, destWarehouse = 3, 1
		sourceRemark, destRemark = "调拨出库 -> 主仓", "调拨入库 <- 云仓"
	}

	// 简单的获取变动前库存（基于内存对象，事务内已加锁）
	var sourceBefore, destBefore int
	if sourceWarehouse == 1 { sourceBefore = product.MainStock } else if sourceWarehouse == 2 { sourceBefore = product.StoreStock } else if sourceWarehouse == 3 { sourceBefore = product.CloudStock } else if sourceWarehouse == 4 { sourceBefore = product.SampleStock }
	if destWarehouse == 1 { destBefore = product.MainStock } else if destWarehouse == 2 { destBefore = product.StoreStock } else if destWarehouse == 3 { destBefore = product.CloudStock } else if destWarehouse == 4 { destBefore = product.SampleStock }

	// 写入出库流
	WriteInventoryLog(tx, product.ID, sourceWarehouse, "transfer", sourceBefore, -req.Quantity, "", sourceRemark)
	// 写入入库流
	WriteInventoryLog(tx, product.ID, destWarehouse, "transfer", destBefore, req.Quantity, "", destRemark)

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "事务提交失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "调拨成功"})
}

// StocktakeRequest 盘库请求
type StocktakeRequest struct {
	WarehouseType int    `json:"warehouse_type" binding:"required,oneof=1 2 3"`
	ActualStock   int    `json:"actual_stock" binding:"min=0"`
	Remark        string `json:"remark"`
	// StocktakeType: 1-正常损耗(盘亏计入财务), 2-业务调整(仅修正库存，不计亏损)
	StocktakeType int `json:"stocktake_type"`
}

// ProductStocktake 盘点接口
func ProductStocktake(c *gin.Context) {
	id := c.Param("id")
	var req StocktakeRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": 400, "msg": "参数错误: " + err.Error()})
		return
	}

	tx := config.DB.Begin()
	defer func() {
		if r := recover(); r != nil {
			tx.Rollback()
		}
	}()

	// 加锁商品行（FOR UPDATE），防止盘点时被并发订单覆盖库存
	var product models.Product
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, id).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{"code": 404, "msg": "商品不存在"})
		return
	}

	var beforeStock int
	var updateField string
	var warehouseName string
	if req.WarehouseType == 1 {
		beforeStock = product.MainStock
		updateField = "main_stock"
		warehouseName = "主仓"
	} else if req.WarehouseType == 2 {
		beforeStock = product.StoreStock
		updateField = "store_stock"
		warehouseName = "门店"
	} else if req.WarehouseType == 3 {
		beforeStock = product.CloudStock
		updateField = "cloud_stock"
		warehouseName = "云仓"
	}

	diff := req.ActualStock - beforeStock

	// 更新库存
	if err := tx.Model(&product).UpdateColumn(updateField, req.ActualStock).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "库存更新失败"})
		return
	}

	// 默认盘点性质为正常损耗
	stocktakeType := req.StocktakeType
	if stocktakeType == 0 {
		stocktakeType = 1
	}

	// 记录盘点
	record := models.StocktakeRecord{
		ProductID:     product.ID,
		WarehouseType: req.WarehouseType,
		StocktakeType: stocktakeType,
		BeforeStock:   beforeStock,
		AfterStock:    req.ActualStock,
		Difference:    diff,
		Remark:        req.Remark,
	}
	if err := tx.Create(&record).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "保存盘点记录失败"})
		return
	}

	// 记录库存流水
	if err := WriteInventoryLog(tx, product.ID, req.WarehouseType, "stocktake", beforeStock, diff, "", fmt.Sprintf("库存盘点：%s", req.Remark)); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "库存流水写入失败"})
		return
	}

	// 仅当盘亏 且 性质为"正常损耗(1)"时，才生成财务亏损记录
	// "业务调整(2)"（如其它门店销售、移库）仅修正库存，不计亏损
	if diff < 0 && stocktakeType == 1 {
		lossAmount := float64(-diff) * product.LatestCost

		// 优先寻找公户，否则找任意账户
		var account models.Account
		if err := tx.Where("name = ?", "公户").First(&account).Error; err != nil {
			tx.First(&account) // 兜底，忽略错误
		}

		if account.ID != 0 {
			if err := tx.Model(&account).UpdateColumn("balance", gorm.Expr("balance - ?", lossAmount)).Error; err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "扣除盘亏财务账户失败"})
				return
			}
			
			var updatedAccount models.Account
			tx.First(&updatedAccount, account.ID)
			if err := WriteFinancialLog(tx, account.ID, 2, "盘亏损耗", lossAmount, updatedAccount.Balance, "", fmt.Sprintf("%s商品 %s 盘亏 %d 件，损耗成本 ¥%.2f", warehouseName, product.Name, -diff, lossAmount)); err != nil {
				tx.Rollback()
				c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "生成财务损失记录失败"})
				return
			}
		}
	}

	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": 500, "msg": "事务提交失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"code": 200, "msg": "盘点完成", "difference": diff})
}
