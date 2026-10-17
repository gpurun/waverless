package config

import "os"

// GatewayConfig controls Traefik dynamic route publishing.
type GatewayConfig struct {
	Enabled           bool   `yaml:"enabled"`
	TraefikDynamicDir string `yaml:"traefik_dynamic_dir"`
	PublicBaseDomain  string `yaml:"public_base_domain"`
	HealthCheckPath   string `yaml:"health_check_path"`
	EdgeAuthMiddleware string `yaml:"edge_auth_middleware"`
}

func DefaultGatewayConfig() GatewayConfig {
	return GatewayConfig{
		Enabled:            false,
		TraefikDynamicDir:  "/etc/traefik/dynamic/generated",
		PublicBaseDomain:   "rungpu.cn",
		HealthCheckPath:    "/health/ready",
		EdgeAuthMiddleware: "serverless-edge-auth@file",
	}
}

func (c *Config) ResolvedGateway() GatewayConfig {
	g := c.Gateway
	if v := os.Getenv("GATEWAY_TRAEFIK_DYNAMIC_DIR"); v != "" {
		g.TraefikDynamicDir = v
		g.Enabled = true
	}
	if os.Getenv("GATEWAY_ENABLED") == "true" {
		g.Enabled = true
	}
	if v := os.Getenv("SERVERLESS_PUBLIC_BASE_DOMAIN"); v != "" {
		g.PublicBaseDomain = v
	}
	def := DefaultGatewayConfig()
	if g.TraefikDynamicDir == "" {
		g.TraefikDynamicDir = def.TraefikDynamicDir
	}
	if g.PublicBaseDomain == "" {
		g.PublicBaseDomain = def.PublicBaseDomain
	}
	if g.HealthCheckPath == "" {
		g.HealthCheckPath = def.HealthCheckPath
	}
	if g.EdgeAuthMiddleware == "" {
		g.EdgeAuthMiddleware = def.EdgeAuthMiddleware
	}
	return g
}
