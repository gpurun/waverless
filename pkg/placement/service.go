package placement

import (
	"context"
	"fmt"
	"strings"

	"waverless/pkg/config"
	"waverless/pkg/store/mysql"
	"waverless/pkg/store/mysql/model"
)

// Service selects a provider offer using configurable greedy cost.
type Service struct {
	offers *mysql.ProviderOfferRepository
	cfg    config.PlacementConfig
}

// NewService creates a placement service.
func NewService(offers *mysql.ProviderOfferRepository, cfg config.PlacementConfig) *Service {
	return &Service{offers: offers, cfg: cfg}
}

// Select picks the lowest-score offer that satisfies inventory and region constraints.
func (s *Service) Select(ctx context.Context, req Request, candidates []string) (Decision, error) {
	if s.offers == nil {
		return Decision{}, fmt.Errorf("provider offers repository is not configured")
	}
	if req.GpuType == "" || req.GpuCountPerWorker <= 0 {
		return Decision{}, fmt.Errorf("gpuType and gpuCount are required for placement")
	}
	billing := req.BillingMode
	if billing == "" {
		billing = "on_demand"
	}
	if req.PinnedProviderID != "" {
		candidates = []string{strings.ToLower(req.PinnedProviderID)}
	}
	candidates = filterCandidates(candidates, s.cfg.CandidateProviders, req.ExcludeProviders)
	if len(candidates) == 0 {
		return Decision{}, fmt.Errorf("no enabled deployment providers for placement")
	}

	rows, err := s.offers.ListForPlacement(ctx, req.GpuType, req.GpuCountPerWorker, billing, candidates)
	if err != nil {
		return Decision{}, err
	}
	allow := map[string]struct{}{}
	for _, id := range req.RegionAllowlist {
		id = strings.TrimSpace(id)
		if id != "" {
			allow[id] = struct{}{}
		}
	}
	restrictRegion := len(allow) > 0

	var best *model.ProviderOffer
	var bestScore float64
	for i := range rows {
		row := &rows[i]
		if restrictRegion {
			if row.LogicalAzID == "" {
				continue
			}
			if _, ok := allow[row.LogicalAzID]; !ok {
				continue
			}
		}
		if req.DesiredReplicas > 0 && row.AvailableCap < req.DesiredReplicas {
			continue
		}
		openCost := s.imageOpenCost(req.ImageDigest, row.ProviderID, row.Region)
		score := row.UnitPrice + openCost
		if best == nil || score < bestScore {
			best = row
			bestScore = score
		}
	}
	if best == nil {
		return Decision{}, fmt.Errorf("no provider offer matches gpuType=%s gpuCount=%d billing=%s", req.GpuType, req.GpuCountPerWorker, billing)
	}
	return Decision{
		ProviderID:     best.ProviderID,
		PhysicalRegion: best.Region,
		ProviderSkuID:  best.SkuID,
		LogicalAzID:    best.LogicalAzID,
		ResourceMark:   best.ResourceMark,
		UnitCost:       best.UnitPrice,
		OpenCost:       bestScore - best.UnitPrice,
		Score:          bestScore,
	}, nil
}

func (s *Service) imageOpenCost(digest, providerID, region string) float64 {
	if digest == "" || !s.cfg.AllowColdStart {
		return 0
	}
	// M1: no materializer — apply flat penalty when digest present (binding lookup in M3).
	_ = providerID
	_ = region
	return s.cfg.ImageNotReadyPenalty
}

func filterCandidates(deployments, configCandidates, exclude []string) []string {
	ex := map[string]struct{}{}
	for _, id := range exclude {
		ex[strings.ToLower(strings.TrimSpace(id))] = struct{}{}
	}
	base := deployments
	if len(configCandidates) > 0 {
		base = configCandidates
	}
	out := make([]string, 0, len(base))
	seen := map[string]struct{}{}
	for _, id := range base {
		id = strings.ToLower(strings.TrimSpace(id))
		if id == "" {
			continue
		}
		if _, skip := ex[id]; skip {
			continue
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	return out
}
