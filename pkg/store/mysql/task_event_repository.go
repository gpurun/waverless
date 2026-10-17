package mysql

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// TaskEventRepository handles task event persistence.
type TaskEventRepository struct {
	ds *Datastore
}

// NewTaskEventRepository creates a new task event repository
func NewTaskEventRepository(ds *Datastore) *TaskEventRepository {
	return &TaskEventRepository{ds: ds}
}

// RecordEvent creates a new task event
func (r *TaskEventRepository) RecordEvent(ctx context.Context, event *TaskEvent) error {
	if event.EventID == "" {
		event.EventID = generateEventID()
	}
	if event.EventTime.IsZero() {
		event.EventTime = time.Now()
	}
	return r.ds.DB(ctx).Create(event).Error
}

// GetTaskEvents retrieves all events for a task (ordered by time)
func (r *TaskEventRepository) GetTaskEvents(ctx context.Context, taskID string) ([]*TaskEvent, error) {
	var events []*TaskEvent
	err := r.ds.DB(ctx).
		Where("task_id = ?", taskID).
		Order("event_time ASC").
		Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get task events: %w", err)
	}
	return events, nil
}

// GetTaskTimeline retrieves task timeline (simplified event list)
func (r *TaskEventRepository) GetTaskTimeline(ctx context.Context, taskID string) ([]*TaskEvent, error) {
	var events []*TaskEvent
	err := r.ds.DB(ctx).
		Where("task_id = ?", taskID).
		Select("event_type, event_time, worker_id, error_message, from_status, to_status").
		Order("event_time ASC").
		Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("failed to get task timeline: %w", err)
	}
	return events, nil
}

// DeleteOldEvents deletes events older than the specified duration
func (r *TaskEventRepository) DeleteOldEvents(ctx context.Context, olderThan time.Duration) (int64, error) {
	cutoffTime := time.Now().Add(-olderThan)

	result := r.ds.DB(ctx).
		Where("event_time < ?", cutoffTime).
		Delete(&TaskEvent{})

	if result.Error != nil {
		return 0, fmt.Errorf("failed to delete old events: %w", result.Error)
	}

	return result.RowsAffected, nil
}

// generateEventID generates a unique event ID
func generateEventID() string {
	return uuid.New().String()
}

// CleanupOldEvents removes task events older than the given time in batches
func (r *TaskEventRepository) CleanupOldEvents(ctx context.Context, before time.Time) (int64, error) {
	const batchSize = 5000
	var total int64
	for {
		result := r.ds.DB(ctx).Where("event_time < ?", before).Limit(batchSize).Delete(&TaskEvent{})
		if result.Error != nil {
			return total, result.Error
		}
		total += result.RowsAffected
		if result.RowsAffected < batchSize {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	return total, nil
}
