package middleware

import (
	"net/http"
	"strings"

	"waverless/pkg/config"
	"waverless/pkg/logger"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// ControlAuthMiddleware authenticates gpurun/cloud service-to-service control API calls.
func ControlAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		expected := ""
		if config.GlobalConfig != nil {
			expected = config.GlobalConfig.ControlServiceAPIKey()
		}
		if expected == "" {
			logger.WarnCtx(c.Request.Context(), "control API rejected: service API key not configured")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized", "code": "CONTROL_AUTH_NOT_CONFIGURED"})
			c.Abort()
			return
		}

		authHeader := strings.TrimSpace(c.GetHeader("Authorization"))
		authHeader = strings.TrimPrefix(authHeader, "Bearer ")
		authHeader = strings.TrimSpace(authHeader)
		if authHeader == "" || authHeader != expected {
			logger.WarnCtx(c.Request.Context(), "control API unauthorized request")
			c.JSON(http.StatusUnauthorized, gin.H{"error": "unauthorized"})
			c.Abort()
			return
		}

		requestID := strings.TrimSpace(c.GetHeader("X-Request-Id"))
		if requestID == "" {
			requestID = strings.TrimSpace(c.GetHeader("X-Request-ID"))
		}
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Set("control_request_id", requestID)
		c.Next()
	}
}
