package control

import (
	"context"
	"testing"

	"waverless/pkg/interfaces"
	"waverless/pkg/placement"
	"waverless/pkg/store/mysql"
	storemodel "waverless/pkg/store/mysql/model"
)

func TestBuildActiveSliceFromDecision_AutoWithoutProviderSku(t *testing.T) {
	unit := 1.23
	dec := placement.Decision{
		ProviderID:     "gongji",
		PhysicalRegion: "cn-east-1",
		LogicalAzID:    "az-logical-1",
		ProviderSkuID:  "sku-gpu-a100",
		UnitCost:       unit,
	}
	slice := BuildActiveSliceFromDecision(dec)
	if slice == nil {
		t.Fatal("expected active slice")
	}
	if slice.ProviderSkuID != "sku-gpu-a100" {
		t.Fatalf("providerSkuId %q", slice.ProviderSkuID)
	}
	if slice.UnitCostSnapshot == nil || *slice.UnitCostSnapshot != unit {
		t.Fatalf("unit cost %+v", slice.UnitCostSnapshot)
	}
}

func TestGetRuntimeByExternalID(t *testing.T) {
	ctx := context.Background()
	extID := "compute-base-abc"
	internal := internalEndpointName(extID)
	repo := &stubEndpointRepo{
		byExternal: map[string]*mysql.Endpoint{
			extID: {
				Endpoint:           internal,
				ExternalEndpointID: extID,
				Status:             "active",
				Replicas:           2,
				GPUType:            "A100",
				PlacementMode:      storemodel.PlacementModeAuto,
				BillingMode:        "on_demand",
			},
		},
	}
	slices := &stubSliceRepo{
		active: map[string]*storemodel.EndpointProviderSlice{
			internal: {
				ID:             42,
				Endpoint:       internal,
				ProviderID:     "gongji",
				PhysicalRegion: "cn-east-1",
				LogicalAzID:    "az-1",
				ProviderSkuID:  "sku-1",
				Status:         storemodel.SliceStatusActive,
			},
		},
	}
	provider := &mockDeployProvider{}
	provider.getAppStatus = func(_ context.Context, _ string) (*interfaces.AppStatus, error) {
		return &interfaces.AppStatus{ReadyReplicas: 1, TotalReplicas: 2, Status: "Running"}, nil
	}
	svc := NewService(repo, slices, nil, nil, provider, nil, "https://waverless.example")

	runtime, err := svc.GetRuntime(ctx, extID)
	if err != nil {
		t.Fatalf("GetRuntime: %v", err)
	}
	if runtime.ExternalEndpointID != extID {
		t.Fatalf("external id %q", runtime.ExternalEndpointID)
	}
	if runtime.ActiveSlice == nil || runtime.ActiveSlice.ProviderSkuID != "sku-1" {
		t.Fatalf("active slice %+v", runtime.ActiveSlice)
	}
	if runtime.ReadyReplicas != 1 || runtime.ObservedReplicas != 2 {
		t.Fatalf("replicas ready=%d observed=%d", runtime.ReadyReplicas, runtime.ObservedReplicas)
	}
}

type stubEndpointRepo struct {
	byExternal map[string]*mysql.Endpoint
}

func (s *stubEndpointRepo) GetByExternalEndpointID(_ context.Context, externalID string) (*mysql.Endpoint, error) {
	if s.byExternal == nil {
		return nil, nil
	}
	return s.byExternal[externalID], nil
}

func (s *stubEndpointRepo) Get(_ context.Context, name string) (*mysql.Endpoint, error) {
	for _, ep := range s.byExternal {
		if ep.Endpoint == name {
			return ep, nil
		}
	}
	return nil, nil
}

func (s *stubEndpointRepo) UpdatePlacementFields(context.Context, string, map[string]interface{}) error {
	return nil
}

func (s *stubEndpointRepo) UpdateReplicas(context.Context, string, int) error { return nil }

type stubSliceRepo struct {
	active map[string]*storemodel.EndpointProviderSlice
}

func (s *stubSliceRepo) GetActiveByEndpoint(_ context.Context, endpoint string) (*storemodel.EndpointProviderSlice, error) {
	if s.active == nil {
		return nil, nil
	}
	return s.active[endpoint], nil
}

func (s *stubSliceRepo) ListActiveByEndpoint(ctx context.Context, endpoint string) ([]storemodel.EndpointProviderSlice, error) {
	sl, err := s.GetActiveByEndpoint(ctx, endpoint)
	if sl == nil {
		return nil, err
	}
	return []storemodel.EndpointProviderSlice{*sl}, nil
}

type mockDeployProvider struct {
	getAppStatus func(ctx context.Context, endpoint string) (*interfaces.AppStatus, error)
}

func (m *mockDeployProvider) Deploy(ctx context.Context, req *interfaces.DeployRequest) (*interfaces.DeployResponse, error) {
	return nil, nil
}
func (m *mockDeployProvider) GetApp(ctx context.Context, endpoint string) (*interfaces.AppInfo, error) {
	return nil, nil
}
func (m *mockDeployProvider) ListApps(ctx context.Context) ([]*interfaces.AppInfo, error) { return nil, nil }
func (m *mockDeployProvider) DeleteApp(ctx context.Context, endpoint string) error         { return nil }
func (m *mockDeployProvider) ScaleApp(ctx context.Context, endpoint string, replicas int) error {
	return nil
}
func (m *mockDeployProvider) GetAppStatus(ctx context.Context, endpoint string) (*interfaces.AppStatus, error) {
	if m.getAppStatus != nil {
		return m.getAppStatus(ctx, endpoint)
	}
	return nil, nil
}
func (m *mockDeployProvider) UpdateDeployment(ctx context.Context, req *interfaces.UpdateDeploymentRequest) (*interfaces.DeployResponse, error) {
	return nil, nil
}
func (m *mockDeployProvider) ListSpecs(ctx context.Context) ([]*interfaces.SpecInfo, error) { return nil, nil }
func (m *mockDeployProvider) GetSpec(ctx context.Context, name string) (*interfaces.SpecInfo, error) {
	return nil, nil
}
func (m *mockDeployProvider) GetAppLogs(ctx context.Context, endpoint string, lines int, podName ...string) (string, error) {
	return "", nil
}
func (m *mockDeployProvider) GetPods(ctx context.Context, endpoint string) ([]*interfaces.PodInfo, error) {
	return nil, nil
}
func (m *mockDeployProvider) DescribePod(ctx context.Context, endpoint string, podName string) (*interfaces.PodDetail, error) {
	return nil, nil
}
func (m *mockDeployProvider) GetPodYAML(ctx context.Context, endpoint string, podName string) (string, error) {
	return "", nil
}
func (m *mockDeployProvider) IsPodTerminating(ctx context.Context, podName string) (bool, error) {
	return false, nil
}
func (m *mockDeployProvider) ListPVCs(ctx context.Context) ([]*interfaces.PVCInfo, error) { return nil, nil }
func (m *mockDeployProvider) GetDefaultEnv(ctx context.Context) (map[string]string, error) {
	return nil, nil
}
func (m *mockDeployProvider) PreviewDeploymentYAML(ctx context.Context, req *interfaces.DeployRequest) (string, error) {
	return "", nil
}
func (m *mockDeployProvider) WatchReplicas(ctx context.Context, callback interfaces.ReplicaCallback) error {
	return nil
}
