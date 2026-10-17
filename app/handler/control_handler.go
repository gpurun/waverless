package handler

import (
	"net/http"
	"strconv"
	"strings"

	controlsvc "waverless/internal/service/control"
	"waverless/pkg/logger"

	"github.com/gin-gonic/gin"
)

// ControlHandler serves gpurun/cloud service-to-service /control/v1 APIs.
type ControlHandler struct {
	svc *controlsvc.Service
}

func NewControlHandler(svc *controlsvc.Service) *ControlHandler {
	return &ControlHandler{svc: svc}
}

func (h *ControlHandler) CreateEndpoint(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control service unavailable"})
		return
	}
	var req controlsvc.CreateEndpointRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := h.svc.CreateEndpoint(c.Request.Context(), &req)
	if err != nil {
		logger.ErrorCtx(c.Request.Context(), "control create endpoint failed: %v", err)
		writeControlError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ControlHandler) GetEndpointRuntime(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control service unavailable"})
		return
	}
	externalID := c.Param("externalEndpointId")
	resp, err := h.svc.GetRuntime(c.Request.Context(), externalID)
	if err != nil {
		writeControlError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ControlHandler) ListWorkers(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control service unavailable"})
		return
	}
	externalID := c.Param("externalEndpointId")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	resp, err := h.svc.ListWorkers(c.Request.Context(), externalID, page, pageSize)
	if err != nil {
		writeControlError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ControlHandler) PatchEndpoint(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control service unavailable"})
		return
	}
	externalID := c.Param("externalEndpointId")
	var req controlsvc.PatchEndpointRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.PatchEndpoint(c.Request.Context(), externalID, req); err != nil {
		writeControlError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ControlHandler) ScaleEndpoint(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control service unavailable"})
		return
	}
	externalID := c.Param("externalEndpointId")
	var req controlsvc.ScaleRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.ScaleEndpoint(c.Request.Context(), externalID, req); err != nil {
		writeControlError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ControlHandler) DeleteEndpoint(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control service unavailable"})
		return
	}
	externalID := c.Param("externalEndpointId")
	if err := h.svc.DeleteEndpoint(c.Request.Context(), externalID); err != nil {
		writeControlError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *ControlHandler) TerminateWorker(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control service unavailable"})
		return
	}
	externalID := c.Param("externalEndpointId")
	workerID := c.Param("workerId")
	var req controlsvc.TerminateWorkerRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	resp, err := h.svc.TerminateWorker(c.Request.Context(), externalID, workerID, req)
	if err != nil {
		writeControlError(c, err)
		return
	}
	c.JSON(http.StatusOK, resp)
}

func (h *ControlHandler) GetLifecycleCommand(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "control service unavailable"})
		return
	}
	commandID := c.Param("commandId")
	resp := h.svc.GetLifecycleCommand(commandID)
	if resp == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "command not found"})
		return
	}
	c.JSON(http.StatusOK, resp)
}

func writeControlError(c *gin.Context, err error) {
	if err == nil {
		return
	}
	msg := err.Error()
	if strings.Contains(strings.ToLower(msg), "not found") {
		c.JSON(http.StatusNotFound, gin.H{"error": msg})
		return
	}
	if strings.Contains(strings.ToLower(msg), "required") {
		c.JSON(http.StatusBadRequest, gin.H{"error": msg})
		return
	}
	c.JSON(http.StatusInternalServerError, gin.H{"error": msg})
}
