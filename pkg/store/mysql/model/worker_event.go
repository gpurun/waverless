package model

import "time"

// WorkerEventType constants
type WorkerEventType string

const (
	EventWorkerStarted       WorkerEventType = "WORKER_STARTED"        // Pod started
	EventWorkerRegistered    WorkerEventType = "WORKER_REGISTERED"     // Worker registered (ready to pull tasks)
	EventWorkerTaskPulled    WorkerEventType = "WORKER_TASK_PULLED"    // Worker pulled a task
	EventWorkerTaskCompleted WorkerEventType = "WORKER_TASK_COMPLETED" // Worker completed a task
	EventWorkerOffline       WorkerEventType = "WORKER_OFFLINE"        // Worker went offline
)

// WorkerEvent represents a worker lifecycle event
type WorkerEvent struct {
	ID                  int64     `gorm:"primaryKey;autoIncrement"`
	EventID             string    `gorm:"column:event_id;type:varchar(255);not null;uniqueIndex"`
	WorkerID            string    `gorm:"column:worker_id;type:varchar(255);not null;index:idx_worker_event_time,priority:1"`
	Endpoint            string    `gorm:"column:endpoint;type:varchar(255);not null;index:idx_endpoint_event_time,priority:1"`
	EventType           string    `gorm:"column:event_type;type:varchar(50);not null;index:idx_event_type_time,priority:1"`
	EventTime           time.Time `gorm:"column:event_time;type:timestamptz(3);not null;index:idx_worker_event_time,priority:2;index:idx_endpoint_event_time,priority:2;index:idx_event_type_time,priority:2;index:idx_event_time"`
	ColdStartDurationMs *int64    `gorm:"column:cold_start_duration_ms"`
	IdleDurationMs      *int64    `gorm:"column:idle_duration_ms"`
	TaskID              string    `gorm:"column:task_id;type:varchar(255)"`
	Metadata            JSONMap   `gorm:"column:metadata;type:jsonb"`
}

func (WorkerEvent) TableName() string { return "worker_events" }
