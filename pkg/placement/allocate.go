package placement

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"waverless/pkg/store/mysql/model"
)

// OfferReservationKey identifies one offer row for cap reservation.
func OfferReservationKey(providerID, region, skuID string) string {
	return strings.ToLower(strings.TrimSpace(providerID)) + "|" +
		strings.TrimSpace(region) + "|" +
		strings.TrimSpace(skuID)
}

// Allocate greedily splits targetWorkers across lowest-score offers (M2a).
// reserved subtracts already-allocated workers per offer key (existing slices).
func (s *Service) Allocate(ctx context.Context, req Request, targetWorkers int, candidates []string, reserved map[string]int) ([]Allocation, error) {
	if s.offers == nil {
		return nil, fmt.Errorf("provider offers repository is not configured")
	}
	if targetWorkers <= 0 {
		return nil, nil
	}
	if req.GpuType == "" || req.GpuCountPerWorker <= 0 {
		return nil, fmt.Errorf("gpuType and gpuCount are required for placement")
	}
	billing := req.BillingMode
	if billing == "" {
		billing = "on_demand"
	}
	if req.PinnedProviderID != "" {
		candidates = []string{strings.ToLower(req.PinnedProviderID)}
	}
	excludes := append([]string(nil), req.ExcludeProviders...)
	if s.cfg.Pooling.Enabled {
		excludes = append(excludes, "novita")
	}
	candidates = filterCandidates(candidates, s.cfg.CandidateProviders, excludes)
	if len(candidates) == 0 {
		return nil, fmt.Errorf("no enabled deployment providers for placement")
	}

	rows, err := s.offers.ListForPlacement(ctx, req.GpuType, req.GpuCountPerWorker, billing, candidates)
	if err != nil {
		return nil, err
	}
	allow := map[string]struct{}{}
	for _, id := range req.RegionAllowlist {
		id = strings.TrimSpace(id)
		if id != "" {
			allow[id] = struct{}{}
		}
	}
	restrictRegion := len(allow) > 0

	type scored struct {
		row   model.ProviderOffer
		score float64
	}
	scoredRows := make([]scored, 0, len(rows))
	for i := range rows {
		row := rows[i]
		if restrictRegion {
			if row.LogicalAzID == "" {
				continue
			}
			if _, ok := allow[row.LogicalAzID]; !ok {
				continue
			}
		}
		key := OfferReservationKey(row.ProviderID, row.Region, row.SkuID)
		res := 0
		if reserved != nil {
			res = reserved[key]
		}
		capLeft := row.AvailableCap - res
		if capLeft <= 0 {
			continue
		}
		openCost := s.imageOpenCost(req.ImageDigest, row.ProviderID, row.Region)
		score := row.UnitPrice + openCost
		scoredRows = append(scoredRows, scored{row: row, score: score})
	}
	if len(scoredRows) == 0 {
		return nil, fmt.Errorf("no provider offer matches gpuType=%s gpuCount=%d billing=%s", req.GpuType, req.GpuCountPerWorker, billing)
	}
	sort.Slice(scoredRows, func(i, j int) bool {
		if scoredRows[i].score != scoredRows[j].score {
			return scoredRows[i].score < scoredRows[j].score
		}
		if scoredRows[i].row.AvailableCap != scoredRows[j].row.AvailableCap {
			return scoredRows[i].row.AvailableCap > scoredRows[j].row.AvailableCap
		}
		return scoredRows[i].row.Region < scoredRows[j].row.Region
	})

	remaining := targetWorkers
	out := make([]Allocation, 0)
	for _, sr := range scoredRows {
		if remaining <= 0 {
			break
		}
		key := OfferReservationKey(sr.row.ProviderID, sr.row.Region, sr.row.SkuID)
		res := 0
		if reserved != nil {
			res = reserved[key]
		}
		capLeft := sr.row.AvailableCap - res
		if capLeft <= 0 {
			continue
		}
		take := capLeft
		if take > remaining {
			take = remaining
		}
		openCost := sr.score - sr.row.UnitPrice
		out = append(out, Allocation{
			Decision: Decision{
				ProviderID:     sr.row.ProviderID,
				PhysicalRegion: sr.row.Region,
				ProviderSkuID:  sr.row.SkuID,
				LogicalAzID:    sr.row.LogicalAzID,
				ResourceMark:   sr.row.ResourceMark,
				UnitCost:       sr.row.UnitPrice,
				OpenCost:       openCost,
				Score:          sr.score,
			},
			Replicas: take,
		})
		remaining -= take
		if reserved != nil {
			reserved[key] = res + take
		}
	}
	return out, nil
}
