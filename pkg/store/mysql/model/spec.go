package model

import "time"

// Spec database model for resource_specs.
type Spec struct {
	ID          int64  `gorm:"primaryKey;autoIncrement" json:"id"`
	Name        string `gorm:"column:name;type:varchar(100);not null;uniqueIndex:idx_spec_name_unique" json:"name"`
	DisplayName string `gorm:"column:display_name;type:varchar(255);not null" json:"display_name"`
	Category    string `gorm:"column:category;type:varchar(50);not null;index:idx_category" json:"category"` // cpu, gpu

	// Resources
	CPU              string `gorm:"column:cpu;type:varchar(50)" json:"cpu"`
	Memory           string `gorm:"column:memory;type:varchar(50);not null" json:"memory"`
	GPU              string `gorm:"column:gpu;type:varchar(50)" json:"gpu"`
	GPUType          string `gorm:"column:gpu_type;type:varchar(100)" json:"gpu_type"`
	EphemeralStorage string `gorm:"column:ephemeral_storage;type:varchar(50);not null" json:"ephemeral_storage"`
	ShmSize          string `gorm:"column:shm_size;type:varchar(50)" json:"shm_size"`                                       // Shared memory size (e.g., "1Gi", "512Mi")
	ResourceType     string `gorm:"column:resource_type;type:varchar(20);not null;default:serverless" json:"resource_type"` // fixed, serverless

	// Platform-specific configurations (JSON)
	Platforms JSONMap `gorm:"column:platforms;type:jsonb" json:"platforms"`

	// Metadata
	Status    string    `gorm:"column:status;type:varchar(50);not null;default:active;index:idx_status" json:"status"` // active, inactive, deprecated
	CreatedAt time.Time `gorm:"column:created_at;type:timestamptz(3);not null;default:CURRENT_TIMESTAMP(3);index:idx_created_at" json:"created_at"`
	UpdatedAt time.Time `gorm:"column:updated_at;type:timestamptz(3);not null;default:CURRENT_TIMESTAMP(3)" json:"updated_at"`
}

// TableName specifies the table name for Spec
func (Spec) TableName() string {
	return "resource_specs"
}
