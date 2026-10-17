package model

import "time"

// EndpointMinuteStat represents minute-level statistics for an endpoint
type EndpointMinuteStat struct {
	ID                   int64     `gorm:"primaryKey;autoIncrement"`
	Endpoint             string    `gorm:"size:255;not null;uniqueIndex:uk_endpoint_minute,priority:1"`
	StatMinute           time.Time `gorm:"type:timestamptz(3);not null;uniqueIndex:uk_endpoint_minute,priority:2;index:idx_stat_minute"`
	ActiveWorkers        int       `gorm:"default:0"`
	IdleWorkers          int       `gorm:"default:0"`
	AvgWorkerUtilization float64   `gorm:"type:decimal(5,2);default:0"`
	TasksSubmitted       int       `gorm:"default:0"`
	TasksCompleted       int       `gorm:"default:0"`
	TasksFailed          int       `gorm:"default:0"`
	TasksTimeout         int       `gorm:"default:0"`
	TasksRetried         int       `gorm:"default:0"`
	AvgQueueWaitMs       float64   `gorm:"type:decimal(10,2);default:0"`
	AvgExecutionMs       float64   `gorm:"type:decimal(10,2);default:0"`
	P50ExecutionMs       float64   `gorm:"type:decimal(10,2);default:0"`
	P95ExecutionMs       float64   `gorm:"type:decimal(10,2);default:0"`
	AvgIdleDurationSec   float64   `gorm:"type:decimal(10,2);default:0"`
	MaxIdleDurationSec   int       `gorm:"default:0"`
	TotalIdleTimeSec     int       `gorm:"default:0"`
	IdleCount            int       `gorm:"default:0"`
	WorkersCreated       int       `gorm:"default:0"`
	WorkersTerminated    int       `gorm:"default:0"`
	ColdStarts           int       `gorm:"default:0"`
	AvgColdStartMs       float64   `gorm:"type:decimal(10,2);default:0"`
	CreatedAt            time.Time `gorm:"type:timestamptz(3);autoCreateTime"`
}

func (EndpointMinuteStat) TableName() string { return "endpoint_minute_stats" }

// EndpointHourlyStat represents hourly statistics for an endpoint
type EndpointHourlyStat struct {
	ID                   int64     `gorm:"primaryKey;autoIncrement"`
	Endpoint             string    `gorm:"size:255;not null;uniqueIndex:uk_endpoint_hour,priority:1"`
	StatHour             time.Time `gorm:"type:timestamptz(3);not null;uniqueIndex:uk_endpoint_hour,priority:2;index:idx_stat_hour"`
	ActiveWorkers        int       `gorm:"default:0"`
	IdleWorkers          int       `gorm:"default:0"`
	AvgWorkerUtilization float64   `gorm:"type:decimal(5,2);default:0"`
	TasksSubmitted       int       `gorm:"default:0"`
	TasksCompleted       int       `gorm:"default:0"`
	TasksFailed          int       `gorm:"default:0"`
	TasksTimeout         int       `gorm:"default:0"`
	TasksRetried         int       `gorm:"default:0"`
	AvgQueueWaitMs       float64   `gorm:"type:decimal(10,2);default:0"`
	AvgExecutionMs       float64   `gorm:"type:decimal(10,2);default:0"`
	P50ExecutionMs       float64   `gorm:"type:decimal(10,2);default:0"`
	P95ExecutionMs       float64   `gorm:"type:decimal(10,2);default:0"`
	AvgIdleDurationSec   float64   `gorm:"type:decimal(10,2);default:0"`
	MaxIdleDurationSec   int       `gorm:"default:0"`
	TotalIdleTimeSec     int64     `gorm:"default:0"`
	IdleCount            int       `gorm:"default:0"`
	WorkersCreated       int       `gorm:"default:0"`
	WorkersTerminated    int       `gorm:"default:0"`
	ColdStarts           int       `gorm:"default:0"`
	AvgColdStartMs       float64   `gorm:"type:decimal(10,2);default:0"`
	CreatedAt            time.Time `gorm:"type:timestamptz(3);autoCreateTime"`
}

func (EndpointHourlyStat) TableName() string { return "endpoint_hourly_stats" }

// EndpointDailyStat represents daily statistics for an endpoint
type EndpointDailyStat struct {
	ID                   int64     `gorm:"primaryKey;autoIncrement"`
	Endpoint             string    `gorm:"size:255;not null;uniqueIndex:uk_endpoint_date,priority:1"`
	StatDate             time.Time `gorm:"type:date;not null;uniqueIndex:uk_endpoint_date,priority:2;index:idx_stat_date"`
	ActiveWorkers        int       `gorm:"default:0"`
	IdleWorkers          int       `gorm:"default:0"`
	AvgWorkerUtilization float64   `gorm:"type:decimal(5,2);default:0"`
	TasksSubmitted       int       `gorm:"default:0"`
	TasksCompleted       int       `gorm:"default:0"`
	TasksFailed          int       `gorm:"default:0"`
	TasksTimeout         int       `gorm:"default:0"`
	TasksRetried         int       `gorm:"default:0"`
	AvgQueueWaitMs       float64   `gorm:"type:decimal(10,2);default:0"`
	AvgExecutionMs       float64   `gorm:"type:decimal(10,2);default:0"`
	P50ExecutionMs       float64   `gorm:"type:decimal(10,2);default:0"`
	P95ExecutionMs       float64   `gorm:"type:decimal(10,2);default:0"`
	AvgIdleDurationSec   float64   `gorm:"type:decimal(10,2);default:0"`
	MaxIdleDurationSec   int       `gorm:"default:0"`
	TotalIdleTimeSec     int64     `gorm:"default:0"`
	IdleCount            int       `gorm:"default:0"`
	WorkersCreated       int       `gorm:"default:0"`
	WorkersTerminated    int       `gorm:"default:0"`
	ColdStarts           int       `gorm:"default:0"`
	AvgColdStartMs       float64   `gorm:"type:decimal(10,2);default:0"`
	CreatedAt            time.Time `gorm:"type:timestamptz(3);autoCreateTime"`
}

func (EndpointDailyStat) TableName() string { return "endpoint_daily_stats" }
