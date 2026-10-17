package gongji

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeResources struct {
	data *ResourceSearchData
	err  error
	hits int
}

func (f *fakeResources) SearchResources(ctx context.Context, filter ResourceSearchFilter) (*ResourceSearchData, error) {
	f.hits++
	if f.err != nil {
		return nil, f.err
	}
	return f.data, nil
}

func sampleOffer(inventory int) *ResourceSearchData {
	price := 16416
	return &ResourceSearchData{
		Count: 1,
		Results: []ResourceDevice{{
			DeviceName: "H20 x 8",
			GPUName:    "H20",
			GPUCount:   8,
			GPUMemory:  786432,
			Memory:     1572864,
			CPUCores:   192,
			Regions: []ResourceRegion{{
				Region:     "hebeif-1",
				RegionName: "河北一区",
				Mark:       ResourceMark{Mark: "mark-hebei"},
				Price:      &price,
				Inventory:  inventory,
			}},
		}},
	}
}

func TestCatalogInventoryZeroIsNotUnknown(t *testing.T) {
	src := &fakeResources{data: sampleOffer(0)}
	catalog := NewCatalog(src, CatalogConfig{FreshTTL: time.Minute})
	if err := catalog.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	catalog.bindings = []RegionBinding{{
		LogicalAzID:      "cn-hebei-az1",
		DisplayName:      "华北·河北一区",
		ComplianceDomain: ComplianceDomestic,
		Availability:     AvailabilitySchedulable,
		ProviderID:       ProviderIDGongji,
		PhysicalRegionID: "hebeif-1",
		ProviderSkuID:    "H20",
		GPUCount:         8,
	}}

	ops := catalog.ListOps("", "", false)
	if len(ops) != 1 || !ops[0].InventoryKnown || ops[0].Inventory == nil || *ops[0].Inventory != 0 {
		t.Fatalf("expected known zero inventory, got %+v", ops)
	}
	if ops[0].Freshness != FreshnessFresh || ops[0].Schedulable {
		t.Fatalf("fresh zero must not be schedulable: %+v", ops[0])
	}
	_, err := catalog.ResolveMark("cn-hebei-az1", "H20")
	var ge *Error
	if !errors.As(err, &ge) || ge.Code != CodeCapacityExhausted {
		t.Fatalf("got %v", err)
	}
	catalogView := catalog.ListCatalog("", "", false)
	if catalogView[0].LogicalAzID == "" || ops[0].ResourceMark == "" {
		t.Fatal("ops mark missing")
	}
	if catalogView[0].GPUSku != "H20" {
		t.Fatalf("catalog sku %+v", catalogView[0])
	}
}

func TestCatalogPullFailureBecomesStaleNotZero(t *testing.T) {
	now := time.Date(2026, 10, 17, 9, 0, 0, 0, time.UTC)
	src := &fakeResources{data: sampleOffer(3)}
	catalog := NewCatalog(src, CatalogConfig{FreshTTL: time.Minute})
	catalog.SetClock(func() time.Time { return now })
	if err := catalog.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	catalog.bindings = []RegionBinding{{
		LogicalAzID:      "cn-hebei-az1",
		DisplayName:      "华北·河北一区",
		ComplianceDomain: ComplianceDomestic,
		Availability:     AvailabilitySchedulable,
		ProviderID:       ProviderIDGongji,
		PhysicalRegionID: "hebeif-1",
		ProviderSkuID:    "H20",
		GPUCount:         8,
	}}

	src.err = &Error{Code: CodeProviderTimeout, Message: "gongji request timed out"}
	now = now.Add(10 * time.Second)
	if err := catalog.Refresh(context.Background()); err == nil {
		t.Fatal("expected refresh error")
	}
	ops := catalog.ListOps("", "", false)
	if ops[0].Freshness != FreshnessStale || !ops[0].InventoryKnown || *ops[0].Inventory != 3 {
		t.Fatalf("stale snapshot lost the last count: %+v", ops[0])
	}
	if ops[0].Schedulable {
		t.Fatal("stale inventory must not stay schedulable")
	}
	if _, err := catalog.ResolveMark("cn-hebei-az1", "H20"); err == nil {
		t.Fatal("expected resolve to fail closed")
	}
}

func TestCatalogFirstFailureIsUnknown(t *testing.T) {
	src := &fakeResources{err: errors.New("dial failed")}
	catalog := NewCatalog(src, CatalogConfig{FreshTTL: time.Minute})
	if err := catalog.Refresh(context.Background()); err == nil {
		t.Fatal("expected error")
	}
	snap := catalog.Snapshot()
	if snap.Freshness != FreshnessUnknown || len(snap.Items) != 0 {
		t.Fatalf("%+v", snap)
	}
}

func TestMaintenanceBlocksSchedulable(t *testing.T) {
	src := &fakeResources{data: sampleOffer(4)}
	catalog := NewCatalog(src, CatalogConfig{FreshTTL: time.Minute})
	if err := catalog.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	catalog.bindings = []RegionBinding{{
		LogicalAzID:      "cn-hebei-az1",
		DisplayName:      "华北·河北一区",
		ComplianceDomain: ComplianceDomestic,
		Availability:     AvailabilitySchedulable,
		ProviderID:       ProviderIDGongji,
		PhysicalRegionID: "hebeif-1",
		ProviderSkuID:    "H20",
		GPUCount:         8,
	}}
	if err := catalog.SetAvailability("cn-hebei-az1", AvailabilityMaintenance, "vendor window"); err != nil {
		t.Fatal(err)
	}
	ops := catalog.ListOps("", "", false)
	if ops[0].Availability != AvailabilityMaintenance || ops[0].Schedulable {
		t.Fatalf("%+v", ops[0])
	}
	if _, err := catalog.ResolveMark("cn-hebei-az1", ""); err == nil {
		t.Fatal("maintenance region must not resolve a mark")
	}
}

func TestRegistryRejectsDuplicateAndEmptySKU(t *testing.T) {
	_, err := ParseRegionFile([]byte(`
version: 1
regions:
  - logical_az_id: cn-hebei-az1
    display_name: 华北
    compliance_domain: domestic
    availability: schedulable
    providers:
      - provider_id: gongji
        physical_region_id: hebeif-1
        provider_sku_id: ""
        gpu_count: 1
`))
	if err == nil {
		t.Fatal("expected invalid sku")
	}

	_, err = ParseRegionFile([]byte(`
version: 1
regions:
  - logical_az_id: cn-hebei-az1
    display_name: 华北
    compliance_domain: domestic
    providers:
      - provider_id: gongji
        physical_region_id: hebeif-1
        provider_sku_id: H20
        gpu_count: 8
  - logical_az_id: cn-hebei-az1
    display_name: 重复
    compliance_domain: domestic
    providers:
      - provider_id: gongji
        physical_region_id: xingjiangf-1
        provider_sku_id: H20
        gpu_count: 8
`))
	if err == nil {
		t.Fatal("expected duplicate logical az")
	}
}
