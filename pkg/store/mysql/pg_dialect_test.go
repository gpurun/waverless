package mysql

import (
	"context"
	"os"
	"testing"
	"time"
)

// TestPostgresDialectQueries exercises the PostgreSQL-specific SQL paths:
//   - worker heartbeat upsert (ON CONFLICT ... DO UPDATE)
//   - task_statistics upserts (ON CONFLICT (scope_type, scope_value))
//   - monitoring aggregation using EXTRACT(EPOCH ...) instead of TIMESTAMPDIFF
//
// It is skipped unless PG_TEST_DSN is set, e.g.:
//
//	PG_TEST_DSN="host=127.0.0.1 port=15432 user=wavespeed password=... dbname=waverless sslmode=disable" go test ./pkg/store/mysql -run TestPostgresDialectQueries
func TestPostgresDialectQueries(t *testing.T) {
	dsn := os.Getenv("PG_TEST_DSN")
	if dsn == "" {
		t.Skip("PG_TEST_DSN not set; skipping PostgreSQL dialect test")
	}

	repo, err := NewPostgresRepository(dsn, nil)
	if err != nil {
		t.Fatalf("failed to connect to postgres: %v", err)
	}
	defer repo.Close()

	ctx := context.Background()
	const workerID = "pg-smoke-worker"
	const endpoint = "pg-smoke-endpoint"

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
	now := time.Now()
	if _, err := repo.Monitoring.AggregateMinuteStats(ctx, endpoint, now.Add(-time.Minute), now); err != nil {
		t.Fatalf("AggregateMinuteStats failed: %v", err)
	}

	// Cleanup best-effort.
	_ = repo.Worker.Delete(ctx, workerID)
}
