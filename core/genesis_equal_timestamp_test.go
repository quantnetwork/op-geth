package core

import (
	"errors"
	"math/big"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/misc/eip1559"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/params"
	"github.com/ethereum/go-ethereum/triedb"
)

// equalTimestampGenesis is an OP test chain whose optimism config has
// AllowEqualTimestamps set as given. The flag does not change the genesis hash.
func equalTimestampGenesis(allow bool) *Genesis {
	cfg := *params.OptimismTestConfig
	op := *cfg.Optimism
	op.AllowEqualTimestamps = allow
	cfg.Optimism = &op
	minBaseFee := uint64(0)
	return &Genesis{
		Config:     &cfg,
		Alloc:      types.GenesisAlloc{},
		ExtraData:  eip1559.EncodeOptimismExtraData(&cfg, 0, 250, 6, &minBaseFee),
		BaseFee:    big.NewInt(params.InitialBaseFee),
		Difficulty: big.NewInt(0),
		GasLimit:   30_000_000,
		Timestamp:  1000,
	}
}

// advanceHeadPastGenesis makes a canonical header #1 the head header, which is
// all SetupGenesisBlock reads of the chain head.
func advanceHeadPastGenesis(t *testing.T, db ethdb.Database, genesisHash common.Hash) {
	t.Helper()
	h := &types.Header{ParentHash: genesisHash, Number: big.NewInt(1), Time: 1000, Difficulty: big.NewInt(0), BaseFee: big.NewInt(params.InitialBaseFee)}
	rawdb.WriteHeader(db, h)
	rawdb.WriteCanonicalHash(db, h.Hash(), 1)
	rawdb.WriteHeadHeaderHash(db, h.Hash())
	if head := rawdb.ReadHeadHeader(db); head == nil || head.Number.Uint64() != 1 {
		t.Fatalf("head not advanced: %v", head)
	}
}

func storedAllowEqualTimestamps(t *testing.T, db ethdb.Database, h common.Hash) bool {
	t.Helper()
	cfg := rawdb.ReadChainConfig(db, h)
	if cfg == nil || cfg.Optimism == nil {
		t.Fatal("no stored optimism config")
	}
	return cfg.Optimism.AllowEqualTimestamps
}

// TestAllowEqualTimestampsChangeGuard: once the chain is past genesis, a genesis
// whose allowEqualTimestamps differs from the stored config is refused and the
// stored value kept; the same value, or no genesis, still opens the chain. A
// fresh database and a chain still at genesis take the given value, as before.
func TestAllowEqualTimestampsChangeGuard(t *testing.T) {
	if equalTimestampGenesis(true).ToBlock().Hash() != equalTimestampGenesis(false).ToBlock().Hash() {
		t.Fatal("test premise: the flag must not change the genesis hash")
	}
	setup := func(t *testing.T, allow bool) (ethdb.Database, *triedb.Database, common.Hash) {
		t.Helper()
		db := rawdb.NewMemoryDatabase()
		tdb := triedb.NewDatabase(db, triedb.HashDefaults)
		cfg, h, compat, err := SetupGenesisBlock(db, tdb, equalTimestampGenesis(allow))
		if err != nil || compat != nil {
			t.Fatalf("fresh db: err=%v compat=%v", err, compat)
		}
		if cfg.Optimism.AllowEqualTimestamps != allow || storedAllowEqualTimestamps(t, db, h) != allow {
			t.Fatalf("fresh db: flag not taken, want %v", allow)
		}
		return db, tdb, h
	}

	for _, stored := range []bool{true, false} {
		t.Run(map[bool]string{true: "stored-on", false: "stored-off"}[stored], func(t *testing.T) {
			t.Run("change at genesis allowed", func(t *testing.T) {
				db, tdb, h := setup(t, stored)
				cfg, _, compat, err := SetupGenesisBlock(db, tdb, equalTimestampGenesis(!stored))
				if err != nil || compat != nil {
					t.Fatalf("err=%v compat=%v", err, compat)
				}
				if cfg.Optimism.AllowEqualTimestamps != !stored || storedAllowEqualTimestamps(t, db, h) != !stored {
					t.Fatalf("flag not updated at genesis, want %v", !stored)
				}
			})
			t.Run("change past genesis refused", func(t *testing.T) {
				db, tdb, h := setup(t, stored)
				advanceHeadPastGenesis(t, db, h)
				_, _, _, err := SetupGenesisBlock(db, tdb, equalTimestampGenesis(!stored))
				if !errors.Is(err, params.ErrAllowEqualTimestampsChanged) {
					t.Fatalf("want ErrAllowEqualTimestampsChanged, got %v", err)
				}
				t.Logf("refused: %v", err)
				if storedAllowEqualTimestamps(t, db, h) != stored {
					t.Fatalf("stored flag changed to %v", !stored)
				}
			})
			t.Run("unchanged past genesis allowed", func(t *testing.T) {
				db, tdb, h := setup(t, stored)
				advanceHeadPastGenesis(t, db, h)
				for _, g := range []*Genesis{equalTimestampGenesis(stored), nil} {
					cfg, _, compat, err := SetupGenesisBlock(db, tdb, g)
					if err != nil || compat != nil {
						t.Fatalf("genesis nil=%v: err=%v compat=%v", g == nil, err, compat)
					}
					if cfg.Optimism.AllowEqualTimestamps != stored || storedAllowEqualTimestamps(t, db, h) != stored {
						t.Fatalf("genesis nil=%v: flag changed", g == nil)
					}
				}
			})
		})
	}
}
