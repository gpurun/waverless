package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	gongjisvc "waverless/internal/service/gongji"
	"waverless/pkg/config"
	"waverless/pkg/provider/gongji"
)

func TestCreateServerlessResolvesMarkAndDisablesVendorAutoscale(t *testing.T) {
	gin.SetMode(gin.TestMode)
	var created gongji.CreateTaskRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case gongji.PathResourceSearch:
			_, _ = io.WriteString(w, `{"code":"0000","message":"success","data":{"count":1,"results":[{"device_name":"H20 x 8","gpu_name":"H20","gpu_count":8,"gpu_memory":1,"memory":1,"cpu_cores":1,"disk_size":1,"disk_type":"HDD","regions":[{"region":"hebeif-1","region_name":"河北一区","inventory":2,"price":10,"discount_price":10,"mark":{"mark":"mark-hebei"}}]}]}}`)
		case gongji.PathTaskCreate:
			if err := json.NewDecoder(r.Body).Decode(&created); err != nil {
				t.Fatal(err)
			}
			_, _ = io.WriteString(w, `{"code":"0000","message":"success","data":{"task_id":42}}`)
		default:
			t.Fatalf("path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client, err := gongji.NewClient(&config.GongjiConfig{BaseURL: srv.URL, Token: "t", RequestTimeout: 5})
	if err != nil {
		t.Fatal(err)
	}
	catalog := gongji.NewCatalog(client, gongji.CatalogConfig{})
	if err := catalog.Refresh(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := catalog.UpsertBinding(gongji.RegionBinding{
		LogicalAzID:      "cn-hebei-az1",
		DisplayName:      "华北·河北一区",
		ComplianceDomain: gongji.ComplianceDomestic,
		Availability:     gongji.AvailabilitySchedulable,
		PhysicalRegionID: "hebeif-1",
		ProviderSkuID:    "H20",
		GPUCount:         8,
	}); err != nil {
		t.Fatal(err)
	}
	h := NewGongjiHandler(gongjisvc.NewService(client, catalog))
	router := gin.New()
	router.GET("/api/v1/gongji/regions", h.ListRegions)
	router.POST("/api/v1/gongji/serverless/tasks", h.CreateServerless)

	catalogReq := httptest.NewRequest(http.MethodGet, "/api/v1/gongji/regions?complianceDomain=domestic", nil)
	catalogResp := httptest.NewRecorder()
	router.ServeHTTP(catalogResp, catalogReq)
	if catalogResp.Code != http.StatusOK || strings.Contains(catalogResp.Body.String(), "mark-hebei") || strings.Contains(catalogResp.Body.String(), "hebeif-1") {
		t.Fatalf("catalog leaked vendor fields: %s", catalogResp.Body.String())
	}

	body := strings.NewReader(`{"taskName":"ep-demo","image":"harbor.example/app:1","points":1,"logicalAzId":"cn-hebei-az1","providerSkuId":"H20","minReplicas":0,"maxReplicas":3}`)
	createReq := httptest.NewRequest(http.MethodPost, "/api/v1/gongji/serverless/tasks", body)
	createReq.Header.Set("Content-Type", "application/json")
	createResp := httptest.NewRecorder()
	router.ServeHTTP(createResp, createReq)
	if createResp.Code != http.StatusAccepted {
		t.Fatalf("create %d %s", createResp.Code, createResp.Body.String())
	}
	if created.DynamicPodStrategy == nil || created.DynamicPodStrategy.Switch {
		t.Fatalf("vendor autoscale must be off: %+v", created.DynamicPodStrategy)
	}
	if created.Resources[0].Mark != "mark-hebei" {
		t.Fatalf("mark %s", created.Resources[0].Mark)
	}
}
