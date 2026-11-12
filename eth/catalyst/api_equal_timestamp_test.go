package catalyst

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/beacon/engine"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/consensus/misc/eip1559"
	"github.com/ethereum/go-ethereum/core"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/miner"
	"github.com/ethereum/go-ethereum/params"
	"github.com/stretchr/testify/require"
)

// equalTimestampGenesis is an OP Holocene chain (Cancun, no Prague) whose
// optimism config has AllowEqualTimestamps set as given.
func equalTimestampGenesis(allow bool) *core.Genesis {
	cfg := *params.OptimismTestConfig
	cfg.IsthmusTime = nil
	cfg.JovianTime = nil
	cfg.PragueTime = nil
	cfg.OsakaTime = nil
	op := *cfg.Optimism
	op.AllowEqualTimestamps = allow
	cfg.Optimism = &op
	minBaseFee := uint64(0)
	return &core.Genesis{
		Config:     &cfg,
		Alloc:      types.GenesisAlloc{testAddr: {Balance: testBalance}},
		ExtraData:  eip1559.EncodeOptimismExtraData(&cfg, 0, 250, 6, &minBaseFee),
		BaseFee:    big.NewInt(params.InitialBaseFee),
		Difficulty: big.NewInt(0),
		GasLimit:   30_000_000,
		Timestamp:  1000,
	}
}

// buildOn runs the sequencer's Engine API round trip for one block stamped ts
// on parent: forkchoiceUpdated with attributes, getPayload, newPayload, and
// forkchoiceUpdated to make it the head. It returns the first error or invalid
// status it meets.
func buildOn(t *testing.T, api *ConsensusAPI, parent common.Hash, ts uint64) (common.Hash, error) {
	t.Helper()
	dep, err := types.NewTx(&types.DepositTx{}).MarshalBinary()
	require.NoError(t, err)
	gasLimit := uint64(30_000_000)
	attrs := &engine.PayloadAttributes{
		Timestamp:     ts,
		Withdrawals:   []*types.Withdrawal{},
		BeaconRoot:    &common.Hash{},
		Transactions:  [][]byte{dep},
		NoTxPool:      true,
		GasLimit:      &gasLimit,
		EIP1559Params: eip1559.EncodeHolocene1559Params(250, 6),
	}
	fc := engine.ForkchoiceStateV1{HeadBlockHash: parent, SafeBlockHash: parent, FinalizedBlockHash: parent}
	resp, err := api.ForkchoiceUpdatedV3(fc, attrs)
	if err != nil {
		return common.Hash{}, err
	}
	if resp.PayloadID == nil {
		return common.Hash{}, errors.New("no payload id")
	}
	env, err := api.GetPayloadV3(*resp.PayloadID)
	if err != nil {
		return common.Hash{}, err
	}
	status, err := api.NewPayloadV3(context.Background(), *env.ExecutionPayload, []common.Hash{}, &common.Hash{})
	if err != nil {
		return common.Hash{}, err
	}
	if status.Status != engine.VALID {
		return common.Hash{}, fmt.Errorf("newPayload status %s", status.Status)
	}
	h := env.ExecutionPayload.BlockHash
	fc = engine.ForkchoiceStateV1{HeadBlockHash: h, SafeBlockHash: h, FinalizedBlockHash: h}
	if _, err := api.ForkchoiceUpdatedV3(fc, nil); err != nil {
		return common.Hash{}, err
	}
	return h, nil
}

func startEqualTimestampNode(t *testing.T, allow bool) *ConsensusAPI {
	genesis := equalTimestampGenesis(allow)
	ethcfg := &ethconfig.Config{Genesis: genesis, SyncMode: ethconfig.FullSync, TrieTimeout: time.Minute, TrieDirtyCache: 256, TrieCleanCache: 256, Miner: miner.DefaultConfig}
	n, ethservice := startEthServiceWithConfigFn(t, nil, ethcfg)
	t.Cleanup(func() { n.Close() })
	return newConsensusAPIWithoutHeartbeat(ethservice)
}

// TestEqualTimestampEngineRoundTrip: with allowEqualTimestamps the sequencer
// round trip builds and imports a block stamped like its parent; without it the
// same request is refused, as before.
func TestEqualTimestampEngineRoundTrip(t *testing.T) {
	t.Run("allowed", func(t *testing.T) {
		api := startEqualTimestampNode(t, true)
		head := api.eth.BlockChain().CurrentBlock()
		h1, err := buildOn(t, api, head.Hash(), head.Time+1)
		require.NoError(t, err)
		h2, err := buildOn(t, api, h1, head.Time+1) // same second as its parent
		require.NoError(t, err)
		h3, err := buildOn(t, api, h2, head.Time+1)
		require.NoError(t, err)
		cur := api.eth.BlockChain().CurrentBlock()
		require.Equal(t, h3, cur.Hash())
		require.Equal(t, uint64(3), cur.Number.Uint64())
		require.Equal(t, head.Time+1, cur.Time)
		// Going back in time is still refused.
		_, err = buildOn(t, api, h3, head.Time)
		require.Error(t, err)
	})
	t.Run("default", func(t *testing.T) {
		api := startEqualTimestampNode(t, false)
		head := api.eth.BlockChain().CurrentBlock()
		h1, err := buildOn(t, api, head.Hash(), head.Time+1)
		require.NoError(t, err)
		_, err = buildOn(t, api, h1, head.Time+1)
		require.Error(t, err)
		require.Equal(t, uint64(1), api.eth.BlockChain().CurrentBlock().Number.Uint64())
	})
}

// TestEqualTimestampNewPayloadOnly: a verifier (newPayload only) imports a
// block stamped like its parent only with the flag; the block is built on a
// node that allows it.
func TestEqualTimestampNewPayloadOnly(t *testing.T) {
	seq := startEqualTimestampNode(t, true)
	head := seq.eth.BlockChain().CurrentBlock()
	h1, err := buildOn(t, seq, head.Hash(), head.Time+1)
	require.NoError(t, err)
	h2, err := buildOn(t, seq, h1, head.Time+1)
	require.NoError(t, err)
	b1 := seq.eth.BlockChain().GetBlockByHash(h1)
	b2 := seq.eth.BlockChain().GetBlockByHash(h2)

	for _, allow := range []bool{true, false} {
		ver := startEqualTimestampNode(t, allow)
		require.Equal(t, head.Hash(), ver.eth.BlockChain().CurrentBlock().Hash(), "same genesis")
		for i, b := range []*types.Block{b1, b2} {
			ed := engine.BlockToExecutableData(b, nil, nil, nil).ExecutionPayload
			status, err := ver.NewPayloadV3(context.Background(), *ed, []common.Hash{}, &common.Hash{})
			require.NoError(t, err)
			want := engine.VALID
			if i == 1 && !allow {
				want = engine.INVALID
			}
			require.Equal(t, want, status.Status, "allow=%v block %d", allow, i+1)
		}
	}
}
