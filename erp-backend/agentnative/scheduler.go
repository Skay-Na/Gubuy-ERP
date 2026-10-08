package agentnative

import (
	"context"
	"log"
	"time"

	"gorm.io/gorm"
)

// StartRuntimeWorker resumes durable tasks after startup and polls for newly queued tasks.
// The daily scan uses the legacy interval until persistent schedule definitions are enabled.
func StartRuntimeWorker(db *gorm.DB) {
	go func() {
		service := MonitorService{DB: db}
		process := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if _, err := service.ProcessPending(ctx); err != nil {
				log.Printf("agent runtime task processing failed: %v", err)
			}
		}
		scan := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if _, err := service.Scan(ctx); err != nil {
				log.Printf("agent monitor scan failed: %v", err)
				return
			}
			process()
		}
		scan()
		workerTicker := time.NewTicker(5 * time.Second)
		defer workerTicker.Stop()
		scanTicker := time.NewTicker(24 * time.Hour)
		defer scanTicker.Stop()
		for {
			select {
			case <-workerTicker.C:
				process()
			case <-scanTicker.C:
				scan()
			}
		}
	}()
}

// StartDailyMonitor runs only when explicitly enabled by the deployment.
// Signals and work are persisted before processing, so a restart can resume
// CREATED tasks or expired RUNNING leases.
func StartDailyMonitor(db *gorm.DB) {
	go func() {
		service := MonitorService{DB: db}
		run := func() {
			ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
			defer cancel()
			if _, err := service.Scan(ctx); err != nil {
				log.Printf("agent monitor scan failed: %v", err)
				return
			}
			if _, err := service.ProcessPending(ctx); err != nil {
				log.Printf("agent monitor task processing failed: %v", err)
			}
		}
		run()
		ticker := time.NewTicker(24 * time.Hour)
		defer ticker.Stop()
		for range ticker.C {
			run()
		}
	}()
}
