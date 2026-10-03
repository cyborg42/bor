package core

import (
	"testing"

	"github.com/ethereum/go-ethereum/consensus/ethash"
	"github.com/ethereum/go-ethereum/core/history"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/params"
)

// A database truncated in place must start only under the matching custom
// prune point; any other setting would serve a wrong history cutoff over RPC.
func TestCustomHistoryPruneStartup(t *testing.T) {
	gspec := &Genesis{Config: params.TestChainConfig}
	_, blocks, receipts := GenerateChainWithGenesis(gspec, ethash.NewFaker(), 64, nil)

	db, err := rawdb.Open(rawdb.NewMemoryDatabase(), rawdb.OpenOptions{Ancient: t.TempDir()})
	if err != nil {
		t.Fatalf("failed to create freezer db: %v", err)
	}
	defer db.Close()

	chain, err := NewBlockChain(db, gspec, ethash.NewFaker(), DefaultConfig().WithStateScheme(rawdb.HashScheme))
	if err != nil {
		t.Fatalf("failed to create chain: %v", err)
	}
	headers := make([]*types.Header, len(blocks))
	for i, block := range blocks {
		headers[i] = block.Header()
	}
	if n, err := chain.InsertHeaderChain(headers); err != nil {
		t.Fatalf("failed to insert header %d: %v", n, err)
	}
	if n, err := chain.InsertReceiptChain(blocks, types.EncodeBlockReceiptLists(receipts), uint64(len(blocks))); err != nil {
		t.Fatalf("failed to insert receipt %d: %v", n, err)
	}
	chain.Stop()

	const cutoff = 32
	if _, err := db.TruncateTail(cutoff); err != nil {
		t.Fatalf("failed to truncate tail: %v", err)
	}
	target := &history.PrunePoint{BlockNumber: cutoff, BlockHash: blocks[cutoff-1].Hash()}

	open := func(mode history.HistoryMode, point *history.PrunePoint) (*BlockChain, error) {
		cfg := DefaultConfig().WithStateScheme(rawdb.HashScheme)
		cfg.ChainHistoryMode = mode
		cfg.ChainHistoryTarget = point
		return NewBlockChain(db, gspec, ethash.NewFaker(), cfg)
	}
	for name, point := range map[string]*history.PrunePoint{
		"wrong hash":   {BlockNumber: cutoff, BlockHash: blocks[cutoff].Hash()},
		"above tail":   {BlockNumber: cutoff + 1, BlockHash: blocks[cutoff].Hash()},
		"below tail":   {BlockNumber: cutoff - 1, BlockHash: blocks[cutoff-2].Hash()},
		"no point set": nil,
	} {
		if _, err := open(history.KeepCustom, point); err == nil {
			t.Errorf("%s: startup accepted", name)
		}
	}
	if _, err := open(history.KeepAll, nil); err == nil {
		t.Error("KeepAll accepted a truncated database")
	}

	chain, err = open(history.KeepCustom, target)
	if err != nil {
		t.Fatalf("matching prune point rejected: %v", err)
	}
	defer chain.Stop()

	if number, hash := chain.HistoryPruningCutoff(); number != cutoff || hash != target.BlockHash {
		t.Errorf("cutoff = %d %x, want %d %x", number, hash, cutoff, target.BlockHash)
	}
	if chain.GetBlockByNumber(cutoff-1) != nil {
		t.Error("block below the cutoff still has a body")
	}
	if chain.GetBlockByNumber(cutoff) == nil {
		t.Error("cutoff block lost its body")
	}
}
