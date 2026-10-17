package gongjisvc

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"waverless/pkg/provider/gongji"
)

const (
	defaultPageSize = 20
	maxPageSize     = 100
)

// TaskBinder records endpoint → Gongji task id for later scale/terminate.
type TaskBinder interface {
	BindTaskID(endpoint string, taskID int64)
}

// Service maintains Gongji region inventory and serverless/job task lifecycles.
type Service struct {
	client  *gongji.Client
	catalog *gongji.Catalog
	binder  TaskBinder
}

// NewService wires the Gongji client and region catalog.
func NewService(client *gongji.Client, catalog *gongji.Catalog) *Service {
	return &Service{client: client, catalog: catalog}
}

// SetTaskBinder attaches the deployment provider cache.
func (s *Service) SetTaskBinder(binder TaskBinder) {
	s.binder = binder
}

// RegionPage is a server-side page of logical AZs.
type RegionPage struct {
	Items    any    `json:"items"`
	Total    int    `json:"total"`
	Page     int    `json:"page"`
	PageSize int    `json:"pageSize"`
	View     string `json:"view"`
}

// ListRegions returns the logical AZ catalog. view=ops includes vendor fields.
func (s *Service) ListRegions(complianceDomain, gpuSku, view string, schedulableOnly bool, page, pageSize int) RegionPage {
	page, pageSize = clampPage(page, pageSize)
	if view == "ops" {
		items := s.catalog.ListOps(complianceDomain, gpuSku, schedulableOnly)
		paged := pageSlice(items, page, pageSize)
		return RegionPage{Items: paged, Total: len(items), Page: page, PageSize: pageSize, View: "ops"}
	}
	items := s.catalog.ListCatalog(complianceDomain, gpuSku, schedulableOnly)
	paged := pageSlice(items, page, pageSize)
	return RegionPage{Items: paged, Total: len(items), Page: page, PageSize: pageSize, View: "catalog"}
}

// ResourceSnapshot returns the latest vendor inventory pull.
func (s *Service) ResourceSnapshot(page, pageSize int) gongji.ResourceSnapshot {
	page, pageSize = clampPage(page, pageSize)
	snap := s.catalog.Snapshot()
	total := len(snap.Items)
	snap.Items = pageSlice(snap.Items, page, pageSize)
	snap.Total = total
	snap.Page = page
	snap.PageSize = pageSize
	return snap
}

// RefreshInventory pulls Gongji resource search immediately.
func (s *Service) RefreshInventory(ctx context.Context) error {
	return s.catalog.Refresh(ctx)
}

// SetAvailability changes platform sellability. It does not call the vendor.
func (s *Service) SetAvailability(ctx context.Context, logicalAzID, availability, reason string) error {
	return s.catalog.SetAvailabilityContext(ctx, logicalAzID, availability, reason)
}

// RegistryVersion returns monotonic binding and inventory pull versions.
func (s *Service) RegistryVersion() gongji.RegistryVersionInfo {
	return s.catalog.VersionInfo()
}

// ListInventoryPulls returns paginated Gongji inventory sync history.
func (s *Service) ListInventoryPulls(ctx context.Context, page, pageSize int) ([]gongji.InventoryPullSummary, int64, error) {
	return s.catalog.ListInventoryPulls(ctx, page, pageSize)
}

// UpsertBinding registers a logical AZ against an observed physical SKU.
func (s *Service) UpsertBinding(binding gongji.RegionBinding) error {
	return s.catalog.UpsertBinding(binding)
}

// CreateServerlessInput is the platform create body. Resource marks stay optional.
type CreateServerlessInput struct {
	TaskName           string `json:"taskName"`
	Image              string `json:"image"`
	Points             int    `json:"points"`
	LogicalAzID        string `json:"logicalAzId"`
	ProviderSkuID      string `json:"providerSkuId"`
	ResourceMark       string `json:"resourceMark,omitempty"`
	Env                string `json:"env,omitempty"`
	ServicePort        int    `json:"servicePort,omitempty"`
	MinReplicas        int    `json:"minReplicas"`
	MaxReplicas        int    `json:"maxReplicas"`
	RepositoryUsername string `json:"repositoryUsername,omitempty"`
	RepositoryPassword string `json:"repositoryPassword,omitempty"`
}

// TaskPage is one vendor page of deployment tasks.
type TaskPage struct {
	Items    []gongji.TaskDetail `json:"items"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"pageSize"`
}

// ListServerless lists deployment tasks.
func (s *Service) ListServerless(ctx context.Context, status, search string, page, pageSize int) (*TaskPage, error) {
	page, pageSize = clampPage(page, pageSize)
	data, err := s.client.SearchTasks(ctx, gongji.TaskListFilter{
		Type:        gongji.TaskTypeDeploy,
		Status:      status,
		SearchValue: search,
		Page:        page,
		PageSize:    pageSize,
	})
	if err != nil {
		return nil, err
	}
	return &TaskPage{Items: data.Results, Total: data.Count, Page: page, PageSize: pageSize}, nil
}

// GetServerless returns one deployment task.
func (s *Service) GetServerless(ctx context.Context, taskID int64) (*gongji.TaskDetail, error) {
	return s.client.GetTaskDetail(ctx, taskID)
}

// CreateServerless creates a deployment with vendor autoscaling forced off.
func (s *Service) CreateServerless(ctx context.Context, in CreateServerlessInput) (*gongji.CreateTaskData, error) {
	if strings.TrimSpace(in.TaskName) == "" || strings.TrimSpace(in.Image) == "" {
		return nil, fmt.Errorf("taskName and image are required")
	}
	if in.Points < 0 {
		return nil, fmt.Errorf("points must be >= 0")
	}
	mark, err := s.resolveMark(in.LogicalAzID, in.ProviderSkuID, in.ResourceMark)
	if err != nil {
		return nil, err
	}
	maxReplicas := in.MaxReplicas
	if maxReplicas <= 0 {
		maxReplicas = in.Points
	}
	if maxReplicas <= 0 {
		maxReplicas = 1
	}
	port := in.ServicePort
	if port <= 0 {
		port = 8000
	}
	req := &gongji.CreateTaskRequest{
		TaskType:           gongji.TaskTypeDeploy,
		TaskName:           in.TaskName,
		Points:             in.Points,
		DynamicPodStrategy: gongji.DisabledDynamicPodStrategy(in.MinReplicas, maxReplicas),
		Resources:          []gongji.DeployResource{{Mark: mark}},
		Services: []gongji.DeployService{{
			ServiceName:  sanitizeServiceName(in.TaskName),
			ServiceImage: in.Image,
			Env:          in.Env,
			RemotePorts:  []gongji.RemotePort{{ServicePort: port}},
			StartScriptV2: &gongji.StartScriptV2{
				Args: []string{},
			},
		}},
		RepositoryUsername: in.RepositoryUsername,
		RepositoryPassword: in.RepositoryPassword,
	}
	data, err := s.client.CreateTask(ctx, req)
	if err != nil {
		return nil, err
	}
	if s.binder != nil {
		s.binder.BindTaskID(in.TaskName, data.TaskID)
	}
	return data, nil
}

// PauseServerless pauses a deployment. The vendor releases compute.
func (s *Service) PauseServerless(ctx context.Context, taskID int64) error {
	return s.client.PauseTask(ctx, taskID)
}

// RecoverServerless resumes a paused deployment.
func (s *Service) RecoverServerless(ctx context.Context, taskID int64) error {
	return s.client.RecoverTask(ctx, taskID)
}

// ScaleServerless sets the absolute point count.
func (s *Service) ScaleServerless(ctx context.Context, taskID int64, points int) error {
	if points < 0 {
		return fmt.Errorf("points must be >= 0")
	}
	return s.client.ChangePoints(ctx, taskID, points)
}

// UpdateServerless replaces the full task document. body must round-trip a detail response.
func (s *Service) UpdateServerless(ctx context.Context, body json.RawMessage) error {
	return s.client.UpdateTask(ctx, body)
}

// DeleteServerless deletes a deployment. The vendor does not allow recovery.
func (s *Service) DeleteServerless(ctx context.Context, taskID int64) error {
	return s.client.StopTask(ctx, taskID)
}

// ListPoints lists worker points for a deployment or job task.
func (s *Service) ListPoints(ctx context.Context, taskID int64, page, pageSize int) (*gongji.PointsListData, error) {
	page, pageSize = clampPage(page, pageSize)
	return s.client.ListPoints(ctx, taskID, page, pageSize)
}

// DeletePoint removes one vendor point.
func (s *Service) DeletePoint(ctx context.Context, pointID int64) error {
	return s.client.DeletePod(ctx, pointID)
}

// CreateJobInput creates a Gongji Job batch task.
type CreateJobInput struct {
	TaskName         string            `json:"taskName"`
	LogicalAzID      string            `json:"logicalAzId"`
	ProviderSkuID    string            `json:"providerSkuId"`
	ResourceMark     string            `json:"resourceMark,omitempty"`
	Image            string            `json:"image"`
	ServiceName      string            `json:"serviceName"`
	Points           int               `json:"points"`
	Parallelism      int               `json:"parallelism"`
	TimeoutSec       int               `json:"timeoutSec"`
	EstimatedExecSec *int              `json:"estimatedExecSec,omitempty"`
	SubType          string            `json:"subType,omitempty"`
	RestartPolicy    string            `json:"restartPolicy,omitempty"`
	BackoffLimit     int               `json:"backoffLimit,omitempty"`
	Env              map[string]string `json:"env,omitempty"`
	Command          []string          `json:"command,omitempty"`
	Args             []string          `json:"args,omitempty"`
	ServicePort      int               `json:"servicePort,omitempty"`
}

// JobPage is one vendor page of Job tasks.
type JobPage struct {
	Items    []gongji.JobSummary `json:"items"`
	Total    int64               `json:"total"`
	Page     int                 `json:"page"`
	PageSize int                 `json:"pageSize"`
}

// ListJobs lists Job batch tasks.
func (s *Service) ListJobs(ctx context.Context, status, search string, page, pageSize int) (*JobPage, error) {
	page, pageSize = clampPage(page, pageSize)
	data, err := s.client.SearchJobs(ctx, gongji.JobListFilter{
		Status:      status,
		SearchValue: search,
		Page:        page,
		PageSize:    pageSize,
	})
	if err != nil {
		return nil, err
	}
	return &JobPage{Items: data.Results, Total: data.Count, Page: page, PageSize: pageSize}, nil
}

// GetJob returns the vendor Job document.
func (s *Service) GetJob(ctx context.Context, taskID int64) (json.RawMessage, error) {
	return s.client.GetJob(ctx, taskID)
}

// CreateJob creates a Job batch task on an observed region mark.
func (s *Service) CreateJob(ctx context.Context, in CreateJobInput) (*gongji.CreateTaskData, error) {
	if err := validateJob(in); err != nil {
		return nil, err
	}
	mark, err := s.resolveMark(in.LogicalAzID, in.ProviderSkuID, in.ResourceMark)
	if err != nil {
		return nil, err
	}
	policy := in.RestartPolicy
	if policy == "" {
		policy = "OnFailure"
	}
	backoff := in.BackoffLimit
	if backoff <= 0 {
		backoff = 3
	}
	weight := &gongji.JobResourceWeight{CPUWeight: 1, MemWeight: 1, GPUWeight: 1}
	envs := make([]gongji.JobEnv, 0, len(in.Env))
	for k, v := range in.Env {
		envs = append(envs, gongji.JobEnv{Name: k, Value: v})
	}
	var subType *string
	if in.SubType != "" {
		subType = &in.SubType
	}
	service := gongji.JobService{
		ServiceName:    in.ServiceName,
		ServiceImage:   in.Image,
		ResourceWeight: weight,
		Env:            envs,
	}
	if in.ServicePort > 0 {
		service.RemotePorts = []gongji.RemotePort{{ServicePort: in.ServicePort}}
	}
	if len(in.Command) > 0 || len(in.Args) > 0 {
		service.StartScript = &gongji.JobStartScript{Command: in.Command, Args: in.Args}
	}
	req := &gongji.CreateJobRequest{
		TaskName:  in.TaskName,
		SubType:   subType,
		Resources: []gongji.JobResource{{Mark: mark}},
		Points:    in.Points,
		JobSupport: gongji.JobSupport{
			EstimatedExecSec: in.EstimatedExecSec,
			TimeoutSec:       in.TimeoutSec,
			Parallelism:      in.Parallelism,
			ModParam:         gongji.JobModParam{Default: &gongji.JobModDefault{BackoffLimit: backoff, RestartPolicy: policy}},
		},
		Services: []gongji.JobService{service},
	}
	return s.client.CreateJob(ctx, req)
}

// StopJob stops a Job batch task.
func (s *Service) StopJob(ctx context.Context, taskID int64) error {
	return s.client.StopJob(ctx, taskID)
}

func (s *Service) resolveMark(logicalAzID, sku, explicit string) (string, error) {
	if explicit != "" {
		return explicit, nil
	}
	if logicalAzID == "" {
		return "", fmt.Errorf("logicalAzId or resourceMark is required")
	}
	if s.catalog == nil {
		return "", fmt.Errorf("region catalog is not configured")
	}
	return s.catalog.ResolveMark(logicalAzID, sku)
}

func validateJob(in CreateJobInput) error {
	if strings.TrimSpace(in.TaskName) == "" || strings.TrimSpace(in.Image) == "" || strings.TrimSpace(in.ServiceName) == "" {
		return fmt.Errorf("taskName, image, and serviceName are required")
	}
	if in.Points <= 0 {
		return fmt.Errorf("points must be > 0")
	}
	if in.Parallelism <= 0 {
		return fmt.Errorf("parallelism must be > 0")
	}
	if in.TimeoutSec <= 0 {
		return fmt.Errorf("timeoutSec must be > 0")
	}
	if in.SubType != "" && in.SubType != "Spot" {
		return fmt.Errorf("subType must be empty or Spot")
	}
	if in.SubType == "Spot" && (in.EstimatedExecSec == nil || *in.EstimatedExecSec <= 0) {
		return fmt.Errorf("estimatedExecSec is required for Spot jobs")
	}
	if in.RestartPolicy != "" && in.RestartPolicy != "OnFailure" && in.RestartPolicy != "Never" {
		return fmt.Errorf("restartPolicy must be OnFailure or Never")
	}
	return nil
}

func sanitizeServiceName(name string) string {
	name = strings.ToLower(name)
	name = strings.ReplaceAll(name, "_", "-")
	if len(name) > 48 {
		name = name[:48]
	}
	return name
}

func clampPage(page, pageSize int) (int, int) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = defaultPageSize
	}
	if pageSize > maxPageSize {
		pageSize = maxPageSize
	}
	return page, pageSize
}

func pageSlice[T any](items []T, page, pageSize int) []T {
	if len(items) == 0 {
		return []T{}
	}
	start := (page - 1) * pageSize
	if start >= len(items) {
		return []T{}
	}
	end := start + pageSize
	if end > len(items) {
		end = len(items)
	}
	return items[start:end]
}
