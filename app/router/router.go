package router

import (
	"waverless/app/handler"
	"waverless/app/middleware"
	_ "waverless/docs"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
)

// Handlers aggregates all handlers
type Handlers struct {
	Task        *handler.TaskHandler
	Worker      *handler.WorkerHandler
	Endpoint    *handler.EndpointHandler
	Autoscaler  *handler.AutoScalerHandler
	Statistics  *handler.StatisticsHandler
	Spec        *handler.SpecHandler
	Image       *handler.ImageHandler
	Monitoring  *handler.MonitoringHandler
	StatusEvent *handler.StatusEventHandler
	Portal      *handler.PortalHandler
	Control     *handler.ControlHandler
}

// Router is the main router structure
type Router struct {
	handlers *Handlers
	// Keep old fields for backward compatibility
	taskHandler        *handler.TaskHandler
	workerHandler      *handler.WorkerHandler
	endpointHandler    *handler.EndpointHandler
	autoscalerHandler  *handler.AutoScalerHandler
	statisticsHandler  *handler.StatisticsHandler
	specHandler        *handler.SpecHandler
	imageHandler       *handler.ImageHandler
	monitoringHandler  *handler.MonitoringHandler
	statusEventHandler *handler.StatusEventHandler
	portalHandler      *handler.PortalHandler
	controlHandler     *handler.ControlHandler
	gongjiHandler      *handler.GongjiHandler
}

// NewRouter creates a new Router
func NewRouter(
	taskHandler *handler.TaskHandler,
	workerHandler *handler.WorkerHandler,
	endpointHandler *handler.EndpointHandler,
	autoscalerHandler *handler.AutoScalerHandler,
	statisticsHandler *handler.StatisticsHandler,
	specHandler *handler.SpecHandler,
	imageHandler *handler.ImageHandler,
	monitoringHandler *handler.MonitoringHandler,
	statusEventHandler *handler.StatusEventHandler,
	portalHandler *handler.PortalHandler,
	controlHandler *handler.ControlHandler,
) *Router {
	handlers := &Handlers{
		Task:        taskHandler,
		Worker:      workerHandler,
		Endpoint:    endpointHandler,
		Autoscaler:  autoscalerHandler,
		Statistics:  statisticsHandler,
		Spec:        specHandler,
		Image:       imageHandler,
		Monitoring:  monitoringHandler,
		StatusEvent: statusEventHandler,
		Portal:      portalHandler,
		Control:     controlHandler,
	}

	return &Router{
		handlers:           handlers,
		taskHandler:        taskHandler,
		workerHandler:      workerHandler,
		endpointHandler:    endpointHandler,
		autoscalerHandler:  autoscalerHandler,
		statisticsHandler:  statisticsHandler,
		specHandler:        specHandler,
		imageHandler:       imageHandler,
		monitoringHandler:  monitoringHandler,
		statusEventHandler: statusEventHandler,
		portalHandler:      portalHandler,
		controlHandler:     controlHandler,
	}
}

// SetGongjiHandler registers Gongji region and task routes when the provider is enabled.
func (r *Router) SetGongjiHandler(h *handler.GongjiHandler) {
	if r != nil {
		r.gongjiHandler = h
	}
}

// Setup configures all routes
func (r *Router) Setup(engine *gin.Engine) {
	// Global middleware
	engine.Use(middleware.Recovery())
	engine.Use(middleware.Logger())

	// Register route groups
	r.setupV1Routes(engine)     // Client API (/v1/*)
	r.setupV2Routes(engine)     // Worker API (/v2/*)
	r.setupAdminRoutes(engine)  // Admin API (/api/v1/*)
	r.setupPortalRoutes(engine) // Portal Southbound API (/portal/v1/*)
	r.setupControlRoutes(engine) // Cloud control API (/control/v1/*)
	r.setupHealthRoutes(engine) // Health check
	r.setupGongjiRoutes(engine)
	r.setupControlRoutes(engine)

	// Swagger API documentation UI
	engine.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))
}
