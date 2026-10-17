package model

import "time"

// ProviderRegionRegistryMeta tracks monotonic registry version for binding changes.
type ProviderRegionRegistryMeta struct {
	ProviderID      string    `gorm:"column:provider_id;type:varchar(32);primaryKey"`
	RegistryVersion int64     `gorm:"column:registry_version;not null;default:1"`
	UpdatedAt       time.Time `gorm:"column:updated_at;type:timestamptz(3);not null;autoUpdateTime"`
}

func (ProviderRegionRegistryMeta) TableName() string { return "provider_region_registry_meta" }

// ProviderRegionBinding is the persisted logical AZ → Gongji physical binding.
type ProviderRegionBinding struct {
	LogicalAzID       string    `gorm:"column:logical_az_id;type:varchar(64);primaryKey"`
	DisplayName       string    `gorm:"column:display_name;type:varchar(255);not null"`
	ComplianceDomain  string    `gorm:"column:compliance_domain;type:varchar(32);not null"`
	Availability      string    `gorm:"column:availability;type:varchar(32);not null;default:schedulable"`
	MaintenanceReason string    `gorm:"column:maintenance_reason;type:text"`
	ProviderID        string    `gorm:"column:provider_id;type:varchar(32);not null;default:gongji"`
	PhysicalRegionID  string    `gorm:"column:physical_region_id;type:varchar(128);not null"`
	ProviderSkuID     string    `gorm:"column:provider_sku_id;type:varchar(128);not null"`
	GPUCount          int       `gorm:"column:gpu_count;not null"`
	RegistryVersion   int64     `gorm:"column:registry_version;not null;default:1"`
	CreatedAt         time.Time `gorm:"column:created_at;type:timestamptz(3);autoCreateTime"`
	UpdatedAt         time.Time `gorm:"column:updated_at;type:timestamptz(3);autoUpdateTime"`
}

func (ProviderRegionBinding) TableName() string { return "provider_region_bindings" }

// ProviderInventoryPull is one vendor inventory sync attempt.
type ProviderInventoryPull struct {
	ID          int64      `gorm:"column:id;primaryKey;autoIncrement"`
	ProviderID  string     `gorm:"column:provider_id;type:varchar(32);not null;default:gongji;uniqueIndex:uq_provider_inventory_pull_version,priority:1"`
	Version     int64      `gorm:"column:version;not null;uniqueIndex:uq_provider_inventory_pull_version,priority:2"`
	Status      string     `gorm:"column:status;type:varchar(16);not null"`
	ErrorMessage string    `gorm:"column:error_message;type:text"`
	ObservedAt  time.Time  `gorm:"column:observed_at;type:timestamptz(3);not null"`
	FreshUntil  *time.Time `gorm:"column:fresh_until;type:timestamptz(3)"`
	DeviceCount int        `gorm:"column:device_count;not null;default:0"`
	OfferCount  int        `gorm:"column:offer_count;not null;default:0"`
	CreatedAt   time.Time  `gorm:"column:created_at;type:timestamptz(3);autoCreateTime"`
}

func (ProviderInventoryPull) TableName() string { return "provider_inventory_pulls" }

// ProviderInventoryOffer is one region/SKU row captured in a pull version.
type ProviderInventoryOffer struct {
	ID               int64  `gorm:"column:id;primaryKey;autoIncrement"`
	PullID           int64  `gorm:"column:pull_id;not null;index"`
	PullVersion      int64  `gorm:"column:pull_version;not null;index"`
	ProviderID       string `gorm:"column:provider_id;type:varchar(32);not null;default:gongji"`
	PhysicalRegionID string `gorm:"column:physical_region_id;type:varchar(128);not null"`
	RegionName       string `gorm:"column:region_name;type:varchar(255);not null;default:''"`
	GPUName          string `gorm:"column:gpu_name;type:varchar(128);not null"`
	GPUCount         int    `gorm:"column:gpu_count;not null"`
	DeviceName       string `gorm:"column:device_name;type:varchar(255);not null;default:''"`
	ResourceMark     string `gorm:"column:resource_mark;type:text;not null"`
	Inventory        int    `gorm:"column:inventory;not null;default:0"`
	Price            *int   `gorm:"column:price"`
	DiscountPrice    *int   `gorm:"column:discount_price"`
	GPUMemory        int    `gorm:"column:gpu_memory;not null;default:0"`
	Memory           int    `gorm:"column:memory;not null;default:0"`
	CPUCores         int    `gorm:"column:cpu_cores;not null;default:0"`
}

func (ProviderInventoryOffer) TableName() string { return "provider_inventory_offers" }
