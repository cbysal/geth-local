// Copyright 2014 The go-ethereum Authors
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

// geth is the official command-line client for Ethereum.
package main

import (
	"context"
	"fmt"
	"math/big"
	"math/rand"
	"net/http"
	_ "net/http/pprof"
	"os"
	"sort"
	"time"

	"github.com/ethereum/go-ethereum/accounts"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/console/prompt"
	"github.com/ethereum/go-ethereum/emu"
	"github.com/ethereum/go-ethereum/eth"
	"github.com/ethereum/go-ethereum/internal/debug"
	"github.com/ethereum/go-ethereum/internal/ethapi"
	"github.com/ethereum/go-ethereum/internal/flags"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/node"

	"github.com/urfave/cli/v2"
)

const (
	clientIdentifier = "ethemu" // Client identifier to advertise over the network
)

var (
	// flags that configure the node
	nodeFlags = flags.Merge([]cli.Flag{
		utils.IdentityFlag,
		utils.UnlockedAccountFlag,
		utils.PasswordFileFlag,
		utils.MinFreeDiskSpaceFlag,
		utils.KeyStoreDirFlag,
		utils.OverrideShanghai,
		utils.TxPoolLocalsFlag,
		utils.TxPoolNoLocalsFlag,
		utils.TxPoolJournalFlag,
		utils.TxPoolRejournalFlag,
		utils.TxPoolPriceLimitFlag,
		utils.TxPoolPriceBumpFlag,
		utils.TxPoolAccountSlotsFlag,
		utils.TxPoolGlobalSlotsFlag,
		utils.TxPoolAccountQueueFlag,
		utils.TxPoolGlobalQueueFlag,
		utils.TxPoolLifetimeFlag,
		utils.SyncModeFlag,
		utils.GCModeFlag,
		utils.SnapshotFlag,
		utils.TxLookupLimitFlag,
		utils.BloomFilterSizeFlag,
		utils.CacheFlag,
		utils.CacheDatabaseFlag,
		utils.CacheTrieFlag,
		utils.CacheTrieJournalFlag,
		utils.CacheTrieRejournalFlag,
		utils.CacheGCFlag,
		utils.CacheSnapshotFlag,
		utils.CacheNoPrefetchFlag,
		utils.CachePreimagesFlag,
		utils.CacheLogSizeFlag,
		utils.FDLimitFlag,
		utils.ListenPortFlag,
		utils.DiscoveryPortFlag,
		utils.MaxPeersFlag,
		utils.MaxPendingPeersFlag,
		utils.MiningEnabledFlag,
		utils.MinerThreadsFlag,
		utils.MinerNotifyFlag,
		utils.MinerGasLimitFlag,
		utils.MinerGasPriceFlag,
		utils.MinerEtherbaseFlag,
		utils.MinerExtraDataFlag,
		utils.MinerRecommitIntervalFlag,
		utils.MinerNoVerifyFlag,
		utils.MinerNewPayloadTimeout,
		utils.NodeKeyFileFlag,
		utils.NodeKeyHexFlag,
		utils.VMEnableDebugFlag,
		utils.NetworkIdFlag,
		utils.NoCompactionFlag,
		utils.GpoBlocksFlag,
		utils.GpoPercentileFlag,
		utils.GpoMaxGasPriceFlag,
		utils.GpoIgnoreGasPriceFlag,
	}, utils.DatabasePathFlags)

	rpcFlags = []cli.Flag{
		utils.HTTPEnabledFlag,
		utils.HTTPListenAddrFlag,
		utils.HTTPPortFlag,
		utils.HTTPCORSDomainFlag,
		utils.AuthListenFlag,
		utils.AuthPortFlag,
		utils.AuthVirtualHostsFlag,
		utils.JWTSecretFlag,
		utils.HTTPVirtualHostsFlag,
		utils.HTTPApiFlag,
		utils.HTTPPathPrefixFlag,
		utils.WSEnabledFlag,
		utils.WSListenAddrFlag,
		utils.WSPortFlag,
		utils.WSApiFlag,
		utils.WSAllowedOriginsFlag,
		utils.WSPathPrefixFlag,
		utils.IPCDisabledFlag,
		utils.IPCPathFlag,
		utils.InsecureUnlockAllowedFlag,
		utils.RPCGlobalGasCapFlag,
		utils.RPCGlobalEVMTimeoutFlag,
		utils.RPCGlobalTxFeeCapFlag,
		utils.AllowUnprotectedTxs,
	}

	emuFlags = []cli.Flag{
		utils.ForceFlag,
		utils.NodesFlag,
		utils.MinersFlag,
		utils.MinPeerFlag,
		utils.MaxPeerFlag,
		utils.MinTxFlag,
		utils.MaxTxFlag,
		utils.BlockTimeFlag,
	}
)

var app = flags.NewApp("the go-ethereum command line interface")

func init() {
	// Initialize the CLI app and start Geth
	app.Action = ethemu
	app.HideVersion = true // we have a command to print the version
	app.Commands = []*cli.Command{
		// See gencmd.go
		genCommand,
		// See initcmd.go:
		initCommand,
	}
	sort.Sort(cli.CommandsByName(app.Commands))

	app.Flags = flags.Merge(
		nodeFlags,
		rpcFlags,
		consoleFlags,
		debug.Flags,
		emuFlags,
	)

	app.Before = func(ctx *cli.Context) error {
		flags.MigrateGlobalFlags(ctx)
		return debug.Setup(ctx)
	}
	app.After = func(ctx *cli.Context) error {
		prompt.Stdin.Close() // Resets terminal mode.
		return nil
	}
}

func main() {
	go func() {
		http.ListenAndServe("0.0.0.0:6060", nil)
	}()
	if err := app.Run(os.Args); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// ethemu is the main entry point into the system if no special subcommand is run.
// It creates a default node based on the command line arguments and runs it in
// blocking mode, waiting for it to be shut down.
func ethemu(ctx *cli.Context) error {
	dataDir := ctx.String(utils.DataDirFlag.Name)
	emu.LoadConfig(dataDir)
	stopSig := make(chan struct{})

	nodes := make(map[common.Address]*node.Node)
	var firstNode *node.Node
	eths := make(map[common.Address]*eth.Ethereum)
	for _, node := range emu.Global.Nodes {
		stack, eth, backend := makeFullNode(ctx, node)
		nodes[node.Address] = stack
		if firstNode == nil {
			firstNode = stack
		}
		eths[node.Address] = eth

		startNode(ctx, stack, backend, false, node)
	}

	for _, node := range emu.Global.Nodes {
		for _, peer := range node.Peers {
			nodes[node.Address].Server().AddPeer(nodes[peer].Server().Self())
		}
	}

	go func() {
		sleepMin, sleepMax := emu.Global.MinTxInterval, emu.Global.MaxTxInterval
		value := big.NewInt(100000)
		addrs := make([]common.Address, 0, len(emu.Global.Nodes)-1)
		for _, node := range emu.Global.Nodes {
			if !node.IsMiner {
				addrs = append(addrs, node.Address)
			}
		}
		for {
			from := addrs[rand.Intn(len(addrs))]
			to := from
			for from == to {
				to = addrs[rand.Intn(len(addrs))]
			}
			tx := ethapi.TransactionArgs{From: &from, To: &to, Value: (*hexutil.Big)(value)}
			timer := time.NewTimer(time.Duration(rand.Intn(int(sleepMax-sleepMin))+int(sleepMin)) * time.Millisecond)
			select {
			case <-stopSig:
				return
			case <-timer.C:
				eths[from].SendTransaction(context.Background(), tx)
			}
		}
	}()

	go func() {
		sealers := make([]*eth.Ethereum, 0)
		for addr, node := range emu.Global.Nodes {
			if node.IsMiner {
				sealers = append(sealers, eths[addr])
			}
		}
		for {
			sealer := sealers[rand.Intn(len(sealers))]
			etherbase, err := sealer.Etherbase()
			if err != nil {
				return
			}
			timer := time.NewTimer(time.Duration(emu.Global.Period) * time.Second)
			select {
			case <-stopSig:
				return
			case <-timer.C:
				log.Warn("Sealing time", "sealer", etherbase)
				sealer.Miner().Work()
				go func() {
					time.Sleep(time.Second)
					header := sealer.APIBackend.CurrentHeader()
					fmt.Println("blockNumber", header.Number, "trans", sealer.BlockChain().GetBlockByNumber(header.Number.Uint64()).Transactions().Len())
					pending, queued := sealer.APIBackend.TxPool().Stats()
					fmt.Println("pending", pending, "queued", queued)
				}()
			}
		}
	}()

	for _, node := range nodes {
		node.Wait()
	}
	return nil
}

// startNode boots up the system node and all registered protocols, after which
// it unlocks any requested accounts, and starts the RPC/IPC interfaces and the
// miner.
func startNode(ctx *cli.Context, stack *node.Node, backend ethapi.Backend, isConsole bool, emuNode *emu.Node) {
	// Start up the node itself
	utils.StartNode(ctx, stack, isConsole)

	// Unlock any account specifically requested
	unlockAccounts(ctx, stack)

	// Register wallet event handlers to open and auto-derive wallets
	events := make(chan accounts.WalletEvent, 16)
	stack.AccountManager().Subscribe(events)

	// Start auxiliary services if enabled
	if ctx.Bool(utils.MiningEnabledFlag.Name) || emuNode.IsMiner {
		// Mining only makes sense if a full Ethereum node is running
		if ctx.String(utils.SyncModeFlag.Name) == "light" {
			utils.Fatalf("Light clients do not support mining")
		}
		ethBackend, ok := backend.(*eth.EthAPIBackend)
		if !ok {
			utils.Fatalf("Ethereum service not running")
		}
		// Set the gas price to the limits from the CLI and start mining
		gasprice := flags.GlobalBig(ctx, utils.MinerGasPriceFlag.Name)
		ethBackend.TxPool().SetGasPrice(gasprice)
		// start mining
		threads := ctx.Int(utils.MinerThreadsFlag.Name)
		if err := ethBackend.StartMining(threads); err != nil {
			utils.Fatalf("Failed to start mining: %v", err)
		}
	}
}

// unlockAccounts unlocks any account specifically requested.
func unlockAccounts(ctx *cli.Context, stack *node.Node) {
	ks := stack.AccountManager().Backends(keystore.KeyStoreType)[0].(*keystore.KeyStore)
	for _, account := range ks.Accounts() {
		unlockAccount(ks, account.Address.String())
	}
}
