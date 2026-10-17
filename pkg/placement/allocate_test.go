package placement

import "testing"

func TestOfferReservationKeyStable(t *testing.T) {
	k := OfferReservationKey("Gongji", "r1", "sku-a")
	if k != "gongji|r1|sku-a" {
		t.Fatalf("key %q", k)
	}
}
