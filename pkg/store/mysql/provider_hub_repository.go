package mysql

import (
	"context"
	"fmt"
	"strings"
	"time"

	"waverless/pkg/store/mysql/model"

	"gorm.io/gorm/clause"
)

// ProviderSliceRepository persists endpoint_provider_slices.
type ProviderSliceRepository struct {
	ds *Datastore
}

func NewProviderSliceRepository(ds *Datastore) *ProviderSliceRepository {
	return &ProviderSliceRepository{ds: ds}
}

func (r *ProviderSliceRepository) GetActiveByEndpoint(ctx context.Context, endpoint string) (*model.EndpointProviderSlice, error) {
	rows, err := r.ListActiveByEndpoint(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, fmt.Errorf("no active slice for endpoint %q", endpoint)
	}
	return &rows[0], nil
}

func (r *ProviderSliceRepository) ListActiveByEndpoint(ctx context.Context, endpoint string) ([]model.EndpointProviderSlice, error) {
	var rows []model.EndpointProviderSlice
	err := r.ds.DB(ctx).
		Where("endpoint = ? AND status = ?", endpoint, model.SliceStatusActive).
		Order("id ASC").
		Find(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("list active slices: %w", err)
	}
	return rows, nil
}

func (r *ProviderSliceRepository) FindActiveSlice(ctx context.Context, endpoint, providerID, physicalRegion, skuID string) (*model.EndpointProviderSlice, error) {
	var row model.EndpointProviderSlice
	err := r.ds.DB(ctx).
		Where("endpoint = ? AND status = ? AND provider_id = ? AND physical_region = ? AND provider_sku_id = ?",
			endpoint, model.SliceStatusActive, providerID, physicalRegion, skuID).
		First(&row).Error
	if err != nil {
		return nil, err
	}
	return &row, nil
}

func (r *ProviderSliceRepository) Create(ctx context.Context, slice *model.EndpointProviderSlice) error {
	return r.ds.DB(ctx).Create(slice).Error
}

func (r *ProviderSliceRepository) UpdateFields(ctx context.Context, id int64, updates map[string]interface{}) error {
	updates["updated_at"] = time.Now()
	return r.ds.DB(ctx).Model(&model.EndpointProviderSlice{}).Where("id = ?", id).Updates(updates).Error
}

// ProviderOfferRepository reads/writes placement offer cache.
type ProviderOfferRepository struct {
	ds *Datastore
}

func NewProviderOfferRepository(ds *Datastore) *ProviderOfferRepository {
	return &ProviderOfferRepository{ds: ds}
}

func (r *ProviderOfferRepository) ListForPlacement(ctx context.Context, gpuType string, gpuCount int, billingMode string, providerIDs []string) ([]model.ProviderOffer, error) {
	if len(providerIDs) == 0 {
		return nil, nil
	}
	gpuType = normalizeGPUType(gpuType)
	var rows []model.ProviderOffer
	q := r.ds.DB(ctx).
		Where("gpu_type = ? AND gpu_count = ? AND billing_mode = ?", gpuType, gpuCount, billingMode).
		Where("provider_id IN ?", providerIDs).
		Where("available_cap > 0")
	if err := q.Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list provider offers: %w", err)
	}
	return rows, nil
}

func (r *ProviderOfferRepository) UpsertBatch(ctx context.Context, offers []model.ProviderOffer) error {
	if len(offers) == 0 {
		return nil
	}
	now := time.Now()
	for i := range offers {
		offers[i].GPUType = normalizeGPUType(offers[i].GPUType)
		offers[i].SyncedAt = now
	}
	const batch = 100
	for i := 0; i < len(offers); i += batch {
		end := i + batch
		if end > len(offers) {
			end = len(offers)
		}
		if err := r.ds.DB(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{
				{Name: "provider_id"}, {Name: "sku_id"}, {Name: "gpu_count"}, {Name: "region"}, {Name: "billing_mode"},
			},
			DoUpdates: clause.AssignmentColumns([]string{
				"gpu_type", "logical_az_id", "unit_price", "currency", "available_cap", "resource_mark", "synced_at",
			}),
		}).Create(offers[i:end]).Error; err != nil {
			return err
		}
	}
	return nil
}

func normalizeGPUType(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}
