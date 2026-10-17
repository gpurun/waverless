package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	imodel "waverless/internal/model"
	"waverless/internal/service"
	endpointsvc "waverless/internal/service/endpoint"
	"waverless/pkg/interfaces"
	"waverless/pkg/logger"
	"waverless/pkg/placement"
	"waverless/pkg/store/mysql"
	storemodel "waverless/pkg/store/mysql/model"
)

// CreateEndpointRequest mirrors the Cloud /control/v1 wire contract (camelCase JSON).
type CreateEndpointRequest struct {
	ExternalEndpointID string            `json:"externalEndpointId"`
	Name               string            `json:"name"`
	Image              string            `json:"image"`
	LogicalImage       string            `json:"logicalImage"`
	PlacementMode      string            `json:"placementMode"`
	RegionAllowlist    []string          `json:"regionAllowlist"`
	LogicalAzID        string            `json:"logicalAzId"`
	ProviderID         string            `json:"providerId"`
	PhysicalRegionID   string            `json:"physicalRegionId"`
	ProviderSkuID      string            `json:"providerSkuId"`
	MinReplicas        int               `json:"minReplicas"`
	MaxReplicas        int               `json:"maxReplicas"`
	ScaleUpWaitSeconds int               `json:"scaleUpWaitSeconds"`
	TargetPort         *int              `json:"targetPort"`
	Command            string            `json:"command"`
	Env                map[string]string `json:"env"`
	GpuSku             string            `json:"gpuSku"`
	GpuType            string            `json:"gpuType"`
	GpuCount           *int              `json:"gpuCount"`
	BillingMode        string            `json:"billingMode"`
	Actor              map[string]string `json:"actor"`
}

type ActiveSlice struct {
	ProviderID       string   `json:"providerId,omitempty"`
	PhysicalRegionID string   `json:"physicalRegionId,omitempty"`
	Region           string   `json:"region,omitempty"`
	LogicalAzID      string   `json:"logicalAzId,omitempty"`
	ProviderSkuID    string   `json:"providerSkuId,omitempty"`
	SkuID            string   `json:"skuId,omitempty"`
	SliceID          int64    `json:"sliceId,omitempty"`
	UnitCostSnapshot *float64 `json:"unitCostSnapshot,omitempty"`
	Status           string   `json:"status,omitempty"`
}

type CreateEndpointResponse struct {
	ExternalEndpointID   string       `json:"externalEndpointId"`
	WaverlessEndpointID  string       `json:"waverlessEndpointId,omitempty"`
	ProviderDeploymentID string       `json:"providerDeploymentId,omitempty"`
	Status               string       `json:"status"`
	PublicURL            string       `json:"publicUrl,omitempty"`
	DesiredReplicas      int          `json:"desiredReplicas,omitempty"`
	ObservedReplicas     int          `json:"observedReplicas,omitempty"`
	ReadyReplicas        int          `json:"readyReplicas,omitempty"`
	ActiveSlice          *ActiveSlice `json:"activeSlice,omitempty"`
}

type EndpointRuntime struct {
	ExternalEndpointID string       `json:"externalEndpointId"`
	Status             string       `json:"status"`
	LogicalAzID        string       `json:"logicalAzId"`
	ProviderID         string       `json:"providerId"`
	PhysicalRegionID   string       `json:"physicalRegionId,omitempty"`
	DesiredReplicas    int          `json:"desiredReplicas"`
	ObservedReplicas   int          `json:"observedReplicas"`
	ReadyReplicas      int          `json:"readyReplicas"`
	FailedReplicas     int          `json:"failedReplicas,omitempty"`
	PublicURL          string       `json:"publicUrl,omitempty"`
	ActiveSlice        *ActiveSlice `json:"activeSlice,omitempty"`
	GpuType            string       `json:"gpuType,omitempty"`
	BillingMode        string       `json:"billingMode,omitempty"`
	PlacementMode      string       `json:"placementMode,omitempty"`
	RegionAllowlist    []string     `json:"regionAllowlist,omitempty"`
}

type Worker struct {
	WorkerID           string   `json:"workerId"`
	EndpointID         string   `json:"endpointId"`
	Status             string   `json:"status"`
	LogicalAzID        string   `json:"logicalAzId"`
	ProviderID         string   `json:"providerId"`
	PhysicalRegionID   string   `json:"physicalRegionId,omitempty"`
	SkuID              string   `json:"skuId,omitempty"`
	SliceID            int64    `json:"sliceId,omitempty"`
	UnitCostSnapshot   *float64 `json:"unitCostSnapshot,omitempty"`
	CurrentJobs        int      `json:"currentJobs"`
	LastHeartbeatAt    *string  `json:"lastHeartbeatAt"`
	CreatedAt          string   `json:"createdAt,omitempty"`
	ReadyAt            *string  `json:"readyAt"`
	TerminatedAt       *string  `json:"terminatedAt"`
	Failure            *Failure `json:"failure"`
}

type Failure struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

type WorkerList struct {
	Items    []Worker `json:"items"`
	Total    int      `json:"total"`
	Page     int      `json:"page"`
	PageSize int      `json:"pageSize"`
}

type ScaleRequest struct {
	MinReplicas        int  `json:"minReplicas"`
	MaxReplicas        int  `json:"maxReplicas"`
	ScaleUpWaitSeconds *int `json:"scaleUpWaitSeconds"`
}

type PatchEndpointRequest struct {
	RegionAllowlist    []string `json:"regionAllowlist"`
	MinReplicas        *int     `json:"minReplicas"`
	MaxReplicas        *int     `json:"maxReplicas"`
	ScaleUpWaitSeconds *int     `json:"scaleUpWaitSeconds"`
}

type TerminateWorkerRequest struct {
	IdempotencyKey       string            `json:"idempotencyKey"`
	ReasonCode           string            `json:"reasonCode"`
	Reason               string            `json:"reason"`
	Actor                map[string]string `json:"actor"`
	Source               string            `json:"source"`
	RequestID            string            `json:"requestId"`
	DrainTimeoutSeconds  int               `json:"drainTimeoutSeconds"`
	InFlightPolicy       string            `json:"inFlightPolicy"`
}

type LifecycleCommandResponse struct {
	CommandID          string `json:"commandId"`
	State              string `json:"state"`
	WorkerID           string `json:"workerId,omitempty"`
	ExternalEndpointID string `json:"externalEndpointId,omitempty"`
}

type endpointStore interface {
	GetByExternalEndpointID(ctx context.Context, externalID string) (*mysql.Endpoint, error)
	Get(ctx context.Context, endpointName string) (*mysql.Endpoint, error)
	UpdatePlacementFields(ctx context.Context, endpointName string, updates map[string]interface{}) error
	UpdateReplicas(ctx context.Context, endpointName string, replicas int) error
}

type sliceStore interface {
	GetActiveByEndpoint(ctx context.Context, endpoint string) (*storemodel.EndpointProviderSlice, error)
	ListActiveByEndpoint(ctx context.Context, endpoint string) ([]storemodel.EndpointProviderSlice, error)
}

// Service implements control-plane orchestration for gpurun/cloud.
type Service struct {
	endpointRepo   endpointStore
	sliceRepo      sliceStore
	endpointSvc    *endpointsvc.Service
	workerSvc      *service.WorkerService
	deployProvider interfaces.DeploymentProvider
	placement      *placement.Service
	lifecycle      *LifecycleStore
	baseURL        string
}

func NewService(
	endpointRepo endpointStore,
	sliceRepo sliceStore,
	endpointSvc *endpointsvc.Service,
	workerSvc *service.WorkerService,
	deployProvider interfaces.DeploymentProvider,
	placementSvc *placement.Service,
	baseURL string,
) *Service {
	return &Service{
		endpointRepo:   endpointRepo,
		sliceRepo:      sliceRepo,
		endpointSvc:    endpointSvc,
		workerSvc:      workerSvc,
		deployProvider: deployProvider,
		placement:      placementSvc,
		lifecycle:      NewLifecycleStore(),
		baseURL:        strings.TrimRight(strings.TrimSpace(baseURL), "/"),
	}
}

func (s *Service) LifecycleStore() *LifecycleStore {
	return s.lifecycle
}

func (s *Service) CreateEndpoint(ctx context.Context, req *CreateEndpointRequest) (*CreateEndpointResponse, error) {
	if req == nil {
		return nil, fmt.Errorf("request is required")
	}
	ext := strings.TrimSpace(req.ExternalEndpointID)
	if ext == "" {
		return nil, fmt.Errorf("externalEndpointId is required")
	}
	if strings.TrimSpace(req.Image) == "" {
		return nil, fmt.Errorf("image is required")
	}

	if s.endpointRepo != nil {
		existing, err := s.endpointRepo.GetByExternalEndpointID(ctx, ext)
		if err != nil {
			return nil, err
		}
		if existing != nil {
			return s.buildCreateResponse(ctx, existing)
		}
	}

	internalName := internalEndpointName(ext)
	gpuType := strings.TrimSpace(req.GpuType)
	if gpuType == "" {
		gpuType = strings.TrimSpace(req.GpuSku)
	}
	if gpuType == "" {
		return nil, fmt.Errorf("gpuType is required for placement")
	}
	gpuCount := 1
	if req.GpuCount != nil && *req.GpuCount > 0 {
		gpuCount = *req.GpuCount
	}

	mode := strings.ToLower(strings.TrimSpace(req.PlacementMode))
	if mode == "" {
		mode = storemodel.PlacementModeAuto
	}
	billing := strings.TrimSpace(req.BillingMode)
	if billing == "" {
		billing = "on_demand"
	}

	replicas := req.MinReplicas
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

	logicalImage := strings.TrimSpace(req.LogicalImage)
	if logicalImage == "" {
		logicalImage = req.Image
	}

	labels := map[string]string{
		"external_endpoint_id": ext,
		"endpoint_mode":        "http_service",
	}
	if req.ProviderSkuID != "" {
		labels["provider_sku_id"] = req.ProviderSkuID
	}
	if req.LogicalAzID != "" {
		labels["logical_az_id"] = req.LogicalAzID
	}
	if req.ProviderID != "" {
		labels["providerId"] = req.ProviderID
	}
	if req.PhysicalRegionID != "" {
		labels["physical_region_id"] = req.PhysicalRegionID
	}
	if req.TargetPort != nil && *req.TargetPort > 0 {
		labels["gongji_worker_port"] = fmt.Sprintf("%d", *req.TargetPort)
	}
	if req.Actor != nil {
		if tid := req.Actor["tenantId"]; tid != "" {
			labels["tenant_id"] = tid
		}
	}

	skipValidate := false
	deployReq := &interfaces.DeployRequest{
		Endpoint:           internalName,
		SpecName:           gpuType,
		Image:              req.Image,
		Replicas:           replicas,
		MinReplicas:        req.MinReplicas,
		MaxReplicas:        maxReplicas,
		GpuCount:           gpuCount,
		Env:                req.Env,
		Labels:             labels,
		ValidateImage:      &skipValidate,
		GpuType:            gpuType,
		BillingMode:        billing,
		LogicalImage:       logicalImage,
		PlacementMode:      mode,
		RegionAllowlist:    req.RegionAllowlist,
		PinnedProviderID:   strings.TrimSpace(req.ProviderID),
		ExternalEndpointID: ext,
	}

	meta := &interfaces.EndpointMetadata{
		Name:               internalName,
		DisplayName:        strings.TrimSpace(req.Name),
		SpecName:           gpuType,
		Image:              req.Image,
		Replicas:           replicas,
		GpuCount:           gpuCount,
		Env:                req.Env,
		MinReplicas:        req.MinReplicas,
		MaxReplicas:        maxReplicas,
		ScaleUpWaitSeconds: req.ScaleUpWaitSeconds,
		Status:             "Deploying",
		Labels:             labels,
	}

	if s.endpointSvc == nil {
		return nil, fmt.Errorf("endpoint service not configured")
	}
	if _, err := s.endpointSvc.Deploy(ctx, deployReq, meta); err != nil {
		return nil, err
	}

	placementUpdates := map[string]interface{}{
		"external_endpoint_id": ext,
		"placement_mode":       mode,
		"gpu_type":             gpuType,
		"billing_mode":         billing,
		"logical_image":        logicalImage,
		"endpoint_mode":        "http_service",
	}
	if len(req.RegionAllowlist) > 0 {
		placementUpdates["region_allowlist"] = storemodel.JSONStringArray(req.RegionAllowlist)
	}
	if s.endpointRepo != nil {
		_ = s.endpointRepo.UpdatePlacementFields(ctx, internalName, placementUpdates)
	}

	ep, err := s.endpointRepo.Get(ctx, internalName)
	if err != nil {
		return nil, err
	}
	return s.buildCreateResponse(ctx, ep)
}

func (s *Service) GetRuntime(ctx context.Context, externalID string) (*EndpointRuntime, error) {
	ep, err := s.resolveEndpoint(ctx, externalID)
	if err != nil {
		return nil, err
	}
	return s.buildRuntime(ctx, ep)
}

func (s *Service) ListWorkers(ctx context.Context, externalID string, page, pageSize int) (*WorkerList, error) {
	ep, err := s.resolveEndpoint(ctx, externalID)
	if err != nil {
		return nil, err
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}

	if s.workerSvc == nil {
		return &WorkerList{Items: []Worker{}, Total: 0, Page: page, PageSize: pageSize}, nil
	}
	workers, err := s.workerSvc.ListWorkersForSync(ctx, ep.Endpoint)
	if err != nil {
		return nil, err
	}

	slice, _ := s.activeSlice(ctx, ep.Endpoint)
	total := len(workers)
	start := (page - 1) * pageSize
	if start > total {
		start = total
	}
	end := start + pageSize
	if end > total {
		end = total
	}

	items := make([]Worker, 0, end-start)
	for _, w := range workers[start:end] {
		items = append(items, mapWorker(w, ep.ExternalEndpointID, slice))
	}
	return &WorkerList{Items: items, Total: total, Page: page, PageSize: pageSize}, nil
}

func (s *Service) PatchEndpoint(ctx context.Context, externalID string, patch PatchEndpointRequest) error {
	ep, err := s.resolveEndpoint(ctx, externalID)
	if err != nil {
		return err
	}
	meta, err := s.endpointSvc.GetEndpoint(ctx, ep.Endpoint)
	if err != nil || meta == nil {
		return fmt.Errorf("endpoint not found")
	}
	if patch.MinReplicas != nil {
		meta.MinReplicas = *patch.MinReplicas
	}
	if patch.MaxReplicas != nil {
		meta.MaxReplicas = *patch.MaxReplicas
	}
	if patch.ScaleUpWaitSeconds != nil {
		meta.ScaleUpWaitSeconds = *patch.ScaleUpWaitSeconds
	}
	if err := s.endpointSvc.SaveEndpoint(ctx, meta); err != nil {
		return err
	}
	if patch.RegionAllowlist != nil && s.endpointRepo != nil {
		_ = s.endpointRepo.UpdatePlacementFields(ctx, ep.Endpoint, map[string]interface{}{
			"region_allowlist": storemodel.JSONStringArray(patch.RegionAllowlist),
		})
	}
	return nil
}

func (s *Service) ScaleEndpoint(ctx context.Context, externalID string, req ScaleRequest) error {
	ep, err := s.resolveEndpoint(ctx, externalID)
	if err != nil {
		return err
	}
	meta, err := s.endpointSvc.GetEndpoint(ctx, ep.Endpoint)
	if err != nil || meta == nil {
		return fmt.Errorf("endpoint not found")
	}
	meta.MinReplicas = req.MinReplicas
	meta.MaxReplicas = req.MaxReplicas
	if req.ScaleUpWaitSeconds != nil {
		meta.ScaleUpWaitSeconds = *req.ScaleUpWaitSeconds
	}
	if err := s.endpointSvc.SaveEndpoint(ctx, meta); err != nil {
		return err
	}

	desired := ep.Replicas
	if desired < req.MinReplicas {
		desired = req.MinReplicas
	}
	if req.MaxReplicas >= 0 && desired > req.MaxReplicas {
		desired = req.MaxReplicas
	}
	if s.deployProvider != nil && desired != ep.Replicas {
		if err := s.deployProvider.ScaleApp(ctx, ep.Endpoint, desired); err != nil {
			return err
		}
		_ = s.endpointRepo.UpdateReplicas(ctx, ep.Endpoint, desired)
	}
	return nil
}

func (s *Service) DeleteEndpoint(ctx context.Context, externalID string) error {
	ep, err := s.resolveEndpoint(ctx, externalID)
	if err != nil {
		return err
	}
	if s.endpointSvc == nil {
		return fmt.Errorf("endpoint service not configured")
	}
	return s.endpointSvc.DeleteDeployment(ctx, ep.Endpoint)
}

func (s *Service) TerminateWorker(ctx context.Context, externalID, workerID string, req TerminateWorkerRequest) (*LifecycleCommandResponse, error) {
	ep, err := s.resolveEndpoint(ctx, externalID)
	if err != nil {
		return nil, err
	}
	cmd := s.lifecycle.Create(workerID, externalID)

	go s.runTerminate(context.Background(), ep.Endpoint, workerID, req, cmd.CommandID)

	return &LifecycleCommandResponse{
		CommandID:          cmd.CommandID,
		State:              string(CommandAccepted),
		WorkerID:           workerID,
		ExternalEndpointID: externalID,
	}, nil
}

func (s *Service) GetLifecycleCommand(commandID string) *LifecycleCommandResponse {
	cmd := s.lifecycle.Get(commandID)
	if cmd == nil {
		return nil
	}
	return &LifecycleCommandResponse{
		CommandID:          cmd.CommandID,
		State:              string(cmd.State),
		WorkerID:           cmd.WorkerID,
		ExternalEndpointID: cmd.ExternalEndpointID,
	}
}

func (s *Service) runTerminate(ctx context.Context, endpoint, workerID string, req TerminateWorkerRequest, commandID string) {
	s.lifecycle.UpdateState(commandID, CommandDraining)
	if s.workerSvc != nil {
		_ = s.workerSvc.UpdateWorkerStatus(ctx, workerID, imodel.WorkerStatusDraining)
	}
	s.lifecycle.UpdateState(commandID, CommandTerminating)

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		reason = strings.TrimSpace(req.ReasonCode)
	}
	if reason == "" {
		reason = "control_terminate"
	}

	var err error
	if s.deployProvider != nil {
		if terminator, ok := s.deployProvider.(interfaces.WorkerTerminator); ok {
			err = terminator.TerminateWorker(ctx, endpoint, workerID, reason)
		} else {
			err = fmt.Errorf("provider does not support worker termination")
		}
	} else {
		err = fmt.Errorf("deployment provider not configured")
	}

	if err != nil {
		logger.WarnCtx(ctx, "control terminate worker failed endpoint=%s worker=%s: %v", endpoint, workerID, err)
		s.lifecycle.UpdateState(commandID, CommandFailed)
		return
	}
	s.lifecycle.UpdateState(commandID, CommandSucceeded)
}

func (s *Service) resolveEndpoint(ctx context.Context, externalID string) (*mysql.Endpoint, error) {
	if s.endpointRepo == nil {
		return nil, fmt.Errorf("endpoint repository not configured")
	}
	ep, err := s.endpointRepo.GetByExternalEndpointID(ctx, externalID)
	if err != nil {
		return nil, err
	}
	if ep == nil {
		return nil, fmt.Errorf("endpoint not found")
	}
	return ep, nil
}

func (s *Service) buildCreateResponse(ctx context.Context, ep *mysql.Endpoint) (*CreateEndpointResponse, error) {
	runtime, err := s.buildRuntime(ctx, ep)
	if err != nil {
		return nil, err
	}
	resp := &CreateEndpointResponse{
		ExternalEndpointID:  strings.TrimSpace(ep.ExternalEndpointID),
		WaverlessEndpointID: ep.Endpoint,
		Status:              runtime.Status,
		PublicURL:           runtime.PublicURL,
		DesiredReplicas:     runtime.DesiredReplicas,
		ObservedReplicas:    runtime.ObservedReplicas,
		ReadyReplicas:       runtime.ReadyReplicas,
		ActiveSlice:         runtime.ActiveSlice,
	}
	if slice, _ := s.activeSlice(ctx, ep.Endpoint); slice != nil && slice.ProviderRef != "" {
		resp.ProviderDeploymentID = slice.ProviderRef
	}
	return resp, nil
}

func (s *Service) buildRuntime(ctx context.Context, ep *mysql.Endpoint) (*EndpointRuntime, error) {
	if ep == nil {
		return nil, fmt.Errorf("endpoint is nil")
	}
	ext := strings.TrimSpace(ep.ExternalEndpointID)
	desired := ep.Replicas
	observed := desired
	ready := 0
	status := ep.Status
	if status == "" {
		status = "active"
	}

	if s.deployProvider != nil {
		if st, err := s.deployProvider.GetAppStatus(ctx, ep.Endpoint); err == nil && st != nil {
			observed = int(st.TotalReplicas)
			ready = int(st.ReadyReplicas)
			if st.Status != "" {
				status = st.Status
			}
		}
	} else if ep.RuntimeState != nil {
		if rr, ok := ep.RuntimeState["readyReplicas"].(float64); ok {
			ready = int(rr)
		}
	}

	slice, _ := s.activeSlice(ctx, ep.Endpoint)
	active := sliceToActive(slice)
	providerID := ""
	logicalAz := ""
	physicalRegion := ""
	if active != nil {
		providerID = active.ProviderID
		logicalAz = active.LogicalAzID
		physicalRegion = active.PhysicalRegionID
	}
	if providerID == "" && ep.Labels != nil {
		if v, ok := ep.Labels["providerId"].(string); ok {
			providerID = v
		}
	}

	allowlist := []string{}
	if len(ep.RegionAllowlist) > 0 {
		allowlist = append(allowlist, ep.RegionAllowlist...)
	}

	return &EndpointRuntime{
		ExternalEndpointID: ext,
		Status:             status,
		LogicalAzID:        logicalAz,
		ProviderID:         providerID,
		PhysicalRegionID:   physicalRegion,
		DesiredReplicas:    desired,
		ObservedReplicas:   observed,
		ReadyReplicas:      ready,
		ActiveSlice:        active,
		GpuType:            ep.GPUType,
		BillingMode:        ep.BillingMode,
		PlacementMode:      ep.PlacementMode,
		RegionAllowlist:    allowlist,
		PublicURL:          s.publicURL(ep.Endpoint),
	}, nil
}

func (s *Service) activeSlice(ctx context.Context, endpoint string) (*storemodel.EndpointProviderSlice, error) {
	if s.sliceRepo == nil {
		return nil, fmt.Errorf("slice repository not configured")
	}
	if epRow, err := s.endpointRepo.Get(ctx, endpoint); err == nil && epRow != nil && epRow.ActiveSliceID != nil {
		// Best-effort: list active slices and match id
		slices, err := s.sliceRepo.ListActiveByEndpoint(ctx, endpoint)
		if err == nil {
			for _, sl := range slices {
				if sl.ID == *epRow.ActiveSliceID {
					copy := sl
					return &copy, nil
				}
			}
		}
	}
	sl, err := s.sliceRepo.GetActiveByEndpoint(ctx, endpoint)
	if err != nil {
		return nil, err
	}
	return sl, nil
}

func sliceToActive(sl *storemodel.EndpointProviderSlice) *ActiveSlice {
	if sl == nil {
		return nil
	}
	out := &ActiveSlice{
		ProviderID:       sl.ProviderID,
		PhysicalRegionID: sl.PhysicalRegion,
		Region:           sl.PhysicalRegion,
		LogicalAzID:      sl.LogicalAzID,
		ProviderSkuID:    sl.ProviderSkuID,
		SkuID:            sl.ProviderSkuID,
		SliceID:          sl.ID,
		UnitCostSnapshot: sl.UnitCostSnapshot,
		Status:           sl.Status,
	}
	return out
}

func mapWorker(w *storemodel.Worker, externalEndpointID string, slice *storemodel.EndpointProviderSlice) Worker {
	status := mapWorkerStatus(w.Status)
	var lastHB *string
	if !w.LastHeartbeat.IsZero() {
		v := w.LastHeartbeat.UTC().Format(time.RFC3339)
		lastHB = &v
	}
	var readyAt *string
	if w.PodReadyAt != nil {
		v := w.PodReadyAt.UTC().Format(time.RFC3339)
		readyAt = &v
	}
	var terminatedAt *string
	if w.TerminatedAt != nil {
		v := w.TerminatedAt.UTC().Format(time.RFC3339)
		terminatedAt = &v
	}
	var failure *Failure
	if w.FailureType != "" || w.FailureReason != "" {
		failure = &Failure{Code: w.FailureType, Message: w.FailureReason}
	}

	out := Worker{
		WorkerID:         w.WorkerID,
		EndpointID:       externalEndpointID,
		Status:           status,
		CurrentJobs:      w.CurrentJobs,
		LastHeartbeatAt:  lastHB,
		CreatedAt:        w.CreatedAt.UTC().Format(time.RFC3339),
		ReadyAt:          readyAt,
		TerminatedAt:     terminatedAt,
		Failure:          failure,
	}
	if slice != nil {
		out.LogicalAzID = slice.LogicalAzID
		out.ProviderID = slice.ProviderID
		out.PhysicalRegionID = slice.PhysicalRegion
		out.SkuID = slice.ProviderSkuID
		out.SliceID = slice.ID
		out.UnitCostSnapshot = slice.UnitCostSnapshot
	}
	return out
}

func mapWorkerStatus(status string) string {
	switch strings.ToUpper(strings.TrimSpace(status)) {
	case "ONLINE":
		return "READY"
	case "STARTING":
		return "STARTING"
	case "BUSY":
		return "BUSY"
	case "DRAINING":
		return "DRAINING"
	case "OFFLINE":
		return "TERMINATED"
	default:
		if status == "" {
			return "UNKNOWN"
		}
		return strings.ToUpper(status)
	}
}

func internalEndpointName(externalID string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(externalID)))
	return "cbx-" + hex.EncodeToString(sum[:8])
}

func (s *Service) publicURL(endpoint string) string {
	if s.baseURL == "" {
		return ""
	}
	return s.baseURL + "/v1/endpoints/" + endpoint
}

// PreviewPlacementDecision exposes placement selection for tests (auto mode, no providerSkuId).
func PreviewPlacementDecision(ctx context.Context, placementSvc *placement.Service, req placement.Request, providers []string) (placement.Decision, error) {
	if placementSvc == nil {
		return placement.Decision{}, fmt.Errorf("placement not configured")
	}
	return placementSvc.Select(ctx, req, providers)
}

// BuildActiveSliceFromDecision maps placement output to wire activeSlice.
func BuildActiveSliceFromDecision(dec placement.Decision) *ActiveSlice {
	if dec.ProviderID == "" {
		return nil
	}
	unit := dec.UnitCost
	return &ActiveSlice{
		ProviderID:       dec.ProviderID,
		PhysicalRegionID: dec.PhysicalRegion,
		Region:           dec.PhysicalRegion,
		LogicalAzID:      dec.LogicalAzID,
		ProviderSkuID:    dec.ProviderSkuID,
		SkuID:            dec.ProviderSkuID,
		UnitCostSnapshot: &unit,
		Status:           storemodel.SliceStatusActive,
	}
}

// DecodeRegionAllowlist helps tests assert JSON round-trips.
func DecodeRegionAllowlist(raw json.RawMessage) ([]string, error) {
	var out []string
	if len(raw) == 0 {
		return out, nil
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, err
	}
	return out, nil
}
