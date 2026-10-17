package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"waverless/pkg/config"

	"github.com/gin-gonic/gin"
)

func TestControlAuthMiddleware_RejectsMissingKey(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config.GlobalConfig = &config.Config{
		Server: config.ServerConfig{},
	}

	r := gin.New()
	r.Use(ControlAuthMiddleware())
	r.GET("/control/v1/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/control/v1/ping", nil)
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestControlAuthMiddleware_AcceptsBearer(t *testing.T) {
	gin.SetMode(gin.TestMode)
	config.GlobalConfig = &config.Config{
		Server: config.ServerConfig{ServiceAPIKey: "svc-secret"},
	}

	r := gin.New()
	r.Use(ControlAuthMiddleware())
	r.GET("/control/v1/ping", func(c *gin.Context) { c.Status(http.StatusOK) })

	req := httptest.NewRequest(http.MethodGet, "/control/v1/ping", nil)
	req.Header.Set("Authorization", "Bearer svc-secret")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
}
