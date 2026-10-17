package model

import "time"

const (
	SliceStatusActive   = "active"
	SliceStatusDraining = "draining"
	SliceStatusFailed   = "failed"
	SliceStatusRetired  = "retired"

	PlacementModeAuto = "auto"
	PlacementModePin  = "pin"
)

// EndpointProviderSlice is one provider pool attached to an endpoint.
type EndpointProviderSlice struct {
	ID               int64      `gorm:"column:id;primaryKey;autoIncrement"`
	Endpoint         string     `gorm:"column:endpoint;type:varchar(255);not null;index"`
	ProviderID       string     `gorm:"column:provider_id;type:varchar(32);not null"`
	ProviderRef      string     `gorm:"column:provider_ref;type:varchar(255);not null;default:''"`
	ProviderSkuID    string     `gorm:"column:provider_sku_id;type:varchar(128);not null;default:''"`
	PhysicalRegion   string     `gorm:"column:physical_region;type:varchar(128);not null;default:''"`
	LogicalAzID      string     `gorm:"column:logical_az_id;type:varchar(64);not null;default:''"`
	DesiredReplicas  int        `gorm:"column:desired_replicas;not null;default:0"`
	ReadyReplicas    int        `gorm:"column:ready_replicas;not null;default:0"`
	Status           string     `gorm:"column:status;type:varchar(32);not null;default:active"`
	UnitCostSnapshot *float64   `gorm:"column:unit_cost_snapshot;type:numeric(15,4)"`
	Currency         string     `gorm:"column:currency;type:varchar(8);not null;default:USD"`
	ImageBindingID   *int64     `gorm:"column:image_binding_id"`
	ResourceMark     string     `gorm:"column:resource_mark;type:text;not null;default:''"`
	DeployKey        string     `gorm:"column:deploy_key;type:varchar(255);not null;default:''"`
	LastScaleAt      *time.Time `gorm:"column:last_scale_at;type:timestamptz(3)"`
	CreatedAt        time.Time  `gorm:"column:created_at;type:timestamptz(3);autoCreateTime"`
	UpdatedAt        time.Time  `gorm:"column:updated_at;type:timestamptz(3);autoUpdateTime"`
}

func (EndpointProviderSlice) TableName() string { return "endpoint_provider_slices" }

// ProviderOffer is a cached inventory/price row for Placement.
type ProviderOffer struct {
	ID           int64     `gorm:"column:id;primaryKey;autoIncrement"`
	ProviderID   string    `gorm:"column:provider_id;type:varchar(32);not null;uniqueIndex:uq_provider_offers_row,priority:1"`
	SkuID        string    `gorm:"column:sku_id;type:varchar(128);not null;uniqueIndex:uq_provider_offers_row,priority:2"`
	GPUType      string    `gorm:"column:gpu_type;type:varchar(64);not null;index"`
	GPUCount     int       `gorm:"column:gpu_count;not null;uniqueIndex:uq_provider_offers_row,priority:3"`
	Region       string    `gorm:"column:region;type:varchar(128);not null;default:'';uniqueIndex:uq_provider_offers_row,priority:4"`
	LogicalAzID  string    `gorm:"column:logical_az_id;type:varchar(64);not null;default:''"`
	BillingMode  string    `gorm:"column:billing_mode;type:varchar(32);not null;default:on_demand;uniqueIndex:uq_provider_offers_row,priority:5"`
	UnitPrice    float64   `gorm:"column:unit_price;type:numeric(15,4);not null;default:0"`
	Currency     string    `gorm:"column:currency;type:varchar(8);not null;default:USD"`
	AvailableCap int       `gorm:"column:available_cap;not null;default:0"`
	ResourceMark string    `gorm:"column:resource_mark;type:text;not null;default:''"`
	SyncedAt     time.Time `gorm:"column:synced_at;type:timestamptz(3);not null;autoUpdateTime"`
}

func (ProviderOffer) TableName() string { return "provider_offers" }

// ImageBinding tracks per-provider image materialization state (M3-ready).
type ImageBinding struct {
	ID                int64      `gorm:"column:id;primaryKey;autoIncrement"`
	Digest            string     `gorm:"column:digest;type:varchar(255);not null;uniqueIndex:uq_image_bindings_key,priority:1"`
	ProviderID        string     `gorm:"column:provider_id;type:varchar(32);not null;uniqueIndex:uq_image_bindings_key,priority:2"`
	PhysicalRegion    string     `gorm:"column:physical_region;type:varchar(128);not null;uniqueIndex:uq_image_bindings_key,priority:3"`
	VendorRef         string     `gorm:"column:vendor_ref;type:text;not null;default:''"`
	Ready             bool       `gorm:"column:ready;not null;default:false"`
	MaterializeState  string     `gorm:"column:materialize_state;type:varchar(32);not null;default:none"`
	OpenCostEstimate  *float64   `gorm:"column:open_cost_estimate;type:numeric(15,4)"`
	UpdatedAt         time.Time  `gorm:"column:updated_at;type:timestamptz(3);autoUpdateTime"`
}

func (ImageBinding) TableName() string { return "image_bindings" }
