package hub

import (
	"context"
	"fmt"
	"strings"
	"time"

	"waverless/pkg/interfaces"
	"waverless/pkg/placement"
	"waverless/pkg/store/mysql/model"
)

// DeploymentProvider routes CoreDeploymentProvider calls through active endpoint slices.
type DeploymentProvider struct {
	hub *Hub
}

type pendingSlice struct {
	slice   *model.EndpointProviderSlice
	gpuType string
	mode    string
	billing string
	logical string
}

// NewDeploymentProvider wraps a hub as a DeploymentProvider facade.
func NewDeploymentProvider(h *Hub) *DeploymentProvider {
	return &DeploymentProvider{hub: h}
}

func (d *DeploymentProvider) Deploy(ctx context.Context, req *interfaces.DeployRequest) (*interfaces.DeployResponse, error) {
	if d.hub == nil || req == nil {
		return nil, fmt.Errorf("hub deploy: invalid state")
	}
	mode := strings.ToLower(strings.TrimSpace(req.PlacementMode))
	if mode == "" && len(d.hub.deployments) > 1 {
		mode = model.PlacementModeAuto
	}
	if d.hub.poolingEnabled() && mode == model.PlacementModeAuto && d.hub.placement != nil {
		return d.deployPooled(ctx, req)
	}
	provider, slice, err := d.hub.resolveForDeploy(ctx, req)
	if err != nil {
		return nil, err
	}
	resp, err := provider.Deploy(ctx, req)
	if err != nil {
		return nil, err
	}
	if slice != nil && d.hub.slices != nil {
		_ = d.hub.slices.UpdateFields(ctx, slice.ID, map[string]interface{}{
			"desired_replicas": req.Replicas,
			"provider_ref":     d.extractProviderRef(provider, req.Endpoint),
		})
	}
	return resp, nil
}

func (d *DeploymentProvider) ScaleApp(ctx context.Context, endpoint string, replicas int) error {
	if d.hub.poolingEnabled() && d.hub.slices != nil && d.hub.placement != nil {
		if err := d.ReconcileSlices(ctx, endpoint, replicas); err != nil {
			return err
		}
		if d.hub.endpoints != nil {
			_ = d.hub.endpoints.UpdatePlacementFields(ctx, endpoint, map[string]interface{}{
				"replicas": replicas,
			})
		}
		return nil
	}
	provider, _, err := d.hub.resolveForEndpoint(ctx, endpoint)
	if err != nil {
		return err
	}
	if err := provider.ScaleApp(ctx, endpoint, replicas); err != nil {
		return err
	}
	if d.hub.slices != nil {
		slice, err := d.hub.slices.GetActiveByEndpoint(ctx, endpoint)
		if err == nil && slice != nil {
			now := time.Now()
			_ = d.hub.slices.UpdateFields(ctx, slice.ID, map[string]interface{}{
				"desired_replicas": replicas,
				"last_scale_at":    now,
			})
		}
	}
	return nil
}

func (d *DeploymentProvider) GetApp(ctx context.Context, endpoint string) (*interfaces.AppInfo, error) {
	provider, _, err := d.hub.resolveForEndpoint(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	return provider.GetApp(ctx, endpoint)
}

func (d *DeploymentProvider) ListApps(ctx context.Context) ([]*interfaces.AppInfo, error) {
	seen := map[string]struct{}{}
	var out []*interfaces.AppInfo
	for _, id := range d.hub.Deployments() {
		p, ok := d.hub.Get(id)
		if !ok {
			continue
		}
		apps, err := p.ListApps(ctx)
		if err != nil {
			continue
		}
		for _, app := range apps {
			if app == nil {
				continue
			}
			if _, dup := seen[app.Name]; dup {
				continue
			}
			seen[app.Name] = struct{}{}
			out = append(out, app)
		}
	}
	return out, nil
}

func (d *DeploymentProvider) DeleteApp(ctx context.Context, endpoint string) error {
	provider, _, err := d.hub.resolveForEndpoint(ctx, endpoint)
	if err != nil {
		return err
	}
	return provider.DeleteApp(ctx, endpoint)
}

func (d *DeploymentProvider) GetAppStatus(ctx context.Context, endpoint string) (*interfaces.AppStatus, error) {
	if d.hub.poolingEnabled() && d.hub.slices != nil {
		if st, err := d.aggregateAppStatus(ctx, endpoint); err == nil && st != nil {
			return st, nil
		}
	}
	provider, _, err := d.hub.resolveForEndpoint(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	return provider.GetAppStatus(ctx, endpoint)
}

func (d *DeploymentProvider) aggregateAppStatus(ctx context.Context, endpoint string) (*interfaces.AppStatus, error) {
	slices, err := d.hub.slices.ListActiveByEndpoint(ctx, endpoint)
	if err != nil || len(slices) == 0 {
		return nil, err
	}
	var total, ready int32
	for i := range slices {
		sl := &slices[i]
		provider, ok := d.hub.Get(sl.ProviderID)
		if !ok {
			continue
		}
		key := SliceDeployKey(endpoint, sl, strings.TrimSpace(sl.DeployKey) == endpoint || sl.DeployKey == "")
		st, err := provider.GetAppStatus(ctx, key)
		if err != nil {
			continue
		}
		total += st.TotalReplicas
		ready += st.ReadyReplicas
	}
	return &interfaces.AppStatus{
		Endpoint:          endpoint,
		Status:            "Running",
		ReadyReplicas:     ready,
		AvailableReplicas: ready,
		TotalReplicas:     total,
		Message:           fmt.Sprintf("pooled slices=%d desired_sum=%d", len(slices), sumSliceDesired(slices)),
	}, nil
}

func sumSliceDesired(slices []model.EndpointProviderSlice) int {
	n := 0
	for _, sl := range slices {
		n += sl.DesiredReplicas
	}
	return n
}

func (d *DeploymentProvider) UpdateDeployment(ctx context.Context, req *interfaces.UpdateDeploymentRequest) (*interfaces.DeployResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("update request is nil")
	}
	provider, _, err := d.hub.resolveForEndpoint(ctx, req.Endpoint)
	if err != nil {
		return nil, err
	}
	resp, err := provider.UpdateDeployment(ctx, req)
	if err != nil {
		return nil, err
	}
	if req.Replicas != nil && d.hub.slices != nil {
		slice, sErr := d.hub.slices.GetActiveByEndpoint(ctx, req.Endpoint)
		if sErr == nil && slice != nil {
			_ = d.hub.slices.UpdateFields(ctx, slice.ID, map[string]interface{}{
				"desired_replicas": *req.Replicas,
			})
		}
	}
	return resp, nil
}

// Optional FullDeploymentProvider methods delegate to the first provider that supports them.

func (d *DeploymentProvider) ListSpecs(ctx context.Context) ([]*interfaces.SpecInfo, error) {
	var out []*interfaces.SpecInfo
	for _, id := range d.hub.Deployments() {
		p, ok := d.hub.Get(id)
		if !ok {
			continue
		}
		specs, err := p.ListSpecs(ctx)
		if err != nil {
			continue
		}
		out = append(out, specs...)
	}
	return out, nil
}

func (d *DeploymentProvider) GetSpec(ctx context.Context, specName string) (*interfaces.SpecInfo, error) {
	for _, id := range d.hub.Deployments() {
		p, ok := d.hub.Get(id)
		if !ok {
			continue
		}
		spec, err := p.GetSpec(ctx, specName)
		if err == nil && spec != nil {
			return spec, nil
		}
	}
	return nil, fmt.Errorf("spec %s not found on any provider", specName)
}

func (d *DeploymentProvider) GetAppLogs(ctx context.Context, endpoint string, lines int, podName ...string) (string, error) {
	p, _, err := d.hub.resolveForEndpoint(ctx, endpoint)
	if err != nil {
		return "", err
	}
	return p.GetAppLogs(ctx, endpoint, lines, podName...)
}

func (d *DeploymentProvider) GetPods(ctx context.Context, endpoint string) ([]*interfaces.PodInfo, error) {
	p, _, err := d.hub.resolveForEndpoint(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	return p.GetPods(ctx, endpoint)
}

func (d *DeploymentProvider) DescribePod(ctx context.Context, endpoint, podName string) (*interfaces.PodDetail, error) {
	p, _, err := d.hub.resolveForEndpoint(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	return p.DescribePod(ctx, endpoint, podName)
}

func (d *DeploymentProvider) GetPodYAML(ctx context.Context, endpoint, podName string) (string, error) {
	p, _, err := d.hub.resolveForEndpoint(ctx, endpoint)
	if err != nil {
		return "", err
	}
	return p.GetPodYAML(ctx, endpoint, podName)
}

func (d *DeploymentProvider) IsPodTerminating(ctx context.Context, podName string) (bool, error) {
	for _, id := range d.hub.Deployments() {
		p, ok := d.hub.Get(id)
		if !ok {
			continue
		}
		term, err := p.IsPodTerminating(ctx, podName)
		if err == nil {
			return term, nil
		}
	}
	return false, fmt.Errorf("pod %s not found", podName)
}

func (d *DeploymentProvider) ListPVCs(ctx context.Context) ([]*interfaces.PVCInfo, error) {
	for _, id := range d.hub.Deployments() {
		p, ok := d.hub.Get(id)
		if !ok {
			continue
		}
		pvcs, err := p.ListPVCs(ctx)
		if err == nil {
			return pvcs, nil
		}
	}
	return nil, fmt.Errorf("no provider supports ListPVCs")
}

func (d *DeploymentProvider) GetDefaultEnv(ctx context.Context) (map[string]string, error) {
	for _, id := range d.hub.Deployments() {
		p, ok := d.hub.Get(id)
		if !ok {
			continue
		}
		env, err := p.GetDefaultEnv(ctx)
		if err == nil {
			return env, nil
		}
	}
	return map[string]string{}, nil
}

func (d *DeploymentProvider) PreviewDeploymentYAML(ctx context.Context, req *interfaces.DeployRequest) (string, error) {
	p, err := d.hub.defaultProvider()
	if err != nil {
		return "", err
	}
	return p.PreviewDeploymentYAML(ctx, req)
}

func (d *DeploymentProvider) WatchReplicas(ctx context.Context, callback interfaces.ReplicaCallback) error {
	p, err := d.hub.defaultProvider()
	if err != nil {
		return err
	}
	return p.WatchReplicas(ctx, callback)
}

func (h *Hub) defaultProvider() (interfaces.DeploymentProvider, error) {
	if len(h.deployments) == 0 {
		return nil, fmt.Errorf("no deployments configured")
	}
	p, ok := h.Get(h.deployments[0])
	if !ok {
		return nil, fmt.Errorf("default provider %s is not registered", h.deployments[0])
	}
	return p, nil
}

func (h *Hub) resolveForEndpoint(ctx context.Context, endpoint string) (interfaces.DeploymentProvider, *model.EndpointProviderSlice, error) {
	if h.slices != nil {
		slice, err := h.slices.GetActiveByEndpoint(ctx, endpoint)
		if err == nil && slice != nil {
			p, ok := h.Get(slice.ProviderID)
			if ok {
				return p, slice, nil
			}
		}
	}
	if len(h.deployments) == 1 {
		p, ok := h.Get(h.deployments[0])
		if ok {
			return p, nil, nil
		}
	}
	if h.endpoints != nil {
		ep, err := h.endpoints.Get(ctx, endpoint)
		if err == nil && ep != nil && ep.Labels != nil {
			if pid, _ := ep.Labels["providerId"].(string); pid != "" {
				if p, ok := h.Get(pid); ok {
					return p, nil, nil
				}
			}
		}
	}
	p, err := h.defaultProvider()
	return p, nil, err
}

func (h *Hub) resolveForDeploy(ctx context.Context, req *interfaces.DeployRequest) (interfaces.DeploymentProvider, *model.EndpointProviderSlice, error) {
	if _, slice, err := h.resolveForEndpoint(ctx, req.Endpoint); err == nil && slice != nil {
		p, ok := h.Get(slice.ProviderID)
		if ok {
			return p, slice, nil
		}
	}
	mode := strings.ToLower(strings.TrimSpace(req.PlacementMode))
	if mode == "" {
		if len(h.deployments) > 1 {
			mode = model.PlacementModeAuto
		} else {
			mode = model.PlacementModePin
		}
	}
	if mode != model.PlacementModeAuto || h.placement == nil || h.slices == nil {
		p, err := h.defaultProvider()
		return p, nil, err
	}
	gpuType := req.GpuType
	if gpuType == "" {
		gpuType = req.SpecName
	}
	dec, err := h.placement.Select(ctx, placement.Request{
		GpuType:           gpuType,
		GpuCountPerWorker: req.GpuCount,
		DesiredReplicas:   req.Replicas,
		ImageDigest:       req.ImageDigest,
		BillingMode:       req.BillingMode,
		RegionAllowlist:   req.RegionAllowlist,
		PinnedProviderID:  req.PinnedProviderID,
	}, h.deployments)
	if err != nil {
		return nil, nil, err
	}
	if req.Labels == nil {
		req.Labels = map[string]string{}
	}
	if dec.LogicalAzID != "" {
		req.Labels["logical_az_id"] = dec.LogicalAzID
	}
	if dec.ProviderSkuID != "" {
		req.Labels["provider_sku_id"] = dec.ProviderSkuID
	}
	if dec.ResourceMark != "" {
		req.Labels["gongji_resource_mark"] = dec.ResourceMark
	}
	req.Labels["providerId"] = dec.ProviderID
	p, ok := h.Get(dec.ProviderID)
	if !ok {
		return nil, nil, fmt.Errorf("placement chose unknown provider %s", dec.ProviderID)
	}
	unit := dec.UnitCost
	slice := &model.EndpointProviderSlice{
		Endpoint:         req.Endpoint,
		ProviderID:       dec.ProviderID,
		ProviderSkuID:    dec.ProviderSkuID,
		PhysicalRegion:   dec.PhysicalRegion,
		LogicalAzID:      dec.LogicalAzID,
		DesiredReplicas:  req.Replicas,
		Status:           model.SliceStatusActive,
		UnitCostSnapshot: &unit,
		ResourceMark:     dec.ResourceMark,
		DeployKey:        req.Endpoint,
	}
	h.pending.Store(req.Endpoint, pendingSlice{
		slice:   slice,
		gpuType: gpuType,
		mode:    mode,
		billing: req.BillingMode,
		logical: req.LogicalImage,
	})
	return p, nil, nil
}

// CommitSlice persists a pending slice after endpoint metadata exists.
func (d *DeploymentProvider) CommitSlice(ctx context.Context, endpoint string) error {
	if d.hub == nil || d.hub.slices == nil {
		return nil
	}
	raw, ok := d.hub.pending.LoadAndDelete(endpoint)
	if !ok {
		return nil
	}
	pending, ok := raw.(pendingSlice)
	if !ok || pending.slice == nil {
		return nil
	}
	if err := d.hub.slices.Create(ctx, pending.slice); err != nil {
		return err
	}
	if d.hub.endpoints != nil {
		updates := map[string]interface{}{
			"active_slice_id": pending.slice.ID,
			"gpu_type":        pending.gpuType,
			"placement_mode":  pending.mode,
		}
		if pending.billing != "" {
			updates["billing_mode"] = pending.billing
		}
		if pending.logical != "" {
			updates["logical_image"] = pending.logical
		}
		_ = d.hub.endpoints.UpdatePlacementFields(ctx, endpoint, updates)
	}
	return nil
}

func (d *DeploymentProvider) extractProviderRef(provider interfaces.DeploymentProvider, endpoint string) string {
	app, err := provider.GetApp(context.Background(), endpoint)
	if err != nil || app == nil {
		return ""
	}
	return app.Name
}
