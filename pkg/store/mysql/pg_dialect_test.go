package mysql

import (
	"context"
	"fmt"
	"os"
	"testing"
	"time"

	"waverless/pkg/store/mysql/model"
)

// TestPostgresDialectQueries exercises the PostgreSQL-specific SQL paths:
//   - worker heartbeat upsert (ON CONFLICT ... DO UPDATE)
//   - task_statistics upserts (ON CONFLICT (scope_type, scope_value))
//   - monitoring aggregation using EXTRACT(EPOCH ...) instead of TIMESTAMPDIFF
//   - every repository's schema-backed CRUD paths and JSON/time round trips
//
// It is skipped unless PG_TEST_DSN is set, e.g.:
//
//	PG_TEST_DSN="host=127.0.0.1 port=15433 user=wavespeed password=wavespeed dbname=waverless sslmode=disable TimeZone=UTC" go test ./pkg/store/mysql -run TestPostgresDialectQueries
func TestPostgresDialectQueries(t *testing.T) {
	dsn := os.Getenv("PG_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set; skipping PostgreSQL dialect test")
	}

	repo, err := NewPostgresRepository(dsn, nil)
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}
	t.Cleanup(func() { _ = repo.Close() })

	ctx := context.Background()
	suffix := fmt.Sprintf("%d", time.Now().UnixNano())
	workerID := "pg-smoke-worker-" + suffix
	endpoint := "pg-smoke-endpoint-" + suffix
	taskID := "pg-smoke-task-" + suffix
	eventID := "pg-smoke-event-" + suffix
	specName := "pg-smoke-spec-" + suffix
	now := time.Now().UTC().Truncate(time.Millisecond)

	t.Cleanup(func() {
		_ = repo.GetDatastore().DB(ctx).Where("worker_id = ?", workerID).Delete(&model.StatusEvent{}).Error
		_ = repo.GetDatastore().DB(ctx).Where("worker_id = ?", workerID).Delete(&model.WorkerEvent{}).Error
		_ = repo.GetDatastore().DB(ctx).Where("endpoint = ?", endpoint).Delete(&model.WorkerResourceSnapshot{}).Error
		_ = repo.GetDatastore().DB(ctx).Where("task_id = ?", taskID).Delete(&model.TaskEvent{}).Error
		_ = repo.Task.Delete(ctx, taskID)
		_ = repo.Worker.Delete(ctx, workerID)
		_ = repo.Endpoint.Delete(ctx, endpoint)
		_ = repo.AutoscalerConfig.Delete(ctx, endpoint)
		_ = repo.GetDatastore().DB(ctx).Where("spec_name = ?", specName).Delete(&model.SpecCapacity{}).Error
		_ = repo.Spec.Delete(ctx, specName)
		_ = repo.ScalingEvent.Delete(ctx, eventID)
		_, _ = repo.Monitoring.CleanupOldMinuteStats(ctx, now.Add(time.Second))
		_, _ = repo.Monitoring.CleanupOldHourlyStats(ctx, now.Add(time.Second))
		_, _ = repo.Monitoring.CleanupOldDailyStats(ctx, now.AddDate(0, 0, 1))
	})

	// Exercise the SQL schema, JSONB serializers, timestamps, and unique keys used
	// by regular service requests.
	spec := &model.Spec{Name: specName, DisplayName: "PG Smoke", Category: "gpu", Memory: "1Gi", EphemeralStorage: "10Gi", Platforms: model.JSONMap{"generic": map[string]interface{}{"enabled": true}}}
	if err := repo.Spec.Create(ctx, spec); err != nil {
		t.Fatalf("create spec failed: %v", err)
	}
	if _, err := repo.Spec.Get(ctx, specName); err != nil {
		t.Fatalf("get spec failed: %v", err)
	}
	if err := repo.SpecCapacity.Upsert(ctx, &model.SpecCapacity{SpecName: specName, Status: model.CapacityAvailable}); err != nil {
		t.Fatalf("upsert spec capacity failed: %v", err)
	}
	if _, err := repo.SpecCapacity.Get(ctx, specName); err != nil {
		t.Fatalf("get spec capacity failed: %v", err)
	}
	if err := repo.Endpoint.Create(ctx, &model.Endpoint{Endpoint: endpoint, SpecName: specName, Image: "example/image:latest", EnablePtrace: true, Env: model.JSONMap{"K": "V"}, RuntimeState: model.JSONMap{"phase": "Running"}}); err != nil {
		t.Fatalf("create endpoint failed: %v", err)
	}
	if _, err := repo.Endpoint.Get(ctx, endpoint); err != nil {
		t.Fatalf("get endpoint failed: %v", err)
	}
	if err := repo.AutoscalerConfig.Create(ctx, &model.AutoscalerConfig{Endpoint: endpoint}); err != nil {
		t.Fatalf("create autoscaler config failed: %v", err)
	}
	if _, err := repo.AutoscalerConfig.Get(ctx, endpoint); err != nil {
		t.Fatalf("get autoscaler config failed: %v", err)
	}

	if err := repo.Task.Create(ctx, &model.Task{TaskID: taskID, Endpoint: endpoint, Input: model.JSONMap{"prompt": "smoke"}, Status: "PENDING"}); err != nil {
		t.Fatalf("create task failed: %v", err)
	}
	if _, err := repo.Task.Get(ctx, taskID); err != nil {
		t.Fatalf("get task failed: %v", err)
	}
	if err := repo.TaskEvent.RecordEvent(ctx, &model.TaskEvent{EventID: eventID, TaskID: taskID, Endpoint: endpoint, EventType: "TASK_CREATED", EventTime: now, Metadata: model.JSONMap{"source": "pg-test"}}); err != nil {
		t.Fatalf("create task event failed: %v", err)
	}
	if _, err := repo.TaskEvent.GetTaskEvents(ctx, taskID); err != nil {
		t.Fatalf("get task events failed: %v", err)
	}
	if err := repo.ScalingEvent.Create(ctx, &model.ScalingEvent{EventID: eventID, Endpoint: endpoint, Timestamp: now, Action: "scale_up", FromReplicas: 0, ToReplicas: 1, Reason: "smoke", PreemptedFrom: model.JSONStringArray{"other"}}); err != nil {
		t.Fatalf("create scaling event failed: %v", err)
	}
	if _, err := repo.ScalingEvent.ListByEndpoint(ctx, endpoint, 10); err != nil {
		t.Fatalf("get scaling event failed: %v", err)
	}
	if err := repo.StatusEvent.Create(ctx, &model.StatusEvent{WorkerID: workerID, Endpoint: endpoint, EventType: "STATUS_CHANGE", NewStatus: "ONLINE", SpotStatus: model.JSONMap{"available": true}}); err != nil {
		t.Fatalf("create status event failed: %v", err)
	}
	if _, err := repo.StatusEvent.ListByWorker(ctx, workerID, 10, 0); err != nil {
		t.Fatalf("list status events failed: %v", err)
	}
	if err := repo.Monitoring.UpsertMinuteStat(ctx, &model.EndpointMinuteStat{Endpoint: endpoint, StatMinute: now}); err != nil {
		t.Fatalf("upsert minute stats failed: %v", err)
	}
	if err := repo.Monitoring.UpsertHourlyStat(ctx, &model.EndpointHourlyStat{Endpoint: endpoint, StatHour: now}); err != nil {
		t.Fatalf("upsert hourly stats failed: %v", err)
	}
	if err := repo.Monitoring.UpsertDailyStat(ctx, &model.EndpointDailyStat{Endpoint: endpoint, StatDate: now}); err != nil {
		t.Fatalf("upsert daily stats failed: %v", err)
	}
	if _, err := repo.Monitoring.GetMinuteStats(ctx, endpoint, now.Add(-time.Minute), now.Add(time.Minute)); err != nil {
		t.Fatalf("list minute stats failed: %v", err)
	}
	if _, err := repo.Monitoring.GetHourlyStats(ctx, endpoint, now.Add(-time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatalf("list hourly stats failed: %v", err)
	}
	if _, err := repo.Monitoring.GetDailyStats(ctx, endpoint, now.AddDate(0, 0, -1), now.AddDate(0, 0, 1)); err != nil {
		t.Fatalf("list daily stats failed: %v", err)
	}
	var tableCount int64
	if err := repo.GetDatastore().DB(ctx).Raw(`
		SELECT COUNT(*) FROM information_schema.tables
		WHERE table_schema = 'public'
	`).Scan(&tableCount).Error; err != nil {
		t.Fatalf("count PostgreSQL application tables failed: %v", err)
	}
	if tableCount != 15 {
		t.Fatalf("expected all 15 application tables, got %d", tableCount)
	}
	if _, err := repo.Monitoring.GetAllEndpoints(ctx); err != nil {
		t.Fatalf("list endpoints for monitoring failed: %v", err)
	}
	if _, err := repo.Monitoring.GetDistinctEndpoints(ctx, now.Add(-time.Hour), now.Add(time.Hour)); err != nil {
		t.Fatalf("list distinct endpoints failed: %v", err)
	}
	if err := repo.Monitoring.SaveWorkerEvent(ctx, &model.WorkerEvent{EventID: "pg-smoke-worker-event-" + suffix, WorkerID: workerID, Endpoint: endpoint, EventType: "WORKER_STARTED", EventTime: now, Metadata: model.JSONMap{"source": "pg-test"}}); err != nil {
		t.Fatalf("create worker event failed: %v", err)
	}
	if _, err := repo.Monitoring.GetRealtimeMetrics(ctx, endpoint); err != nil {
		t.Fatalf("realtime metrics query failed: %v", err)
	}
	if _, err := repo.TaskStatistics.ListTopEndpoints(ctx, 10); err != nil {
		t.Fatalf("list top endpoints failed: %v", err)
	}
	if err := repo.TaskStatistics.RefreshGlobalStatistics(ctx); err != nil {
		t.Fatalf("refresh global statistics before read failed: %v", err)
	}
	if err := repo.TaskStatistics.RefreshEndpointStatistics(ctx, endpoint); err != nil {
		t.Fatalf("refresh endpoint statistics before read failed: %v", err)
	}
	if _, err := repo.TaskStatistics.GetGlobalStatistics(ctx); err != nil {
		t.Fatalf("get global statistics failed: %v", err)
	}
	if _, err := repo.TaskStatistics.GetEndpointStatistics(ctx, endpoint); err != nil {
		t.Fatalf("get endpoint statistics failed: %v", err)
	}

	// 1. Worker upsert: first insert, then conflict update (exercises ON CONFLICT).
	if err := repo.Worker.UpdateHeartbeat(ctx, workerID, endpoint, []string{"task-1"}, 1, "v1"); err != nil {
		t.Fatalf("heartbeat insert failed: %v", err)
	}
	if err := repo.Worker.UpdateHeartbeat(ctx, workerID, endpoint, nil, 0, ""); err != nil {
		t.Fatalf("heartbeat conflict update failed: %v", err)
	}
	w, err := repo.Worker.Get(ctx, workerID)
	if err != nil {
		t.Fatalf("get worker failed: %v", err)
	}
	if w.Endpoint != endpoint {
		t.Fatalf("unexpected worker endpoint: %s", w.Endpoint)
	}

	// 2. Task statistics upserts.
	if err := repo.TaskStatistics.RefreshGlobalStatistics(ctx); err != nil {
		t.Fatalf("RefreshGlobalStatistics failed: %v", err)
	}
	if err := repo.TaskStatistics.RefreshEndpointStatistics(ctx, endpoint); err != nil {
		t.Fatalf("RefreshEndpointStatistics failed: %v", err)
	}
	if err := repo.TaskStatistics.IncrementStatistics(ctx, endpoint, "PENDING", "IN_PROGRESS", 1); err != nil {
		t.Fatalf("IncrementStatistics failed: %v", err)
	}
	if err := repo.TaskStatistics.RefreshGlobalStatistics(ctx); err != nil {
		t.Fatalf("RefreshGlobalStatistics (2nd) failed: %v", err)
	}

	// 3. Monitoring aggregation (TIMESTAMPDIFF -> EXTRACT(EPOCH ...)).
	if _, err := repo.Monitoring.AggregateMinuteStats(ctx, endpoint, now.Add(-time.Minute), now); err != nil {
		t.Fatalf("AggregateMinuteStats failed: %v", err)
	}
	if _, err := repo.Monitoring.AggregateHourlyStats(ctx, endpoint, now.Truncate(time.Hour)); err != nil {
		t.Fatalf("AggregateHourlyStats failed: %v", err)
	}
	if _, err := repo.Monitoring.AggregateDailyStats(ctx, endpoint, now.Truncate(24*time.Hour)); err != nil {
		t.Fatalf("AggregateDailyStats failed: %v", err)
	}
	if _, err := repo.Monitoring.GetRealtimeMetrics(ctx, endpoint); err != nil {
		t.Fatalf("GetRealtimeMetrics failed: %v", err)
	}

	// Cleanup best-effort.
	_ = repo.Worker.Delete(ctx, workerID)
}
