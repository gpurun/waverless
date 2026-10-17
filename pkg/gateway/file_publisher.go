package gateway

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// FilePublisher writes Traefik dynamic config to a directory (atomic replace).
type FilePublisher struct {
	dir      string
	filename string
	mu       sync.Mutex
	routes   map[string]EndpointRoute
}

func NewFilePublisher(dir, filename string) *FilePublisher {
	if filename == "" {
		filename = "endpoints.yml"
	}
	return &FilePublisher{
		dir:      dir,
		filename: filename,
		routes:   map[string]EndpointRoute{},
	}
}

func (p *FilePublisher) UpsertEndpoint(_ context.Context, route EndpointRoute) error {
	if p == nil {
		return nil
	}
	key := route.RouteID
	if key == "" {
		key = route.PublicHost
	}
	p.mu.Lock()
	p.routes[key] = route
	p.mu.Unlock()
	return p.flush()
}

func (p *FilePublisher) RemoveEndpoint(_ context.Context, routeID string) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	delete(p.routes, routeID)
	p.mu.Unlock()
	return p.flush()
}

func (p *FilePublisher) SyncEndpointBackends(ctx context.Context, endpoint string, backendURLs []string) error {
	if p == nil {
		return nil
	}
	p.mu.Lock()
	r, ok := p.routes[endpoint]
	if !ok {
		p.mu.Unlock()
		return nil
	}
	r.Backends = append([]string(nil), backendURLs...)
	p.routes[endpoint] = r
	p.mu.Unlock()
	return p.flush()
}

func (p *FilePublisher) flush() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.dir == "" {
		return fmt.Errorf("gateway dynamic dir is not configured")
	}
	list := make([]EndpointRoute, 0, len(p.routes))
	for _, r := range p.routes {
		list = append(list, r)
	}
	body := RenderDynamicFile(list)
	if err := os.MkdirAll(p.dir, 0o755); err != nil {
		return err
	}
	target := filepath.Join(p.dir, p.filename)
	tmp := target + ".tmp"
	if err := os.WriteFile(tmp, []byte(body), 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, target)
}
