package config

import (
	"os"
	"strings"
)

// PoolingConfig controls multi-slice / multi-region allocation (M2a+).
type PoolingConfig struct {
	Enabled              bool `yaml:"enabled"`
	MaxSlicesPerEndpoint int  `yaml:"max_slices_per_endpoint"`
}

// PlacementConfig controls automatic provider selection.
type PlacementConfig struct {
	Strategy             string        `yaml:"strategy"`
	ImageNotReadyPenalty float64       `yaml:"image_not_ready_penalty"`
	AllowColdStart       bool          `yaml:"allow_cold_start"`
	CandidateProviders   []string      `yaml:"candidate_providers"`
	Pooling              PoolingConfig `yaml:"pooling"`
}

// DefaultPlacementConfig returns M1 defaults.
func DefaultPlacementConfig() PlacementConfig {
	return PlacementConfig{
		Strategy:             "greedy_cost",
		ImageNotReadyPenalty: 1.0,
		AllowColdStart:       true,
		Pooling: PoolingConfig{
			Enabled:              false,
			MaxSlicesPerEndpoint: 8,
		},
	}
}

// EnabledDeployments returns deployment provider ids that are configured and enabled.
func (c *Config) EnabledDeployments() []string {
	raw := c.rawDeploymentList()
	out := make([]string, 0, len(raw))
	seen := map[string]struct{}{}
	for _, id := range raw {
		id = strings.ToLower(strings.TrimSpace(id))
		if id == "" {
			continue
		}
		if id == "suanli" {
			id = "gongji"
		}
		if !c.isDeploymentProviderEnabled(id) {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}

func (c *Config) rawDeploymentList() []string {
	if v := os.Getenv("PROVIDERS_DEPLOYMENTS"); v != "" {
		return strings.Split(v, ",")
	}
	if c.Providers != nil {
		if len(c.Providers.Deployments) > 0 {
			return c.Providers.Deployments
		}
		if c.Providers.Deployment != "" {
			return []string{c.Providers.Deployment}
		}
	}
	return []string{"k8s"}
}

func (c *Config) isDeploymentProviderEnabled(id string) bool {
	switch id {
	case "gongji":
		return c.Gongji.Enabled
	case "novita":
		return c.Novita.Enabled
	case "k8s", "kubernetes":
		return c.K8s.Enabled
	case "docker":
		return true
	default:
		return true
	}
}
