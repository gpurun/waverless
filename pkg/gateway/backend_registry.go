package gateway

import "context"

// WorkerBackendEvent notifies Gateway Controller or Traefik writers (§8.1).
type WorkerBackendEvent struct {
	Type                 string
	Endpoint             string
	ExternalEndpointID   string
	WorkerID             string
	BackendURL           string
	SliceID              int64
}

const (
	EventWorkerReady    = "WorkerReady"
	EventWorkerRemoved  = "WorkerRemoved"
	EventWorkerDraining = "WorkerDraining"
)

// BackendRegistry updates HTTP backends for http_service endpoints.
type BackendRegistry interface {
	Publish(ctx context.Context, ev WorkerBackendEvent) error
	SyncEndpointBackends(ctx context.Context, endpoint string, backendURLs []string) error
}

type noopRegistry struct{}

func (noopRegistry) Publish(context.Context, WorkerBackendEvent) error { return nil }

func (noopRegistry) SyncEndpointBackends(context.Context, string, []string) error { return nil }

// DefaultRegistry is used until Traefik / Gateway Controller wiring is configured.
var DefaultRegistry BackendRegistry = noopRegistry{}
