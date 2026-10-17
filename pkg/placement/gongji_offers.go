package placement

import (
	"context"

	"waverless/pkg/provider/gongji"
	"waverless/pkg/store/mysql"
	"waverless/pkg/store/mysql/model"
)

// SyncGongjiCatalog upserts provider_offers from a fresh Gongji catalog snapshot.
func SyncGongjiCatalog(ctx context.Context, catalog *gongji.Catalog, repo *mysql.ProviderOfferRepository) error {
	if catalog == nil || repo == nil {
		return nil
	}
	snap := catalog.Snapshot()
	offers := make([]model.ProviderOffer, 0, len(snap.Items))
	for _, item := range snap.Items {
		if item.Inventory <= 0 || item.GPUName == "" {
			continue
		}
		price := 0.0
		if item.DiscountPrice != nil {
			price = float64(*item.DiscountPrice)
		} else if item.Price != nil {
			price = float64(*item.Price)
		}
		offers = append(offers, model.ProviderOffer{
			ProviderID:   gongji.ProviderIDGongji,
			SkuID:        item.GPUName,
			GPUType:      item.GPUName,
			GPUCount:     item.GPUCount,
			Region:       item.PhysicalRegionID,
			LogicalAzID:  item.LogicalAzID,
			BillingMode:  "on_demand",
			UnitPrice:    price,
			Currency:     "CNY",
			AvailableCap: item.Inventory,
			ResourceMark: item.ResourceMark,
		})
	}
	return repo.UpsertBatch(ctx, offers)
}
