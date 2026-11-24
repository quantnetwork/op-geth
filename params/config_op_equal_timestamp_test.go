package params

import (
	"errors"
	"testing"
)

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

func TestCheckAllowEqualTimestampsCompatible(t *testing.T) {
	on := &ChainConfig{Optimism: &OptimismConfig{AllowEqualTimestamps: true}}
	off := &ChainConfig{Optimism: &OptimismConfig{}}
	for _, c := range []struct {
		name          string
		stored, newer *ChainConfig
		head          uint64
		wantErr       bool
	}{
		{"on->off at genesis", on, off, 0, false},
		{"off->on at genesis", off, on, 0, false},
		{"on->off past genesis", on, off, 1, true},
		{"off->on past genesis", off, on, 1, true},
		{"on->on past genesis", on, on, 1, false},
		{"off->off past genesis", off, off, 1, false},
		{"nil optimism->off past genesis", &ChainConfig{}, off, 1, false},
		{"nil optimism->on past genesis", &ChainConfig{}, on, 1, true},
		{"nil config->off past genesis", nil, off, 1, false},
	} {
		err := c.stored.CheckAllowEqualTimestampsCompatible(c.newer, c.head)
		if (err != nil) != c.wantErr {
			t.Errorf("%s: err=%v, wantErr %v", c.name, err, c.wantErr)
		}
		if err != nil && !errors.Is(err, ErrAllowEqualTimestampsChanged) {
			t.Errorf("%s: err %v is not ErrAllowEqualTimestampsChanged", c.name, err)
		}
	}
}
