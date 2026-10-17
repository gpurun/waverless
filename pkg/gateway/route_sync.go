package gateway

import (
	"context"
	"fmt"
	"strings"

	"waverless/pkg/config"
	"waverless/pkg/provider/gongji"
	"waverless/pkg/store/mysql"
	"waverless/pkg/store/mysql/model"
)

// RouteSync publishes Traefik pools from Gongji point URLs.
type RouteSync struct {
	endpoints *mysql.EndpointRepository
	slices    *mysql.ProviderSliceRepository
	gongji    *gongji.GongjiDeploymentProvider
	cfg       config.GatewayConfig
	pub       *FilePublisher
}

func NewRouteSync(
	endpoints *mysql.EndpointRepository,
	slices *mysql.ProviderSliceRepository,
	gp *gongji.GongjiDeploymentProvider,
	cfg config.GatewayConfig,
	pub *FilePublisher,
) *RouteSync {
	return &RouteSync{
		endpoints: endpoints,
		slices:    slices,
		gongji:    gp,
		cfg:       cfg,
		pub:       pub,
	}
}

func (s *RouteSync) SyncEndpoint(ctx context.Context, endpoint string) error {
	if s == nil || s.pub == nil || s.endpoints == nil {
		return nil
	}
	ep, err := s.endpoints.Get(ctx, endpoint)
	if err != nil {
		return err
	}
	if strings.ToLower(strings.TrimSpace(ep.EndpointMode)) == "queue_worker" {
		return nil
	}
	host := strings.TrimSpace(ep.PublicHost)
	if host == "" {
		host = publicHostFromEndpoint(ep)
	}
	if host == "" {
		return nil
	}
	backends, err := s.collectBackends(ctx, endpoint, ep.GpuCount)
	if err != nil {
		return err
	}
	if len(backends) == 0 {
		return s.pub.RemoveEndpoint(ctx, endpoint)
	}
	route := EndpointRoute{
		RouteID:            endpoint,
		PublicHost:         host,
		Backends:           backends,
		HealthCheckPath:    s.cfg.HealthCheckPath,
		EdgeAuthMiddleware: s.cfg.EdgeAuthMiddleware,
		CertResolver:       "letsencrypt",
	}
	if err := s.pub.UpsertEndpoint(ctx, route); err != nil {
		return err
	}
	_ = s.endpoints.UpdatePlacementFields(ctx, endpoint, map[string]interface{}{
		"gateway_status": "active",
	})
	return nil
}

func publicHostFromEndpoint(ep *model.Endpoint) string {
	if ep == nil {
		return ""
	}
	if h := strings.TrimSpace(ep.PublicHost); h != "" {
		return h
	}
	ext := strings.TrimSpace(ep.ExternalEndpointID)
	if ext != "" {
		return ext + "." + strings.TrimSpace(defaultDomain())
	}
	return ""
}

func defaultDomain() string {
	if config.GlobalConfig != nil {
		return config.GlobalConfig.ResolvedGateway().PublicBaseDomain
	}
	return "rungpu.cn"
}

func (s *RouteSync) collectBackends(ctx context.Context, endpoint string, _ int) ([]string, error) {
	if s.gongji == nil {
		return nil, fmt.Errorf("gongji provider is not registered")
	}
	gp := s.gongji
	deployKeys := []string{endpoint}
	if s.slices != nil {
		slices, err := s.slices.ListActiveByEndpoint(ctx, endpoint)
		if err == nil && len(slices) > 0 {
			deployKeys = deployKeys[:0]
			for i := range slices {
				sl := &slices[i]
				key := sliceDeployKey(endpoint, sl)
				if key != "" {
					deployKeys = append(deployKeys, key)
				}
			}
		}
	}
	seen := map[string]struct{}{}
	var out []string
	for _, key := range deployKeys {
		workers, err := gp.ListWorkers(ctx, key)
		if err != nil {
			continue
		}
		for _, w := range workers {
			if w == nil {
				continue
			}
			u := strings.TrimSpace(w.IP)
			if u == "" && w.Labels != nil {
				u = strings.TrimSpace(w.Labels["backend_url"])
			}
			if u == "" || !isRunningPhase(w.Phase, w.Status) {
				continue
			}
			if _, dup := seen[u]; dup {
				continue
			}
			seen[u] = struct{}{}
			out = append(out, u)
		}
	}
	return out, nil
}

func isRunningPhase(phase, status string) bool {
	p := strings.ToLower(strings.TrimSpace(phase))
	s := strings.ToLower(strings.TrimSpace(status))
	if p == "running" || s == "running" || s == "ready" {
		return true
	}
	return p != "failed" && p != "terminated" && s != "terminated"
}

func sliceDeployKey(endpoint string, sl *model.EndpointProviderSlice) string {
	if sl == nil {
		return endpoint
	}
	if dk := strings.TrimSpace(sl.DeployKey); dk != "" {
		return dk
	}
	if sl.LogicalAzID == "" && sl.PhysicalRegion == "" {
		return endpoint
	}
	az := strings.TrimSpace(sl.LogicalAzID)
	if az == "" {
		az = strings.TrimSpace(sl.PhysicalRegion)
	}
	if az == "" {
		return fmt.Sprintf("%s--s%d", endpoint, sl.ID)
	}
	return fmt.Sprintf("%s--%s", endpoint, az)
}
