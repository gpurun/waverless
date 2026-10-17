package gongji

import "encoding/json"

// API paths (Gongji OpenAPI v1, base https://openapi.suanli.cn)
const (
	PathTaskCreate       = "/api/deployment/task/create"
	PathTaskUpdate       = "/api/deployment/task/update"
	PathTaskChangePoints = "/api/deployment/task/change_points"
	PathTaskDetail       = "/api/deployment/task/detail"
	PathTaskStop         = "/api/deployment/task/stop"
	PathTaskPause        = "/api/deployment/task/pause"
	PathTaskRecover      = "/api/deployment/task/recover"
	PathTaskDeletePod    = "/api/deployment/task/delete_pod"
	PathTaskPoints       = "/api/deployment/task/points"
	PathTaskSearch       = "/api/deployment/task/search"
	PathResourceSearch   = "/api/deployment/resource/search"
	PathJobCreate        = "/api/task/job/create"
	PathJobSearch        = "/api/task/job/search"
	PathJobDetail        = "/api/task/job/detail"
	PathJobStop          = "/api/task/job/stop"

	APIVersion     = "1.0.0"
	SuccessCode    = "0000"
	DefaultBaseURL = "https://openapi.suanli.cn"
	TaskTypeDeploy = "Deployment"
	DeviceTypeGPU  = "GpuDevice"
	DeviceTypeCPU  = "CpuDevice"
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
	AccessURL  string `json:"access_url,omitempty"`
	URL        string `json:"url,omitempty"`
	Domain     string `json:"domain,omitempty"`
	ServiceURL string `json:"service_url,omitempty"`
	RemoteURL  string `json:"remote_url,omitempty"`
}

// TaskListFilter is the query for GET /api/deployment/task/search.
type TaskListFilter struct {
	Type        string
	Status      string
	SearchValue string
	Page        int
	PageSize    int
}

type TaskSearchData struct {
	Count   int64        `json:"count"`
	Results []TaskDetail `json:"results"`
}

// DeviceResource is the vendor device shape nested under a region mark.
type DeviceResource struct {
	DeviceName string `json:"device_name"`
	Region     string `json:"region"`
	GPUName    string `json:"gpu_name"`
	GPUCount   int    `json:"gpu_count"`
	GPUMemory  int    `json:"gpu_memory"`
	Memory     int    `json:"memory"`
	CPUCores   int    `json:"cpu_cores"`
}

// ResourceMark is resources[].mark from resource search and task payloads.
type ResourceMark struct {
	Resource   *DeviceResource `json:"resource,omitempty"`
	RegionName string          `json:"region_name,omitempty"`
	Mark       string          `json:"mark"`
}

// ResourceRegion is one physical region offer inside a device.
type ResourceRegion struct {
	Region        string       `json:"region"`
	RegionName    string       `json:"region_name"`
	Mark          ResourceMark `json:"mark"`
	Price         *int         `json:"price"`
	DiscountPrice *int         `json:"discount_price"`
	Inventory     int          `json:"inventory"`
}

// ResourceDevice is one row from GET /api/deployment/resource/search.
type ResourceDevice struct {
	DeviceName string           `json:"device_name"`
	Regions    []ResourceRegion `json:"regions"`
	GPUName    string           `json:"gpu_name"`
	GPUMemory  int              `json:"gpu_memory"`
	GPUCount   int              `json:"gpu_count"`
	Memory     int              `json:"memory"`
	CPUCores   int              `json:"cpu_cores"`
	DiskSize   int              `json:"disk_size"`
	DiskType   string           `json:"disk_type"`
}

// ResourceSearchFilter selects Deployment GPU/CPU inventory.
type ResourceSearchFilter struct {
	TaskType   string
	DeviceType string
}

// ResourceSearchData is the resource search payload.
type ResourceSearchData struct {
	Count   int              `json:"count"`
	Results []ResourceDevice `json:"results"`
}

// JobListFilter is the query for GET /api/task/job/search.
type JobListFilter struct {
	Status      string
	SearchValue string
	GroupTask   *bool
	GroupID     int64
	Page        int
	PageSize    int
}

// JobSummary is the stable subset used to list Job tasks.
type JobSummary struct {
	TaskID   int64           `json:"task_id"`
	TaskName string          `json:"task_name"`
	Status   string          `json:"status"`
	Points   int             `json:"points"`
	SubType  string          `json:"sub_type,omitempty"`
	Raw      json.RawMessage `json:"-"`
}

// JobListData is a page of Job tasks.
type JobListData struct {
	Count   int64        `json:"count"`
	Results []JobSummary `json:"results"`
}

// JobEnv is a name/value pair on a Job container.
type JobEnv struct {
	Name  string `json:"name"`
	Value string `json:"value,omitempty"`
}

// JobResourceWeight is required by the Job create schema.
type JobResourceWeight struct {
	CPUWeight int `json:"cpu_weight"`
	MemWeight int `json:"mem_weight"`
	GPUWeight int `json:"gpu_weight"`
}

// JobStartScript is the container command.
type JobStartScript struct {
	Command []string `json:"command,omitempty"`
	Args    []string `json:"args,omitempty"`
}

// JobService is one container in a Job task.
type JobService struct {
	ServiceName    string             `json:"service_name"`
	ServiceImage   string             `json:"service_image"`
	ResourceWeight *JobResourceWeight `json:"resource_weight,omitempty"`
	RemotePorts    []RemotePort       `json:"remote_ports,omitempty"`
	Env            []JobEnv           `json:"env,omitempty"`
	StartScript    *JobStartScript    `json:"start_script,omitempty"`
}

// JobModDefault is the non-indexed completion mode.
type JobModDefault struct {
	BackoffLimit  int    `json:"backoff_limit"`
	RestartPolicy string `json:"restart_policy"`
}

// JobModIndex is the indexed completion mode.
type JobModIndex struct {
	IndexBackoffLimit int  `json:"index_backoff_limit"`
	MaxFailedIndex    *int `json:"max_failed_index,omitempty"`
}

// JobModParam selects default or index completion. Exactly one branch is set.
type JobModParam struct {
	Default *JobModDefault `json:"default,omitempty"`
	Index   *JobModIndex   `json:"index,omitempty"`
}

// JobSupport is the Job batch completion policy.
type JobSupport struct {
	EstimatedExecSec *int        `json:"estimated_exec_sec,omitempty"`
	TimeoutSec       int         `json:"timeout_sec"`
	Parallelism      int         `json:"parallelism"`
	ModParam         JobModParam `json:"mod_param"`
}

// JobResource is one resources[] entry. Mark is required; nested resource is optional.
type JobResource struct {
	Resource   *DeviceResource `json:"resource,omitempty"`
	RegionName string          `json:"region_name,omitempty"`
	Mark       string          `json:"mark"`
	Weight     *int            `json:"weight,omitempty"`
}

// CreateJobRequest creates a Gongji Job batch task.
type CreateJobRequest struct {
	TaskName   string        `json:"task_name"`
	SubType    *string       `json:"sub_type,omitempty"`
	Resources  []JobResource `json:"resources"`
	Points     int           `json:"points"`
	JobSupport JobSupport    `json:"job_support"`
	Services   []JobService  `json:"services"`
}
