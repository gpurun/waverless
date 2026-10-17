package gongji

import "encoding/json"

// API paths (Gongji OpenAPI v1, base https://openapi.suanli.cn)
const (
	PathTaskCreate       = "/api/deployment/task/create"
	PathTaskChangePoints = "/api/deployment/task/change_points"
	PathTaskDetail       = "/api/deployment/task/detail"
	PathTaskStop         = "/api/deployment/task/stop"
	PathTaskDeletePod    = "/api/deployment/task/delete_pod"
	PathTaskPoints       = "/api/deployment/task/points"
	PathTaskSearch       = "/api/deployment/task/search"
	PathResourceSearch   = "/api/deployment/resource/search"

	APIVersion     = "1.0.0"
	SuccessCode    = "0000"
	DefaultBaseURL = "https://openapi.suanli.cn"
	TaskTypeDeploy = "Deployment"
)

// APIResponse is the common Gongji response envelope.
type APIResponse struct {
	Code    string          `json:"code"`
	Message string          `json:"message"`
	Msg     string          `json:"msg"`
	Data    json.RawMessage `json:"data"`
}

func (r *APIResponse) Text() string {
	if r.Message != "" {
		return r.Message
	}
	return r.Msg
}

// CreateTaskRequest creates a Deployment task with vendor elasticity OFF.
type CreateTaskRequest struct {
	TaskType           string              `json:"task_type,omitempty"`
	TaskName           string              `json:"task_name"`
	RepositoryUsername string              `json:"repository_username,omitempty"`
	RepositoryPassword string              `json:"repository_password,omitempty"`
	Points             int                 `json:"points,omitempty"`
	DynamicPodStrategy *DynamicPodStrategy `json:"dynamic_pod_strategy,omitempty"`
	Resources          []DeployResource    `json:"resources"`
	Services           []DeployService     `json:"services"`
}

// DynamicPodStrategy controls Gongji native autoscaling. P0 always sets Switch=false.
type DynamicPodStrategy struct {
	MinWorkers       int   `json:"min_workers"`
	MaxWorkers       int   `json:"max_workers"`
	IdleTimeout      int   `json:"idle_timeout"`
	ExecutionTimeout int   `json:"execution_timeout"`
	Switch           bool  `json:"switch"`
	Benchmark        []any `json:"benchmark"`
}

// DisabledDynamicPodStrategy returns a strategy that keeps vendor elasticity off.
func DisabledDynamicPodStrategy(min, max int) *DynamicPodStrategy {
	if max <= 0 {
		max = 1
	}
	if min < 0 {
		min = 0
	}
	return &DynamicPodStrategy{
		MinWorkers:       min,
		MaxWorkers:       max,
		IdleTimeout:      300,
		ExecutionTimeout: 3600,
		Switch:           false,
		Benchmark:        []any{},
	}
}

type DeployResource struct {
	Mark       string `json:"mark"`
	RegionName string `json:"region_name,omitempty"`
}

type DeployService struct {
	ServiceName   string         `json:"service_name"`
	ServiceImage  string         `json:"service_image"`
	Env           string         `json:"env,omitempty"`
	SharedMemSize *int           `json:"shared_mem_size,omitempty"`
	RemotePorts   []RemotePort   `json:"remote_ports"`
	StartScriptV2 *StartScriptV2 `json:"start_script_v2,omitempty"`
}

type RemotePort struct {
	ServicePort int `json:"service_port"`
}

type StartScriptV2 struct {
	Command *string  `json:"command,omitempty"`
	Args    []string `json:"args"`
}

type CreateTaskData struct {
	TaskID int64 `json:"task_id"`
}

type ChangePointsRequest struct {
	TaskID int64 `json:"task_id"`
	Points int   `json:"points"`
}

type TaskIDRequest struct {
	TaskID int64 `json:"task_id"`
}

type DeletePodRequest struct {
	PointID int64 `json:"point_id"`
}

// TaskDetail is a subset of Gongji task detail fields we need.
type TaskDetail struct {
	TaskID        int64           `json:"task_id"`
	TaskName      string          `json:"task_name"`
	Status        string          `json:"status"`
	Points        int             `json:"points"`
	RunningPoints int             `json:"runing_points"` // vendor typo in API
	BillingPoints int             `json:"billing_points"`
	Services      []DeployService `json:"services"`
}

type PointsListData struct {
	Count   int64   `json:"count"`
	Results []Point `json:"results"`
}

type Point struct {
	PointID    int64  `json:"point_id"`
	Name       string `json:"name"`
	Status     string `json:"status"`
	Region     string `json:"region"`
	RegionName string `json:"region_name"`
}

type TaskSearchRequest struct {
	TaskName string `json:"task_name,omitempty"`
	Page     int    `json:"page,omitempty"`
	PageSize int    `json:"page_size,omitempty"`
}

type TaskSearchData struct {
	Count   int64        `json:"count"`
	Results []TaskDetail `json:"results"`
}
