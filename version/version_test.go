package version

import (
	"context"
	"testing"
)

func TestNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"v0.3.0", "0.2.0", true}, {"0.2.1", "0.2.0", true}, {"1.0.0", "0.9.9", true},
		{"0.2.0", "0.2.0", false}, {"0.1.9", "0.2.0", false}, {"", "0.2.0", false}, {"latest", "0.2.0", false},
		{"v0.3.0-rc1", "0.2.0", true},
	}
	for _, c := range cases {
		if got := Newer(c.a, c.b); got != c.want {
			t.Errorf("Newer(%q, %q) = %v", c.a, c.b, got)
		}
	}
}

func TestRefresh(t *testing.T) {
	prev := fetch
	t.Cleanup(func() { fetch = prev; latest = "" })
	fetch = func(context.Context) (string, error) { return "v99.0.0", nil }
	Refresh(context.Background())
	if !UpdateAvailable() || Latest() != "v99.0.0" {
		t.Fatalf("latest = %q", Latest())
	}
}
