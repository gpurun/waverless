package gongji

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	FreshnessFresh   = "fresh"
	FreshnessStale   = "stale"
	FreshnessUnknown = "unknown"

	StatusHealthy  = "healthy"
	StatusDegraded = "degraded"
	StatusUnknown  = "unknown"
)

// ResourceSource loads vendor inventory. *Client implements it.
type ResourceSource interface {
	SearchResources(ctx context.Context, filter ResourceSearchFilter) (*ResourceSearchData, error)
}

// CatalogConfig controls registry load, freshness, and maintenance persistence.
type CatalogConfig struct {
	RegionsFile string
	StateFile   string
	FreshTTL    time.Duration
	DeviceType  string
}

// Catalog is the in-memory logical AZ registry joined with Gongji inventory snapshots.
// A failed poll never rewrites a previous inventory count to zero.
type Catalog struct {
	source ResourceSource
	cfg    CatalogConfig
	store  CatalogStore
	now    func() time.Time

	mu                       sync.RWMutex
	providerID               string
	registryVersion          int64
	latestPullVersion        int64
	latestSuccessPullVersion int64
	bindings                 []RegionBinding
	overrides                map[string]string // logicalAzId -> availability (file overlay only)
	offers                   map[string]observedOffer
	observedAt               time.Time
	freshUntil               time.Time
	lastError                string
	everSuccess              bool
}

type observedOffer struct {
	Region        string
	RegionName    string
	GPUName       string
	GPUCount      int
	GPUMemory     int
	Memory        int
	CPUCores      int
	DeviceName    string
	Mark          string
	Price         *int
	DiscountPrice *int
	Inventory     int
}

// CatalogRegion is the tenant-safe logical AZ view. It has no mark, price, or physical id.
type CatalogRegion struct {
	LogicalAzID      string `json:"logicalAzId"`
	DisplayName      string `json:"displayName"`
	ComplianceDomain string `json:"complianceDomain"`
	Availability     string `json:"availability"`
	GPUSku           string `json:"gpuSku"`
	GPUCount         int    `json:"gpuCount"`
	Status           string `json:"status"`
	Schedulable      bool   `json:"schedulable"`
}

// OpsRegion is the operator view, including vendor inventory and the resource mark.
type OpsRegion struct {
	CatalogRegion
	ProviderID       string     `json:"providerId"`
	PhysicalRegionID string     `json:"physicalRegionId"`
	ProviderSkuID    string     `json:"providerSkuId"`
	ResourceMark     string     `json:"resourceMark,omitempty"`
	Inventory        *int       `json:"inventory,omitempty"`
	InventoryKnown   bool       `json:"inventoryKnown"`
	Price            *int       `json:"price,omitempty"`
	DiscountPrice    *int       `json:"discountPrice,omitempty"`
	ObservedAt       *time.Time `json:"observedAt,omitempty"`
	FreshUntil       *time.Time `json:"freshUntil,omitempty"`
	Freshness        string     `json:"freshness"`
	LastError        string     `json:"lastError,omitempty"`
}

// ObservedResource is one vendor region offer, including unbound SKUs.
type ObservedResource struct {
	PhysicalRegionID string `json:"physicalRegionId"`
	RegionName       string `json:"regionName"`
	GPUName          string `json:"gpuName"`
	GPUCount         int    `json:"gpuCount"`
	GPUMemory        int    `json:"gpuMemory"`
	Memory           int    `json:"memory"`
	CPUCores         int    `json:"cpuCores"`
	DeviceName       string `json:"deviceName"`
	ResourceMark     string `json:"resourceMark,omitempty"`
	Inventory        int    `json:"inventory"`
	Price            *int   `json:"price,omitempty"`
	DiscountPrice    *int   `json:"discountPrice,omitempty"`
	LogicalAzID      string `json:"logicalAzId,omitempty"`
}

// ResourceSnapshot is the latest vendor pull plus freshness.
type ResourceSnapshot struct {
	Freshness  string             `json:"freshness"`
	ObservedAt *time.Time         `json:"observedAt,omitempty"`
	FreshUntil *time.Time         `json:"freshUntil,omitempty"`
	LastError  string             `json:"lastError,omitempty"`
	Items      []ObservedResource `json:"items"`
	Total      int                `json:"total"`
	Page       int                `json:"page"`
	PageSize   int                `json:"pageSize"`
}

type persistedState struct {
	Availability map[string]string `json:"availability"`
	Bindings     []RegionBinding   `json:"bindings"`
}

// NewCatalog builds a catalog. Call Load before serving.
func NewCatalog(source ResourceSource, cfg CatalogConfig) *Catalog {
	if cfg.FreshTTL <= 0 {
		cfg.FreshTTL = 60 * time.Second
	}
	if cfg.DeviceType == "" {
		cfg.DeviceType = DeviceTypeGPU
	}
	return &Catalog{
		source:     source,
		cfg:        cfg,
		now:        time.Now,
		providerID: ProviderIDGongji,
		overrides:  map[string]string{},
		offers:     map[string]observedOffer{},
	}
}

// SetStore enables database-backed registry and versioned inventory snapshots.
func (c *Catalog) SetStore(store CatalogStore) {
	c.store = store
}

// SetClock overrides the clock. Tests use it.
func (c *Catalog) SetClock(now func() time.Time) {
	if now != nil {
		c.now = now
	}
}

// Load reads the versioned registry and any maintenance overlay.
// Invalid registry content fails closed. A missing file leaves the registry empty.
// When a CatalogStore is configured, bindings and the latest successful inventory
// snapshot are loaded from the database; YAML seeds an empty registry only.
func (c *Catalog) Load(ctx context.Context) error {
	if c.store != nil {
		return c.loadFromStore(ctx)
	}
	bindings, err := LoadRegionFile(c.cfg.RegionsFile)
	if err != nil {
		return err
	}
	state, err := loadState(c.cfg.StateFile)
	if err != nil {
		return err
	}
	merged, overrides := mergeState(bindings, state)
	if err := validateBindings(merged); err != nil {
		return err
	}
	c.mu.Lock()
	c.bindings = merged
	c.overrides = overrides
	c.mu.Unlock()
	return nil
}

func (c *Catalog) loadFromStore(ctx context.Context) error {
	yamlBindings, err := LoadRegionFile(c.cfg.RegionsFile)
	if err != nil {
		return err
	}
	if err := c.store.SeedBindingsIfEmpty(ctx, c.providerID, yamlBindings); err != nil {
		return err
	}
	persisted, err := c.store.Load(ctx, c.providerID)
	if err != nil {
		return err
	}
	if err := validateBindings(persisted.Bindings); err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	c.bindings = persisted.Bindings
	c.overrides = map[string]string{}
	c.registryVersion = persisted.RegistryVersion
	c.latestPullVersion = persisted.LatestInventoryPullVersion
	c.latestSuccessPullVersion = persisted.LatestSuccessPullVersion
	if persisted.LatestSuccess != nil {
		c.applySnapshotLocked(*persisted.LatestSuccess)
	}
	return nil
}

func (c *Catalog) applySnapshotLocked(snap InventoryPullSnapshot) {
	c.offers = map[string]observedOffer{}
	for key, row := range snap.Offers {
		if key == "" {
			key = offerKey(row.Region, row.GPUName, row.GPUCount)
		}
		c.offers[key] = inventoryRowToOffer(row)
	}
	c.observedAt = snap.ObservedAt
	c.freshUntil = snap.FreshUntil
	c.lastError = ""
	c.everSuccess = true
}

func inventoryRowToOffer(row InventoryOfferRow) observedOffer {
	return observedOffer{
		Region:        row.Region,
		RegionName:    row.RegionName,
		GPUName:       row.GPUName,
		GPUCount:      row.GPUCount,
		GPUMemory:     row.GPUMemory,
		Memory:        row.Memory,
		CPUCores:      row.CPUCores,
		DeviceName:    row.DeviceName,
		Mark:          row.Mark,
		Price:         row.Price,
		DiscountPrice: row.DiscountPrice,
		Inventory:     row.Inventory,
	}
}

// VersionInfo returns monotonic registry and inventory pull versions.
func (c *Catalog) VersionInfo() RegistryVersionInfo {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return RegistryVersionInfo{
		ProviderID:                    c.providerID,
		RegistryVersion:               c.registryVersion,
		LatestInventoryPullVersion:    c.latestPullVersion,
		LatestSuccessInventoryVersion: c.latestSuccessPullVersion,
	}
}

// ListInventoryPulls returns paginated sync history when a store is configured.
func (c *Catalog) ListInventoryPulls(ctx context.Context, page, pageSize int) ([]InventoryPullSummary, int64, error) {
	if c.store == nil {
		return nil, 0, fmt.Errorf("inventory pull history requires a metadata database")
	}
	return c.store.ListInventoryPulls(ctx, c.providerID, page, pageSize)
}

// Start polls inventory until ctx is cancelled. The first refresh runs immediately.
func (c *Catalog) Start(ctx context.Context, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	_ = c.Refresh(ctx)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = c.Refresh(ctx)
		}
	}
}

// Refresh pulls vendor inventory. On failure the previous counts stay in place
// and become stale once freshUntil passes. The first failure with no snapshot is unknown.
func (c *Catalog) Refresh(ctx context.Context) error {
	if c.source == nil {
		return fmtError(CodeRegionUnavailable, "gongji resource source is not configured")
	}
	data, err := c.source.SearchResources(ctx, ResourceSearchFilter{
		TaskType:   TaskTypeDeploy,
		DeviceType: c.cfg.DeviceType,
	})
	now := c.now()
	c.mu.Lock()
	defer c.mu.Unlock()
	if err != nil {
		c.lastError = sanitizePullError(err)
		if c.everSuccess {
			// A failed pull must not stay sellable. Keep the last counts, but mark them stale.
			c.freshUntil = now.Add(-time.Second)
		}
		c.recordPullLocked(ctx, InventoryPullStatusFailed, c.lastError, nil, 0, now, nil)
		return err
	}
	next := map[string]observedOffer{}
	for _, device := range data.Results {
		for _, region := range device.Regions {
			gpuName := device.GPUName
			gpuCount := device.GPUCount
			if region.Mark.Resource != nil {
				if region.Mark.Resource.GPUName != "" {
					gpuName = region.Mark.Resource.GPUName
				}
				if region.Mark.Resource.GPUCount > 0 {
					gpuCount = region.Mark.Resource.GPUCount
				}
			}
			if region.Region == "" || gpuName == "" || gpuCount <= 0 {
				continue
			}
			next[offerKey(region.Region, gpuName, gpuCount)] = observedOffer{
				Region:        region.Region,
				RegionName:    region.RegionName,
				GPUName:       gpuName,
				GPUCount:      gpuCount,
				GPUMemory:     device.GPUMemory,
				Memory:        device.Memory,
				CPUCores:      device.CPUCores,
				DeviceName:    device.DeviceName,
				Mark:          region.Mark.Mark,
				Price:         region.Price,
				DiscountPrice: region.DiscountPrice,
				Inventory:     region.Inventory,
			}
		}
	}
	c.offers = next
	c.observedAt = now
	freshUntil := now.Add(c.cfg.FreshTTL)
	c.freshUntil = freshUntil
	c.lastError = ""
	c.everSuccess = true
	c.recordPullLocked(ctx, InventoryPullStatusSuccess, "", next, len(data.Results), now, &freshUntil)
	return nil
}

func (c *Catalog) recordPullLocked(ctx context.Context, status, errMsg string, offers map[string]observedOffer, deviceCount int, observedAt time.Time, freshUntil *time.Time) {
	if c.store == nil {
		return
	}
	rec := InventoryPullRecord{
		Status:       status,
		ErrorMessage: errMsg,
		ObservedAt:   observedAt,
		FreshUntil:   freshUntil,
		DeviceCount:  deviceCount,
	}
	if len(offers) > 0 {
		rec.Offers = make([]InventoryOfferRow, 0, len(offers))
		for _, offer := range offers {
			rec.Offers = append(rec.Offers, InventoryOfferRow{
				Region:        offer.Region,
				RegionName:    offer.RegionName,
				GPUName:       offer.GPUName,
				GPUCount:      offer.GPUCount,
				GPUMemory:     offer.GPUMemory,
				Memory:        offer.Memory,
				CPUCores:      offer.CPUCores,
				DeviceName:    offer.DeviceName,
				Mark:          offer.Mark,
				Inventory:     offer.Inventory,
				Price:         offer.Price,
				DiscountPrice: offer.DiscountPrice,
			})
		}
	}
	version, err := c.store.RecordInventoryPull(ctx, c.providerID, rec)
	if err != nil {
		return
	}
	c.latestPullVersion = version
	if status == InventoryPullStatusSuccess {
		c.latestSuccessPullVersion = version
	}
}

// ListCatalog returns tenant-safe logical AZs.
func (c *Catalog) ListCatalog(complianceDomain, gpuSku string, schedulableOnly bool) []CatalogRegion {
	ops := c.ListOps(complianceDomain, gpuSku, schedulableOnly)
	out := make([]CatalogRegion, 0, len(ops))
	for _, item := range ops {
		out = append(out, item.CatalogRegion)
	}
	return out
}

// ListOps returns operator logical AZ rows.
func (c *Catalog) ListOps(complianceDomain, gpuSku string, schedulableOnly bool) []OpsRegion {
	c.mu.RLock()
	defer c.mu.RUnlock()
	freshness := c.freshnessLocked()
	out := make([]OpsRegion, 0, len(c.bindings))
	for _, binding := range c.bindings {
		if complianceDomain != "" && binding.ComplianceDomain != complianceDomain {
			continue
		}
		if gpuSku != "" && binding.ProviderSkuID != gpuSku {
			continue
		}
		item := c.projectLocked(binding, freshness)
		if schedulableOnly && !item.Schedulable {
			continue
		}
		out = append(out, item)
	}
	return out
}

// Snapshot returns the raw vendor offers for maintenance. Marks are included.
func (c *Catalog) Snapshot() ResourceSnapshot {
	c.mu.RLock()
	defer c.mu.RUnlock()
	items := make([]ObservedResource, 0, len(c.offers))
	logicalByOffer := map[string]string{}
	for _, binding := range c.bindings {
		logicalByOffer[offerKey(binding.PhysicalRegionID, binding.ProviderSkuID, binding.GPUCount)] = binding.LogicalAzID
	}
	for _, offer := range c.offers {
		items = append(items, ObservedResource{
			PhysicalRegionID: offer.Region,
			RegionName:       offer.RegionName,
			GPUName:          offer.GPUName,
			GPUCount:         offer.GPUCount,
			GPUMemory:        offer.GPUMemory,
			Memory:           offer.Memory,
			CPUCores:         offer.CPUCores,
			DeviceName:       offer.DeviceName,
			ResourceMark:     offer.Mark,
			Inventory:        offer.Inventory,
			Price:            offer.Price,
			DiscountPrice:    offer.DiscountPrice,
			LogicalAzID:      logicalByOffer[offerKey(offer.Region, offer.GPUName, offer.GPUCount)],
		})
	}
	snap := ResourceSnapshot{
		Freshness: c.freshnessLocked(),
		LastError: c.lastError,
		Items:     items,
		Total:     len(items),
	}
	if c.everSuccess {
		observed := c.observedAt
		fresh := c.freshUntil
		snap.ObservedAt = &observed
		snap.FreshUntil = &fresh
	}
	return snap
}

// ResolveMark returns the vendor mark for a schedulable, fresh binding.
// available=0 is capacity exhaustion. Stale or unknown inventory is not sold as zero.
func (c *Catalog) ResolveMark(logicalAzID, sku string) (string, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	var binding *RegionBinding
	for i := range c.bindings {
		if c.bindings[i].LogicalAzID != logicalAzID {
			continue
		}
		if sku != "" && c.bindings[i].ProviderSkuID != sku {
			continue
		}
		binding = &c.bindings[i]
		break
	}
	if binding == nil {
		return "", fmtError(CodeRegionUnavailable, "logical AZ %s is not registered", logicalAzID)
	}
	availability := c.effectiveAvailabilityLocked(*binding)
	if availability != AvailabilitySchedulable {
		return "", fmtError(CodeRegionUnavailable, "logical AZ %s is %s", logicalAzID, availability)
	}
	freshness := c.freshnessLocked()
	if freshness != FreshnessFresh {
		return "", fmtError(CodeRegionUnavailable, "logical AZ %s inventory is %s", logicalAzID, freshness)
	}
	offer, ok := c.offers[offerKey(binding.PhysicalRegionID, binding.ProviderSkuID, binding.GPUCount)]
	if !ok || offer.Mark == "" {
		return "", fmtError(CodeRegionUnavailable, "logical AZ %s has no observed Gongji resource", logicalAzID)
	}
	if offer.Inventory <= 0 {
		return "", fmtError(CodeCapacityExhausted, "logical AZ %s has no available capacity", logicalAzID)
	}
	return offer.Mark, nil
}

// SetAvailability records a platform maintenance state. It does not call Gongji
// and does not terminate existing workers.
func (c *Catalog) SetAvailability(logicalAzID, availability, reason string) error {
	return c.SetAvailabilityContext(context.Background(), logicalAzID, availability, reason)
}

// SetAvailabilityContext updates sellability and persists when a store is configured.
func (c *Catalog) SetAvailabilityContext(ctx context.Context, logicalAzID, availability, reason string) error {
	if !validAvailability(availability) {
		return fmt.Errorf("availability %q is invalid", availability)
	}
	if reason == "" && availability != AvailabilitySchedulable {
		return fmt.Errorf("reason is required when taking a region out of service")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	found := false
	for i := range c.bindings {
		if c.bindings[i].LogicalAzID != logicalAzID {
			continue
		}
		found = true
		if c.store != nil {
			v, err := c.store.SetAvailability(ctx, c.providerID, logicalAzID, availability, reason)
			if err != nil {
				return err
			}
			c.registryVersion = v
			c.bindings[i].Availability = availability
			return nil
		}
		break
	}
	if !found {
		return fmtError(CodeRegionUnavailable, "logical AZ %s is not registered", logicalAzID)
	}
	if c.overrides == nil {
		c.overrides = map[string]string{}
	}
	c.overrides[logicalAzID] = availability
	return c.persistLocked()
}

// UpsertBinding adds or replaces a logical AZ binding.
// The physical region and SKU must already appear in a successful inventory snapshot.
func (c *Catalog) UpsertBinding(binding RegionBinding) error {
	return c.UpsertBindingContext(context.Background(), binding)
}

func (c *Catalog) UpsertBindingContext(ctx context.Context, binding RegionBinding) error {
	if !logicalAzPattern.MatchString(binding.LogicalAzID) {
		return fmt.Errorf("logical_az_id %q is invalid", binding.LogicalAzID)
	}
	if binding.DisplayName == "" {
		return fmt.Errorf("display_name is required")
	}
	if binding.ComplianceDomain != ComplianceDomestic && binding.ComplianceDomain != ComplianceOverseas {
		return fmt.Errorf("compliance_domain %q is invalid", binding.ComplianceDomain)
	}
	if binding.Availability == "" {
		binding.Availability = AvailabilitySchedulable
	}
	if !validAvailability(binding.Availability) {
		return fmt.Errorf("availability %q is invalid", binding.Availability)
	}
	binding.ProviderID = ProviderIDGongji
	if binding.PhysicalRegionID == "" || binding.ProviderSkuID == "" || binding.GPUCount <= 0 {
		return fmt.Errorf("physical region, sku, and gpu_count are required")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.freshnessLocked() == FreshnessUnknown {
		return fmtError(CodeRegionUnavailable, "cannot bind a region before inventory has been observed")
	}
	if _, ok := c.offers[offerKey(binding.PhysicalRegionID, binding.ProviderSkuID, binding.GPUCount)]; !ok {
		return fmtError(CodeRegionUnavailable, "physical region %s sku %s x%d is not in the latest inventory snapshot", binding.PhysicalRegionID, binding.ProviderSkuID, binding.GPUCount)
	}
	next := append([]RegionBinding(nil), c.bindings...)
	replaced := false
	for i := range next {
		if next[i].LogicalAzID == binding.LogicalAzID {
			next[i] = binding
			replaced = true
			break
		}
	}
	if !replaced {
		next = append(next, binding)
	}
	if err := validateBindings(next); err != nil {
		return err
	}
	c.bindings = next
	if c.store != nil {
		v, err := c.store.UpsertBinding(ctx, c.providerID, binding)
		if err != nil {
			return err
		}
		c.registryVersion = v
		return nil
	}
	return c.persistLocked()
}

func (c *Catalog) projectLocked(binding RegionBinding, freshness string) OpsRegion {
	availability := c.effectiveAvailabilityLocked(binding)
	item := OpsRegion{
		CatalogRegion: CatalogRegion{
			LogicalAzID:      binding.LogicalAzID,
			DisplayName:      binding.DisplayName,
			ComplianceDomain: binding.ComplianceDomain,
			Availability:     availability,
			GPUSku:           binding.ProviderSkuID,
			GPUCount:         binding.GPUCount,
			Status:           StatusUnknown,
		},
		ProviderID:       binding.ProviderID,
		PhysicalRegionID: binding.PhysicalRegionID,
		ProviderSkuID:    binding.ProviderSkuID,
		Freshness:        freshness,
		LastError:        c.lastError,
	}
	if c.everSuccess {
		observed := c.observedAt
		fresh := c.freshUntil
		item.ObservedAt = &observed
		item.FreshUntil = &fresh
	}
	offer, ok := c.offers[offerKey(binding.PhysicalRegionID, binding.ProviderSkuID, binding.GPUCount)]
	if freshness == FreshnessFresh && ok {
		inv := offer.Inventory
		item.Inventory = &inv
		item.InventoryKnown = true
		item.ResourceMark = offer.Mark
		item.Price = offer.Price
		item.DiscountPrice = offer.DiscountPrice
		if availability == AvailabilitySchedulable && inv > 0 && offer.Mark != "" {
			item.Status = StatusHealthy
			item.Schedulable = true
		} else {
			item.Status = StatusDegraded
		}
		return item
	}
	if freshness == FreshnessStale && ok {
		inv := offer.Inventory
		item.Inventory = &inv
		item.InventoryKnown = true
		item.ResourceMark = offer.Mark
		item.Price = offer.Price
		item.DiscountPrice = offer.DiscountPrice
		item.Status = StatusUnknown
		return item
	}
	item.Status = StatusUnknown
	item.InventoryKnown = false
	return item
}

func (c *Catalog) effectiveAvailabilityLocked(binding RegionBinding) string {
	if override, ok := c.overrides[binding.LogicalAzID]; ok && override != "" {
		return override
	}
	return binding.Availability
}

func (c *Catalog) freshnessLocked() string {
	if !c.everSuccess {
		return FreshnessUnknown
	}
	if c.now().After(c.freshUntil) {
		return FreshnessStale
	}
	return FreshnessFresh
}

func (c *Catalog) persistLocked() error {
	if c.store != nil || c.cfg.StateFile == "" {
		return nil
	}
	state := persistedState{
		Availability: c.overrides,
		Bindings:     c.bindings,
	}
	buf, err := json.MarshalIndent(state, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.cfg.StateFile), 0o755); err != nil && !os.IsExist(err) {
		return err
	}
	tmp := c.cfg.StateFile + ".tmp"
	if err := os.WriteFile(tmp, buf, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, c.cfg.StateFile)
}

func loadState(path string) (persistedState, error) {
	if path == "" {
		return persistedState{}, nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return persistedState{}, nil
		}
		return persistedState{}, fmt.Errorf("read region state %s: %w", path, err)
	}
	var state persistedState
	if err := json.Unmarshal(data, &state); err != nil {
		return persistedState{}, fmt.Errorf("parse region state: %w", err)
	}
	return state, nil
}

func mergeState(bindings []RegionBinding, state persistedState) ([]RegionBinding, map[string]string) {
	overrides := map[string]string{}
	for id, availability := range state.Availability {
		if validAvailability(availability) {
			overrides[id] = availability
		}
	}
	if len(state.Bindings) == 0 {
		return bindings, overrides
	}
	return state.Bindings, overrides
}

func validateBindings(bindings []RegionBinding) error {
	seenAZ := map[string]struct{}{}
	seenBind := map[string]struct{}{}
	for _, binding := range bindings {
		if _, ok := seenAZ[binding.LogicalAzID]; ok {
			return fmt.Errorf("duplicate logical_az_id %s", binding.LogicalAzID)
		}
		seenAZ[binding.LogicalAzID] = struct{}{}
		key := bindingKey(binding.ProviderID, binding.PhysicalRegionID, binding.ProviderSkuID, binding.GPUCount)
		if _, ok := seenBind[key]; ok {
			return fmt.Errorf("duplicate provider binding %s", key)
		}
		seenBind[key] = struct{}{}
	}
	return nil
}

func sanitizePullError(err error) string {
	if ge, ok := AsError(err); ok {
		return ge.Error()
	}
	return "gongji inventory pull failed"
}
