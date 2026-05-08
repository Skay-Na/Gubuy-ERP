package controllers

import (
	"erp-backend/config"
	"erp-backend/models"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// InboundRequest 入库请求体
type InboundRequest struct {
	ProductID     uint    `json:"product_id" binding:"required"`
	Quantity      int     `json:"quantity" binding:"required,gt=0"`
	UnitCost      float64 `json:"unit_cost" binding:"required,gt=0"`
	AccountID     uint    `json:"account_id" binding:"required"`
	WarehouseType int     `json:"warehouse_type" binding:"required,oneof=1 2 3"`
}

// CreateInbound 处理商品入库，同步更新账户余额（支出）
func CreateInbound(c *gin.Context) {
	var req InboundRequest
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

	// 2. 验证账户存在
	var account models.Account
	if err := tx.First(&account, req.AccountID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  "支付账户不存在",
		})
		return
	}

	// 3. 计算并创建入库明细记录
	totalCost := float64(req.Quantity) * req.UnitCost
	accountID := req.AccountID
	inboundRecord := models.InboundRecord{
		ProductID: req.ProductID,
		Quantity:  req.Quantity,
		UnitCost:  req.UnitCost,
		TotalCost: totalCost,
		AccountID: &accountID,
	}

	if err := tx.Create(&inboundRecord).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "入库记录保存失败",
		})
		return
	}

	// 4. 更新对应的商品记录 (库存累加 + 最新进价覆盖)
	var product models.Product
	if err := tx.First(&product, req.ProductID).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusNotFound, gin.H{
			"code": 404,
			"msg":  fmt.Sprintf("商品 ID %d 不存在", req.ProductID),
		})
		return
	}

	updates := map[string]interface{}{}
	if req.WarehouseType == 1 {
		updates["main_stock"] = gorm.Expr("main_stock + ?", req.Quantity)
	} else if req.WarehouseType == 2 {
		updates["store_stock"] = gorm.Expr("store_stock + ?", req.Quantity)
	} else if req.WarehouseType == 3 {
		updates["cloud_stock"] = gorm.Expr("cloud_stock + ?", req.Quantity)
	}
	updates["latest_cost"] = req.UnitCost

	if err := tx.Model(&product).Updates(updates).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "商品库存或价格更新失败",
		})
		return
	}

	// 5. 更新账户余额（采购支出，减少余额）
	if err := tx.Model(&account).UpdateColumn("balance", gorm.Expr("balance - ?", totalCost)).Error; err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "账户余额扣除失败",
		})
		return
	}

	// 5.5 同步记录财务明细，修正仪表盘总支出不平的问题
	var updatedAccount models.Account
	tx.First(&updatedAccount, accountID)
	if err := WriteFinancialLog(tx, accountID, 2, "采购货款", totalCost, updatedAccount.Balance, "", fmt.Sprintf("采购入库 商品 ID: %d, 入库数量: %d件, 成本价: ¥%.2f", req.ProductID, req.Quantity, req.UnitCost)); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "财务流水写入失败",
		})
		return
	}

	// 5.6 记录库存流水
	beforeQty := 0
	if req.WarehouseType == 1 {
		beforeQty = product.MainStock
	} else if req.WarehouseType == 2 {
		beforeQty = product.StoreStock
	} else if req.WarehouseType == 3 {
		beforeQty = product.CloudStock
	}

	if err := WriteInventoryLog(tx, req.ProductID, req.WarehouseType, "purchase", beforeQty, req.Quantity, "", fmt.Sprintf("采购入库：数量 %d", req.Quantity)); err != nil {
		tx.Rollback()
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "库存流水写入失败",
		})
		return
	}

	// 6. 提交事务
	if err := tx.Commit().Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"code": 500,
			"msg":  "事务提交失败",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"code": 200,
		"msg":  "采购入库成功，账户余额已更新",
		"data": inboundRecord,
	})
}
