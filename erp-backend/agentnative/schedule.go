package agentnative

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"erp-backend/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrScheduleNotFound = errors.New("schedule not found")
	ErrInvalidSchedule  = errors.New("invalid schedule configuration")
)

type ScheduleService struct {
	DB  *gorm.DB
	Now func() time.Time
}

type ScheduleInput struct {
	OwnerID        string
	TaskType       string
	Timezone       string
	TimeOfDay      string
	IdempotencyKey string
}

func (s ScheduleService) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

func (s ScheduleService) Create(ctx context.Context, input ScheduleInput) (models.AgentSchedule, error) {
	if s.DB == nil || strings.TrimSpace(input.OwnerID) == "" || input.TaskType != "daily_inventory_check" {
		return models.AgentSchedule{}, ErrInvalidSchedule
	}
	if input.Timezone == "" {
		input.Timezone = "Asia/Shanghai"
	}
	if input.TimeOfDay == "" {
		input.TimeOfDay = "09:00"
	}
	if _, err := NextDailyRun(s.now(), input.Timezone, input.TimeOfDay); err != nil {
		return models.AgentSchedule{}, ErrInvalidSchedule
	}
	params, _ := json.Marshal(map[string]string{"scope": "all"})
	next, _ := NextDailyRun(s.now(), input.Timezone, input.TimeOfDay)
	var idempotencyKey *string
	if input.IdempotencyKey != "" {
		if len(input.IdempotencyKey) > 140 {
			return models.AgentSchedule{}, ErrInvalidSchedule
		}
		// Keep the stored key globally unique for compatibility with existing
		// schemas while making the caller's key an owner-scoped idempotency key.
		keyHash := sha256.Sum256([]byte(input.OwnerID + "\x00" + input.IdempotencyKey))
		key := "owner:" + hex.EncodeToString(keyHash[:])
		idempotencyKey = &key
		var existing models.AgentSchedule
		err := s.DB.WithContext(ctx).Where("owner_id = ? AND idempotency_key = ?", input.OwnerID, key).First(&existing).Error
		if err == nil {
			return existing, nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return models.AgentSchedule{}, err
		}
	}
	schedule := models.AgentSchedule{
		OwnerID: input.OwnerID, IdempotencyKey: idempotencyKey, TaskType: input.TaskType, ParametersJSON: string(params),
		Timezone: input.Timezone, ScheduleExpression: "daily " + input.TimeOfDay,
		NextRunAt: next.UTC(), Enabled: true, Status: "active",
	}
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var existing models.AgentSchedule
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("owner_id = ? AND task_type = ?", input.OwnerID, input.TaskType).First(&existing).Error
		if err == nil {
			if existing.Timezone == input.Timezone && existing.ScheduleExpression == schedule.ScheduleExpression && existing.Status != "cancelled" {
				schedule = existing
				return nil
			}
			if err := tx.Model(&existing).Updates(map[string]any{
				"idempotency_key": idempotencyKey, "parameters_json": string(params), "timezone": input.Timezone,
				"schedule_expression": schedule.ScheduleExpression, "next_run_at": next.UTC(), "enabled": true, "status": "active",
			}).Error; err != nil {
				return err
			}
			schedule = existing
			schedule.IdempotencyKey = idempotencyKey
			schedule.ParametersJSON, schedule.Timezone = string(params), input.Timezone
			schedule.ScheduleExpression, schedule.NextRunAt = "daily "+input.TimeOfDay, next.UTC()
			schedule.Enabled, schedule.Status = true, "active"
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		result := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&schedule)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return tx.Where("owner_id = ? AND task_type = ?", input.OwnerID, input.TaskType).First(&schedule).Error
		}
		return nil
	})
	if err != nil {
		return models.AgentSchedule{}, fmt.Errorf("create or reuse schedule: %w", err)
	}
	return schedule, nil
}

func (s ScheduleService) List(ctx context.Context, ownerID string) ([]models.AgentSchedule, error) {
	items := []models.AgentSchedule{}
	if err := s.DB.WithContext(ctx).Where("owner_id = ?", ownerID).Order("id DESC").Limit(100).Find(&items).Error; err != nil {
		return nil, fmt.Errorf("list schedules: %w", err)
	}
	return items, nil
}

func (s ScheduleService) Update(ctx context.Context, ownerID string, id uint, timezone, timeOfDay string) (models.AgentSchedule, error) {
	if id == 0 || strings.TrimSpace(ownerID) == "" || timezone == "" || timeOfDay == "" {
		return models.AgentSchedule{}, ErrInvalidSchedule
	}
	next, err := NextDailyRun(s.now(), timezone, timeOfDay)
	if err != nil {
		return models.AgentSchedule{}, ErrInvalidSchedule
	}
	var schedule models.AgentSchedule
	result := s.DB.WithContext(ctx).Model(&schedule).Where("id = ? AND owner_id = ? AND task_type = ?", id, ownerID, "daily_inventory_check").
		Updates(map[string]any{"timezone": timezone, "schedule_expression": "daily " + timeOfDay, "next_run_at": next.UTC()})
	if result.Error != nil {
		return models.AgentSchedule{}, fmt.Errorf("update schedule: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		return models.AgentSchedule{}, ErrScheduleNotFound
	}
	if err := s.DB.WithContext(ctx).Where("id = ? AND owner_id = ?", id, ownerID).First(&schedule).Error; err != nil {
		return models.AgentSchedule{}, err
	}
	return schedule, nil
}

func (s ScheduleService) SetState(ctx context.Context, ownerID string, id uint, action string) (models.AgentSchedule, error) {
	if id == 0 || strings.TrimSpace(ownerID) == "" {
		return models.AgentSchedule{}, ErrInvalidSchedule
	}
	if action != "pause" && action != "resume" && action != "cancel" {
		return models.AgentSchedule{}, ErrInvalidSchedule
	}
	var schedule models.AgentSchedule
	err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND owner_id = ?", id, ownerID).First(&schedule).Error; errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrScheduleNotFound
		} else if err != nil {
			return err
		}
		updates := map[string]any{}
		switch action {
		case "pause":
			updates = map[string]any{"enabled": false, "status": "paused"}
		case "resume":
			if schedule.Status == "cancelled" {
				return ErrInvalidSchedule
			}
			timeOfDay := strings.TrimPrefix(schedule.ScheduleExpression, "daily ")
			next, err := NextDailyRun(s.now(), schedule.Timezone, timeOfDay)
			if err != nil {
				return ErrInvalidSchedule
			}
			updates = map[string]any{"enabled": true, "status": "active", "next_run_at": next.UTC()}
		case "cancel":
			updates = map[string]any{"enabled": false, "status": "cancelled"}
		}
		if err := tx.Model(&schedule).Updates(updates).Error; err != nil {
			return err
		}
		if action == "cancel" {
			if err := tx.Model(&models.AgentScheduleRun{}).Where("schedule_id = ? AND status = ?", id, "QUEUED").
				Updates(map[string]any{"status": "CANCELLED", "finished_at": s.now()}).Error; err != nil {
				return err
			}
			if err := tx.Model(&models.AgentWorkTask{}).Where("id IN (?) AND status = ?",
				tx.Model(&models.AgentScheduleRun{}).Select("linked_task_id").Where("schedule_id = ? AND status = ?", id, "CANCELLED"), "CREATED").
				Updates(map[string]any{"status": "CANCELLED", "step": "schedule_cancelled"}).Error; err != nil {
				return err
			}
		}
		return tx.Where("id = ? AND owner_id = ?", id, ownerID).First(&schedule).Error
	})
	if err != nil {
		return models.AgentSchedule{}, fmt.Errorf("change schedule state: %w", err)
	}
	if schedule.ID == 0 {
		return models.AgentSchedule{}, ErrScheduleNotFound
	}
	if err := s.DB.WithContext(ctx).Where("id = ? AND owner_id = ?", id, ownerID).First(&schedule).Error; err != nil {
		return models.AgentSchedule{}, err
	}
	return schedule, nil
}

func (s ScheduleService) History(ctx context.Context, ownerID string, scheduleID uint) ([]models.AgentScheduleRun, error) {
	items := []models.AgentScheduleRun{}
	err := s.DB.WithContext(ctx).
		Joins("JOIN agent_schedules ON agent_schedules.id = agent_schedule_runs.schedule_id").
		Where("agent_schedules.id = ? AND agent_schedules.owner_id = ?", scheduleID, ownerID).
		Order("agent_schedule_runs.planned_run_at DESC").Limit(100).Find(&items).Error
	if err != nil {
		return nil, fmt.Errorf("list schedule history: %w", err)
	}
	return items, nil
}

// NextDailyRun resolves a wall-clock time in an IANA timezone. It skips missed
// occurrences and handles spring-forward gaps by selecting the first valid
// minute at or after the configured wall clock.
func NextDailyRun(now time.Time, timezone, timeOfDay string) (time.Time, error) {
	location, err := time.LoadLocation(timezone)
	if err != nil {
		return time.Time{}, err
	}
	parsed, err := time.Parse("15:04", timeOfDay)
	if err != nil || parsed.Format("15:04") != timeOfDay {
		return time.Time{}, errors.New("time of day must use HH:MM")
	}
	hour, minute, _ := parsed.Clock()
	localNow := now.In(location)
	for offset := 0; offset < 370; offset++ {
		day := localNow.AddDate(0, 0, offset)
		candidate := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, location)
		for candidate.In(location).Format("2006-01-02") == day.Format("2006-01-02") &&
			candidate.In(location).Format("15:04") < timeOfDay {
			candidate = candidate.Add(time.Minute)
		}
		if candidate.In(location).Format("2006-01-02") != day.Format("2006-01-02") {
			continue
		}
		if candidate.After(now) && candidate.In(location).Format("15:04") >= timeOfDay {
			return candidate, nil
		}
	}
	return time.Time{}, errors.New("could not resolve next daily run")
}

// DispatchDue serializes each due schedule under a row lock. Its transaction
// advances next_run_at and writes one uniquely keyed run and task together.
func (s ScheduleService) DispatchDue(ctx context.Context, now time.Time) (int, error) {
	if s.DB == nil {
		return 0, errors.New("database unavailable")
	}
	if now.IsZero() {
		now = s.now()
	}
	var due []models.AgentSchedule
	if err := s.DB.WithContext(ctx).Select("id").Where("enabled = ? AND status = ? AND next_run_at <= ?", true, "active", now.UTC()).
		Order("next_run_at").Limit(100).Find(&due).Error; err != nil {
		return 0, fmt.Errorf("read due schedules: %w", err)
	}
	dispatched := 0
	for _, candidate := range due {
		created := false
		err := s.DB.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			var schedule models.AgentSchedule
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ?", candidate.ID).First(&schedule).Error; err != nil {
				return err
			}
			if !schedule.Enabled || schedule.Status != "active" || schedule.NextRunAt.After(now.UTC()) {
				return nil
			}
			timeOfDay := strings.TrimPrefix(schedule.ScheduleExpression, "daily ")
			next, err := NextDailyRun(now, schedule.Timezone, timeOfDay)
			if err != nil {
				return fmt.Errorf("resolve next schedule occurrence: %w", err)
			}
			planned := schedule.NextRunAt.UTC()
			run := models.AgentScheduleRun{ScheduleID: schedule.ID, PlannedRunAt: planned, Status: "QUEUED"}
			insert := tx.Clauses(clause.OnConflict{DoNothing: true}).Create(&run)
			if insert.Error != nil {
				return insert.Error
			}
			if insert.RowsAffected == 1 {
				key := fmt.Sprintf("schedule:%d:%s", schedule.ID, planned.Format(time.RFC3339))
				goal := "按当前可售库存和近30天有效订单销量检查库存风险，给出有数据依据的结果。只分析，不自动采购。"
				input, _ := json.Marshal(plannerTaskInput{Goal: goal})
				task := models.AgentWorkTask{
					TenantID: SingleTenant, Kind: "schedule_inventory_check", Status: "CREATED", Step: "queued",
					CreatedBy: "system_scheduler", IdempotencyKey: &key, InputJSON: string(input),
				}
				if err := tx.Create(&task).Error; err != nil {
					return err
				}
				run.LinkedTaskID = &task.ID
				if err := tx.Model(&run).Update("linked_task_id", task.ID).Error; err != nil {
					return err
				}
				created = true
			}
			return tx.Model(&schedule).Updates(map[string]any{"last_run_at": planned, "next_run_at": next.UTC()}).Error
		})
		if err != nil {
			return dispatched, err
		}
		if created {
			dispatched++
		}
	}
	return dispatched, nil
}

// StartPersistentScheduler only triggers configured schedules. It does not
// activate the legacy daily monitor ticker.
func StartPersistentScheduler(db *gorm.DB) {
	go func() {
		service := ScheduleService{DB: db}
		run := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
			defer cancel()
			if _, err := service.DispatchDue(ctx, time.Now()); err != nil {
				fmt.Printf("agent schedule dispatch failed: %v\n", err)
			}
		}
		run()
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			run()
		}
	}()
}
