package cli

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/history"
	"github.com/ethereum/go-ethereum/core/rawdb"
	"github.com/ethereum/go-ethereum/ethdb"
	"github.com/ethereum/go-ethereum/internal/cli/flagset"
	"github.com/ethereum/go-ethereum/internal/cli/server"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/params"
)

// PruneHistoryCommand removes block bodies and receipts below an operator-chosen
// block from the chain freezer in place. Unlike prune-block it never replaces the
// ancient directory, so the path scheme state history kept next to the chain
// freezer survives.
type PruneHistoryCommand struct {
	*Meta

	datadirAncient string
	cache          int
	target         string
}

// MarkDown implements cli.MarkDown interface
func (c *PruneHistoryCommand) MarkDown() string {
	items := []string{
		"# Prune chain history",
		"The ```bor snapshot prune-history``` command removes block bodies and receipts below a given block from the ancient store, in place, while keeping headers and the canonical hash table. It works with both the hash and the path storage scheme: the state history freezer of the path scheme is not touched.",
		`
The target is given as "<block number>:<block hash>". The block itself is kept. Take the pair from a node that still has the history and pick a block that is already in the ancient store (at least 90000 blocks behind head). The pair is checked against the canonical chain in this database before anything is removed.

After pruning, start bor with ` + "`--history.chain <block number>:<block hash>`" + ` (or ` + "`[history] chain`" + ` in the config file) set to the same value; startup is refused while the setting and the database tail disagree. Re-running the command with a higher target prunes further; pruned history cannot be restored.`,
		c.Flags().MarkDown(),
	}

	return strings.Join(items, "\n\n")
}

// Help implements the cli.Command interface
func (c *PruneHistoryCommand) Help() string {
	return `Usage: bor snapshot prune-history --datadir <datadir> --history.chain <block number>:<block hash>

  This command removes block bodies and receipts below the given block from the ancient store` + c.Flags().Help()
}

// Synopsis implements the cli.Command interface
func (c *PruneHistoryCommand) Synopsis() string {
	return "Prune block bodies and receipts below a given block"
}

// Flags: datadir, datadir.ancient, cache, history.chain
func (c *PruneHistoryCommand) Flags() *flagset.Flagset {
	flags := c.NewFlagSet("prune-history")

	flags.StringFlag(&flagset.StringFlag{
		Name:    "datadir.ancient",
		Value:   &c.datadirAncient,
		Usage:   "Path of the ancient data directory",
		Default: "",
	})

	flags.IntFlag(&flagset.IntFlag{
		Name:    "cache",
		Usage:   "Megabytes of memory allocated to internal caching",
		Value:   &c.cache,
		Default: 1024,
		Group:   "Cache",
	})

	flags.StringFlag(&flagset.StringFlag{
		Name:  "history.chain",
		Usage: `Pruning target as "<block number>:<block hash>"; bodies and receipts of the blocks below it are removed`,
		Value: &c.target,
	})

	return flags
}

// Run implements the cli.Command interface
func (c *PruneHistoryCommand) Run(args []string) int {
	flags := c.Flags()

	if err := flags.Parse(args); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	if c.dataDir == "" {
		c.UI.Error("datadir is required")
		return 1
	}

	target, err := history.ParsePrunePoint(c.target)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	// Name is pinned because node.Config otherwise derives the instance directory
	// from the executable name, and a renamed binary would open an empty database.
	stack, err := node.New(&node.Config{
		Name:    "bor",
		DataDir: c.dataDir,
	})
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	defer stack.Close()

	dbHandles, err := server.MakeDatabaseHandles(0)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	chaindb, err := stack.OpenDatabaseWithFreezer(chaindataPath, c.cache, dbHandles, c.datadirAncient, "", false, true, false, false, false, false)
	if err != nil {
		c.UI.Error(err.Error())
		return 1
	}
	defer chaindb.Close()

	if err := pruneChainHistory(chaindb, target); err != nil {
		c.UI.Error(err.Error())
		return 1
	}

	c.UI.Output(fmt.Sprintf("Set history.chain = %q before starting bor.", target.String()))

	return 0
}

func pruneChainHistory(db ethdb.Database, target *history.PrunePoint) error {
	tail, err := db.Tail()
	if err != nil {
		return err
	}

	switch {
	case tail == target.BlockNumber:
		log.Info("Database already pruned to target block", "tail", tail)
		return nil
	case tail > target.BlockNumber:
		return fmt.Errorf("database is already pruned to block %d, beyond target %d", tail, target.BlockNumber)
	}

	headHash := rawdb.ReadHeadHeaderHash(db)
	head, ok := rawdb.ReadHeaderNumber(db, headHash)
	if !ok {
		return errors.New("head header not found")
	}

	if head < target.BlockNumber+params.FullImmutabilityThreshold {
		return fmt.Errorf("chain not far enough past target block %d, need %d more blocks",
			target.BlockNumber, target.BlockNumber+params.FullImmutabilityThreshold-head)
	}

	frozen, err := db.Ancients()
	if err != nil {
		return err
	}

	if target.BlockNumber > frozen {
		return fmt.Errorf("target block %d is not in the ancient store yet (frozen %d)", target.BlockNumber, frozen)
	}

	if hash := rawdb.ReadCanonicalHash(db, target.BlockNumber); hash != target.BlockHash {
		return fmt.Errorf("target block hash mismatch: got %s, want %s", hash.Hex(), target.BlockHash.Hex())
	}

	log.Info("Starting history pruning", "head", head, "frozen", frozen, "tail", tail, "target", target.BlockNumber, "targetHash", target.BlockHash)

	start := time.Now()

	// Index entries go first, so a tail at the target implies no lookups point
	// below it. An interrupted run is completed by running it again: freezer
	// repair on open finishes a partial truncation across the tables.
	rawdb.PruneTransactionIndex(db, target.BlockNumber)
	log.Info("Pruned transaction index", "elapsed", common.PrettyDuration(time.Since(start)))

	truncateStart := time.Now()

	if _, err := db.TruncateTail(target.BlockNumber); err != nil {
		return fmt.Errorf("failed to truncate ancient data: %v", err)
	}

	log.Info("Truncated ancient store", "elapsed", common.PrettyDuration(time.Since(truncateStart)))
	log.Info("History pruning completed", "tail", target.BlockNumber, "elapsed", common.PrettyDuration(time.Since(start)))

	return nil
}
