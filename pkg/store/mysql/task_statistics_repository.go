package mysql

import (
	"context"
	"fmt"
	"time"

	"waverless/pkg/store/mysql/model"
)

// TaskStatisticsRepository handles task statistics persistence.
type TaskStatisticsRepository struct {
	ds *Datastore
}

// NewTaskStatisticsRepository creates a new task statistics repository
func NewTaskStatisticsRepository(ds *Datastore) *TaskStatisticsRepository {
	return &TaskStatisticsRepository{ds: ds}
}

// GetGlobalStatistics retrieves global task statistics
func (r *TaskStatisticsRepository) GetGlobalStatistics(ctx context.Context) (*model.TaskStatistics, error) {
	var stats model.TaskStatistics
	err := r.ds.DB(ctx).
		Where("scope_type = ? AND scope_value = ?", "global", "global").
		First(&stats).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get global statistics: %w", err)
	}
	return &stats, nil
}

// GetEndpointStatistics retrieves statistics for a specific endpoint
func (r *TaskStatisticsRepository) GetEndpointStatistics(ctx context.Context, endpoint string) (*model.TaskStatistics, error) {
	var stats model.TaskStatistics
	err := r.ds.DB(ctx).
		Where("scope_type = ? AND scope_value = ?", "endpoint", endpoint).
		First(&stats).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get endpoint statistics: %w", err)
	}
	return &stats, nil
}

// ListTopEndpoints retrieves top N endpoints by task volume
func (r *TaskStatisticsRepository) ListTopEndpoints(ctx context.Context, limit int) ([]*model.TaskStatistics, error) {
	if limit <= 0 {
		limit = 10
	}

	var stats []*model.TaskStatistics
	err := r.ds.DB(ctx).
		Where("scope_type = ?", "endpoint").
		Order("total_count DESC").
		Limit(limit).
		Find(&stats).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list top endpoints: %w", err)
	}
	return stats, nil
}

// upsertTaskStatisticsSQL returns the dialect-specific upsert statement for the
// task_statistics table. The unique key is (scope_type, scope_value).
func (r *TaskStatisticsRepository) upsertTaskStatisticsSQL() string {
	if r.ds.IsPostgres() {
		return `
		INSERT INTO task_statistics
			(scope_type, scope_value, pending_count, in_progress_count, completed_count, failed_count, cancelled_count, total_count, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT (scope_type, scope_value) DO UPDATE SET
			pending_count = EXCLUDED.pending_count,
			in_progress_count = EXCLUDED.in_progress_count,
			completed_count = EXCLUDED.completed_count,
			failed_count = EXCLUDED.failed_count,
			cancelled_count = EXCLUDED.cancelled_count,
			total_count = EXCLUDED.total_count,
			updated_at = EXCLUDED.updated_at
	`
	}
	return `
		INSERT INTO task_statistics
			(scope_type, scope_value, pending_count, in_progress_count, completed_count, failed_count, cancelled_count, total_count, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON DUPLICATE KEY UPDATE
			pending_count = VALUES(pending_count),
			in_progress_count = VALUES(in_progress_count),
			completed_count = VALUES(completed_count),
			failed_count = VALUES(failed_count),
			cancelled_count = VALUES(cancelled_count),
			total_count = VALUES(total_count),
			updated_at = VALUES(updated_at)
	`
}

// RefreshGlobalStatistics recalculates and updates global statistics from tasks table
// OPTIMIZATION: Only counts active tasks (PENDING, IN_PROGRESS) from full table scan.
// For historical tasks (COMPLETED, FAILED, CANCELLED), relies on incremental updates to avoid scanning large datasets.
func (r *TaskStatisticsRepository) RefreshGlobalStatistics(ctx context.Context) error {
	now := time.Now()

	// Calculate statistics from tasks table
	type TaskStats struct {
		PendingCount    int
		InProgressCount int
		CompletedCount  int
		FailedCount     int
		CancelledCount  int
		TotalCount      int
	}

	var stats TaskStats
	// OPTIMIZATION: Use UNION ALL with idx_status index to avoid full table scan
	// Each subquery uses the idx_status index efficiently
	err := r.ds.DB(ctx).Raw(`
		SELECT
			SUM(pending_count) as pending_count,
			SUM(in_progress_count) as in_progress_count,
			SUM(completed_count) as completed_count,
			SUM(failed_count) as failed_count,
			SUM(cancelled_count) as cancelled_count,
			SUM(total_count) as total_count
		FROM (
			SELECT COUNT(*) as pending_count, 0 as in_progress_count, 0 as completed_count, 0 as failed_count, 0 as cancelled_count, COUNT(*) as total_count
			FROM tasks WHERE status = 'PENDING'
			UNION ALL
			SELECT 0, COUNT(*), 0, 0, 0, COUNT(*)
			FROM tasks WHERE status = 'IN_PROGRESS'
			UNION ALL
			SELECT 0, 0, COUNT(*), 0, 0, COUNT(*)
			FROM tasks WHERE status = 'COMPLETED'
			UNION ALL
			SELECT 0, 0, 0, COUNT(*), 0, COUNT(*)
			FROM tasks WHERE status = 'FAILED'
			UNION ALL
			SELECT 0, 0, 0, 0, COUNT(*), COUNT(*)
			FROM tasks WHERE status = 'CANCELLED'
		) AS status_counts
	`).Scan(&stats).Error
	if err != nil {
		return fmt.Errorf("failed to calculate global statistics: %w", err)
	}

	// Upsert statistics
	globalValue := "global"
	globalStats := model.TaskStatistics{
		ScopeType:       "global",
		ScopeValue:      &globalValue,
		PendingCount:    stats.PendingCount,
		InProgressCount: stats.InProgressCount,
		CompletedCount:  stats.CompletedCount,
		FailedCount:     stats.FailedCount,
		CancelledCount:  stats.CancelledCount,
		TotalCount:      stats.TotalCount,
		UpdatedAt:       now,
	}

	// Use dialect-specific upsert
	err = r.ds.DB(ctx).Exec(r.upsertTaskStatisticsSQL(),
		globalStats.ScopeType, globalStats.ScopeValue, globalStats.PendingCount, globalStats.InProgressCount,
		globalStats.CompletedCount, globalStats.FailedCount, globalStats.CancelledCount,
		globalStats.TotalCount, globalStats.UpdatedAt).Error

	if err != nil {
		return fmt.Errorf("failed to upsert global statistics: %w", err)
	}

	return nil
}

// RefreshEndpointStatistics recalculates and updates per-endpoint statistics
// OPTIMIZATION: Use idx_endpoint_status index efficiently with separate queries per status
func (r *TaskStatisticsRepository) RefreshEndpointStatistics(ctx context.Context, endpoint string) error {
	now := time.Now()

	// Calculate statistics for specific endpoint
	type TaskStats struct {
		Endpoint        string
		PendingCount    int
		InProgressCount int
		CompletedCount  int
		FailedCount     int
		CancelledCount  int
		TotalCount      int
	}

	var stats TaskStats
	// OPTIMIZATION: Use UNION ALL with idx_endpoint_status composite index
	// Each subquery can use the index (endpoint, status) efficiently
	endpointExpr := "?"
	if r.ds.IsPostgres() {
		// PostgreSQL cannot infer the type of a bare parameter in a SELECT list.
		endpointExpr = "?::text"
	}
	statsQuery := fmt.Sprintf(`
		SELECT
			%s as endpoint,
			SUM(pending_count) as pending_count,
			SUM(in_progress_count) as in_progress_count,
			SUM(completed_count) as completed_count,
			SUM(failed_count) as failed_count,
			SUM(cancelled_count) as cancelled_count,
			SUM(total_count) as total_count
		FROM (
			SELECT COUNT(*) as pending_count, 0 as in_progress_count, 0 as completed_count, 0 as failed_count, 0 as cancelled_count, COUNT(*) as total_count
			FROM tasks WHERE endpoint = ? AND status = 'PENDING'
			UNION ALL
			SELECT 0, COUNT(*), 0, 0, 0, COUNT(*)
			FROM tasks WHERE endpoint = ? AND status = 'IN_PROGRESS'
			UNION ALL
			SELECT 0, 0, COUNT(*), 0, 0, COUNT(*)
			FROM tasks WHERE endpoint = ? AND status = 'COMPLETED'
			UNION ALL
			SELECT 0, 0, 0, COUNT(*), 0, COUNT(*)
			FROM tasks WHERE endpoint = ? AND status = 'FAILED'
			UNION ALL
			SELECT 0, 0, 0, 0, COUNT(*), COUNT(*)
			FROM tasks WHERE endpoint = ? AND status = 'CANCELLED'
		) AS status_counts
	`, endpointExpr)
	err := r.ds.DB(ctx).Raw(statsQuery, endpoint, endpoint, endpoint, endpoint, endpoint, endpoint).Scan(&stats).Error
	if err != nil {
		return fmt.Errorf("failed to calculate endpoint statistics: %w", err)
	}

	// Upsert statistics
	endpointStats := model.TaskStatistics{
		ScopeType:       "endpoint",
		ScopeValue:      &endpoint,
		PendingCount:    stats.PendingCount,
		InProgressCount: stats.InProgressCount,
		CompletedCount:  stats.CompletedCount,
		FailedCount:     stats.FailedCount,
		CancelledCount:  stats.CancelledCount,
		TotalCount:      stats.TotalCount,
		UpdatedAt:       now,
	}

	err = r.ds.DB(ctx).Exec(r.upsertTaskStatisticsSQL(),
		endpointStats.ScopeType, endpointStats.ScopeValue, endpointStats.PendingCount,
		endpointStats.InProgressCount, endpointStats.CompletedCount, endpointStats.FailedCount,
		endpointStats.CancelledCount, endpointStats.TotalCount, endpointStats.UpdatedAt).Error

	if err != nil {
		return fmt.Errorf("failed to upsert endpoint statistics: %w", err)
	}

	return nil
}

// RefreshAllEndpointStatistics recalculates and updates all endpoint statistics
// Uses pagination to avoid locking large tables
func (r *TaskStatisticsRepository) RefreshAllEndpointStatistics(ctx context.Context) error {
	// First, get all distinct endpoints with pagination
	var allEndpoints []string
	batchSize := 100
	offset := 0

	for {
		var endpoints []string
		err := r.ds.DB(ctx).
			Table("tasks").
			Select("DISTINCT endpoint").
			Where("endpoint IS NOT NULL AND endpoint != ''").
			Order("endpoint").
			Limit(batchSize).
			Offset(offset).
			Pluck("endpoint", &endpoints).Error

		if err != nil {
			return fmt.Errorf("failed to fetch endpoints (offset %d): %w", offset, err)
		}

		if len(endpoints) == 0 {
			break
		}

		allEndpoints = append(allEndpoints, endpoints...)

		if len(endpoints) < batchSize {
			break
		}

		offset += batchSize
	}

	// Refresh statistics for each endpoint
	for _, endpoint := range allEndpoints {
		if err := r.RefreshEndpointStatistics(ctx, endpoint); err != nil {
			// Log error but continue processing other endpoints
			return fmt.Errorf("failed to refresh statistics for endpoint %s: %w", endpoint, err)
		}
	}

	return nil
}

// IncrementStatistics atomically increments statistics for a specific status change
// This is called when a task transitions from one status to another
func (r *TaskStatisticsRepository) IncrementStatistics(ctx context.Context, endpoint string, fromStatus, toStatus string, count int) error {
	if count <= 0 {
		count = 1
	}
	now := time.Now()

	// Update global statistics
	if err := r.updateStatisticsForStatusChange(ctx, "global", nil, fromStatus, toStatus, count, now); err != nil {
		return fmt.Errorf("failed to update global statistics: %w", err)
	}

	// Update endpoint statistics (if endpoint is not empty)
	if endpoint != "" {
		if err := r.updateStatisticsForStatusChange(ctx, "endpoint", &endpoint, fromStatus, toStatus, count, now); err != nil {
			return fmt.Errorf("failed to update endpoint statistics: %w", err)
		}
	}

	return nil
}

// updateStatisticsForStatusChange updates statistics for a status transition
func (r *TaskStatisticsRepository) updateStatisticsForStatusChange(ctx context.Context, scopeType string, scopeValue *string, fromStatus, toStatus string, count int, now time.Time) error {
	// For global scope, always use "global" string instead of NULL
	actualScopeValue := scopeValue
	if scopeValue == nil {
		globalValue := "global"
		actualScopeValue = &globalValue
	}

	// Build SQL with GREATEST() to prevent negative counts
	// The update expressions reference the existing row, which must be qualified
	// with the table name on PostgreSQL to disambiguate from EXCLUDED.
	prefix := ""
	conflictClause := "ON DUPLICATE KEY UPDATE"
	if r.ds.IsPostgres() {
		prefix = "task_statistics."
		conflictClause = "ON CONFLICT (scope_type, scope_value) DO UPDATE SET"
	}
	sql := fmt.Sprintf(`
		INSERT INTO task_statistics (scope_type, scope_value, pending_count, in_progress_count, completed_count, failed_count, cancelled_count, total_count, updated_at)
		VALUES (?, ?,
			CASE WHEN ? = 'PENDING' THEN ? ELSE 0 END,
			CASE WHEN ? = 'IN_PROGRESS' THEN ? ELSE 0 END,
			CASE WHEN ? = 'COMPLETED' THEN ? ELSE 0 END,
			CASE WHEN ? = 'FAILED' THEN ? ELSE 0 END,
			CASE WHEN ? = 'CANCELLED' THEN ? ELSE 0 END,
			?, ?)
		%s
			pending_count = GREATEST(0, %spending_count - CASE WHEN ? = 'PENDING' THEN ? ELSE 0 END + CASE WHEN ? = 'PENDING' THEN ? ELSE 0 END),
			in_progress_count = GREATEST(0, %sin_progress_count - CASE WHEN ? = 'IN_PROGRESS' THEN ? ELSE 0 END + CASE WHEN ? = 'IN_PROGRESS' THEN ? ELSE 0 END),
			completed_count = GREATEST(0, %scompleted_count - CASE WHEN ? = 'COMPLETED' THEN ? ELSE 0 END + CASE WHEN ? = 'COMPLETED' THEN ? ELSE 0 END),
			failed_count = GREATEST(0, %sfailed_count - CASE WHEN ? = 'FAILED' THEN ? ELSE 0 END + CASE WHEN ? = 'FAILED' THEN ? ELSE 0 END),
			cancelled_count = GREATEST(0, %scancelled_count - CASE WHEN ? = 'CANCELLED' THEN ? ELSE 0 END + CASE WHEN ? = 'CANCELLED' THEN ? ELSE 0 END),
			total_count = GREATEST(0, %stotal_count + CASE WHEN ? = '' THEN ? ELSE 0 END),
			updated_at = ?
	`, conflictClause,
		prefix, prefix, prefix, prefix, prefix, prefix)
	args := []interface{}{
		scopeType, *actualScopeValue,
		toStatus, count, toStatus, count, toStatus, count, toStatus, count, toStatus, count, // INSERT VALUES
		count, now,
		fromStatus, count, toStatus, count, // pending_count
		fromStatus, count, toStatus, count, // in_progress_count
		fromStatus, count, toStatus, count, // completed_count
		fromStatus, count, toStatus, count, // failed_count
		fromStatus, count, toStatus, count, // cancelled_count
		fromStatus, count, // total_count (only increment if fromStatus is empty, meaning new task)
		now,
	}

	result := r.ds.DB(ctx).Exec(sql, args...)
	if result.Error != nil {
		return result.Error
	}

	return nil
}
