package controllers

import (
	"strings"
	"testing"
	"time"

	"erp-backend/models"

	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
)

// 成本基准回归测试：锁定"入库不记账、成本只在销售时产生"的记账模型。
//
// 规则（用户 2026-10-29 确认）：
//  1. 采购入库/员工申请入库/盘点增库只记库存和成本基准，不产生任何资金流水；
//     进货成本流水只在销售开单那一刻按订单记在收款账户（order_controller 快照）。
//  2. 零成本盘盈/增库绝不稀释移动加权平均（历史 7-10 稀释事故的回归防护）。
//  3. 无成本基准库存遇到实价采购时必须报错阻断，静默摊薄。
//  4. 成本基准锚点建立后，锚点之前的零成本历史事件（如期初建库存）不参与重放。

func newCostTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		t.Fatalf("打开内存数据库失败: %v", err)
	}
	if err := db.AutoMigrate(&models.Product{}, &models.InventoryLog{}, &models.HistoricalCostRepair{}); err != nil {
		t.Fatalf("迁移测试表失败: %v", err)
	}
	return db
}

func createCostProduct(t *testing.T, db *gorm.DB, name string, avg, latest float64, storeStock int) models.Product {
	t.Helper()
	p := models.Product{Name: name, AvgCost: avg, LatestCost: latest, StoreStock: storeStock}
	if err := db.Create(&p).Error; err != nil {
		t.Fatalf("创建商品失败: %v", err)
	}
	return p
}

func addCostLog(t *testing.T, db *gorm.DB, productID uint, logType string, changeQty int, unitCost float64, at time.Time) {
	t.Helper()
	l := models.InventoryLog{
		ProductID:     productID,
		WarehouseType: 2,
		LogType:       logType,
		ChangeQty:     changeQty,
		UnitCost:      unitCost,
		RelatedNo:     "TEST",
		CreatedAt:     at,
	}
	if err := db.Create(&l).Error; err != nil {
		t.Fatalf("写入库存流水失败: %v", err)
	}
}

func addCostBaseline(t *testing.T, db *gorm.DB, productID uint, qty int, avg float64, at time.Time) {
	t.Helper()
	b := models.HistoricalCostRepair{
		ProductID:       productID,
		UnitCost:        avg,
		PreviousUnitCost: avg,
		StockQty:        qty,
		CostBaseline:    true,
		Remark:          "测试成本基准",
		CreatedAt:       at,
	}
	if err := db.Create(&b).Error; err != nil {
		t.Fatalf("写入成本基准失败: %v", err)
	}
}

func reloadProduct(t *testing.T, db *gorm.DB, id uint) models.Product {
	t.Helper()
	var p models.Product
	if err := db.First(&p, id).Error; err != nil {
		t.Fatalf("读取商品失败: %v", err)
	}
	return p
}

const costTestTolerance = 1e-6

// T1: 盘盈（零成本增库）只加数量，不动平均成本——2026-07-10 稀释事故回归防护。
func TestCostRecalcZeroCostStocktakeDoesNotDilute(t *testing.T) {
	db := newCostTestDB(t)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	p := createCostProduct(t, db, "稀释防护测试机", 30, 30, 10)
	addCostBaseline(t, db, p.ID, 10, 30, base)
	// 员工盘盈 2 台，未填成本（unit_cost=0）
	addCostLog(t, db, p.ID, "stocktake", 2, 0, base.Add(time.Hour))
	if err := db.Model(&models.Product{}).Where("id = ?", p.ID).UpdateColumn("store_stock", 12).Error; err != nil {
		t.Fatalf("更新库存失败: %v", err)
	}

	if err := RecalculateSingleProductCosts(db, p.ID); err != nil {
		t.Fatalf("重算失败: %v", err)
	}
	got := reloadProduct(t, db, p.ID)
	if got.AvgCost != 30 {
		t.Errorf("零成本盘盈后平均成本应为 30（不被稀释），实际 %v", got.AvgCost)
	}
	if got.LatestCost != 30 {
		t.Errorf("零成本盘盈不应改写最新进价，实际 %v", got.LatestCost)
	}
}

// T2: 实价采购入库推进移动加权（30→35 涨价，混合加权 32.5）。
func TestCostRecalcPurchaseUpdatesWeightedAverage(t *testing.T) {
	db := newCostTestDB(t)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	p := createCostProduct(t, db, "加权测试机", 30, 30, 10)
	addCostBaseline(t, db, p.ID, 10, 30, base)
	addCostLog(t, db, p.ID, "purchase", 10, 35, base.Add(time.Hour))
	if err := db.Model(&models.Product{}).Where("id = ?", p.ID).UpdateColumn("store_stock", 20).Error; err != nil {
		t.Fatalf("更新库存失败: %v", err)
	}

	if err := RecalculateSingleProductCosts(db, p.ID); err != nil {
		t.Fatalf("重算失败: %v", err)
	}
	got := reloadProduct(t, db, p.ID)
	if diff := got.AvgCost - 32.5; diff > costTestTolerance || diff < -costTestTolerance {
		t.Errorf("10台@30 + 10台@35 的加权平均应为 32.5，实际 %v", got.AvgCost)
	}
	if got.LatestCost != 35 {
		t.Errorf("最新进价应为 35，实际 %v", got.LatestCost)
	}
}

// T3: 销售扣减不改变平均成本——成本在开单时按当时的基准快照进订单，账面基准保持。
func TestCostRecalcSaleKeepsAverageCost(t *testing.T) {
	db := newCostTestDB(t)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	p := createCostProduct(t, db, "销售快照测试机", 30, 30, 10)
	addCostBaseline(t, db, p.ID, 10, 30, base)
	addCostLog(t, db, p.ID, "sale", -3, 30, base.Add(time.Hour))
	if err := db.Model(&models.Product{}).Where("id = ?", p.ID).UpdateColumn("store_stock", 7).Error; err != nil {
		t.Fatalf("更新库存失败: %v", err)
	}

	if err := RecalculateSingleProductCosts(db, p.ID); err != nil {
		t.Fatalf("重算失败: %v", err)
	}
	got := reloadProduct(t, db, p.ID)
	if got.AvgCost != 30 {
		t.Errorf("销售扣减后平均成本应保持 30，实际 %v", got.AvgCost)
	}
}

// T4: 无成本基准的库存遇到实价采购必须报错阻断，绝不静默摊薄（(0+1099)/2=549.5 事故场景）。
func TestCostRecalcUnpricedStockBlocksPricedPurchase(t *testing.T) {
	db := newCostTestDB(t)
	base := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	p := createCostProduct(t, db, "阻断测试机", 0, 0, 2)
	// 无基准：先盘盈 1 台零成本，再实价采购 1 台
	addCostLog(t, db, p.ID, "stocktake", 1, 0, base)
	addCostLog(t, db, p.ID, "purchase", 1, 1099, base.Add(time.Hour))

	err := RecalculateSingleProductCosts(db, p.ID)
	if err == nil {
		t.Fatal("存在无成本基准库存时应报错阻断，不应静默重算")
	}
	if !strings.Contains(err.Error(), "请先补录成本") {
		t.Errorf("报错应提示先补录成本，实际: %v", err)
	}
	got := reloadProduct(t, db, p.ID)
	if got.AvgCost != 0 {
		t.Errorf("报错阻断时不得改写商品成本，实际 avg_cost=%v", got.AvgCost)
	}
}

// T5: 成本基准锚点建立后，锚点之前的零成本历史事件（如期初建库存）不参与重放。
func TestCostRecalcBaselineIsolatesLegacyZeroCostEvents(t *testing.T) {
	db := newCostTestDB(t)
	base := time.Date(2026, 5, 16, 0, 0, 0, 0, time.UTC)
	p := createCostProduct(t, db, "期初锚点测试机", 2699, 2699, 50)
	// 期初建库存：50 台成本流水记 0（历史遗留形态）
	addCostLog(t, db, p.ID, "purchase", 50, 0, base)
	// 之后商品首次被触碰（盘点/入库）时自动建立基准，锚定当时正确的 avg_cost
	addCostBaseline(t, db, p.ID, 50, 2699, base.Add(24*time.Hour))
	// 基准之后正常卖 2 台
	addCostLog(t, db, p.ID, "sale", -2, 2699, base.Add(48*time.Hour))
	if err := db.Model(&models.Product{}).Where("id = ?", p.ID).UpdateColumn("store_stock", 48).Error; err != nil {
		t.Fatalf("更新库存失败: %v", err)
	}

	if err := RecalculateSingleProductCosts(db, p.ID); err != nil {
		t.Fatalf("重算失败: %v", err)
	}
	got := reloadProduct(t, db, p.ID)
	if got.AvgCost != 2699 {
		t.Errorf("基准锚点应隔离期初零成本事件，平均成本应保持 2699，实际 %v", got.AvgCost)
	}
}
