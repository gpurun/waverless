package placement

// Request is input to automatic provider selection.
type Request struct {
	GpuType           string
	GpuCountPerWorker int
	DesiredReplicas   int
	ImageDigest       string
	BillingMode       string
	RegionAllowlist   []string
	PinnedProviderID  string
	ExcludeProviders  []string
}

// Decision is the chosen provider/region/SKU for one new slice.
type Decision struct {
	ProviderID     string  `json:"providerId"`
	PhysicalRegion string  `json:"physicalRegion"`
	ProviderSkuID  string  `json:"providerSkuId"`
	LogicalAzID    string  `json:"logicalAzId,omitempty"`
	ResourceMark   string  `json:"resourceMark,omitempty"`
	UnitCost       float64 `json:"unitCost"`
	OpenCost       float64 `json:"openCost"`
	Score          float64 `json:"score"`
	ImageBindingID *int64  `json:"imageBindingId,omitempty"`
}

// Allocation assigns a worker count to one offer row (M2 pooling).
type Allocation struct {
	Decision
	Replicas int `json:"replicas"`
}
