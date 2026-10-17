package hub

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"waverless/pkg/gateway"
	"waverless/pkg/interfaces"
	"waverless/pkg/logger"
	"waverless/pkg/placement"
	"waverless/pkg/store/mysql/model"
)

type taskIDBinder interface {
	BindTaskID(endpoint string, taskID int64)
}

func (h *Hub) poolingEnabled() bool {
	if h == nil || h.cfg == nil {
		return false
	}
	return h.cfg.Placement.Pooling.Enabled
}

func (h *Hub) maxSlices() int {
	if h.cfg != nil && h.cfg.Placement.Pooling.MaxSlicesPerEndpoint > 0 {
		return h.cfg.Placement.Pooling.MaxSlicesPerEndpoint
	}
	return 8
}

// ReconcileSlices sets endpoint-level desired workers across active slices (M2a).
func (d *DeploymentProvider) ReconcileSlices(ctx context.Context, endpoint string, desiredReplicas int) error {
	if d.hub == nil || d.hub.slices == nil {
		return fmt.Errorf("hub slice repository is not configured")
	}
	if desiredReplicas < 0 {
		return fmt.Errorf("desired replicas must be >= 0")
	}

	slices, err := d.hub.slices.ListActiveByEndpoint(ctx, endpoint)
	if err != nil {
		return err
	}
	currentSum := 0
	for _, sl := range slices {
		currentSum += sl.DesiredReplicas
	}

	if desiredReplicas > currentSum {
		if err := d.scaleUpSlices(ctx, endpoint, slices, desiredReplicas-currentSum); err != nil {
			return err
		}
	} else if desiredReplicas < currentSum {
		if err := d.scaleDownSlices(ctx, endpoint, slices, currentSum-desiredReplicas); err != nil {
			return err
		}
	}

	if err := d.syncSliceScales(ctx, endpoint, desiredReplicas); err != nil {
		return err
	}
	_ = gateway.DefaultRegistry.SyncEndpointBackends(ctx, endpoint, nil)
	return nil
}

func (d *DeploymentProvider) scaleDownSlices(ctx context.Context, endpoint string, slices []model.EndpointProviderSlice, excess int) error {
	sort.Slice(slices, func(i, j int) bool {
		ci := unitCost(slices[i])
		cj := unitCost(slices[j])
		if ci != cj {
			return ci > cj
		}
		return slices[i].ID > slices[j].ID
	})
	for i := range slices {
		if excess <= 0 {
			break
		}
		sl := &slices[i]
		if sl.DesiredReplicas <= 0 {
			continue
		}
		cut := 1
		if cut > excess {
			cut = excess
		}
		if sl.DesiredReplicas < cut {
			cut = sl.DesiredReplicas
		}
		newDesired := sl.DesiredReplicas - cut
		if err := d.hub.slices.UpdateFields(ctx, sl.ID, map[string]interface{}{
			"desired_replicas": newDesired,
			"last_scale_at":    time.Now(),
		}); err != nil {
			return err
		}
		sl.DesiredReplicas = newDesired
		excess -= cut
	}
	if excess > 0 {
		logger.WarnCtx(ctx, "reconcile scale-down shortfall endpoint=%s remaining=%d", endpoint, excess)
	}
	return nil
}

func unitCost(sl model.EndpointProviderSlice) float64 {
	if sl.UnitCostSnapshot != nil {
		return *sl.UnitCostSnapshot
	}
	return 0
}

func (d *DeploymentProvider) scaleUpSlices(ctx context.Context, endpoint string, slices []model.EndpointProviderSlice, delta int) error {
	if delta <= 0 {
		return nil
	}
	req, err := d.hub.placementRequestForEndpoint(ctx, endpoint)
	if err != nil {
		return err
	}
	reserved := reservationFromSlices(slices)
	allocs, err := d.hub.placement.Allocate(ctx, req, delta, d.hub.deployments, reserved)
	if err != nil {
		return err
	}
	if len(allocs) == 0 {
		logger.WarnCtx(ctx, "allocate returned no rows for endpoint=%s delta=%d", endpoint, delta)
		return nil
	}
	activeCount := len(slices)
	hadNoSlices := len(slices) == 0
	for _, alloc := range allocs {
		if activeCount >= d.hub.maxSlices() {
			logger.WarnCtx(ctx, "max_slices_per_endpoint reached for %s", endpoint)
			break
		}
		sl, err := d.hub.slices.FindActiveSlice(ctx, endpoint, alloc.ProviderID, alloc.PhysicalRegion, alloc.ProviderSkuID)
		if err == nil && sl != nil {
			newDesired := sl.DesiredReplicas + alloc.Replicas
			if err := d.hub.slices.UpdateFields(ctx, sl.ID, map[string]interface{}{
				"desired_replicas": newDesired,
				"last_scale_at":    time.Now(),
			}); err != nil {
				return err
			}
			continue
		}
		primary := hadNoSlices && activeCount == 0
		if err := d.createSliceFromAllocation(ctx, endpoint, req, alloc, primary); err != nil {
			return err
		}
		activeCount++
	}
	return nil
}

func reservationFromSlices(slices []model.EndpointProviderSlice) map[string]int {
	out := map[string]int{}
	for _, sl := range slices {
		key := placement.OfferReservationKey(sl.ProviderID, sl.PhysicalRegion, sl.ProviderSkuID)
		out[key] += sl.DesiredReplicas
	}
	return out
}

func (d *DeploymentProvider) syncSliceScales(ctx context.Context, endpoint string, endpointDesired int) error {
	slices, err := d.hub.slices.ListActiveByEndpoint(ctx, endpoint)
	if err != nil {
		return err
	}
	sum := 0
	for _, sl := range slices {
		sum += sl.DesiredReplicas
		if err := d.scaleOneSlice(ctx, endpoint, &sl); err != nil {
			return err
		}
	}
	if sum != endpointDesired {
		logger.InfoCtx(ctx, "reconcile endpoint=%s desired=%d slice_sum=%d (capacity shortfall possible)", endpoint, endpointDesired, sum)
	}
	return nil
}

func (d *DeploymentProvider) scaleOneSlice(ctx context.Context, endpoint string, sl *model.EndpointProviderSlice) error {
	provider, ok := d.hub.Get(sl.ProviderID)
	if !ok {
		return fmt.Errorf("provider %s not registered", sl.ProviderID)
	}
	deployKey := SliceDeployKey(endpoint, sl, strings.TrimSpace(sl.DeployKey) == endpoint || sl.DeployKey == "")
	if deployKey == "" {
		deployKey = endpoint
	}
	if err := provider.ScaleApp(ctx, deployKey, sl.DesiredReplicas); err != nil {
		return fmt.Errorf("scale slice %d (%s): %w", sl.ID, deployKey, err)
	}
	now := time.Now()
	_ = d.hub.slices.UpdateFields(ctx, sl.ID, map[string]interface{}{
		"last_scale_at": now,
	})
	return nil
}

func (h *Hub) placementRequestForEndpoint(ctx context.Context, endpoint string) (placement.Request, error) {
	req := placement.Request{BillingMode: "on_demand"}
	if h.endpoints == nil {
		return req, fmt.Errorf("endpoint repository is not configured")
	}
	ep, err := h.endpoints.Get(ctx, endpoint)
	if err != nil {
		return req, err
	}
	req.GpuType = ep.GPUType
	if req.GpuType == "" {
		req.GpuType = ep.SpecName
	}
	req.GpuCountPerWorker = ep.GpuCount
	if ep.BillingMode != "" {
		req.BillingMode = ep.BillingMode
	}
	req.ImageDigest = ep.ImageDigest
	if len(ep.RegionAllowlist) > 0 {
		req.RegionAllowlist = []string(ep.RegionAllowlist)
	}
	return req, nil
}

func (d *DeploymentProvider) createSliceFromAllocation(ctx context.Context, endpoint string, base placement.Request, alloc placement.Allocation, primary bool) error {
	if d.hub.slices == nil {
		return fmt.Errorf("slice repository missing")
	}
	unit := alloc.UnitCost
	deployKey := endpoint
	if !primary {
		az := alloc.LogicalAzID
		if az == "" {
			az = alloc.PhysicalRegion
		}
		deployKey = fmt.Sprintf("%s--%s", endpoint, az)
	}
	slice := &model.EndpointProviderSlice{
		Endpoint:         endpoint,
		ProviderID:       alloc.ProviderID,
		ProviderSkuID:    alloc.ProviderSkuID,
		PhysicalRegion:   alloc.PhysicalRegion,
		LogicalAzID:      alloc.LogicalAzID,
		DesiredReplicas:  alloc.Replicas,
		Status:           model.SliceStatusActive,
		UnitCostSnapshot: &unit,
		ResourceMark:     alloc.ResourceMark,
		DeployKey:        deployKey,
	}
	if err := d.hub.slices.Create(ctx, slice); err != nil {
		return err
	}
	provider, ok := d.hub.Get(alloc.ProviderID)
	if !ok {
		return fmt.Errorf("provider %s not registered", alloc.ProviderID)
	}
	deployReq, err := d.hub.buildDeployRequestForSlice(ctx, endpoint, slice, base, alloc)
	if err != nil {
		return err
	}
	deployReq.Endpoint = deployKey
	deployReq.Replicas = alloc.Replicas
	resp, err := provider.Deploy(ctx, deployReq)
	if err != nil {
		return err
	}
	ref := d.extractProviderRef(provider, deployKey)
	if ref == "" && resp != nil {
		ref = parseTaskIDFromMessage(resp.Message)
	}
	if ref != "" {
		_ = d.hub.slices.UpdateFields(ctx, slice.ID, map[string]interface{}{"provider_ref": ref})
		if binder, ok := provider.(taskIDBinder); ok {
			if id, err := strconv.ParseInt(ref, 10, 64); err == nil {
				binder.BindTaskID(deployKey, id)
			}
		}
	}
	return nil
}

func parseTaskIDFromMessage(msg string) string {
	// "created gongji task_id=12345"
	const needle = "task_id="
	if i := strings.Index(msg, needle); i >= 0 {
		rest := msg[i+len(needle):]
		for j, c := range rest {
			if c < '0' || c > '9' {
				rest = rest[:j]
				break
			}
		}
		return rest
	}
	return ""
}

func (h *Hub) buildDeployRequestForSlice(ctx context.Context, endpoint string, slice *model.EndpointProviderSlice, base placement.Request, alloc placement.Allocation) (*interfaces.DeployRequest, error) {
	if h.endpoints == nil {
		return nil, fmt.Errorf("endpoint repository missing")
	}
	ep, err := h.endpoints.Get(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	labels := map[string]string{"providerId": alloc.ProviderID}
	if alloc.LogicalAzID != "" {
		labels["logical_az_id"] = alloc.LogicalAzID
	}
	if alloc.ProviderSkuID != "" {
		labels["provider_sku_id"] = alloc.ProviderSkuID
	}
	if alloc.ResourceMark != "" {
		labels["gongji_resource_mark"] = alloc.ResourceMark
	}
	env := map[string]string{}
	if ep.Env != nil {
		for k, v := range ep.Env {
			if s, ok := v.(string); ok {
				env[k] = s
			}
		}
	}
	return &interfaces.DeployRequest{
		Endpoint:        endpoint,
		SpecName:        ep.SpecName,
		Image:           ep.Image,
		GpuCount:        ep.GpuCount,
		GpuType:         base.GpuType,
		BillingMode:     base.BillingMode,
		ImageDigest:     base.ImageDigest,
		RegionAllowlist: base.RegionAllowlist,
		Env:             env,
		Labels:          labels,
	}, nil
}

func (d *DeploymentProvider) deployPooled(ctx context.Context, req *interfaces.DeployRequest) (*interfaces.DeployResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("deploy request is nil")
	}
	gpuType := req.GpuType
	if gpuType == "" {
		gpuType = req.SpecName
	}
	placementReq := placement.Request{
		GpuType:           gpuType,
		GpuCountPerWorker: req.GpuCount,
		DesiredReplicas:   req.Replicas,
		ImageDigest:       req.ImageDigest,
		BillingMode:       req.BillingMode,
		RegionAllowlist:   req.RegionAllowlist,
		PinnedProviderID:  req.PinnedProviderID,
	}
	allocs, err := d.hub.placement.Allocate(ctx, placementReq, req.Replicas, d.hub.deployments, map[string]int{})
	if err != nil {
		return nil, err
	}
	if len(allocs) > d.hub.maxSlices() {
		return nil, fmt.Errorf("allocation requires %d slices; max_slices_per_endpoint=%d", len(allocs), d.hub.maxSlices())
	}
	var last *interfaces.DeployResponse
	for i, alloc := range allocs {
		if err := d.createSliceFromAllocation(ctx, req.Endpoint, placementReq, alloc, i == 0); err != nil {
			return last, err
		}
	}
	if d.hub.endpoints != nil {
		updates := map[string]interface{}{
			"gpu_type":       gpuType,
			"placement_mode": model.PlacementModeAuto,
		}
		if req.BillingMode != "" {
			updates["billing_mode"] = req.BillingMode
		}
		if req.LogicalImage != "" {
			updates["logical_image"] = req.LogicalImage
		}
		mode := strings.TrimSpace(req.EndpointMode)
		if mode == "" && req.Labels != nil {
			mode = strings.TrimSpace(req.Labels["endpoint_mode"])
		}
		if mode == "" {
			mode = "http_service"
		}
		updates["endpoint_mode"] = mode
		ext := strings.TrimSpace(req.ExternalEndpointID)
		if ext == "" && req.Labels != nil {
			ext = strings.TrimSpace(req.Labels["external_endpoint_id"])
		}
		if ext != "" {
			updates["external_endpoint_id"] = ext
		}
		_ = d.hub.endpoints.UpdatePlacementFields(ctx, req.Endpoint, updates)
	}
	return &interfaces.DeployResponse{
		Endpoint:  req.Endpoint,
		Message:   fmt.Sprintf("pooled deploy: %d slices", len(allocs)),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}
