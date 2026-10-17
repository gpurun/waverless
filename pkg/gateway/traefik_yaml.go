package gateway

import (
	"fmt"
	"sort"
	"strings"
)

// EndpointRoute is one http_service Traefik router + load balancer pool.
type EndpointRoute struct {
	RouteID          string
	PublicHost       string
	Backends         []string
	HealthCheckPath  string
	EdgeAuthMiddleware string
	CertResolver     string
}

// RenderDynamicFile builds a Traefik file provider YAML document for many endpoints.
func RenderDynamicFile(routes []EndpointRoute) string {
	if len(routes) == 0 {
		return "http:\n  routers: {}\n  services: {}\n"
	}
	sort.Slice(routes, func(i, j int) bool {
		return routes[i].RouteID < routes[j].RouteID
	})
	var b strings.Builder
	b.WriteString("http:\n  routers:\n")
	for _, r := range routes {
		if r.PublicHost == "" || len(r.Backends) == 0 {
			continue
		}
		mw := strings.TrimSpace(r.EdgeAuthMiddleware)
		if mw == "" {
			mw = "default-headers@file"
		} else {
			mw = mw + ",default-headers@file"
		}
		b.WriteString(fmt.Sprintf("    %s:\n", sanitizeID(r.RouteID)))
		b.WriteString(fmt.Sprintf("      rule: \"Host(`%s`)\"\n", escapeHost(r.PublicHost)))
		b.WriteString("      entryPoints:\n        - websecure\n")
		b.WriteString(fmt.Sprintf("      middlewares:\n        - %s\n", mw))
		b.WriteString(fmt.Sprintf("      service: %s-pool\n", sanitizeID(r.RouteID)))
		if cr := strings.TrimSpace(r.CertResolver); cr != "" {
			b.WriteString("      tls:\n")
			b.WriteString(fmt.Sprintf("        certResolver: %s\n", cr))
		} else {
			b.WriteString("      tls: {}\n")
		}
	}
	b.WriteString("  services:\n")
	for _, r := range routes {
		if r.PublicHost == "" || len(r.Backends) == 0 {
			continue
		}
		hp := strings.TrimSpace(r.HealthCheckPath)
		if hp == "" {
			hp = "/health/ready"
		}
		b.WriteString(fmt.Sprintf("    %s-pool:\n", sanitizeID(r.RouteID)))
		b.WriteString("      loadBalancer:\n")
		b.WriteString("        strategy: p2c\n")
		b.WriteString("        healthCheck:\n")
		b.WriteString(fmt.Sprintf("          path: %s\n", hp))
		b.WriteString("          interval: 5s\n")
		b.WriteString("          timeout: 2s\n")
		b.WriteString("        servers:\n")
		for _, u := range r.Backends {
			b.WriteString(fmt.Sprintf("          - url: %q\n", u))
		}
	}
	return b.String()
}

func sanitizeID(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "_", "-")
	if len(s) > 48 {
		s = s[:48]
	}
	if s == "" {
		return "ep"
	}
	return s
}

func escapeHost(h string) string {
	return strings.ReplaceAll(h, "`", "")
}
