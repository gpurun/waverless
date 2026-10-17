package mysql

import (
	"context"
	"fmt"

	"waverless/pkg/provider/gongji"
	"waverless/pkg/store/mysql/model"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GongjiCatalogRepository persists Gongji region registry and inventory snapshots.
type GongjiCatalogRepository struct {
	ds *Datastore
}

// NewGongjiCatalogRepository creates the Gongji catalog repository.
func NewGongjiCatalogRepository(ds *Datastore) *GongjiCatalogRepository {
	return &GongjiCatalogRepository{ds: ds}
}

func (r *GongjiCatalogRepository) ensureMeta(ctx context.Context, providerID string) error {
	row := model.ProviderRegionRegistryMeta{ProviderID: providerID, RegistryVersion: 1}
	return r.ds.DB(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "provider_id"}},
		DoNothing: true,
	}).Create(&row).Error
}

func (r *GongjiCatalogRepository) bumpRegistryVersion(ctx context.Context, providerID string) (int64, error) {
	if err := r.ensureMeta(ctx, providerID); err != nil {
		return 0, err
	}
	res := r.ds.DB(ctx).Model(&model.ProviderRegionRegistryMeta{}).
		Where("provider_id = ?", providerID).
		UpdateColumn("registry_version", gorm.Expr("registry_version + 1"))
	if res.Error != nil {
		return 0, res.Error
	}
	var meta model.ProviderRegionRegistryMeta
	if err := r.ds.DB(ctx).Where("provider_id = ?", providerID).First(&meta).Error; err != nil {
		return 0, err
	}
	return meta.RegistryVersion, nil
}

func bindingToModel(providerID string, b gongji.RegionBinding, registryVersion int64) model.ProviderRegionBinding {
	if b.ProviderID == "" {
		b.ProviderID = providerID
	}
	if b.Availability == "" {
		b.Availability = gongji.AvailabilitySchedulable
	}
	return model.ProviderRegionBinding{
		LogicalAzID:      b.LogicalAzID,
		DisplayName:      b.DisplayName,
		ComplianceDomain: b.ComplianceDomain,
		Availability:     b.Availability,
		ProviderID:       b.ProviderID,
		PhysicalRegionID: b.PhysicalRegionID,
		ProviderSkuID:    b.ProviderSkuID,
		GPUCount:         b.GPUCount,
		RegistryVersion:  registryVersion,
	}
}

func bindingFromModel(row model.ProviderRegionBinding) gongji.RegionBinding {
	return gongji.RegionBinding{
		LogicalAzID:      row.LogicalAzID,
		DisplayName:      row.DisplayName,
		ComplianceDomain: row.ComplianceDomain,
		Availability:     row.Availability,
		ProviderID:       row.ProviderID,
		PhysicalRegionID: row.PhysicalRegionID,
		ProviderSkuID:    row.ProviderSkuID,
		GPUCount:         row.GPUCount,
	}
}

// Load reads bindings and the latest successful inventory snapshot.
func (r *GongjiCatalogRepository) Load(ctx context.Context, providerID string) (gongji.PersistedCatalog, error) {
	out := gongji.PersistedCatalog{}
	var meta model.ProviderRegionRegistryMeta
	err := r.ds.DB(ctx).Where("provider_id = ?", providerID).First(&meta).Error
	if err != nil && err != gorm.ErrRecordNotFound {
		return out, fmt.Errorf("load registry meta: %w", err)
	}
	if err == nil {
		out.RegistryVersion = meta.RegistryVersion
	}

	var rows []model.ProviderRegionBinding
	if err := r.ds.DB(ctx).Where("provider_id = ?", providerID).Find(&rows).Error; err != nil {
		return out, fmt.Errorf("load bindings: %w", err)
	}
	out.Bindings = make([]gongji.RegionBinding, 0, len(rows))
	for _, row := range rows {
		out.Bindings = append(out.Bindings, bindingFromModel(row))
	}

	var pull model.ProviderInventoryPull
	err = r.ds.DB(ctx).
		Where("provider_id = ? AND status = ?", providerID, gongji.InventoryPullStatusSuccess).
		Order("version DESC").
		First(&pull).Error
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return out, nil
		}
		return out, fmt.Errorf("load latest inventory pull: %w", err)
	}

	var offerRows []model.ProviderInventoryOffer
	if err := r.ds.DB(ctx).Where("pull_id = ?", pull.ID).Find(&offerRows).Error; err != nil {
		return out, fmt.Errorf("load inventory offers: %w", err)
	}
	offers := map[string]gongji.InventoryOfferRow{}
	for _, o := range offerRows {
		key := offerKey(o.PhysicalRegionID, o.GPUName, o.GPUCount)
		offers[key] = gongji.InventoryOfferRow{
			Region:        o.PhysicalRegionID,
			RegionName:    o.RegionName,
			GPUName:       o.GPUName,
			GPUCount:      o.GPUCount,
			GPUMemory:     o.GPUMemory,
			Memory:        o.Memory,
			CPUCores:      o.CPUCores,
			DeviceName:    o.DeviceName,
			Mark:          o.ResourceMark,
			Inventory:     o.Inventory,
			Price:         o.Price,
			DiscountPrice: o.DiscountPrice,
		}
	}
	freshUntil := pull.ObservedAt
	if pull.FreshUntil != nil {
		freshUntil = *pull.FreshUntil
	}
	out.LatestSuccess = &gongji.InventoryPullSnapshot{
		Version:    pull.Version,
		ObservedAt: pull.ObservedAt,
		FreshUntil: freshUntil,
		Offers:     offers,
	}
	out.LatestSuccessPullVersion = pull.Version
	latest, latestSuccess, err := r.LatestPullVersions(ctx, providerID)
	if err != nil {
		return out, err
	}
	out.LatestInventoryPullVersion = latest
	if latestSuccess > 0 {
		out.LatestSuccessPullVersion = latestSuccess
	}
	return out, nil
}

func offerKey(region, gpuName string, gpuCount int) string {
	return fmt.Sprintf("%s|%s|%d", region, gpuName, gpuCount)
}

// SeedBindingsIfEmpty inserts YAML bindings when the DB registry is empty.
func (r *GongjiCatalogRepository) SeedBindingsIfEmpty(ctx context.Context, providerID string, bindings []gongji.RegionBinding) error {
	var count int64
	if err := r.ds.DB(ctx).Model(&model.ProviderRegionBinding{}).Where("provider_id = ?", providerID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 || len(bindings) == 0 {
		return nil
	}
	return r.ds.ExecTx(ctx, func(ctx context.Context) error {
		if err := r.ensureMeta(ctx, providerID); err != nil {
			return err
		}
		models := make([]model.ProviderRegionBinding, 0, len(bindings))
		for _, b := range bindings {
			models = append(models, bindingToModel(providerID, b, 1))
		}
		return r.ds.DB(ctx).Create(&models).Error
	})
}

// UpsertBinding writes one logical AZ binding and bumps registry version.
func (r *GongjiCatalogRepository) UpsertBinding(ctx context.Context, providerID string, binding gongji.RegionBinding) (int64, error) {
	var version int64
	err := r.ds.ExecTx(ctx, func(ctx context.Context) error {
		v, err := r.bumpRegistryVersion(ctx, providerID)
		if err != nil {
			return err
		}
		version = v
		row := bindingToModel(providerID, binding, v)
		return r.ds.DB(ctx).Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "logical_az_id"}},
			DoUpdates: clause.AssignmentColumns([]string{
				"display_name", "compliance_domain", "availability", "maintenance_reason",
				"provider_id", "physical_region_id", "provider_sku_id", "gpu_count", "registry_version", "updated_at",
			}),
		}).Create(&row).Error
	})
	return version, err
}

// SetAvailability updates sellability for one logical AZ.
func (r *GongjiCatalogRepository) SetAvailability(ctx context.Context, providerID, logicalAzID, availability, reason string) (int64, error) {
	var version int64
	err := r.ds.ExecTx(ctx, func(ctx context.Context) error {
		v, err := r.bumpRegistryVersion(ctx, providerID)
		if err != nil {
			return err
		}
		version = v
		updates := map[string]interface{}{
			"availability":     availability,
			"maintenance_reason": nil,
			"registry_version": version,
		}
		if reason != "" {
			updates["maintenance_reason"] = reason
		}
		res := r.ds.DB(ctx).Model(&model.ProviderRegionBinding{}).
			Where("logical_az_id = ? AND provider_id = ?", logicalAzID, providerID).
			Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return nil
	})
	return version, err
}

func (r *GongjiCatalogRepository) nextPullVersion(ctx context.Context, providerID string) (int64, error) {
	var maxVersion *int64
	err := r.ds.DB(ctx).Model(&model.ProviderInventoryPull{}).
		Where("provider_id = ?", providerID).
		Select("MAX(version)").
		Scan(&maxVersion).Error
	if err != nil {
		return 0, err
	}
	if maxVersion == nil {
		return 1, nil
	}
	return *maxVersion + 1, nil
}

// RecordInventoryPull appends one sync attempt and optional offer rows.
func (r *GongjiCatalogRepository) RecordInventoryPull(ctx context.Context, providerID string, rec gongji.InventoryPullRecord) (int64, error) {
	var version int64
	err := r.ds.ExecTx(ctx, func(ctx context.Context) error {
		v, err := r.nextPullVersion(ctx, providerID)
		if err != nil {
			return err
		}
		version = v
		pull := model.ProviderInventoryPull{
			ProviderID:   providerID,
			Version:      v,
			Status:       rec.Status,
			ErrorMessage: rec.ErrorMessage,
			ObservedAt:   rec.ObservedAt,
			FreshUntil:   rec.FreshUntil,
			DeviceCount:  rec.DeviceCount,
			OfferCount:   len(rec.Offers),
		}
		if err := r.ds.DB(ctx).Create(&pull).Error; err != nil {
			return err
		}
		if len(rec.Offers) == 0 {
			return nil
		}
		rows := make([]model.ProviderInventoryOffer, 0, len(rec.Offers))
		for _, o := range rec.Offers {
			rows = append(rows, model.ProviderInventoryOffer{
				PullID:           pull.ID,
				PullVersion:      v,
				ProviderID:       providerID,
				PhysicalRegionID: o.Region,
				RegionName:       o.RegionName,
				GPUName:          o.GPUName,
				GPUCount:         o.GPUCount,
				DeviceName:       o.DeviceName,
				ResourceMark:     o.Mark,
				Inventory:        o.Inventory,
				Price:            o.Price,
				DiscountPrice:    o.DiscountPrice,
				GPUMemory:        o.GPUMemory,
				Memory:           o.Memory,
				CPUCores:         o.CPUCores,
			})
		}
		const batch = 200
		for i := 0; i < len(rows); i += batch {
			end := i + batch
			if end > len(rows) {
				end = len(rows)
			}
			if err := r.ds.DB(ctx).Create(rows[i:end]).Error; err != nil {
				return err
			}
		}
		return nil
	})
	return version, err
}

// ListInventoryPulls returns paginated pull history newest first.
func (r *GongjiCatalogRepository) ListInventoryPulls(ctx context.Context, providerID string, page, pageSize int) ([]gongji.InventoryPullSummary, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	q := r.ds.DB(ctx).Model(&model.ProviderInventoryPull{}).Where("provider_id = ?", providerID)
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var rows []model.ProviderInventoryPull
	offset := (page - 1) * pageSize
	if err := q.Order("version DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	out := make([]gongji.InventoryPullSummary, 0, len(rows))
	for _, row := range rows {
		out = append(out, gongji.InventoryPullSummary{
			Version:      row.Version,
			Status:       row.Status,
			ErrorMessage: row.ErrorMessage,
			ObservedAt:   row.ObservedAt,
			FreshUntil:   row.FreshUntil,
			DeviceCount:  row.DeviceCount,
			OfferCount:   row.OfferCount,
		})
	}
	return out, total, nil
}

// LatestPullVersions returns the newest pull version and newest successful pull version.
func (r *GongjiCatalogRepository) LatestPullVersions(ctx context.Context, providerID string) (latest, latestSuccess int64, err error) {
	var row model.ProviderInventoryPull
	err = r.ds.DB(ctx).Where("provider_id = ?", providerID).Order("version DESC").First(&row).Error
	if err == nil {
		latest = row.Version
	} else if err != gorm.ErrRecordNotFound {
		return 0, 0, err
	}
	err = r.ds.DB(ctx).
		Where("provider_id = ? AND status = ?", providerID, gongji.InventoryPullStatusSuccess).
		Order("version DESC").
		First(&row).Error
	if err == nil {
		latestSuccess = row.Version
	} else if err != gorm.ErrRecordNotFound {
		return latest, 0, err
	}
	return latest, latestSuccess, nil
}

// GetRegistryVersion reads the binding registry version.
func (r *GongjiCatalogRepository) GetRegistryVersion(ctx context.Context, providerID string) (int64, error) {
	var meta model.ProviderRegionRegistryMeta
	err := r.ds.DB(ctx).Where("provider_id = ?", providerID).First(&meta).Error
	if err == gorm.ErrRecordNotFound {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return meta.RegistryVersion, nil
}

// Compile-time check.
var _ gongji.CatalogStore = (*GongjiCatalogRepository)(nil)
