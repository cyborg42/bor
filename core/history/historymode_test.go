package history

import (
	"testing"

	"github.com/ethereum/go-ethereum/common"
)

// The "<number>:<hash>" string is copied by hand into prune commands and node
// configs; anything short of a full, non-zero pair must be rejected up front.
func TestParsePrunePoint(t *testing.T) {
	hash := common.HexToHash("0x6f7c16414e091d817bdbb0e1d0a17f74cd2b42d1a734d9864a7cd37a32514aad")

	point, err := ParsePrunePoint("25182208:" + hash.Hex())
	if err != nil {
		t.Fatalf("valid prune point rejected: %v", err)
	}
	if point.BlockNumber != 25182208 || point.BlockHash != hash {
		t.Fatalf("parsed %+v", point)
	}
	if again, err := ParsePrunePoint(point.String()); err != nil || *again != *point {
		t.Fatalf("String() does not round-trip: %v %+v", err, again)
	}

	for _, input := range []string{
		"25182208",
		hash.Hex(),
		"0:" + hash.Hex(),
		"-1:" + hash.Hex(),
		"abc:" + hash.Hex(),
		"25182208:0x1234",
		"25182208:" + common.Hash{}.Hex(),
	} {
		if _, err := ParsePrunePoint(input); err == nil {
			t.Errorf("ParsePrunePoint(%q) accepted", input)
		}
	}
}
