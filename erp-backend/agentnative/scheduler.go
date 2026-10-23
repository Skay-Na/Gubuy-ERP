package agentnative

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"
)

// StartRuntimeWorker resumes durable tasks after startup and polls for newly queued tasks.
// Inventory monitoring has been retired and is not started here.
func StartRuntimeWorker(db *gorm.DB, planners ...AgentPlanner) {
	startTaskWorker(db, planners...)
}

// StartAgentTaskWorker processes queued durable work without the legacy daily scan.
func StartAgentTaskWorker(db *gorm.DB, planners ...AgentPlanner) {
	startTaskWorker(db, planners...)
}

func startTaskWorker(db *gorm.DB, planners ...AgentPlanner) {
	go func() {
		service := MonitorService{DB: db}
		if len(planners) > 0 {
			service.Planner = planners[0]
		}
		process := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if _, err := service.ProcessPending(ctx); err != nil {
				log.Printf("agent runtime task processing failed: %v", err)
			}
		}
		process()
		workerTicker := time.NewTicker(5 * time.Second)
		defer workerTicker.Stop()
		for {
			select {
			case <-workerTicker.C:
				process()
			}
		}
	}()
}
