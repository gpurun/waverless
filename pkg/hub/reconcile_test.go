package hub

import (
	"sort"
	"testing"

	"waverless/pkg/store/mysql/model"
)

func TestScaleDownSortsByUnitCostDesc(t *testing.T) {
	cheap := 1.0
	dear := 3.0
	slices := []model.EndpointProviderSlice{
		{ID: 1, DesiredReplicas: 2, UnitCostSnapshot: &cheap},
		{ID: 2, DesiredReplicas: 2, UnitCostSnapshot: &dear},
	}
	sort.Slice(slices, func(i, j int) bool {
		ci := unitCost(slices[i])
		cj := unitCost(slices[j])
		if ci != cj {
			return ci > cj
		}
		return slices[i].ID > slices[j].ID
	})
	if slices[0].ID != 2 {
		t.Fatalf("expected dear slice first, got id=%d", slices[0].ID)
	}
}

func TestSliceDeployKeyPrimaryAndDerived(t *testing.T) {
	ep := "my-svc"
	if k := SliceDeployKey(ep, nil, true); k != ep {
		t.Fatalf("primary key %q", k)
	}
	sl := &model.EndpointProviderSlice{LogicalAzID: "az-bj", ID: 9}
	if k := SliceDeployKey(ep, sl, false); k != "my-svc--az-bj" {
		t.Fatalf("derived key %q", k)
	}
}
