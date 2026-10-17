package hub

import (
	"fmt"
	"strings"
	"sync"

	"waverless/pkg/config"
	"waverless/pkg/interfaces"
	"waverless/pkg/placement"
	"waverless/pkg/store/mysql"
)

// ProviderInfo describes one registered deployment backend.
type ProviderInfo struct {
	ID      string `json:"id"`
	Enabled bool   `json:"enabled"`
}

// Hub holds multiple CoreDeploymentProvider instances and routes by endpoint slice.
type Hub struct {
	mu          sync.RWMutex
	providers   map[string]interfaces.DeploymentProvider
	deployments []string
	placement   *placement.Service
	slices      *mysql.ProviderSliceRepository
	endpoints   *mysql.EndpointRepository
	cfg         *config.Config
	pending     sync.Map
}

// New creates a ProviderHub. At least one provider must be registered before use.
func New(cfg *config.Config, slices *mysql.ProviderSliceRepository, endpoints *mysql.EndpointRepository, placementSvc *placement.Service) *Hub {
	return &Hub{
		providers:   map[string]interfaces.DeploymentProvider{},
		deployments: cfg.EnabledDeployments(),
		placement:   placementSvc,
		slices:      slices,
		endpoints:   endpoints,
		cfg:         cfg,
	}
}

// Register adds a deployment provider instance.
func (h *Hub) Register(id string, provider interfaces.DeploymentProvider) {
	if h == nil || provider == nil {
		return
	}
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "suanli" {
		id = "gongji"
	}
	h.mu.Lock()
	defer h.mu.Unlock()
	h.providers[id] = provider
}

// Get returns a provider by id.
func (h *Hub) Get(id string) (interfaces.DeploymentProvider, bool) {
	id = strings.ToLower(strings.TrimSpace(id))
	if id == "suanli" {
		id = "gongji"
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	p, ok := h.providers[id]
	return p, ok
}

// List returns registered provider metadata.
func (h *Hub) List() []ProviderInfo {
	h.mu.RLock()
	defer h.mu.RUnlock()
	out := make([]ProviderInfo, 0, len(h.providers))
	for id := range h.providers {
		out = append(out, ProviderInfo{ID: id, Enabled: true})
	}
	return out
}

// Deployments returns enabled deployment ids from config.
func (h *Hub) Deployments() []string {
	if h == nil {
		return nil
	}
	return append([]string(nil), h.deployments...)
}

// Placement exposes the placement service.
func (h *Hub) Placement() *placement.Service {
	if h == nil {
		return nil
	}
	return h.placement
}

// EnsureRegistered validates that configured deployments were wired.
func (h *Hub) EnsureRegistered() error {
	if h == nil {
		return fmt.Errorf("provider hub is nil")
	}
	h.mu.RLock()
	defer h.mu.RUnlock()
	if len(h.providers) == 0 {
		return fmt.Errorf("no deployment providers registered in hub")
	}
	for _, id := range h.deployments {
		if _, ok := h.providers[id]; !ok {
			return fmt.Errorf("deployment provider %q is enabled but not registered", id)
		}
	}
	return nil
}
