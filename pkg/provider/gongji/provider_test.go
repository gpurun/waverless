package gongji

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"waverless/pkg/config"
	"waverless/pkg/interfaces"
)

func TestDeployDisablesDynamicPodStrategy(t *testing.T) {
	var gotBody CreateTaskRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != PathTaskCreate {
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Fatal(err)
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code":    "0000",
			"message": "success",
			"data":    map[string]any{"task_id": 42},
		})
	}))
	defer srv.Close()

	client, err := NewClient(&config.GongjiConfig{
		Enabled:             true,
		BaseURL:             srv.URL,
		Token:               "test-token",
		DefaultResourceMark: "test-mark",
		RequestTimeout:      5,
	})
	if err != nil {
		t.Fatal(err)
	}
	p := &GongjiDeploymentProvider{client: client, config: &config.GongjiConfig{DefaultResourceMark: "test-mark"}}

	_, err = p.Deploy(context.Background(), &interfaces.DeployRequest{
		Endpoint:    "ep-demo",
		Image:       "harbor.example/app:1",
		Replicas:    1,
		MinReplicas: 0,
		MaxReplicas: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotBody.DynamicPodStrategy == nil || gotBody.DynamicPodStrategy.Switch {
		t.Fatalf("expected dynamic_pod_strategy.switch=false, got %+v", gotBody.DynamicPodStrategy)
	}
	if gotBody.TaskName != "ep-demo" {
		t.Fatalf("task_name=%s", gotBody.TaskName)
	}
}

func TestScaleAppChangePoints(t *testing.T) {
	var got ChangePointsRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case PathTaskChangePoints:
			_ = json.NewDecoder(r.Body).Decode(&got)
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "0000", "message": "success", "data": nil})
		default:
			t.Fatalf("path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client, err := NewClient(&config.GongjiConfig{BaseURL: srv.URL, Token: "t", RequestTimeout: 5})
	if err != nil {
		t.Fatal(err)
	}
	p := &GongjiDeploymentProvider{client: client, config: &config.GongjiConfig{}}
	p.BindTaskID("ep-a", 99)

	if err := p.ScaleApp(context.Background(), "ep-a", 3); err != nil {
		t.Fatal(err)
	}
	if got.TaskID != 99 || got.Points != 3 {
		t.Fatalf("got %+v", got)
	}
}

func TestGetAppStatusMapsReady(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "0000", "message": "success",
			"data": map[string]any{
				"task_id": 7, "task_name": "ep", "status": "Running",
				"points": 2, "runing_points": 1,
			},
		})
	}))
	defer srv.Close()
	client, err := NewClient(&config.GongjiConfig{BaseURL: srv.URL, Token: "t", RequestTimeout: 5})
	if err != nil {
		t.Fatal(err)
	}
	p := &GongjiDeploymentProvider{client: client, config: &config.GongjiConfig{}}
	p.BindTaskID("ep", 7)
	st, err := p.GetAppStatus(context.Background(), "ep")
	if err != nil {
		t.Fatal(err)
	}
	if st.TotalReplicas != 2 || st.ReadyReplicas != 1 {
		t.Fatalf("%+v", st)
	}
}

func TestTerminateWorkerDeletePod(t *testing.T) {
	var pointID int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == PathTaskDeletePod {
			var body DeletePodRequest
			_ = json.NewDecoder(r.Body).Decode(&body)
			pointID = body.PointID
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "0000", "message": "success", "data": nil})
	}))
	defer srv.Close()
	client, err := NewClient(&config.GongjiConfig{BaseURL: srv.URL, Token: "t", RequestTimeout: 5})
	if err != nil {
		t.Fatal(err)
	}
	p := &GongjiDeploymentProvider{client: client, config: &config.GongjiConfig{}}
	p.BindTaskID("ep", 1)
	if err := p.TerminateWorker(context.Background(), "ep", "55"); err != nil {
		t.Fatal(err)
	}
	if pointID != 55 {
		t.Fatalf("point_id=%d", pointID)
	}
}
