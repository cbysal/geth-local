package main

import (
	"fmt"
	mrand "math/rand"
	"os"
	"path"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/cmd/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/emu"
	"github.com/ethereum/go-ethereum/internal/debug"
	"github.com/ethereum/go-ethereum/internal/flags"
	"github.com/urfave/cli/v2"
)

var genCommand = &cli.Command{
	Action: generateConfig,
	Name:   "gen",
	Usage:  "Generate config file",
	Flags: flags.Merge(
		nodeFlags,
		rpcFlags,
		debug.Flags,
		emuFlags),
}

func generateConfig(ctx *cli.Context) error {
	cfg := gethConfig{Node: defaultNodeConfig()}
	utils.SetNodeConfig(ctx, &cfg.Node, nil)
	dataDir := cfg.Node.DataDir
	if ctx.Bool("force") {
		if err := os.RemoveAll(dataDir); err != nil {
			return err
		}
		if err := os.MkdirAll(dataDir, 0755); err != nil {
			return err
		}
	}

	num := ctx.Int(utils.NodesFlag.Name)
	nodes := make([]*emu.Node, num)
	for i := range nodes {
		nodes[i] = &emu.Node{}
	}
	setId(nodes)
	if err := setAddr(ctx, nodes); err != nil {
		return err
	}
	setMiners(ctx, nodes)
	setPeers(ctx, nodes)

	emu.Global.Period = uint64(ctx.Int(utils.BlockTimeFlag.Name))
	emu.Global.MinTxInterval = emu.Global.Period * 1000 / uint64(ctx.Int(utils.MaxTxFlag.Name))
	emu.Global.MaxTxInterval = emu.Global.Period * 1000 / uint64(ctx.Int(utils.MinTxFlag.Name))
	emu.Global.Nodes = make(map[common.Address]*emu.Node)
	for _, node := range nodes {
		emu.Global.Nodes[node.Address] = node
	}
	return emu.StoreConfig(dataDir)
}

func setMiners(ctx *cli.Context, nodes []*emu.Node) {
	num := ctx.Int(utils.MinersFlag.Name)
	miners := mapset.NewSet[int]()
	for miners.Cardinality() < num {
		miners.Add(mrand.Intn(len(nodes)))
	}
	for i, node := range nodes {
		node.IsMiner = miners.Contains(i)
	}
}

func setId(nodes []*emu.Node) {
	for i, node := range nodes {
		node.Identity = uint64(i)
	}
}

func setAddr(ctx *cli.Context, nodes []*emu.Node) error {
	for _, node := range nodes {
		keystorePath := path.Join(ctx.String(utils.DataDirFlag.Name), fmt.Sprintf("emu%06d", node.Identity), "keystore")
		account, err := keystore.StoreKey(keystorePath, "", keystore.LightScryptN, keystore.LightScryptP)
		if err != nil {
			return err
		}
		node.Address = account.Address
	}
	return nil
}

func setPeers(ctx *cli.Context, nodes []*emu.Node) {
	minPeer := ctx.Int(utils.MinPeerFlag.Name)
	maxPeer := ctx.Int(utils.MaxPeerFlag.Name)
	peers := make([]mapset.Set[int], len(nodes))
	for i := range nodes {
		peers[i] = mapset.NewSet[int]()
	}
	check := func() bool {
		for _, node := range peers {
			if size := node.Cardinality(); size < minPeer || size > maxPeer {
				return false
			}
		}
		return true
	}
	for !check() {
		for i := range nodes {
			for peers[i].Cardinality() < minPeer {
				j := mrand.Intn(len(nodes))
				if j == i || peers[j].Cardinality() >= maxPeer {
					continue
				}
				peers[i].Add(j)
				peers[j].Add(i)
			}
		}
	}

	for i := range nodes {
		for peer := range peers[i].Iter() {
			nodes[i].Peers = append(nodes[i].Peers, nodes[peer].Address)
		}
	}
}
