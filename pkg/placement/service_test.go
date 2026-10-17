package placement

import (
	"testing"

	"waverless/pkg/config"
)

func TestFilterCandidates(t *testing.T) {
	got := filterCandidates([]string{"gongji", "novita"}, nil, []string{"novita"})
	if len(got) != 1 || got[0] != "gongji" {
		t.Fatalf("got %v", got)
	}
}

func TestImageOpenCostPenalty(t *testing.T) {
	s := &Service{cfg: config.PlacementConfig{AllowColdStart: true, ImageNotReadyPenalty: 1.5}}
	if v := s.imageOpenCost("sha256:abc", "gongji", "r1"); v != 1.5 {
		t.Fatalf("penalty %v", v)
	}
}
