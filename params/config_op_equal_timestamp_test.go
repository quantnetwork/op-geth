package params

import "testing"

func TestValidChildTimestamp(t *testing.T) {
	allow := &ChainConfig{Optimism: &OptimismConfig{AllowEqualTimestamps: true}}
	for _, c := range []struct {
		name          string
		cfg           *ChainConfig
		parent, child uint64
		want          bool
	}{
		{"nil config, later", nil, 5, 6, true},
		{"nil config, equal", nil, 5, 5, false},
		{"non-op, equal", &ChainConfig{}, 5, 5, false},
		{"op default, equal", &ChainConfig{Optimism: &OptimismConfig{}}, 5, 5, false},
		{"op default, later", &ChainConfig{Optimism: &OptimismConfig{}}, 5, 6, true},
		{"allowed, equal", allow, 5, 5, true},
		{"allowed, later", allow, 5, 6, true},
		{"allowed, earlier", allow, 5, 4, false},
	} {
		if got := c.cfg.ValidChildTimestamp(c.parent, c.child); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
