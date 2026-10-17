package gongji

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"
	"time"

	"waverless/pkg/config"
	"waverless/pkg/interfaces"
	"waverless/pkg/logger"
)

const (
	labelTaskID       = "gongji_task_id"
	labelResourceMark = "gongji_resource_mark"
	labelLogicalAz    = "logical_az_id"
	labelProviderSku  = "provider_sku_id"
	labelWorkerPort   = "gongji_worker_port"
	defaultWorkerPort = 8000
)

// clientInterface allows mocking in tests.
type clientInterface interface {
	CreateTask(ctx context.Context, req *CreateTaskRequest) (*CreateTaskData, error)
	ChangePoints(ctx context.Context, taskID int64, points int) error
	GetTaskDetail(ctx context.Context, taskID int64) (*TaskDetail, error)
	StopTask(ctx context.Context, taskID int64) error
	DeletePod(ctx context.Context, pointID int64) error
	ListPoints(ctx context.Context, taskID int64, page, pageSize int) (*PointsListData, error)
	SearchTasks(ctx context.Context, filter TaskListFilter) (*TaskSearchData, error)
}

// GongjiDeploymentProvider implements CoreDeploymentProvider (and stubs for FullDeploymentProvider)
// against Gongji OpenAPI. It has zero Kubernetes client dependency.
type GongjiDeploymentProvider struct {
	client clientInterface
	config *config.GongjiConfig

	taskCache sync.Map // endpoint name -> int64 taskID
	catalog   *Catalog
}

// NewGongjiDeploymentProvider creates a Gongji provider from global config.
func NewGongjiDeploymentProvider(cfg *config.Config) (interfaces.DeploymentProvider, error) {
	if !cfg.Gongji.Enabled {
		return nil, fmt.Errorf("gongji provider is not enabled in config")
	}
	if cfg.Gongji.Token == "" {
		return nil, fmt.Errorf("gongji token is required (env GONGJI_TOKEN)")
	}

	client, err := NewClient(&cfg.Gongji)
	if err != nil {
		return nil, err
	}

	return &GongjiDeploymentProvider{
		client: client,
		config: &cfg.Gongji,
	}, nil
}

// SetCatalog attaches the region catalog used to resolve resource marks.
func (p *GongjiDeploymentProvider) SetCatalog(catalog *Catalog) {
	if p != nil {
		p.catalog = catalog
	}
}

// Deploy creates a Gongji Deployment task with dynamic_pod_strategy.switch=false.
func (p *GongjiDeploymentProvider) Deploy(ctx context.Context, req *interfaces.DeployRequest) (*interfaces.DeployResponse, error) {
	if req == nil || req.Endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}
	if req.Image == "" {
		return nil, fmt.Errorf("image is required")
	}

	replicas := req.Replicas
	if replicas < 0 {
		replicas = 0
	}
	maxReplicas := req.MaxReplicas
	if maxReplicas <= 0 {
		maxReplicas = replicas
	}
	if maxReplicas <= 0 {
		maxReplicas = 1
	}

	mark, err := p.resolveResourceMark(ctx, req)
	if err != nil {
		return nil, err
	}

	port := defaultWorkerPort
	if req.Labels != nil {
		if v := req.Labels[labelWorkerPort]; v != "" {
			if n, err := strconv.Atoi(v); err == nil && n > 0 {
				port = n
			}
		}
	}

	envStr := formatEnv(req.Env)
	createReq := &CreateTaskRequest{
		TaskType:           TaskTypeDeploy,
		TaskName:           req.Endpoint,
		Points:             replicas,
		DynamicPodStrategy: DisabledDynamicPodStrategy(req.MinReplicas, maxReplicas),
		Resources: []DeployResource{{
			Mark:       mark,
			RegionName: p.config.DefaultRegionName,
		}},
		Services: []DeployService{{
			ServiceName:  sanitizeServiceName(req.Endpoint),
			ServiceImage: req.Image,
			Env:          envStr,
			RemotePorts:  []RemotePort{{ServicePort: port}},
			StartScriptV2: &StartScriptV2{
				Args: []string{},
			},
		}},
	}
	if req.RegistryCredential != nil {
		createReq.RepositoryUsername = req.RegistryCredential.Username
		createReq.RepositoryPassword = req.RegistryCredential.Password
	}

	logger.Infof("gongji Deploy endpoint=%s image=%s points=%d mark=%s (vendor elasticity OFF)",
		req.Endpoint, req.Image, replicas, truncateMark(mark))

	data, err := p.client.CreateTask(ctx, createReq)
	if err != nil {
		return nil, fmt.Errorf("gongji create task: %w", err)
	}
	p.taskCache.Store(req.Endpoint, data.TaskID)

	return &interfaces.DeployResponse{
		Endpoint:  req.Endpoint,
		Message:   fmt.Sprintf("created gongji task_id=%d", data.TaskID),
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

func (p *GongjiDeploymentProvider) resolveResourceMark(ctx context.Context, req *interfaces.DeployRequest) (string, error) {
	if req.Labels != nil {
		if m := req.Labels[labelResourceMark]; m != "" {
			return m, nil
		}
	}
	if p.catalog != nil && req.Labels != nil {
		az := req.Labels[labelLogicalAz]
		if az != "" {
			mark, err := p.catalog.ResolveMark(az, req.Labels[labelProviderSku])
			if err != nil {
				return "", err
			}
			return mark, nil
		}
	}
	if p.config != nil && p.config.DefaultResourceMark != "" {
		return p.config.DefaultResourceMark, nil
	}
	return "", fmt.Errorf("gongji resource mark is required (config gongji.default_resource_mark, label %s, or logical_az_id)", labelResourceMark)
}

func formatEnv(env map[string]string) string {
	if len(env) == 0 {
		return ""
	}
	parts := make([]string, 0, len(env))
	for k, v := range env {
		parts = append(parts, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(parts, "\n")
}

func sanitizeServiceName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	if len(name) > 48 {
		name = name[:48]
	}
	return name
}

func truncateMark(m string) string {
	if len(m) <= 16 {
		return m
	}
	return m[:8] + "..." + m[len(m)-4:]
}

// GetApp retrieves application details from Gongji task detail.
func (p *GongjiDeploymentProvider) GetApp(ctx context.Context, endpoint string) (*interfaces.AppInfo, error) {
	taskID, err := p.resolveTaskID(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	detail, err := p.client.GetTaskDetail(ctx, taskID)
	if err != nil {
		return nil, err
	}
	image := ""
	if len(detail.Services) > 0 {
		image = detail.Services[0].ServiceImage
	}
	return &interfaces.AppInfo{
		Name:              endpoint,
		Type:              "GongjiDeployment",
		Status:            mapStatus(detail.Status),
		Replicas:          int32(detail.Points),
		ReadyReplicas:     int32(detail.RunningPoints),
		AvailableReplicas: int32(detail.RunningPoints),
		Image:             image,
		Labels: map[string]string{
			labelTaskID: strconv.FormatInt(taskID, 10),
		},
		CreatedAt: "",
	}, nil
}

func mapStatus(s string) string {
	switch strings.ToLower(s) {
	case "running":
		return "Running"
	case "pending":
		return "Pending"
	case "paused", "end":
		return "Stopped"
	default:
		if s == "" {
			return "Unknown"
		}
		return s
	}
}

// ListApps lists known apps from cache (and optional search by cached names).
func (p *GongjiDeploymentProvider) ListApps(ctx context.Context) ([]*interfaces.AppInfo, error) {
	apps := make([]*interfaces.AppInfo, 0)
	var walkErr error
	p.taskCache.Range(func(key, value any) bool {
		name := key.(string)
		app, err := p.GetApp(ctx, name)
		if err != nil {
			logger.Warnf("gongji ListApps skip %s: %v", name, err)
			return true
		}
		apps = append(apps, app)
		return true
	})
	return apps, walkErr
}

// DeleteApp stops/deletes the Gongji task.
func (p *GongjiDeploymentProvider) DeleteApp(ctx context.Context, endpoint string) error {
	taskID, err := p.resolveTaskID(ctx, endpoint)
	if err != nil {
		return err
	}
	if err := p.client.StopTask(ctx, taskID); err != nil {
		return fmt.Errorf("gongji stop task: %w", err)
	}
	p.taskCache.Delete(endpoint)
	return nil
}

// ScaleApp sets absolute desired points via change_points — the only scale write path.
func (p *GongjiDeploymentProvider) ScaleApp(ctx context.Context, endpoint string, replicas int) error {
	if replicas < 0 {
		return fmt.Errorf("replicas must be >= 0")
	}
	taskID, err := p.resolveTaskID(ctx, endpoint)
	if err != nil {
		return err
	}
	logger.Infof("gongji ScaleApp endpoint=%s task_id=%d points=%d", endpoint, taskID, replicas)
	if err := p.client.ChangePoints(ctx, taskID, replicas); err != nil {
		return fmt.Errorf("gongji change_points: %w", err)
	}
	return nil
}

// GetAppStatus returns desired vs ready points.
func (p *GongjiDeploymentProvider) GetAppStatus(ctx context.Context, endpoint string) (*interfaces.AppStatus, error) {
	taskID, err := p.resolveTaskID(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	detail, err := p.client.GetTaskDetail(ctx, taskID)
	if err != nil {
		return nil, err
	}
	return &interfaces.AppStatus{
		Endpoint:          endpoint,
		Status:            mapStatus(detail.Status),
		ReadyReplicas:     int32(detail.RunningPoints),
		AvailableReplicas: int32(detail.RunningPoints),
		TotalReplicas:     int32(detail.Points),
		Message:           fmt.Sprintf("task_id=%d desired=%d ready=%d", taskID, detail.Points, detail.RunningPoints),
	}, nil
}

// UpdateDeployment updates replicas via ScaleApp; image/spec updates are not supported in P0.
func (p *GongjiDeploymentProvider) UpdateDeployment(ctx context.Context, req *interfaces.UpdateDeploymentRequest) (*interfaces.DeployResponse, error) {
	if req == nil || req.Endpoint == "" {
		return nil, fmt.Errorf("endpoint is required")
	}
	if req.Image != "" {
		return nil, fmt.Errorf("gongji UpdateDeployment image change not supported in P0; recreate the task")
	}
	if req.Replicas != nil {
		if err := p.ScaleApp(ctx, req.Endpoint, *req.Replicas); err != nil {
			return nil, err
		}
	}
	return &interfaces.DeployResponse{
		Endpoint:  req.Endpoint,
		Message:   "updated",
		CreatedAt: time.Now().UTC().Format(time.RFC3339),
	}, nil
}

// TerminateWorker deletes a single Gongji point by point_id (or cached worker name).
func (p *GongjiDeploymentProvider) TerminateWorker(ctx context.Context, endpoint, workerID string) error {
	pointID, err := strconv.ParseInt(workerID, 10, 64)
	if err != nil {
		// try resolve via points list by name
		taskID, rerr := p.resolveTaskID(ctx, endpoint)
		if rerr != nil {
			return fmt.Errorf("invalid point_id %q: %w", workerID, err)
		}
		points, lerr := p.client.ListPoints(ctx, taskID, 1, 200)
		if lerr != nil {
			return fmt.Errorf("invalid point_id %q and list points failed: %v / %v", workerID, err, lerr)
		}
		found := int64(0)
		for _, pt := range points.Results {
			if pt.Name == workerID || strconv.FormatInt(pt.PointID, 10) == workerID {
				found = pt.PointID
				break
			}
		}
		if found == 0 {
			return fmt.Errorf("worker/point %q not found for endpoint %s", workerID, endpoint)
		}
		pointID = found
	}
	logger.Infof("gongji TerminateWorker endpoint=%s point_id=%d", endpoint, pointID)
	return p.client.DeletePod(ctx, pointID)
}

// ListWorkers maps Gongji points to PodInfo-like records.
func (p *GongjiDeploymentProvider) ListWorkers(ctx context.Context, endpoint string) ([]*interfaces.PodInfo, error) {
	taskID, err := p.resolveTaskID(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	data, err := p.client.ListPoints(ctx, taskID, 1, 200)
	if err != nil {
		return nil, err
	}
	out := make([]*interfaces.PodInfo, 0, len(data.Results))
	for _, pt := range data.Results {
		out = append(out, &interfaces.PodInfo{
			Name:     pt.Name,
			Phase:    pt.Status,
			Status:   pt.Status,
			WorkerID: strconv.FormatInt(pt.PointID, 10),
			Labels: map[string]string{
				"gongji_point_id": strconv.FormatInt(pt.PointID, 10),
				"gongji_task_id":  strconv.FormatInt(taskID, 10),
				"region":          pt.Region,
			},
		})
	}
	return out, nil
}

func (p *GongjiDeploymentProvider) resolveTaskID(ctx context.Context, endpoint string) (int64, error) {
	if v, ok := p.taskCache.Load(endpoint); ok {
		return v.(int64), nil
	}
	// Best-effort search by task name
	list, err := p.client.SearchTasks(ctx, TaskListFilter{
		Type:        TaskTypeDeploy,
		SearchValue: endpoint,
		Page:        1,
		PageSize:    50,
	})
	if err == nil && list != nil {
		for _, t := range list.Results {
			if t.TaskName == endpoint || strconv.FormatInt(t.TaskID, 10) == endpoint {
				p.taskCache.Store(endpoint, t.TaskID)
				return t.TaskID, nil
			}
		}
	}
	// Allow endpoint to be numeric task_id
	if id, err2 := strconv.ParseInt(endpoint, 10, 64); err2 == nil && id > 0 {
		p.taskCache.Store(endpoint, id)
		return id, nil
	}
	if err != nil {
		return 0, fmt.Errorf("gongji task for endpoint %q not found: %w", endpoint, err)
	}
	return 0, fmt.Errorf("gongji task for endpoint %q not found in cache or search", endpoint)
}

// BindTaskID seeds the endpoint→task_id mapping (e.g. after restart from metadata labels).
func (p *GongjiDeploymentProvider) BindTaskID(endpoint string, taskID int64) {
	if endpoint != "" && taskID > 0 {
		p.taskCache.Store(endpoint, taskID)
	}
}

// --- FullDeploymentProvider stubs (no K8s) ---

func (p *GongjiDeploymentProvider) ListSpecs(ctx context.Context) ([]*interfaces.SpecInfo, error) {
	return []*interfaces.SpecInfo{}, nil
}

func (p *GongjiDeploymentProvider) GetSpec(ctx context.Context, specName string) (*interfaces.SpecInfo, error) {
	return nil, fmt.Errorf("gongji GetSpec not supported; use Region Registry / platform SKU mapping")
}

func (p *GongjiDeploymentProvider) GetAppLogs(ctx context.Context, endpoint string, lines int, podName ...string) (string, error) {
	return "", fmt.Errorf("gongji GetAppLogs not supported")
}

func (p *GongjiDeploymentProvider) GetPods(ctx context.Context, endpoint string) ([]*interfaces.PodInfo, error) {
	return p.ListWorkers(ctx, endpoint)
}

func (p *GongjiDeploymentProvider) DescribePod(ctx context.Context, endpoint string, podName string) (*interfaces.PodDetail, error) {
	return nil, fmt.Errorf("gongji DescribePod not supported")
}

func (p *GongjiDeploymentProvider) GetPodYAML(ctx context.Context, endpoint string, podName string) (string, error) {
	return "", fmt.Errorf("gongji GetPodYAML not supported")
}

func (p *GongjiDeploymentProvider) IsPodTerminating(ctx context.Context, podName string) (bool, error) {
	return false, nil
}

func (p *GongjiDeploymentProvider) ListPVCs(ctx context.Context) ([]*interfaces.PVCInfo, error) {
	return nil, fmt.Errorf("gongji ListPVCs not supported")
}

func (p *GongjiDeploymentProvider) GetDefaultEnv(ctx context.Context) (map[string]string, error) {
	return map[string]string{
		"WAVERLESS_PROVIDER": "gongji",
		"PROVIDER_TYPE":      "gongji",
	}, nil
}

func (p *GongjiDeploymentProvider) PreviewDeploymentYAML(ctx context.Context, req *interfaces.DeployRequest) (string, error) {
	return "", fmt.Errorf("gongji PreviewDeploymentYAML not supported (OpenAPI-only provider)")
}

func (p *GongjiDeploymentProvider) WatchReplicas(ctx context.Context, callback interfaces.ReplicaCallback) error {
	// External providers do not use K8s informers; autoscaler polls GetAppStatus.
	<-ctx.Done()
	return ctx.Err()
}
