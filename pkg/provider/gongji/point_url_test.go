package gongji

import "testing"

func TestPointBackendURL550wLink(t *testing.T) {
	pt := Point{
		AccessURL: "https://deployment-285-yrsrvm6k-8188.550w.link",
		Status:    "Running",
	}
	got := PointBackendURL(pt, 8000)
	want := "https://deployment-285-yrsrvm6k-8188.550w.link"
	if got != want {
		t.Fatalf("got %q want %q", got, want)
	}
}

func TestPointBackendURLFromDomain(t *testing.T) {
	pt := Point{Domain: "deployment-285-yrsrvm6k-8188.550w.link"}
	got := PointBackendURL(pt, 443)
	if got != "https://deployment-285-yrsrvm6k-8188.550w.link" {
		t.Fatalf("got %q", got)
	}
}
