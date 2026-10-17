package router

import (
	"waverless/app/middleware"

	"github.com/gin-gonic/gin"
)

// setupControlRoutes registers gpurun/cloud service-to-service APIs.
func (r *Router) setupControlRoutes(engine *gin.Engine) {
	if r.controlHandler == nil {
		return
	}

	control := engine.Group("/control/v1")
	control.Use(middleware.ControlAuthMiddleware())
	{
		control.POST("/endpoints", r.controlHandler.CreateEndpoint)
		control.GET("/endpoints/:externalEndpointId/runtime", r.controlHandler.GetEndpointRuntime)
		control.GET("/endpoints/:externalEndpointId/workers", r.controlHandler.ListWorkers)
		control.PATCH("/endpoints/:externalEndpointId", r.controlHandler.PatchEndpoint)
		control.POST("/endpoints/:externalEndpointId/scale", r.controlHandler.ScaleEndpoint)
		control.DELETE("/endpoints/:externalEndpointId", r.controlHandler.DeleteEndpoint)
		control.POST("/endpoints/:externalEndpointId/workers/:workerId/terminate", r.controlHandler.TerminateWorker)
		control.GET("/lifecycle-commands/:commandId", r.controlHandler.GetLifecycleCommand)
	}
}
