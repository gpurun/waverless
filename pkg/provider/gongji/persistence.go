package gongji

import (
	"context"
	"time"
)

const (
	InventoryPullStatusSuccess = "success"
	InventoryPullStatusFailed  = "failed"
)

// CatalogStore persists logical AZ bindings and versioned inventory pulls.
type CatalogStore interface {
	Load(ctx context.Context, providerID string) (PersistedCatalog, error)
	SeedBindingsIfEmpty(ctx context.Context, providerID string, bindings []RegionBinding) error
	UpsertBinding(ctx context.Context, providerID string, binding RegionBinding) (registryVersion int64, err error)
	SetAvailability(ctx context.Context, providerID, logicalAzID, availability, reason string) (registryVersion int64, err error)
	RecordInventoryPull(ctx context.Context, providerID string, rec InventoryPullRecord) (pullVersion int64, err error)
	ListInventoryPulls(ctx context.Context, providerID string, page, pageSize int) ([]InventoryPullSummary, int64, error)
}

// PersistedCatalog is the DB view used to hydrate an in-memory catalog.
type PersistedCatalog struct {
	Bindings                     []RegionBinding
	RegistryVersion              int64
	LatestInventoryPullVersion   int64
	LatestSuccessPullVersion     int64
	LatestSuccess                *InventoryPullSnapshot
}

// InventoryPullSnapshot is one successful vendor inventory version.
type InventoryPullSnapshot struct {
	Version    int64
	ObservedAt time.Time
	FreshUntil time.Time
	Offers     map[string]InventoryOfferRow
}

// InventoryOfferRow is one region/SKU line in a pull snapshot.
type InventoryOfferRow struct {
	Region, RegionName, GPUName string
	GPUCount, GPUMemory, Memory, CPUCores int
	DeviceName, Mark string
	Inventory int
	Price, DiscountPrice *int
}

// InventoryPullRecord is written on every sync attempt (success or failure).
type InventoryPullRecord struct {
	Status       string
	ErrorMessage string
	ObservedAt   time.Time
	FreshUntil   *time.Time
	DeviceCount  int
	Offers       []InventoryOfferRow
}

// InventoryPullSummary is a list row for operators.
type InventoryPullSummary struct {
	Version      int64      `json:"version"`
	Status       string     `json:"status"`
	ErrorMessage string     `json:"errorMessage,omitempty"`
	ObservedAt   time.Time  `json:"observedAt"`
	FreshUntil   *time.Time `json:"freshUntil,omitempty"`
	DeviceCount  int        `json:"deviceCount"`
	OfferCount   int        `json:"offerCount"`
}

// RegistryVersionInfo exposes monotonic versions for tracing.
type RegistryVersionInfo struct {
	ProviderID                    string `json:"providerId"`
	RegistryVersion               int64  `json:"registryVersion"`
	LatestInventoryPullVersion    int64  `json:"latestInventoryPullVersion,omitempty"`
	LatestSuccessInventoryVersion int64  `json:"latestSuccessInventoryPullVersion,omitempty"`
}
