# Prune chain history

The ```bor snapshot prune-history``` command removes block bodies and receipts below a given block from the ancient store, in place, while keeping headers and the canonical hash table. It works with both the hash and the path storage scheme: the state history freezer of the path scheme is not touched.


The target is given as "<block number>:<block hash>". The block itself is kept. Take the pair from a node that still has the history and pick a block that is already in the ancient store (at least 90000 blocks behind head). The pair is checked against the canonical chain in this database before anything is removed.

After pruning, start bor with `--history.chain <block number>:<block hash>` (or `[history] chain` in the config file) set to the same value; startup is refused while the setting and the database tail disagree. Re-running the command with a higher target prunes further; pruned history cannot be restored.

## Options

- ```datadir```: Path of the data directory to store information

- ```datadir.ancient```: Path of the ancient data directory

- ```history.chain```: Pruning target as "<block number>:<block hash>"; bodies and receipts of the blocks below it are removed

- ```keystore```: Path of the data directory to store keys

### Cache Options

- ```cache```: Megabytes of memory allocated to internal caching (default: 1024)