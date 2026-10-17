package handler

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	gongjisvc "waverless/internal/service/gongji"
	"waverless/pkg/provider/gongji"
)

// GongjiHandler exposes region maintenance and serverless/job lifecycle APIs.
type GongjiHandler struct {
	svc *gongjisvc.Service
}

// NewGongjiHandler creates the Gongji admin handler.
func NewGongjiHandler(svc *gongjisvc.Service) *GongjiHandler {
	return &GongjiHandler{svc: svc}
}

// ListRegions lists logical AZs. The default view omits vendor marks and prices.
func (h *GongjiHandler) ListRegions(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	result := h.svc.ListRegions(
		c.Query("complianceDomain"),
		c.Query("gpuSku"),
		c.DefaultQuery("view", "catalog"),
		c.Query("schedulableOnly") == "true",
		page,
		pageSize,
	)
	c.JSON(http.StatusOK, result)
}

// GetRegistryVersion returns binding and inventory pull version counters.
func (h *GongjiHandler) GetRegistryVersion(c *gin.Context) {
	c.JSON(http.StatusOK, h.svc.RegistryVersion())
}

// ListInventoryPulls lists versioned inventory sync attempts.
func (h *GongjiHandler) ListInventoryPulls(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	items, total, err := h.svc.ListInventoryPulls(c.Request.Context(), page, pageSize)
	if err != nil {
		writeGongjiError(c, err)
		return
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	c.JSON(http.StatusOK, gin.H{
		"items":    items,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
	})
}

// ListResources returns the latest Gongji inventory snapshot for operators.
func (h *GongjiHandler) ListResources(c *gin.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	c.JSON(http.StatusOK, h.svc.ResourceSnapshot(page, pageSize))
}

// RefreshResources pulls inventory now.
func (h *GongjiHandler) RefreshResources(c *gin.Context) {
	if err := h.svc.RefreshInventory(c.Request.Context()); err != nil {
		writeGongjiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "refreshed"})
}

type availabilityRequest struct {
	Availability string `json:"availability"`
	Reason       string `json:"reason"`
}

// SetAvailability changes whether a logical AZ accepts new capacity.
func (h *GongjiHandler) SetAvailability(c *gin.Context) {
	var req availabilityRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.SetAvailability(c.Request.Context(), c.Param("logicalAzId"), req.Availability, req.Reason); err != nil {
		writeGongjiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"logicalAzId": c.Param("logicalAzId"), "availability": req.Availability})
}

// UpsertBinding binds a logical AZ to an observed physical SKU.
func (h *GongjiHandler) UpsertBinding(c *gin.Context) {
	var binding gongji.RegionBinding
	if err := c.ShouldBindJSON(&binding); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.UpsertBinding(binding); err != nil {
		writeGongjiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{"logicalAzId": binding.LogicalAzID, "status": "updated"})
}

// ListServerless lists deployment tasks.
func (h *GongjiHandler) ListServerless(c *gin.Context) {
	page, pageSize := queryPage(c)
	result, err := h.svc.ListServerless(c.Request.Context(), c.Query("status"), c.Query("search"), page, pageSize)
	if err != nil {
		writeGongjiError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetServerless returns one deployment task.
func (h *GongjiHandler) GetServerless(c *gin.Context) {
	taskID, ok := pathTaskID(c)
	if !ok {
		return
	}
	detail, err := h.svc.GetServerless(c.Request.Context(), taskID)
	if err != nil {
		writeGongjiError(c, err)
		return
	}
	c.JSON(http.StatusOK, detail)
}

// CreateServerless creates a deployment task.
func (h *GongjiHandler) CreateServerless(c *gin.Context) {
	var req gongjisvc.CreateServerlessInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	data, err := h.svc.CreateServerless(c.Request.Context(), req)
	if err != nil {
		writeGongjiError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, data)
}

// PauseServerless pauses a deployment.
func (h *GongjiHandler) PauseServerless(c *gin.Context) {
	h.simpleTaskAction(c, h.svc.PauseServerless)
}

// RecoverServerless resumes a paused deployment.
func (h *GongjiHandler) RecoverServerless(c *gin.Context) {
	h.simpleTaskAction(c, h.svc.RecoverServerless)
}

type pointsRequest struct {
	Points int `json:"points"`
}

// ScaleServerless sets absolute desired points.
func (h *GongjiHandler) ScaleServerless(c *gin.Context) {
	taskID, ok := pathTaskID(c)
	if !ok {
		return
	}
	var req pointsRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.ScaleServerless(c.Request.Context(), taskID, req.Points); err != nil {
		writeGongjiError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"taskId": taskID, "points": req.Points, "status": "accepted"})
}

// UpdateServerless replaces a deployment from a full detail document.
func (h *GongjiHandler) UpdateServerless(c *gin.Context) {
	body, err := io.ReadAll(c.Request.Body)
	if err != nil || !json.Valid(body) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "update body must be the full task JSON"})
		return
	}
	if err := h.svc.UpdateServerless(c.Request.Context(), body); err != nil {
		writeGongjiError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"status": "accepted"})
}

// DeleteServerless deletes a deployment task.
func (h *GongjiHandler) DeleteServerless(c *gin.Context) {
	h.simpleTaskAction(c, h.svc.DeleteServerless)
}

// ListPoints lists task points.
func (h *GongjiHandler) ListPoints(c *gin.Context) {
	taskID, ok := pathTaskID(c)
	if !ok {
		return
	}
	page, pageSize := queryPage(c)
	data, err := h.svc.ListPoints(c.Request.Context(), taskID, page, pageSize)
	if err != nil {
		writeGongjiError(c, err)
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"items":    data.Results,
		"total":    data.Count,
		"page":     page,
		"pageSize": pageSize,
	})
}

// DeletePoint deletes one point.
func (h *GongjiHandler) DeletePoint(c *gin.Context) {
	pointID, err := strconv.ParseInt(c.Param("pointId"), 10, 64)
	if err != nil || pointID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid pointId"})
		return
	}
	if err := h.svc.DeletePoint(c.Request.Context(), pointID); err != nil {
		writeGongjiError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"pointId": pointID, "status": "accepted"})
}

// ListJobs lists Job batch tasks.
func (h *GongjiHandler) ListJobs(c *gin.Context) {
	page, pageSize := queryPage(c)
	result, err := h.svc.ListJobs(c.Request.Context(), c.Query("status"), c.Query("search"), page, pageSize)
	if err != nil {
		writeGongjiError(c, err)
		return
	}
	c.JSON(http.StatusOK, result)
}

// GetJob returns one Job task document.
func (h *GongjiHandler) GetJob(c *gin.Context) {
	taskID, ok := pathTaskID(c)
	if !ok {
		return
	}
	raw, err := h.svc.GetJob(c.Request.Context(), taskID)
	if err != nil {
		writeGongjiError(c, err)
		return
	}
	c.Data(http.StatusOK, "application/json", raw)
}

// CreateJob creates a Job batch task.
func (h *GongjiHandler) CreateJob(c *gin.Context) {
	var req gongjisvc.CreateJobInput
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	data, err := h.svc.CreateJob(c.Request.Context(), req)
	if err != nil {
		writeGongjiError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, data)
}

// StopJob stops a Job batch task.
func (h *GongjiHandler) StopJob(c *gin.Context) {
	h.simpleTaskAction(c, h.svc.StopJob)
}

func (h *GongjiHandler) simpleTaskAction(c *gin.Context, fn func(ctx context.Context, taskID int64) error) {
	taskID, ok := pathTaskID(c)
	if !ok {
		return
	}
	if err := fn(c.Request.Context(), taskID); err != nil {
		writeGongjiError(c, err)
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"taskId": taskID, "status": "accepted"})
}

func pathTaskID(c *gin.Context) (int64, bool) {
	taskID, err := strconv.ParseInt(c.Param("taskId"), 10, 64)
	if err != nil || taskID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid taskId"})
		return 0, false
	}
	return taskID, true
}

func queryPage(c *gin.Context) (int, int) {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.Query("pageSize"))
	return page, pageSize
}

func writeGongjiError(c *gin.Context, err error) {
	status := http.StatusBadGateway
	code := gongji.CodeProviderRejected
	if ge, ok := gongji.AsError(err); ok {
		code = ge.Code
		switch ge.Code {
		case gongji.CodeRegionUnavailable, gongji.CodeCapacityExhausted:
			status = http.StatusConflict
		case gongji.CodeProviderRateLimited:
			status = http.StatusTooManyRequests
		case gongji.CodeProviderTimeout:
			status = http.StatusGatewayTimeout
		}
	} else if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(status, gin.H{"code": code, "error": err.Error()})
}
