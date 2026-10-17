package mysql

import (
	"context"
	"fmt"
	"time"
)

// ScalingEventRepository handles scaling event persistence.
type ScalingEventRepository struct {
	ds *Datastore
}

// NewScalingEventRepository creates a new scaling event repository
func NewScalingEventRepository(ds *Datastore) *ScalingEventRepository {
	return &ScalingEventRepository{ds: ds}
}

// Create creates a new scaling event
func (r *ScalingEventRepository) Create(ctx context.Context, event *ScalingEvent) error {
	return r.ds.DB(ctx).Create(event).Error
}

// ListByEndpoint retrieves scaling events for a specific endpoint
func (r *ScalingEventRepository) ListByEndpoint(ctx context.Context, endpoint string, limit int) ([]*ScalingEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	query := r.ds.DB(ctx).Model(&ScalingEvent{}).Order("timestamp DESC").Limit(limit)

	if endpoint != "" {
		query = query.Where("endpoint = ?", endpoint)
	}

	var events []*ScalingEvent
	err := query.Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list scaling events by endpoint: %w", err)
	}
	return events, nil
}

// GetLatestByEndpoint retrieves the most recent scaling event for an endpoint
func (r *ScalingEventRepository) GetLatestByEndpoint(ctx context.Context, endpoint string) (*ScalingEvent, error) {
	var event ScalingEvent
	err := r.ds.DB(ctx).Where("endpoint = ?", endpoint).Order("timestamp DESC").First(&event).Error
	if err != nil {
		return nil, err
	}
	return &event, nil
}

// ListRecent retrieves the most recent scaling events
func (r *ScalingEventRepository) ListRecent(ctx context.Context, limit int) ([]*ScalingEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	var events []*ScalingEvent
	err := r.ds.DB(ctx).
		Order("timestamp DESC").
		Limit(limit).
		Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list recent scaling events: %w", err)
	}
	return events, nil
}

// ListByTimeRange retrieves scaling events within a time range
func (r *ScalingEventRepository) ListByTimeRange(ctx context.Context, startTime, endTime time.Time, limit int) ([]*ScalingEvent, error) {
	if limit <= 0 {
		limit = 1000
	}

	var events []*ScalingEvent
	err := r.ds.DB(ctx).
		Where("timestamp >= ? AND timestamp <= ?", startTime, endTime).
		Order("timestamp DESC").
		Limit(limit).
		Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list scaling events by time range: %w", err)
	}
	return events, nil
}

// ListByEndpointAndTimeRange retrieves scaling events for an endpoint within a time range
func (r *ScalingEventRepository) ListByEndpointAndTimeRange(ctx context.Context, endpoint string, startTime, endTime time.Time, limit int) ([]*ScalingEvent, error) {
	if limit <= 0 {
		limit = 1000
	}

	var events []*ScalingEvent
	err := r.ds.DB(ctx).
		Where("endpoint = ? AND timestamp >= ? AND timestamp <= ?", endpoint, startTime, endTime).
		Order("timestamp DESC").
		Limit(limit).
		Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list scaling events: %w", err)
	}
	return events, nil
}

// Delete deletes a scaling event
func (r *ScalingEventRepository) Delete(ctx context.Context, eventID string) error {
	return r.ds.DB(ctx).Where("event_id = ?", eventID).Delete(&ScalingEvent{}).Error
}

// DeleteOldEvents deletes events older than the specified time
// This is useful for cleanup and archival
func (r *ScalingEventRepository) DeleteOldEvents(ctx context.Context, olderThan time.Time) (int64, error) {
	result := r.ds.DB(ctx).Where("timestamp < ?", olderThan).Delete(&ScalingEvent{})
	if result.Error != nil {
		return 0, fmt.Errorf("failed to delete old events: %w", result.Error)
	}
	return result.RowsAffected, nil
}

// Count counts scaling events with optional filters
func (r *ScalingEventRepository) Count(ctx context.Context, filters map[string]interface{}) (int64, error) {
	query := r.ds.DB(ctx).Model(&ScalingEvent{})

	// Apply filters
	for key, value := range filters {
		query = query.Where(key+" = ?", value)
	}

	var count int64
	err := query.Count(&count).Error
	if err != nil {
		return 0, fmt.Errorf("failed to count scaling events: %w", err)
	}
	return count, nil
}

// List retrieves scaling events with optional filters
func (r *ScalingEventRepository) List(ctx context.Context, filters map[string]interface{}, limit, offset int) ([]*ScalingEvent, error) {
	if limit <= 0 {
		limit = 100
	}

	query := r.ds.DB(ctx).Model(&ScalingEvent{})

	// Apply filters
	for key, value := range filters {
		query = query.Where(key+" = ?", value)
	}

	var events []*ScalingEvent
	err := query.
		Order("timestamp DESC").
		Limit(limit).
		Offset(offset).
		Find(&events).Error
	if err != nil {
		return nil, fmt.Errorf("failed to list scaling events: %w", err)
	}
	return events, nil
}
