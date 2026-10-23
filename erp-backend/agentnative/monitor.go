package agentnative

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"time"

	"erp-backend/models"
	"erp-backend/services"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const SingleTenant = "single"

type StockObservation struct {
	ProductID   uint `json:"product_id"`
	Available   int  `json:"available"`
	Sales30Days int  `json:"sales_30_days"`
}

type LowStockAssessment struct {
	Threshold int
	Quantity  int
	Eligible  bool
}

// AssessLowStock requires at least three sales in the observation window.
// The threshold is a seven-day sales estimate; the draft aims for fourteen days.
// Neither number claims to include lead time or supplier availability.
func AssessLowStock(o StockObservation) LowStockAssessment {
	if o.Available < 0 || o.Sales30Days < 3 {
		return LowStockAssessment{}
	}
	threshold := int(math.Ceil(float64(o.Sales30Days) * 7 / 30))
	if threshold < 2 {
		threshold = 2
	}
	if o.Available >= threshold {
		return LowStockAssessment{Threshold: threshold}
	}
	target := int(math.Ceil(float64(o.Sales30Days) * 14 / 30))
	if target < threshold {
		target = threshold
	}
	quantity := target - o.Available
	if quantity > 100 {
		quantity = 100
	}
	return LowStockAssessment{Threshold: threshold, Quantity: quantity, Eligible: true}
}

type MonitorService struct {
	DB             *gorm.DB
	Now            func() time.Time
	Planner        AgentPlanner
	LeaseDuration  time.Duration
	OnRuntimeEvent func(RuntimeEvent)
}

func (s MonitorService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Observe reads the same business product and sales records used by ERP pages.
// It never changes stock, cost, orders or account balances.
func (s MonitorService) Observe(ctx context.Context) ([]StockObservation, error) {
	if s.DB == nil {
		return nil, errors.New("database unavailable")
	}
	now := s.now()
	contexts, err := (services.RetailReadService{DB: s.DB}).GetReplenishmentContext(ctx, now.AddDate(0, 0, -30), now)
	if err != nil {
		return nil, fmt.Errorf("read replenishment context: %w", err)
	}
	observations := make([]StockObservation, 0, len(contexts))
	for _, item := range contexts {
		observations = append(observations, StockObservation{
			ProductID: item.ProductID, Available: item.Available, Sales30Days: item.Sales,
		})
	}
	return observations, nil
}

// Scan writes at most one low-stock signal per product per local day.
// A unique fingerprint makes repeated scheduler calls idempotent.
func (s MonitorService) Scan(ctx context.Context) (int, error) {
	observations, err := s.Observe(ctx)
	if err != nil {
		return 0, err
	}
	openSignals := []models.AgentSignal{}
	if err := s.DB.WithContext(ctx).Select("product_id").
		Where("tenant_id = ? AND signal_type = ? AND status = ?", SingleTenant, "low_stock", "open").
		Find(&openSignals).Error; err != nil {
		return 0, fmt.Errorf("read open signals: %w", err)
	}
	openProducts := make(map[uint]bool, len(openSignals))
	for _, signal := range openSignals {
		openProducts[signal.ProductID] = true
	}
	count := 0
	for _, observation := range observations {
		assessment := AssessLowStock(observation)
		if !assessment.Eligible {
			if openProducts[observation.ProductID] {
				if err := s.resolveRecoveredStock(ctx, observation.ProductID); err != nil {
					return count, err
				}
			}
			continue
		}
		if openProducts[observation.ProductID] {
			continue
		}
		metrics, err := json.Marshal(struct {
			StockObservation
			Threshold int `json:"threshold"`
		}{StockObservation: observation, Threshold: assessment.Threshold})
		if err != nil {
			return count, fmt.Errorf("encode signal: %w", err)
		}
		now := s.now()
		signal := models.AgentSignal{
			TenantID:    SingleTenant,
			Fingerprint: fmt.Sprintf("%s:low_stock:%d:%s", SingleTenant, observation.ProductID, now.Format("2006-01-02")),
			SignalType:  "low_stock",
			Severity:    "medium",
			ProductID:   observation.ProductID,
			MetricsJSON: string(metrics),
			Evidence:    "近30天有效订单销量和当前门店、云仓库存；样机不计入可售库存",
			Status:      "open",
			DetectedAt:  now,
		}
		inserted := false
		err = s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&signal)
			if result.Error != nil || result.RowsAffected == 0 {
				return result.Error
			}
			inserted = true
			return nil
		})
		if err != nil {
			return count, fmt.Errorf("save signal: %w", err)
		}
		if inserted {
			count++
			openProducts[observation.ProductID] = true
		}
	}
	return count, nil
}

func (s MonitorService) resolveRecoveredStock(ctx context.Context, productID uint) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		signals := []models.AgentSignal{}
		if err := tx.Where(
			"tenant_id = ? AND product_id = ? AND signal_type = ? AND status = ?",
			SingleTenant, productID, "low_stock", "open",
		).Find(&signals).Error; err != nil {
			return err
		}
		for _, signal := range signals {
			if err := tx.Model(&signal).Where("status = ?", "open").Update("status", "resolved").Error; err != nil {
				return err
			}
			var task models.AgentWorkTask
			if err := tx.Where("signal_id = ? AND tenant_id = ?", signal.ID, SingleTenant).First(&task).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.AgentReplenishmentDraft{}).
				Where("task_id = ? AND tenant_id = ? AND status = ?", task.ID, SingleTenant, "suggested").
				Update("status", "resolved").Error; err != nil {
				return err
			}
			if err := tx.Create(&models.AgentActivity{
				TenantID: SingleTenant, TaskID: task.ID, Event: "stock_recovered",
				Detail: "最新库存与销量不再满足缺货规则，信号和内部草案已关闭",
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// ProcessPending claims persisted work. A stale lease can be reclaimed after
// a process restart; unique TaskID on drafts prevents duplicate suggestions.
func (s MonitorService) ProcessPending(ctx context.Context) (int, error) {
	return s.processPending(ctx, 0)
}

func (s MonitorService) processPending(ctx context.Context, onlyTaskID uint) (int, error) {
	if s.DB == nil {
		return 0, errors.New("database unavailable")
	}
	if err := NewRuntimeExecutor(s.DB).ReconcileCompletedTasks(ctx); err != nil {
		return 0, err
	}
	now := s.now()
	var tasks []models.AgentWorkTask
	query := s.DB.WithContext(ctx).Where(
		"tenant_id = ? AND attempts < ? AND (status IN (?, ?) OR (status = ? AND lease_until < ?))",
		SingleTenant, 3, "CREATED", "FAILED", "RUNNING", now,
	).Where("kind NOT IN ?", []string{"replenishment", "replenishment_analysis", "schedule_inventory_check"}).
		Where("created_by NOT IN ?", []string{"system_monitor", "system_scheduler"})
	if onlyTaskID != 0 {
		query = query.Where("id = ?", onlyTaskID)
	}
	err := query.Order("id").Limit(100).Find(&tasks).Error
	if err != nil {
		return 0, fmt.Errorf("read tasks: %w", err)
	}
	completed := 0
	for _, task := range tasks {
		leaseDuration := s.LeaseDuration
		if leaseDuration <= 0 {
			leaseDuration = 2 * time.Minute
		}
		lease := now.Add(leaseDuration)
		claimed := false
		err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			result := tx.Model(&models.AgentWorkTask{}).
				Where("id = ? AND tenant_id = ? AND attempts < ? AND (status IN (?, ?) OR (status = ? AND lease_until < ?))",
					task.ID, SingleTenant, 3, "CREATED", "FAILED", "RUNNING", now).
				Updates(map[string]any{"status": "RUNNING", "step": "planner", "attempts": gorm.Expr("attempts + 1"), "lease_until": lease, "lease_version": gorm.Expr("lease_version + 1")})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected == 0 {
				return nil
			}
			claimed = true
			if task.Status == "RUNNING" {
				if err := tx.Create(&models.AgentActivity{TenantID: SingleTenant, TaskID: task.ID,
					Event: "lease_reclaimed", Detail: "过期 Worker 租约被重新认领；新租约版本已生效"}).Error; err != nil {
					return err
				}
			}
			if task.Kind == "schedule_inventory_check" {
				if err := tx.Model(&models.AgentScheduleRun{}).
					Where("linked_task_id = ? AND status IN ?", task.ID, []string{"QUEUED", "FAILED", "RUNNING"}).
					Updates(map[string]any{"status": "RUNNING", "actual_started_at": now, "finished_at": nil,
						"error": "", "retry_count": gorm.Expr("retry_count + 1"), "lease_until": lease,
						"lease_version": gorm.Expr("lease_version + 1")}).Error; err != nil {
					return fmt.Errorf("start scheduled run: %w", err)
				}
			}
			return nil
		})
		if err != nil {
			return completed, fmt.Errorf("claim task: %w", err)
		}
		if !claimed {
			continue
		}
		task.LeaseVersion++
		runtime := NewRuntimeExecutor(s.DB)
		runtime.Planner = s.Planner
		runtime.Now = s.now
		runtime.Monitor.Now = s.now
		runtime.Monitor.Planner = s.Planner
		runtime.OnEvent = s.OnRuntimeEvent
		taskCtx, stopTaskContext := watchTaskLease(ctx, s.DB, task, s.now)
		var executeErr error
		switch task.Kind {
		case "agent_analysis", "schedule_inventory_check":
			executeErr = runtime.ExecutePlannerTask(taskCtx, task, now)
		case "replenishment_analysis":
			if s.Planner != nil {
				executeErr = runtime.ExecutePlannerTask(taskCtx, task, now)
			} else {
				executeErr = runtime.ExecutePersistedTask(taskCtx, task, now)
			}
		default:
			executeErr = runtime.ExecutePersistedTask(taskCtx, task, now)
		}
		stopTaskContext()
		if errors.Is(executeErr, ErrTaskCancelled) {
			continue
		} else if executeErr != nil {
			failureCtx, cancelFailure := context.WithTimeout(context.Background(), 3*time.Second)
			recorded, err := s.recordTaskFailure(failureCtx, task, truncateRunes(executeErr.Error(), 40), s.now())
			cancelFailure()
			if err != nil {
				return completed, fmt.Errorf("record task failure: %w", err)
			}
			if !recorded {
				continue
			}
			return completed, fmt.Errorf("task %d: %w", task.ID, executeErr)
		}
		completed++
	}
	return completed, nil
}

func (s MonitorService) recordTaskFailure(ctx context.Context, task models.AgentWorkTask, errorCode string, now time.Time) (bool, error) {
	recorded := false
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		updated := tx.Model(&models.AgentWorkTask{}).
			Where("id = ? AND status = ? AND lease_version = ?", task.ID, "RUNNING", task.LeaseVersion).
			Updates(map[string]any{"status": "FAILED", "error_code": errorCode, "lease_until": nil})
		if updated.Error != nil {
			return updated.Error
		}
		if updated.RowsAffected == 0 {
			return nil
		}
		recorded = true
		if err := tx.Model(&models.AgentScheduleRun{}).Where("linked_task_id = ? AND status = ?", task.ID, "RUNNING").
			Updates(map[string]any{"status": "FAILED", "finished_at": now, "error": errorCode, "lease_until": nil}).Error; err != nil {
			return err
		}
		if task.ChatMessageID != nil {
			if err := tx.Model(&models.AIAgentMessage{}).Where("id = ?", *task.ChatMessageID).
				Updates(map[string]any{"status": "incomplete", "content": "Agent 任务执行失败，请在工作台查看真实执行状态。"}).Error; err != nil {
				return err
			}
		}
		return tx.Create(&models.AgentActivity{TenantID: SingleTenant, TaskID: task.ID,
			Event: "task_failed", Detail: "Agent 任务失败；错误分类已记录，未将失败显示为成功"}).Error
	})
	return recorded, err
}

func (s MonitorService) createDraft(ctx context.Context, task models.AgentWorkTask, leaseVersion uint64) error {
	return s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var current models.AgentWorkTask
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND tenant_id = ?", task.ID, SingleTenant).First(&current).Error; err != nil {
			return err
		}
		if err := validateTaskLease(current, models.AgentWorkTask{LeaseVersion: leaseVersion}, s.now()); err != nil {
			return err
		}
		var existing models.AgentReplenishmentDraft
		if err := tx.Where("task_id = ? AND tenant_id = ?", task.ID, SingleTenant).First(&existing).Error; err == nil {
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		var signal models.AgentSignal
		if task.SignalID == nil {
			return errors.New("replenishment task is missing its signal")
		}
		if err := tx.Where("id = ? AND tenant_id = ?", *task.SignalID, SingleTenant).First(&signal).Error; err != nil {
			return err
		}
		var product models.Product
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).First(&product, signal.ProductID).Error; err != nil {
			return err
		}
		var metrics struct {
			StockObservation
			Threshold int `json:"threshold"`
		}
		if err := json.Unmarshal([]byte(signal.MetricsJSON), &metrics); err != nil {
			return err
		}
		currentStock := product.StoreStock + product.CloudStock
		assessment := AssessLowStock(StockObservation{
			ProductID: product.ID, Available: currentStock, Sales30Days: metrics.Sales30Days,
		})
		step := "draft_ready"
		detail := "已生成内部补货草案；供应商、交付周期和在途采购数据缺失，未提交采购订单"
		if !assessment.Eligible {
			step = "resolved"
			detail = "库存变化后不再满足缺货阈值，未生成草案"
			if err := tx.Model(&signal).Update("status", "resolved").Error; err != nil {
				return err
			}
		} else {
			draft := models.AgentReplenishmentDraft{
				TenantID: SingleTenant, TaskID: task.ID, ProductID: product.ID,
				Quantity: assessment.Quantity, StockSnapshot: currentStock,
				Sales30Days: metrics.Sales30Days, CostSnapshot: product.LatestCost,
				DataNote: "仅按近30天销量估算14天目标库存；缺少供应商、交付周期、在途订单和预算数据，需人工核对",
				Status:   "suggested",
			}
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&draft).Error; err != nil {
				return err
			}
		}
		var activityCount int64
		if err := tx.Model(&models.AgentActivity{}).Where("task_id = ? AND event = ?", task.ID, step).Count(&activityCount).Error; err != nil {
			return err
		}
		if activityCount == 0 {
			if err := tx.Create(&models.AgentActivity{
				TenantID: SingleTenant, TaskID: task.ID, Event: step, Detail: detail,
			}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
