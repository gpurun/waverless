package hub

import (
	"fmt"
	"strings"

	"waverless/pkg/store/mysql/model"
)

// SliceDeployKey returns the vendor task name for a slice (Gongji task_name).
// Primary slice uses endpoint; additional regions use a stable derived key.
func SliceDeployKey(endpoint string, slice *model.EndpointProviderSlice, primary bool) string {
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return ""
	}
	if slice != nil && strings.TrimSpace(slice.DeployKey) != "" {
		return slice.DeployKey
	}
	if primary {
		return endpoint
	}
	az := ""
	if slice != nil {
		az = strings.TrimSpace(slice.LogicalAzID)
		if az == "" {
			az = strings.TrimSpace(slice.PhysicalRegion)
		}
	}
	if az == "" {
		return fmt.Sprintf("%s--s%d", endpoint, slice.ID)
	}
	return fmt.Sprintf("%s--%s", endpoint, az)
}
