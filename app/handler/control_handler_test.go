package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"waverless/app/middleware"
	controlsvc "waverless/internal/service/control"
	"waverless/pkg/config"
	"waverless/pkg/store/mysql"
	storemodel "waverless/pkg/store/mysql/model"

	"github.com/gin-gonic/gin"
)

func TestControlHandler_RuntimeRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config.GlobalConfig = &config.Config{Server: config.ServerConfig{ServiceAPIKey: "test-key"}}

	extID := "ext-runtime-1"
	svc := controlsvc.NewService(&controlTestEndpointRepo{
		byExternal: map[string]*mysql.Endpoint{
			extID: {Endpoint: "ep-1", ExternalEndpointID: extID, Status: "active", Replicas: 1},
		},
	}, &controlTestSliceRepo{}, nil, nil, nil, nil, "")

	h := NewControlHandler(svc)
	r := gin.New()
	r.Use(middleware.ControlAuthMiddleware())
	r.GET("/control/v1/endpoints/:externalEndpointId/runtime", h.GetEndpointRuntime)

	req := httptest.NewRequest(http.MethodGet, "/control/v1/endpoints/"+extID+"/runtime", nil)
	req.Header.Set("Authorization", "Bearer test-key")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 got %d %s", rec.Code, rec.Body.String())
	}
	var body map[string]interface{}
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if body["externalEndpointId"] != extID {
		t.Fatalf("body %v", body)
	}
}

func TestControlHandler_AuthRejected(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config.GlobalConfig = &config.Config{Server: config.ServerConfig{ServiceAPIKey: "test-key"}}
	h := NewControlHandler(controlsvc.NewService(nil, nil, nil, nil, nil, nil, ""))
	r := gin.New()
	r.Use(middleware.ControlAuthMiddleware())
	r.GET("/control/v1/endpoints/:externalEndpointId/runtime", h.GetEndpointRuntime)

	req := httptest.NewRequest(http.MethodGet, "/control/v1/endpoints/x/runtime", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 got %d", rec.Code)
	}
}

type controlTestEndpointRepo struct {
	byExternal map[string]*mysql.Endpoint
}

func (s *controlTestEndpointRepo) GetByExternalEndpointID(_ context.Context, externalID string) (*mysql.Endpoint, error) {
	if s.byExternal == nil {
		return nil, nil
	}
	return s.byExternal[externalID], nil
}

func (s *controlTestEndpointRepo) Get(_ context.Context, name string) (*mysql.Endpoint, error) {
	for _, ep := range s.byExternal {
		if ep.Endpoint == name {
			return ep, nil
		}
	}
	return nil, nil
}

func (s *controlTestEndpointRepo) UpdatePlacementFields(context.Context, string, map[string]interface{}) error {
	return nil
}

func (s *controlTestEndpointRepo) UpdateReplicas(context.Context, string, int) error { return nil }

type controlTestSliceRepo struct{}

func (controlTestSliceRepo) GetActiveByEndpoint(context.Context, string) (*storemodel.EndpointProviderSlice, error) {
	return nil, nil
}

func (controlTestSliceRepo) ListActiveByEndpoint(context.Context, string) ([]storemodel.EndpointProviderSlice, error) {
	return nil, nil
}
