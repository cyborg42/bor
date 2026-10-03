// Copyright 2025 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package history

import (
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/params"
)

// HistoryMode configures history pruning.
type HistoryMode uint32

const (
	// KeepAll (default) means that all chain history down to genesis block will be kept.
	KeepAll HistoryMode = iota

	// KeepPostMerge sets the history pruning point to the merge activation block.
	KeepPostMerge

	// KeepCustom sets the history pruning point to a block number and hash supplied
	// by the operator, in place of one of the built-in points.
	KeepCustom
)

func (m HistoryMode) IsValid() bool {
	return m <= KeepCustom
}

func (m HistoryMode) String() string {
	switch m {
	case KeepAll:
		return "all"
	case KeepPostMerge:
		return "postmerge"
	case KeepCustom:
		return "custom"
	default:
		return fmt.Sprintf("invalid HistoryMode(%d)", m)
	}
}

// MarshalText implements encoding.TextMarshaler.
func (m HistoryMode) MarshalText() ([]byte, error) {
	if m.IsValid() {
		return []byte(m.String()), nil
	}
	return nil, fmt.Errorf("unknown history mode %d", m)
}

// UnmarshalText implements encoding.TextUnmarshaler.
func (m *HistoryMode) UnmarshalText(text []byte) error {
	switch string(text) {
	case "all":
		*m = KeepAll
	case "postmerge":
		*m = KeepPostMerge
	case "custom":
		*m = KeepCustom
	default:
		return fmt.Errorf(`unknown sync mode %q, want "all", "postmerge" or "custom"`, text)
	}
	return nil
}

type PrunePoint struct {
	BlockNumber uint64
	BlockHash   common.Hash
}

// String formats the prune point in the "number:hash" form that ParsePrunePoint
// accepts, so a configured point can be echoed back as a command-line argument.
func (p *PrunePoint) String() string {
	if p == nil {
		return ""
	}
	return fmt.Sprintf("%d:%s", p.BlockNumber, p.BlockHash.Hex())
}

// ParsePrunePoint parses a history pruning point given as "<number>:<hash>".
//
// Both halves are required. The number alone cannot be trusted as a pruning point
// because nothing guarantees the canonical chain includes it (a reorged block has
// a number too); the hash alone says nothing about where to cut. The pair is
// checked against the canonical chain when pruning runs and on every startup.
func ParsePrunePoint(input string) (*PrunePoint, error) {
	number, hash, ok := strings.Cut(input, ":")
	if !ok {
		return nil, fmt.Errorf(`invalid prune point %q, want "<block number>:<block hash>"`, input)
	}
	block, err := strconv.ParseUint(number, 10, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid block number %q in prune point: %v", number, err)
	}
	if block == 0 {
		return nil, errors.New("prune point must be above the genesis block")
	}
	var point common.Hash
	if err := point.UnmarshalText([]byte(hash)); err != nil {
		return nil, fmt.Errorf("invalid block hash %q in prune point: %v", hash, err)
	}
	if point == (common.Hash{}) {
		return nil, errors.New("prune point block hash must not be zero")
	}
	return &PrunePoint{BlockNumber: block, BlockHash: point}, nil
}

// PrunePoints the pre-defined history pruning cutoff blocks for known networks.
// They point to the first post-merge block. Any pruning should truncate *up to* but excluding
// given block.
var PrunePoints = map[common.Hash]*PrunePoint{
	// mainnet
	params.MainnetGenesisHash: {
		BlockNumber: 15537393,
		BlockHash:   common.HexToHash("0x55b11b918355b1ef9c5db810302ebad0bf2544255b530cdce90674d5887bb286"),
	},
	// sepolia
	params.SepoliaGenesisHash: {
		BlockNumber: 1450409,
		BlockHash:   common.HexToHash("0x229f6b18ca1552f1d5146deceb5387333f40dc6275aebee3f2c5c4ece07d02db"),
	},
}

// PrunedHistoryError is returned by APIs when the requested history is pruned.
type PrunedHistoryError struct{}

func (e *PrunedHistoryError) Error() string  { return "pruned history unavailable" }
func (e *PrunedHistoryError) ErrorCode() int { return 4444 }
