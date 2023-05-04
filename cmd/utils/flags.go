// Copyright 2015 The go-ethereum Authors
// This file is part of go-ethereum.
//
// go-ethereum is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// go-ethereum is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with go-ethereum. If not, see <http://www.gnu.org/licenses/>.

// Package utils contains internal helper functions for go-ethereum commands.
package utils

import (
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math"
	"math/big"
	"os"
	"path"
	godebug "runtime/debug"
	"strconv"
	"strings"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/fdlimit"
	"github.com/ethereum/go-ethereum/core/txpool"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/emu"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/eth/downloader"
	"github.com/ethereum/go-ethereum/eth/ethconfig"
	"github.com/ethereum/go-ethereum/eth/gasprice"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/internal/flags"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/miner"
	"github.com/ethereum/go-ethereum/node"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/urfave/cli/v2"
)

// These are all the command line flags we support.
// If you add to this list, please remember to include the
// flag in the appropriate command definition.
//
// The flags are defined here so their names and help texts
// are the same for all commands.

var (
	// General settings
	DataDirFlag = &flags.DirectoryFlag{
		Name:     "datadir",
		Usage:    "Data directory for the databases and keystore",
		Value:    flags.DirectoryString(node.DefaultDataDir()),
		Category: flags.EthCategory,
	}
	RemoteDBFlag = &cli.StringFlag{
		Name:     "remotedb",
		Usage:    "URL for remote database",
		Category: flags.LoggingCategory,
	}
	AncientFlag = &flags.DirectoryFlag{
		Name:     "datadir.ancient",
		Usage:    "Root directory for ancient data (default = inside chaindata)",
		Category: flags.EthCategory,
	}
	MinFreeDiskSpaceFlag = &flags.DirectoryFlag{
		Name:     "datadir.minfreedisk",
		Usage:    "Minimum free disk space in MB, once reached triggers auto shut down (default = --cache.gc converted to MB, 0 = disabled)",
		Category: flags.EthCategory,
	}
	KeyStoreDirFlag = &flags.DirectoryFlag{
		Name:     "keystore",
		Usage:    "Directory for the keystore (default = inside the datadir)",
		Category: flags.AccountCategory,
	}
	NetworkIdFlag = &cli.Uint64Flag{
		Name:     "networkid",
		Usage:    "Explicitly set network id (integer)(For testnets: use --rinkeby, --goerli, --sepolia instead)",
		Value:    ethconfig.Defaults.NetworkId,
		Category: flags.EthCategory,
	}

	IdentityFlag = &cli.StringFlag{
		Name:     "identity",
		Usage:    "Custom node name",
		Category: flags.NetworkingCategory,
	}
	DocRootFlag = &flags.DirectoryFlag{
		Name:     "docroot",
		Usage:    "Document Root for HTTPClient file scheme",
		Value:    flags.DirectoryString(flags.HomeDir()),
		Category: flags.APICategory,
	}

	defaultSyncMode = ethconfig.Defaults.SyncMode
	SyncModeFlag    = &flags.TextMarshalerFlag{
		Name:     "syncmode",
		Usage:    `Blockchain sync mode ("snap", "full" or "light")`,
		Value:    &defaultSyncMode,
		Category: flags.EthCategory,
	}
	GCModeFlag = &cli.StringFlag{
		Name:     "gcmode",
		Usage:    `Blockchain garbage collection mode ("full", "archive")`,
		Value:    "full",
		Category: flags.EthCategory,
	}
	SnapshotFlag = &cli.BoolFlag{
		Name:     "snapshot",
		Usage:    `Enables snapshot-database mode (default = enable)`,
		Value:    true,
		Category: flags.EthCategory,
	}
	TxLookupLimitFlag = &cli.Uint64Flag{
		Name:     "txlookuplimit",
		Usage:    "Number of recent blocks to maintain transactions index for (default = about one year, 0 = entire chain)",
		Value:    ethconfig.Defaults.TxLookupLimit,
		Category: flags.EthCategory,
	}
	BloomFilterSizeFlag = &cli.Uint64Flag{
		Name:     "bloomfilter.size",
		Usage:    "Megabytes of memory allocated to bloom-filter for pruning",
		Value:    2048,
		Category: flags.EthCategory,
	}
	OverrideShanghai = &cli.Uint64Flag{
		Name:     "override.shanghai",
		Usage:    "Manually specify the Shanghai fork timestamp, overriding the bundled setting",
		Category: flags.EthCategory,
	}

	// Transaction pool settings
	TxPoolLocalsFlag = &cli.StringFlag{
		Name:     "txpool.locals",
		Usage:    "Comma separated accounts to treat as locals (no flush, priority inclusion)",
		Category: flags.TxPoolCategory,
	}
	TxPoolNoLocalsFlag = &cli.BoolFlag{
		Name:     "txpool.nolocals",
		Usage:    "Disables price exemptions for locally submitted transactions",
		Category: flags.TxPoolCategory,
	}
	TxPoolJournalFlag = &cli.StringFlag{
		Name:     "txpool.journal",
		Usage:    "Disk journal for local transaction to survive node restarts",
		Value:    txpool.DefaultConfig.Journal,
		Category: flags.TxPoolCategory,
	}
	TxPoolRejournalFlag = &cli.DurationFlag{
		Name:     "txpool.rejournal",
		Usage:    "Time interval to regenerate the local transaction journal",
		Value:    txpool.DefaultConfig.Rejournal,
		Category: flags.TxPoolCategory,
	}
	TxPoolPriceLimitFlag = &cli.Uint64Flag{
		Name:     "txpool.pricelimit",
		Usage:    "Minimum gas price limit to enforce for acceptance into the pool",
		Value:    ethconfig.Defaults.TxPool.PriceLimit,
		Category: flags.TxPoolCategory,
	}
	TxPoolPriceBumpFlag = &cli.Uint64Flag{
		Name:     "txpool.pricebump",
		Usage:    "Price bump percentage to replace an already existing transaction",
		Value:    ethconfig.Defaults.TxPool.PriceBump,
		Category: flags.TxPoolCategory,
	}
	TxPoolAccountSlotsFlag = &cli.Uint64Flag{
		Name:     "txpool.accountslots",
		Usage:    "Minimum number of executable transaction slots guaranteed per account",
		Value:    ethconfig.Defaults.TxPool.AccountSlots,
		Category: flags.TxPoolCategory,
	}
	TxPoolGlobalSlotsFlag = &cli.Uint64Flag{
		Name:     "txpool.globalslots",
		Usage:    "Maximum number of executable transaction slots for all accounts",
		Value:    ethconfig.Defaults.TxPool.GlobalSlots,
		Category: flags.TxPoolCategory,
	}
	TxPoolAccountQueueFlag = &cli.Uint64Flag{
		Name:     "txpool.accountqueue",
		Usage:    "Maximum number of non-executable transaction slots permitted per account",
		Value:    ethconfig.Defaults.TxPool.AccountQueue,
		Category: flags.TxPoolCategory,
	}
	TxPoolGlobalQueueFlag = &cli.Uint64Flag{
		Name:     "txpool.globalqueue",
		Usage:    "Maximum number of non-executable transaction slots for all accounts",
		Value:    ethconfig.Defaults.TxPool.GlobalQueue,
		Category: flags.TxPoolCategory,
	}
	TxPoolLifetimeFlag = &cli.DurationFlag{
		Name:     "txpool.lifetime",
		Usage:    "Maximum amount of time non-executable transaction are queued",
		Value:    ethconfig.Defaults.TxPool.Lifetime,
		Category: flags.TxPoolCategory,
	}

	// Performance tuning settings
	CacheFlag = &cli.IntFlag{
		Name:     "cache",
		Usage:    "Megabytes of memory allocated to internal caching (default = 4096 mainnet full node, 128 light mode)",
		Value:    1024,
		Category: flags.PerfCategory,
	}
	CacheDatabaseFlag = &cli.IntFlag{
		Name:     "cache.database",
		Usage:    "Percentage of cache memory allowance to use for database io",
		Value:    50,
		Category: flags.PerfCategory,
	}
	CacheTrieFlag = &cli.IntFlag{
		Name:     "cache.trie",
		Usage:    "Percentage of cache memory allowance to use for trie caching (default = 15% full mode, 30% archive mode)",
		Value:    15,
		Category: flags.PerfCategory,
	}
	CacheTrieJournalFlag = &cli.StringFlag{
		Name:     "cache.trie.journal",
		Usage:    "Disk journal directory for trie cache to survive node restarts",
		Value:    ethconfig.Defaults.TrieCleanCacheJournal,
		Category: flags.PerfCategory,
	}
	CacheTrieRejournalFlag = &cli.DurationFlag{
		Name:     "cache.trie.rejournal",
		Usage:    "Time interval to regenerate the trie cache journal",
		Value:    ethconfig.Defaults.TrieCleanCacheRejournal,
		Category: flags.PerfCategory,
	}
	CacheGCFlag = &cli.IntFlag{
		Name:     "cache.gc",
		Usage:    "Percentage of cache memory allowance to use for trie pruning (default = 25% full mode, 0% archive mode)",
		Value:    25,
		Category: flags.PerfCategory,
	}
	CacheSnapshotFlag = &cli.IntFlag{
		Name:     "cache.snapshot",
		Usage:    "Percentage of cache memory allowance to use for snapshot caching (default = 10% full mode, 20% archive mode)",
		Value:    10,
		Category: flags.PerfCategory,
	}
	CacheNoPrefetchFlag = &cli.BoolFlag{
		Name:     "cache.noprefetch",
		Usage:    "Disable heuristic state prefetch during block import (less CPU and disk IO, more time waiting for data)",
		Category: flags.PerfCategory,
	}
	CachePreimagesFlag = &cli.BoolFlag{
		Name:     "cache.preimages",
		Usage:    "Enable recording the SHA3/keccak preimages of trie keys",
		Category: flags.PerfCategory,
	}
	CacheLogSizeFlag = &cli.IntFlag{
		Name:     "cache.blocklogs",
		Usage:    "Size (in number of blocks) of the log cache for filtering",
		Category: flags.PerfCategory,
		Value:    ethconfig.Defaults.FilterLogCacheSize,
	}
	FDLimitFlag = &cli.IntFlag{
		Name:     "fdlimit",
		Usage:    "Raise the open file descriptor resource limit (default = system fd limit)",
		Category: flags.PerfCategory,
	}

	// Miner settings
	MiningEnabledFlag = &cli.BoolFlag{
		Name:     "mine",
		Usage:    "Enable mining",
		Category: flags.MinerCategory,
	}
	MinerThreadsFlag = &cli.IntFlag{
		Name:     "miner.threads",
		Usage:    "Number of CPU threads to use for mining",
		Value:    0,
		Category: flags.MinerCategory,
	}
	MinerNotifyFlag = &cli.StringFlag{
		Name:     "miner.notify",
		Usage:    "Comma separated HTTP URL list to notify of new work packages",
		Category: flags.MinerCategory,
	}
	MinerGasLimitFlag = &cli.Uint64Flag{
		Name:     "miner.gaslimit",
		Usage:    "Target gas ceiling for mined blocks",
		Value:    ethconfig.Defaults.Miner.GasCeil,
		Category: flags.MinerCategory,
	}
	MinerGasPriceFlag = &flags.BigFlag{
		Name:     "miner.gasprice",
		Usage:    "Minimum gas price for mining a transaction",
		Value:    ethconfig.Defaults.Miner.GasPrice,
		Category: flags.MinerCategory,
	}
	MinerEtherbaseFlag = &cli.StringFlag{
		Name:     "miner.etherbase",
		Usage:    "0x prefixed public address for block mining rewards",
		Category: flags.MinerCategory,
	}
	MinerExtraDataFlag = &cli.StringFlag{
		Name:     "miner.extradata",
		Usage:    "Block extra data set by the miner (default = client version)",
		Category: flags.MinerCategory,
	}
	MinerRecommitIntervalFlag = &cli.DurationFlag{
		Name:     "miner.recommit",
		Usage:    "Time interval to recreate the block being mined",
		Value:    ethconfig.Defaults.Miner.Recommit,
		Category: flags.MinerCategory,
	}
	MinerNoVerifyFlag = &cli.BoolFlag{
		Name:     "miner.noverify",
		Usage:    "Disable remote sealing verification",
		Category: flags.MinerCategory,
	}
	MinerNewPayloadTimeout = &cli.DurationFlag{
		Name:     "miner.newpayload-timeout",
		Usage:    "Specify the maximum time allowance for creating a new payload",
		Value:    ethconfig.Defaults.Miner.NewPayloadTimeout,
		Category: flags.MinerCategory,
	}

	// Account settings
	UnlockedAccountFlag = &cli.StringFlag{
		Name:     "unlock",
		Usage:    "Comma separated list of accounts to unlock",
		Value:    "",
		Category: flags.AccountCategory,
	}
	PasswordFileFlag = &cli.PathFlag{
		Name:      "password",
		Usage:     "Password file to use for non-interactive password input",
		TakesFile: true,
		Category:  flags.AccountCategory,
	}
	InsecureUnlockAllowedFlag = &cli.BoolFlag{
		Name:     "allow-insecure-unlock",
		Usage:    "Allow insecure account unlocking when account-related RPCs are exposed by http",
		Category: flags.AccountCategory,
	}

	// EVM settings
	VMEnableDebugFlag = &cli.BoolFlag{
		Name:     "vmdebug",
		Usage:    "Record information useful for VM and contract debugging",
		Category: flags.VMCategory,
	}

	// API options.
	RPCGlobalGasCapFlag = &cli.Uint64Flag{
		Name:     "rpc.gascap",
		Usage:    "Sets a cap on gas that can be used in eth_call/estimateGas (0=infinite)",
		Value:    ethconfig.Defaults.RPCGasCap,
		Category: flags.APICategory,
	}
	RPCGlobalEVMTimeoutFlag = &cli.DurationFlag{
		Name:     "rpc.evmtimeout",
		Usage:    "Sets a timeout used for eth_call (0=infinite)",
		Value:    ethconfig.Defaults.RPCEVMTimeout,
		Category: flags.APICategory,
	}
	RPCGlobalTxFeeCapFlag = &cli.Float64Flag{
		Name:     "rpc.txfeecap",
		Usage:    "Sets a cap on transaction fee (in ether) that can be sent via the RPC APIs (0 = no cap)",
		Value:    ethconfig.Defaults.RPCTxFeeCap,
		Category: flags.APICategory,
	}
	// Authenticated RPC HTTP settings
	AuthListenFlag = &cli.StringFlag{
		Name:     "authrpc.addr",
		Usage:    "Listening address for authenticated APIs",
		Value:    node.DefaultConfig.AuthAddr,
		Category: flags.APICategory,
	}
	AuthPortFlag = &cli.IntFlag{
		Name:     "authrpc.port",
		Usage:    "Listening port for authenticated APIs",
		Value:    node.DefaultConfig.AuthPort,
		Category: flags.APICategory,
	}
	AuthVirtualHostsFlag = &cli.StringFlag{
		Name:     "authrpc.vhosts",
		Usage:    "Comma separated list of virtual hostnames from which to accept requests (server enforced). Accepts '*' wildcard.",
		Value:    strings.Join(node.DefaultConfig.AuthVirtualHosts, ","),
		Category: flags.APICategory,
	}
	JWTSecretFlag = &flags.DirectoryFlag{
		Name:     "authrpc.jwtsecret",
		Usage:    "Path to a JWT secret to use for authenticated RPC endpoints",
		Category: flags.APICategory,
	}

	// Logging and debug settings
	NoCompactionFlag = &cli.BoolFlag{
		Name:     "nocompaction",
		Usage:    "Disables db compaction after import",
		Category: flags.LoggingCategory,
	}

	// RPC settings
	IPCDisabledFlag = &cli.BoolFlag{
		Name:     "ipcdisable",
		Usage:    "Disable the IPC-RPC server",
		Category: flags.APICategory,
	}
	IPCPathFlag = &flags.DirectoryFlag{
		Name:     "ipcpath",
		Usage:    "Filename for IPC socket/pipe within the datadir (explicit paths escape it)",
		Category: flags.APICategory,
	}
	HTTPEnabledFlag = &cli.BoolFlag{
		Name:     "http",
		Usage:    "Enable the HTTP-RPC server",
		Category: flags.APICategory,
	}
	HTTPListenAddrFlag = &cli.StringFlag{
		Name:     "http.addr",
		Usage:    "HTTP-RPC server listening interface",
		Value:    node.DefaultHTTPHost,
		Category: flags.APICategory,
	}
	HTTPPortFlag = &cli.IntFlag{
		Name:     "http.port",
		Usage:    "HTTP-RPC server listening port",
		Value:    node.DefaultHTTPPort,
		Category: flags.APICategory,
	}
	HTTPCORSDomainFlag = &cli.StringFlag{
		Name:     "http.corsdomain",
		Usage:    "Comma separated list of domains from which to accept cross origin requests (browser enforced)",
		Value:    "",
		Category: flags.APICategory,
	}
	HTTPVirtualHostsFlag = &cli.StringFlag{
		Name:     "http.vhosts",
		Usage:    "Comma separated list of virtual hostnames from which to accept requests (server enforced). Accepts '*' wildcard.",
		Value:    strings.Join(node.DefaultConfig.HTTPVirtualHosts, ","),
		Category: flags.APICategory,
	}
	HTTPApiFlag = &cli.StringFlag{
		Name:     "http.api",
		Usage:    "API's offered over the HTTP-RPC interface",
		Value:    "",
		Category: flags.APICategory,
	}
	HTTPPathPrefixFlag = &cli.StringFlag{
		Name:     "http.rpcprefix",
		Usage:    "HTTP path path prefix on which JSON-RPC is served. Use '/' to serve on all paths.",
		Value:    "",
		Category: flags.APICategory,
	}
	GraphQLCORSDomainFlag = &cli.StringFlag{
		Name:     "graphql.corsdomain",
		Usage:    "Comma separated list of domains from which to accept cross origin requests (browser enforced)",
		Value:    "",
		Category: flags.APICategory,
	}
	GraphQLVirtualHostsFlag = &cli.StringFlag{
		Name:     "graphql.vhosts",
		Usage:    "Comma separated list of virtual hostnames from which to accept requests (server enforced). Accepts '*' wildcard.",
		Value:    strings.Join(node.DefaultConfig.GraphQLVirtualHosts, ","),
		Category: flags.APICategory,
	}
	WSEnabledFlag = &cli.BoolFlag{
		Name:     "ws",
		Usage:    "Enable the WS-RPC server",
		Category: flags.APICategory,
	}
	WSListenAddrFlag = &cli.StringFlag{
		Name:     "ws.addr",
		Usage:    "WS-RPC server listening interface",
		Value:    node.DefaultWSHost,
		Category: flags.APICategory,
	}
	WSPortFlag = &cli.IntFlag{
		Name:     "ws.port",
		Usage:    "WS-RPC server listening port",
		Value:    node.DefaultWSPort,
		Category: flags.APICategory,
	}
	WSApiFlag = &cli.StringFlag{
		Name:     "ws.api",
		Usage:    "API's offered over the WS-RPC interface",
		Value:    "",
		Category: flags.APICategory,
	}
	WSAllowedOriginsFlag = &cli.StringFlag{
		Name:     "ws.origins",
		Usage:    "Origins from which to accept websockets requests",
		Value:    "",
		Category: flags.APICategory,
	}
	WSPathPrefixFlag = &cli.StringFlag{
		Name:     "ws.rpcprefix",
		Usage:    "HTTP path prefix on which JSON-RPC is served. Use '/' to serve on all paths.",
		Value:    "",
		Category: flags.APICategory,
	}
	ExecFlag = &cli.StringFlag{
		Name:     "exec",
		Usage:    "Execute JavaScript statement",
		Category: flags.APICategory,
	}
	PreloadJSFlag = &cli.StringFlag{
		Name:     "preload",
		Usage:    "Comma separated list of JavaScript files to preload into the console",
		Category: flags.APICategory,
	}
	AllowUnprotectedTxs = &cli.BoolFlag{
		Name:     "rpc.allow-unprotected-txs",
		Usage:    "Allow for unprotected (non EIP155 signed) transactions to be submitted via RPC",
		Category: flags.APICategory,
	}

	// Network Settings
	MaxPeersFlag = &cli.IntFlag{
		Name:     "maxpeers",
		Usage:    "Maximum number of network peers (network disabled if set to 0)",
		Value:    node.DefaultConfig.P2P.MaxPeers,
		Category: flags.NetworkingCategory,
	}
	MaxPendingPeersFlag = &cli.IntFlag{
		Name:     "maxpendpeers",
		Usage:    "Maximum number of pending connection attempts (defaults used if set to 0)",
		Value:    node.DefaultConfig.P2P.MaxPendingPeers,
		Category: flags.NetworkingCategory,
	}
	ListenPortFlag = &cli.IntFlag{
		Name:     "port",
		Usage:    "Network listening port",
		Value:    30303,
		Category: flags.NetworkingCategory,
	}
	NodeKeyFileFlag = &cli.StringFlag{
		Name:     "nodekey",
		Usage:    "P2P node key file",
		Category: flags.NetworkingCategory,
	}
	NodeKeyHexFlag = &cli.StringFlag{
		Name:     "nodekeyhex",
		Usage:    "P2P node key as hex (for testing)",
		Category: flags.NetworkingCategory,
	}
	DiscoveryPortFlag = &cli.IntFlag{
		Name:     "discovery.port",
		Usage:    "Use a custom UDP port for P2P discovery",
		Value:    30303,
		Category: flags.NetworkingCategory,
	}

	// Console
	JSpathFlag = &flags.DirectoryFlag{
		Name:     "jspath",
		Usage:    "JavaScript root path for `loadScript`",
		Value:    flags.DirectoryString("."),
		Category: flags.APICategory,
	}
	HttpHeaderFlag = &cli.StringSliceFlag{
		Name:     "header",
		Aliases:  []string{"H"},
		Usage:    "Pass custom headers to the RPC server when using --" + RemoteDBFlag.Name + " or the geth attach console. This flag can be given multiple times.",
		Category: flags.APICategory,
	}

	// Gas price oracle settings
	GpoBlocksFlag = &cli.IntFlag{
		Name:     "gpo.blocks",
		Usage:    "Number of recent blocks to check for gas prices",
		Value:    ethconfig.Defaults.GPO.Blocks,
		Category: flags.GasPriceCategory,
	}
	GpoPercentileFlag = &cli.IntFlag{
		Name:     "gpo.percentile",
		Usage:    "Suggested gas price is the given percentile of a set of recent transaction gas prices",
		Value:    ethconfig.Defaults.GPO.Percentile,
		Category: flags.GasPriceCategory,
	}
	GpoMaxGasPriceFlag = &cli.Int64Flag{
		Name:     "gpo.maxprice",
		Usage:    "Maximum transaction priority fee (or gasprice before London fork) to be recommended by gpo",
		Value:    ethconfig.Defaults.GPO.MaxPrice.Int64(),
		Category: flags.GasPriceCategory,
	}
	GpoIgnoreGasPriceFlag = &cli.Int64Flag{
		Name:     "gpo.ignoreprice",
		Usage:    "Gas price below which gpo will ignore transactions",
		Value:    ethconfig.Defaults.GPO.IgnorePrice.Int64(),
		Category: flags.GasPriceCategory,
	}

	ForceFlag = &cli.BoolFlag{
		Name:     "force",
		Usage:    "Force empty datadir before execution",
		Category: flags.EmuCategory,
	}
	NodesFlag = &cli.IntFlag{
		Name:     "nodes",
		Usage:    "Number of emulated nodes",
		Value:    30,
		Category: flags.EmuCategory,
	}
	MinersFlag = &cli.IntFlag{
		Name:     "miners",
		Usage:    "Number of emulated miners",
		Value:    5,
		Category: flags.EmuCategory,
	}
	MinPeerFlag = &cli.IntFlag{
		Name:     "peer.min",
		Usage:    "Restrict the minimum peer num for each node",
		Value:    20,
		Category: flags.EmuCategory,
	}
	MaxPeerFlag = &cli.IntFlag{
		Name:     "peer.max",
		Usage:    "Restrict the maximum peer num for each node",
		Value:    50,
		Category: flags.EmuCategory,
	}
	MinTxFlag = &cli.IntFlag{
		Name:     "tx.min",
		Usage:    "Restrict the minimum transaction num sent during each block time",
		Value:    150,
		Category: flags.EmuCategory,
	}
	MaxTxFlag = &cli.IntFlag{
		Name:     "tx.max",
		Usage:    "Restrict the maximum transaction num sent during each block time",
		Value:    500,
		Category: flags.EmuCategory,
	}
	BlockTimeFlag = &cli.IntFlag{
		Name:     "block.time",
		Usage:    "Interval of consensus for blocks",
		Value:    3,
		Category: flags.EmuCategory,
	}
	LatencyFlag = &cli.IntFlag{
		Name:     "latency",
		Usage:    "Latency of the network",
		Value:    50,
		Category: flags.EmuCategory,
	}
	BandwidthFlag = &cli.IntFlag{
		Name:     "bandwidth",
		Usage:    "Bandwidth of the network",
		Value:    4096,
		Category: flags.EmuCategory,
	}
	TxModeFlag = &cli.BoolFlag{
		Name:     "txmode",
		Value:    false,
		Category: flags.EmuCategory,
	}
	BlockSizeFlag = &cli.IntFlag{
		Name:     "block.size",
		Usage:    "Size of blocks",
		Value:    1024,
		Category: flags.EmuCategory,
	}
)

var (
	// DatabasePathFlags is the flag group of all database path flags.
	DatabasePathFlags = []cli.Flag{
		DataDirFlag,
		AncientFlag,
		RemoteDBFlag,
		HttpHeaderFlag,
	}
)

// setNodeKey creates a node key from set command line flags, either loading it
// from a file or as a specified hex value. If neither flags were provided, this
// method returns nil and an emphemeral key is to be generated.
func setNodeKey(ctx *cli.Context, cfg *p2p.Config) {
	var (
		hex  = ctx.String(NodeKeyHexFlag.Name)
		file = ctx.String(NodeKeyFileFlag.Name)
		key  *ecdsa.PrivateKey
		err  error
	)
	switch {
	case file != "" && hex != "":
		Fatalf("Options %q and %q are mutually exclusive", NodeKeyFileFlag.Name, NodeKeyHexFlag.Name)
	case file != "":
		if key, err = crypto.LoadECDSA(file); err != nil {
			Fatalf("Option %q: %v", NodeKeyFileFlag.Name, err)
		}
		cfg.PrivateKey = key
	case hex != "":
		if key, err = crypto.HexToECDSA(hex); err != nil {
			Fatalf("Option %q: %v", NodeKeyHexFlag.Name, err)
		}
		cfg.PrivateKey = key
	}
}

// setNodeUserIdent creates the user identifier from CLI flags.
func setNodeUserIdent(ctx *cli.Context, cfg *node.Config) {
	if identity := ctx.String(IdentityFlag.Name); len(identity) > 0 {
		cfg.UserIdent = identity
	}
}

// setListenAddress creates TCP/UDP listening address strings from set command
// line flags
func setListenAddress(ctx *cli.Context, cfg *p2p.Config, emu *emu.Node) {
	if ctx.IsSet(ListenPortFlag.Name) {
		cfg.ListenAddr = fmt.Sprintf(":%d", ctx.Int(ListenPortFlag.Name))
	}
	if ctx.IsSet(DiscoveryPortFlag.Name) {
		cfg.DiscAddr = fmt.Sprintf(":%d", ctx.Int(DiscoveryPortFlag.Name))
	}
	if emu != nil {
		cfg.ListenAddr = fmt.Sprintf(":%d", 40000+emu.Identity)
		cfg.DiscAddr = fmt.Sprintf(":%d", 40000+emu.Identity)
	}
}

// SplitAndTrim splits input separated by a comma
// and trims excessive white space from the substrings.
func SplitAndTrim(input string) (ret []string) {
	l := strings.Split(input, ",")
	for _, r := range l {
		if r = strings.TrimSpace(r); r != "" {
			ret = append(ret, r)
		}
	}
	return ret
}

// setHTTP creates the HTTP RPC listener interface string from the set
// command line flags, returning empty if the HTTP endpoint is disabled.
func setHTTP(ctx *cli.Context, cfg *node.Config, emu *emu.Node) {
	if ctx.Bool(HTTPEnabledFlag.Name) && cfg.HTTPHost == "" {
		cfg.HTTPHost = "127.0.0.1"
		if ctx.IsSet(HTTPListenAddrFlag.Name) {
			cfg.HTTPHost = ctx.String(HTTPListenAddrFlag.Name)
		}
	}

	if ctx.IsSet(HTTPPortFlag.Name) {
		cfg.HTTPPort = ctx.Int(HTTPPortFlag.Name)
	}

	if ctx.IsSet(AuthListenFlag.Name) {
		cfg.AuthAddr = ctx.String(AuthListenFlag.Name)
	}

	if ctx.IsSet(AuthPortFlag.Name) {
		cfg.AuthPort = ctx.Int(AuthPortFlag.Name)
	}
	if emu != nil {
		cfg.AuthPort = int(50000 + emu.Identity)
	}

	if ctx.IsSet(AuthVirtualHostsFlag.Name) {
		cfg.AuthVirtualHosts = SplitAndTrim(ctx.String(AuthVirtualHostsFlag.Name))
	}

	if ctx.IsSet(HTTPCORSDomainFlag.Name) {
		cfg.HTTPCors = SplitAndTrim(ctx.String(HTTPCORSDomainFlag.Name))
	}

	if ctx.IsSet(HTTPApiFlag.Name) {
		cfg.HTTPModules = SplitAndTrim(ctx.String(HTTPApiFlag.Name))
	}

	if ctx.IsSet(HTTPVirtualHostsFlag.Name) {
		cfg.HTTPVirtualHosts = SplitAndTrim(ctx.String(HTTPVirtualHostsFlag.Name))
	}

	if ctx.IsSet(HTTPPathPrefixFlag.Name) {
		cfg.HTTPPathPrefix = ctx.String(HTTPPathPrefixFlag.Name)
	}
	if ctx.IsSet(AllowUnprotectedTxs.Name) {
		cfg.AllowUnprotectedTxs = ctx.Bool(AllowUnprotectedTxs.Name)
	}
}

// setGraphQL creates the GraphQL listener interface string from the set
// command line flags, returning empty if the GraphQL endpoint is disabled.
func setGraphQL(ctx *cli.Context, cfg *node.Config) {
	if ctx.IsSet(GraphQLCORSDomainFlag.Name) {
		cfg.GraphQLCors = SplitAndTrim(ctx.String(GraphQLCORSDomainFlag.Name))
	}
	if ctx.IsSet(GraphQLVirtualHostsFlag.Name) {
		cfg.GraphQLVirtualHosts = SplitAndTrim(ctx.String(GraphQLVirtualHostsFlag.Name))
	}
}

// setWS creates the WebSocket RPC listener interface string from the set
// command line flags, returning empty if the HTTP endpoint is disabled.
func setWS(ctx *cli.Context, cfg *node.Config) {
	if ctx.Bool(WSEnabledFlag.Name) && cfg.WSHost == "" {
		cfg.WSHost = "127.0.0.1"
		if ctx.IsSet(WSListenAddrFlag.Name) {
			cfg.WSHost = ctx.String(WSListenAddrFlag.Name)
		}
	}
	if ctx.IsSet(WSPortFlag.Name) {
		cfg.WSPort = ctx.Int(WSPortFlag.Name)
	}

	if ctx.IsSet(WSAllowedOriginsFlag.Name) {
		cfg.WSOrigins = SplitAndTrim(ctx.String(WSAllowedOriginsFlag.Name))
	}

	if ctx.IsSet(WSApiFlag.Name) {
		cfg.WSModules = SplitAndTrim(ctx.String(WSApiFlag.Name))
	}

	if ctx.IsSet(WSPathPrefixFlag.Name) {
		cfg.WSPathPrefix = ctx.String(WSPathPrefixFlag.Name)
	}
}

// setIPC creates an IPC path configuration from the set command line flags,
// returning an empty string if IPC was explicitly disabled, or the set path.
func setIPC(ctx *cli.Context, cfg *node.Config) {
	CheckExclusive(ctx, IPCDisabledFlag, IPCPathFlag)
	switch {
	case ctx.Bool(IPCDisabledFlag.Name):
		cfg.IPCPath = ""
	case ctx.IsSet(IPCPathFlag.Name):
		cfg.IPCPath = ctx.String(IPCPathFlag.Name)
	}
}

// MakeDatabaseHandles raises out the number of allowed file handles per process
// for Geth and returns half of the allowance to assign to the database.
func MakeDatabaseHandles(max int) int {
	limit, err := fdlimit.Maximum()
	if err != nil {
		Fatalf("Failed to retrieve file descriptor allowance: %v", err)
	}
	switch {
	case max == 0:
		// User didn't specify a meaningful value, use system limits
	case max < 128:
		// User specified something unhealthy, just use system defaults
		log.Error("File descriptor limit invalid (<128)", "had", max, "updated", limit)
	case max > limit:
		// User requested more than the OS allows, notify that we can't allocate it
		log.Warn("Requested file descriptors denied by OS", "req", max, "limit", limit)
	default:
		// User limit is meaningful and within allowed range, use that
		limit = max
	}
	raised, err := fdlimit.Raise(uint64(limit))
	if err != nil {
		Fatalf("Failed to raise file descriptor allowance: %v", err)
	}
	return int(raised / 2) // Leave half for networking and other stuff
}

// MakeAddress converts an account specified directly as a hex encoded string or
// a key index in the key store to an internal account representation.
func MakeAddress(ks *keystore.KeyStore, account string) (accounts.Account, error) {
	// If the specified account is a valid address, return it
	if common.IsHexAddress(account) {
		return accounts.Account{Address: common.HexToAddress(account)}, nil
	}
	// Otherwise try to interpret the account as a keystore index
	index, err := strconv.Atoi(account)
	if err != nil || index < 0 {
		return accounts.Account{}, fmt.Errorf("invalid account address or index %q", account)
	}
	log.Warn("-------------------------------------------------------------------")
	log.Warn("Referring to accounts by order in the keystore folder is dangerous!")
	log.Warn("This functionality is deprecated and will be removed in the future!")
	log.Warn("Please use explicit addresses! (can search via `geth account list`)")
	log.Warn("-------------------------------------------------------------------")

	accs := ks.Accounts()
	if len(accs) <= index {
		return accounts.Account{}, fmt.Errorf("index %d higher than number of accounts %d", index, len(accs))
	}
	return accs[index], nil
}

// setEtherbase retrieves the etherbase from the directly specified command line flags.
func setEtherbase(ctx *cli.Context, cfg *ethconfig.Config, emu *emu.Node) {
	if emu != nil {
		cfg.Miner.Etherbase = emu.Address
		return
	}
	if !ctx.IsSet(MinerEtherbaseFlag.Name) {
		return
	}
	addr := ctx.String(MinerEtherbaseFlag.Name)
	if strings.HasPrefix(addr, "0x") || strings.HasPrefix(addr, "0X") {
		addr = addr[2:]
	}
	b, err := hex.DecodeString(addr)
	if err != nil || len(b) != common.AddressLength {
		Fatalf("-%s: invalid etherbase address %q", MinerEtherbaseFlag.Name, addr)
		return
	}
	cfg.Miner.Etherbase = common.BytesToAddress(b)
}

func SetP2PConfig(ctx *cli.Context, cfg *p2p.Config, emu *emu.Node) {
	setNodeKey(ctx, cfg)
	setListenAddress(ctx, cfg, emu)

	if ctx.IsSet(MaxPeersFlag.Name) {
		cfg.MaxPeers = ctx.Int(MaxPeersFlag.Name)
	}

	if ctx.IsSet(MaxPendingPeersFlag.Name) {
		cfg.MaxPendingPeers = ctx.Int(MaxPendingPeersFlag.Name)
	}
}

// SetNodeConfig applies node-related command line flags to the config.
func SetNodeConfig(ctx *cli.Context, cfg *node.Config, emu *emu.Node) {
	SetP2PConfig(ctx, &cfg.P2P, emu)
	setIPC(ctx, cfg)
	setHTTP(ctx, cfg, emu)
	setGraphQL(ctx, cfg)
	setWS(ctx, cfg)
	setNodeUserIdent(ctx, cfg)
	SetDataDir(ctx, cfg, emu)

	if ctx.IsSet(JWTSecretFlag.Name) {
		cfg.JWTSecret = ctx.String(JWTSecretFlag.Name)
	}

	if ctx.IsSet(KeyStoreDirFlag.Name) {
		cfg.KeyStoreDir = ctx.String(KeyStoreDirFlag.Name)
	}
	if ctx.IsSet(InsecureUnlockAllowedFlag.Name) {
		cfg.InsecureUnlockAllowed = ctx.Bool(InsecureUnlockAllowedFlag.Name)
	}
}

func SetDataDir(ctx *cli.Context, cfg *node.Config, emu *emu.Node) {
	switch {
	case ctx.IsSet(DataDirFlag.Name):
		if emu != nil {
			base := ctx.String(DataDirFlag.Name)
			cfg.DataDir = path.Join(base, fmt.Sprintf("emu%06d", emu.Identity))
		} else {
			cfg.DataDir = ctx.String(DataDirFlag.Name)
		}
	}
}

func setGPO(ctx *cli.Context, cfg *gasprice.Config, light bool) {
	// If we are running the light client, apply another group
	// settings for gas oracle.
	if light {
		*cfg = ethconfig.LightClientGPO
	}
	if ctx.IsSet(GpoBlocksFlag.Name) {
		cfg.Blocks = ctx.Int(GpoBlocksFlag.Name)
	}
	if ctx.IsSet(GpoPercentileFlag.Name) {
		cfg.Percentile = ctx.Int(GpoPercentileFlag.Name)
	}
	if ctx.IsSet(GpoMaxGasPriceFlag.Name) {
		cfg.MaxPrice = big.NewInt(ctx.Int64(GpoMaxGasPriceFlag.Name))
	}
	if ctx.IsSet(GpoIgnoreGasPriceFlag.Name) {
		cfg.IgnorePrice = big.NewInt(ctx.Int64(GpoIgnoreGasPriceFlag.Name))
	}
}

func setTxPool(ctx *cli.Context, cfg *txpool.Config) {
	if ctx.IsSet(TxPoolLocalsFlag.Name) {
		locals := strings.Split(ctx.String(TxPoolLocalsFlag.Name), ",")
		for _, account := range locals {
			if trimmed := strings.TrimSpace(account); !common.IsHexAddress(trimmed) {
				Fatalf("Invalid account in --txpool.locals: %s", trimmed)
			} else {
				cfg.Locals = append(cfg.Locals, common.HexToAddress(account))
			}
		}
	}
	if ctx.IsSet(TxPoolNoLocalsFlag.Name) {
		cfg.NoLocals = ctx.Bool(TxPoolNoLocalsFlag.Name)
	}
	if ctx.IsSet(TxPoolJournalFlag.Name) {
		cfg.Journal = ctx.String(TxPoolJournalFlag.Name)
	}
	if ctx.IsSet(TxPoolRejournalFlag.Name) {
		cfg.Rejournal = ctx.Duration(TxPoolRejournalFlag.Name)
	}
	if ctx.IsSet(TxPoolPriceLimitFlag.Name) {
		cfg.PriceLimit = ctx.Uint64(TxPoolPriceLimitFlag.Name)
	}
	if ctx.IsSet(TxPoolPriceBumpFlag.Name) {
		cfg.PriceBump = ctx.Uint64(TxPoolPriceBumpFlag.Name)
	}
	if ctx.IsSet(TxPoolAccountSlotsFlag.Name) {
		cfg.AccountSlots = ctx.Uint64(TxPoolAccountSlotsFlag.Name)
	}
	if ctx.IsSet(TxPoolGlobalSlotsFlag.Name) {
		cfg.GlobalSlots = ctx.Uint64(TxPoolGlobalSlotsFlag.Name)
	}
	if ctx.IsSet(TxPoolAccountQueueFlag.Name) {
		cfg.AccountQueue = ctx.Uint64(TxPoolAccountQueueFlag.Name)
	}
	if ctx.IsSet(TxPoolGlobalQueueFlag.Name) {
		cfg.GlobalQueue = ctx.Uint64(TxPoolGlobalQueueFlag.Name)
	}
	if ctx.IsSet(TxPoolLifetimeFlag.Name) {
		cfg.Lifetime = ctx.Duration(TxPoolLifetimeFlag.Name)
	}
}

func setMiner(ctx *cli.Context, cfg *miner.Config) {
	if ctx.IsSet(MinerNotifyFlag.Name) {
		cfg.Notify = strings.Split(ctx.String(MinerNotifyFlag.Name), ",")
	}
	if ctx.IsSet(MinerExtraDataFlag.Name) {
		cfg.ExtraData = []byte(ctx.String(MinerExtraDataFlag.Name))
	}
	if ctx.IsSet(MinerGasLimitFlag.Name) {
		cfg.GasCeil = ctx.Uint64(MinerGasLimitFlag.Name)
	}
	if ctx.IsSet(MinerGasPriceFlag.Name) {
		cfg.GasPrice = flags.GlobalBig(ctx, MinerGasPriceFlag.Name)
	}
	if ctx.IsSet(MinerRecommitIntervalFlag.Name) {
		cfg.Recommit = ctx.Duration(MinerRecommitIntervalFlag.Name)
	}
	if ctx.IsSet(MinerNoVerifyFlag.Name) {
		cfg.Noverify = ctx.Bool(MinerNoVerifyFlag.Name)
	}
	if ctx.IsSet(MinerNewPayloadTimeout.Name) {
		cfg.NewPayloadTimeout = ctx.Duration(MinerNewPayloadTimeout.Name)
	}
}

// CheckExclusive verifies that only a single instance of the provided flags was
// set by the user. Each flag might optionally be followed by a string type to
// specialize it further.
func CheckExclusive(ctx *cli.Context, args ...interface{}) {
	set := make([]string, 0, 1)
	for i := 0; i < len(args); i++ {
		// Make sure the next argument is a flag and skip if not set
		flag, ok := args[i].(cli.Flag)
		if !ok {
			panic(fmt.Sprintf("invalid argument, not cli.Flag type: %T", args[i]))
		}
		// Check if next arg extends current and expand its name if so
		name := flag.Names()[0]

		if i+1 < len(args) {
			switch option := args[i+1].(type) {
			case string:
				// Extended flag check, make sure value set doesn't conflict with passed in option
				if ctx.String(flag.Names()[0]) == option {
					name += "=" + option
					set = append(set, "--"+name)
				}
				// shift arguments and continue
				i++
				continue

			case cli.Flag:
			default:
				panic(fmt.Sprintf("invalid argument, not cli.Flag or string extension: %T", args[i+1]))
			}
		}
		// Mark the flag if it's set
		if ctx.IsSet(flag.Names()[0]) {
			set = append(set, "--"+name)
		}
	}
	if len(set) > 1 {
		Fatalf("Flags %v can't be used at the same time", strings.Join(set, ", "))
	}
}

// SetEthConfig applies eth-related command line flags to the config.
func SetEthConfig(ctx *cli.Context, cfg *ethconfig.Config, emu *emu.Node) {
	// Avoid conflicting network flags
	if ctx.String(GCModeFlag.Name) == "archive" && ctx.Uint64(TxLookupLimitFlag.Name) != 0 {
		ctx.Set(TxLookupLimitFlag.Name, "0")
		log.Warn("Disable transaction unindexing for archive node")
	}
	setEtherbase(ctx, cfg, emu)
	setGPO(ctx, &cfg.GPO, ctx.String(SyncModeFlag.Name) == "light")
	setTxPool(ctx, &cfg.TxPool)
	setMiner(ctx, &cfg.Miner)

	// Ensure Go's GC ignores the database cache for trigger percentage
	cache := ctx.Int(CacheFlag.Name)
	gogc := math.Max(20, math.Min(100, 100/(float64(cache)/1024)))

	log.Debug("Sanitizing Go's GC trigger", "percent", int(gogc))
	godebug.SetGCPercent(int(gogc))

	if ctx.IsSet(SyncModeFlag.Name) {
		cfg.SyncMode = *flags.GlobalTextMarshaler(ctx, SyncModeFlag.Name).(*downloader.SyncMode)
	}
	if ctx.IsSet(NetworkIdFlag.Name) {
		cfg.NetworkId = ctx.Uint64(NetworkIdFlag.Name)
	}
	if ctx.IsSet(CacheFlag.Name) || ctx.IsSet(CacheDatabaseFlag.Name) {
		cfg.DatabaseCache = ctx.Int(CacheFlag.Name) * ctx.Int(CacheDatabaseFlag.Name) / 100
	}
	cfg.DatabaseHandles = MakeDatabaseHandles(ctx.Int(FDLimitFlag.Name))
	if ctx.IsSet(AncientFlag.Name) {
		cfg.DatabaseFreezer = ctx.String(AncientFlag.Name)
	}

	if gcmode := ctx.String(GCModeFlag.Name); gcmode != "full" && gcmode != "archive" {
		Fatalf("--%s must be either 'full' or 'archive'", GCModeFlag.Name)
	}
	if ctx.IsSet(GCModeFlag.Name) {
		cfg.NoPruning = ctx.String(GCModeFlag.Name) == "archive"
	}
	if ctx.IsSet(CacheNoPrefetchFlag.Name) {
		cfg.NoPrefetch = ctx.Bool(CacheNoPrefetchFlag.Name)
	}
	// Read the value from the flag no matter if it's set or not.
	cfg.Preimages = ctx.Bool(CachePreimagesFlag.Name)
	if cfg.NoPruning && !cfg.Preimages {
		cfg.Preimages = true
		log.Info("Enabling recording of key preimages since archive mode is used")
	}
	if ctx.IsSet(TxLookupLimitFlag.Name) {
		cfg.TxLookupLimit = ctx.Uint64(TxLookupLimitFlag.Name)
	}
	if ctx.IsSet(CacheFlag.Name) || ctx.IsSet(CacheTrieFlag.Name) {
		cfg.TrieCleanCache = ctx.Int(CacheFlag.Name) * ctx.Int(CacheTrieFlag.Name) / 100
	}
	if ctx.IsSet(CacheTrieJournalFlag.Name) {
		cfg.TrieCleanCacheJournal = ctx.String(CacheTrieJournalFlag.Name)
	}
	if ctx.IsSet(CacheTrieRejournalFlag.Name) {
		cfg.TrieCleanCacheRejournal = ctx.Duration(CacheTrieRejournalFlag.Name)
	}
	if ctx.IsSet(CacheFlag.Name) || ctx.IsSet(CacheGCFlag.Name) {
		cfg.TrieDirtyCache = ctx.Int(CacheFlag.Name) * ctx.Int(CacheGCFlag.Name) / 100
	}
	if ctx.IsSet(CacheFlag.Name) || ctx.IsSet(CacheSnapshotFlag.Name) {
		cfg.SnapshotCache = ctx.Int(CacheFlag.Name) * ctx.Int(CacheSnapshotFlag.Name) / 100
	}
	if ctx.IsSet(CacheLogSizeFlag.Name) {
		cfg.FilterLogCacheSize = ctx.Int(CacheLogSizeFlag.Name)
	}
	if !ctx.Bool(SnapshotFlag.Name) {
		// If snap-sync is requested, this flag is also required
		if cfg.SyncMode == downloader.SnapSync {
			log.Info("Snap sync requested, enabling --snapshot")
		} else {
			cfg.TrieCleanCache += cfg.SnapshotCache
			cfg.SnapshotCache = 0 // Disabled
		}
	}
	if ctx.IsSet(DocRootFlag.Name) {
		cfg.DocRoot = ctx.String(DocRootFlag.Name)
	}
	if ctx.IsSet(VMEnableDebugFlag.Name) {
		// TODO(fjl): force-enable this in --dev mode
		cfg.EnablePreimageRecording = ctx.Bool(VMEnableDebugFlag.Name)
	}

	if ctx.IsSet(RPCGlobalGasCapFlag.Name) {
		cfg.RPCGasCap = ctx.Uint64(RPCGlobalGasCapFlag.Name)
	}
	if cfg.RPCGasCap != 0 {
		log.Info("Set global gas cap", "cap", cfg.RPCGasCap)
	} else {
		log.Info("Global gas cap disabled")
	}
	if ctx.IsSet(RPCGlobalEVMTimeoutFlag.Name) {
		cfg.RPCEVMTimeout = ctx.Duration(RPCGlobalEVMTimeoutFlag.Name)
	}
	if ctx.IsSet(RPCGlobalTxFeeCapFlag.Name) {
		cfg.RPCTxFeeCap = ctx.Float64(RPCGlobalTxFeeCapFlag.Name)
	}
}

// RegisterEthService adds an Ethereum client to the stack.
// The second return value is the full node instance, which may be nil if the
// node is running as a light client.
func RegisterEthService(stack *node.Node, cfg *ethconfig.Config, id int, blockLog *os.File, txLog *os.File) (ethapi.Backend, *eth.Ethereum) {
	backend, err := eth.New(stack, cfg, id, blockLog, txLog)
	if err != nil {
		Fatalf("Failed to register the Ethereum service: %v", err)
	}
	return backend.APIBackend, backend
}
