package gongji

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"waverless/pkg/config"
)

func TestResourceSearchAndTaskLifecycleUseOfficialVerbs(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method+" "+r.URL.Path)
		if r.Header.Get("token") == "" || r.Header.Get("timestamp") == "" || r.Header.Get("version") == "" {
			t.Errorf("missing auth headers")
		}
		switch r.URL.Path {
		case PathResourceSearch:
			if r.Method != http.MethodGet || r.URL.Query().Get("task_type") != TaskTypeDeploy {
				t.Fatalf("resource search %s %s", r.Method, r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"code":"0000","message":"success","data":{"count":1,"results":[{"device_name":"H20 x 8","gpu_name":"H20","gpu_count":8,"gpu_memory":1,"memory":1,"cpu_cores":1,"disk_size":1,"disk_type":"HDD","regions":[{"region":"hebeif-1","region_name":"河北一区","inventory":0,"price":10,"discount_price":10,"mark":{"mark":"m1","resource":{"device_name":"H20 x 8","region":"hebeif-1","gpu_name":"H20","gpu_count":8,"gpu_memory":1,"memory":1,"cpu_cores":1}}}]}]}}`)
		case PathTaskDetail:
			if r.Method != http.MethodGet || r.URL.Query().Get("task_id") != "7" {
				t.Fatalf("detail %s %s", r.Method, r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"code":"0000","message":"success","data":{"task_id":7,"task_name":"ep","status":"Running","points":2,"runing_points":1}}`)
		case PathTaskSearch:
			if r.Method != http.MethodGet || r.URL.Query().Get("search_value") != "ep" || r.URL.Query().Get("type") != TaskTypeDeploy {
				t.Fatalf("search %s %s", r.Method, r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"code":"0000","message":"success","data":{"count":1,"results":[{"task_id":7,"task_name":"ep","status":"Running","points":1,"runing_points":0}]}}`)
		case PathTaskPause, PathTaskRecover, PathTaskStop, PathTaskChangePoints:
			if r.Method != http.MethodPost {
				t.Fatalf("method %s", r.Method)
			}
			_, _ = io.WriteString(w, `{"code":"0000","message":"success","data":null}`)
		case PathJobCreate:
			var body CreateJobRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
				t.Fatal(err)
			}
			if body.JobSupport.Parallelism != 1 || body.Resources[0].Mark != "m1" {
				t.Fatalf("%+v", body)
			}
			_, _ = io.WriteString(w, `{"code":"0000","message":"success","data":{"task_id":99}}`)
		case PathJobSearch:
			if r.Method != http.MethodGet {
				t.Fatalf("job search %s", r.Method)
			}
			_, _ = io.WriteString(w, `{"code":"0000","message":"success","data":{"count":1,"results":[{"task_id":99,"task_name":"job-a","status":"Running","points":1}]}}`)
		case PathJobDetail:
			if r.URL.Query().Get("task_id") != "99" {
				t.Fatalf("job detail %s", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"code":"0000","message":"success","data":{"task_id":99,"status":"Running"}}`)
		case PathJobStop:
			_, _ = io.WriteString(w, `{"code":"0000","message":"success","data":null}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	client, err := NewClient(&config.GongjiConfig{BaseURL: srv.URL, Token: "test-token", RequestTimeout: 5})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	resources, err := client.SearchResources(ctx, ResourceSearchFilter{TaskType: TaskTypeDeploy, DeviceType: DeviceTypeGPU})
	if err != nil {
		t.Fatal(err)
	}
	if resources.Results[0].Regions[0].Inventory != 0 || resources.Results[0].Regions[0].Mark.Mark != "m1" {
		t.Fatalf("%+v", resources.Results[0].Regions[0])
	}
	if _, err := client.GetTaskDetail(ctx, 7); err != nil {
		t.Fatal(err)
	}
	listed, err := client.SearchTasks(ctx, TaskListFilter{Type: TaskTypeDeploy, SearchValue: "ep", Page: 1, PageSize: 20})
	if err != nil || listed.Count != 1 {
		t.Fatalf("%v %+v", err, listed)
	}
	if err := client.PauseTask(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if err := client.RecoverTask(ctx, 7); err != nil {
		t.Fatal(err)
	}
	if err := client.ChangePoints(ctx, 7, 2); err != nil {
		t.Fatal(err)
	}
	if err := client.StopTask(ctx, 7); err != nil {
		t.Fatal(err)
	}
	created, err := client.CreateJob(ctx, &CreateJobRequest{
		TaskName:  "job-a",
		Resources: []JobResource{{Mark: "m1"}},
		Points:    1,
		JobSupport: JobSupport{
			TimeoutSec:  1200,
			Parallelism: 1,
			ModParam:    JobModParam{Default: &JobModDefault{BackoffLimit: 3, RestartPolicy: "OnFailure"}},
		},
		Services: []JobService{{ServiceName: "container-01", ServiceImage: "ubuntu:22.04"}},
	})
	if err != nil || created.TaskID != 99 {
		t.Fatalf("%v %+v", err, created)
	}
	if _, err := client.SearchJobs(ctx, JobListFilter{Page: 1, PageSize: 10}); err != nil {
		t.Fatal(err)
	}
	raw, err := client.GetJob(ctx, 99)
	if err != nil || !strings.Contains(string(raw), `"task_id":99`) {
		t.Fatalf("%v %s", err, raw)
	}
	if err := client.StopJob(ctx, 99); err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(methods, ",")
	for _, want := range []string{
		"GET " + PathResourceSearch,
		"GET " + PathTaskDetail,
		"GET " + PathTaskSearch,
		"POST " + PathJobCreate,
		"POST " + PathJobStop,
	} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing %s in %s", want, joined)
		}
	}
}

func TestRateLimitIsClassified(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"code":"C999","message":"slow down"}`)
	}))
	defer srv.Close()
	client, err := NewClient(&config.GongjiConfig{BaseURL: srv.URL, Token: "t", RequestTimeout: 5})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.SearchResources(context.Background(), ResourceSearchFilter{})
	ge, ok := AsError(err)
	if !ok || ge.Code != CodeProviderRateLimited {
		t.Fatalf("%v", err)
	}
}
