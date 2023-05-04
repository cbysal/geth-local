package main

import (
	"fmt"
	"golang.org/x/exp/rand"
	"math"
	"os"
	"path"

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
	setPeers(ctx, nodes)

	emu.Global.Latency = uint64(ctx.Int(utils.LatencyFlag.Name))
	emu.Global.Bandwidth = uint64(ctx.Int(utils.BandwidthFlag.Name))
	emu.Global.BlockSize = uint64(ctx.Int(utils.BlockSizeFlag.Name))
	emu.Global.Nodes = make(map[common.Address]*emu.Node)
	for _, node := range nodes {
		emu.Global.Nodes[node.Address] = node
	}
	return emu.StoreConfig(dataDir)
}

func setId(nodes []*emu.Node) {
	for i, node := range nodes {
		node.Identity = uint64(i)
	}
}

func setAddr(ctx *cli.Context, nodes []*emu.Node) error {
	for _, node := range nodes {
		keystorePath := path.Join(ctx.String(utils.DataDirFlag.Name), fmt.Sprintf("emu%06d", node.Identity), "keystore")
		account, err := keystore.StoreKey(keystorePath, "")
		if err != nil {
			return err
		}
		node.Address = account.Address
	}
	return nil
}

func setPeers(ctx *cli.Context, nodes []*emu.Node) {
	getPij := func(dens float64, prop float64, maxDis, i, j int) float64 {
		dis := int(math.Abs(float64(i - j)))
		theta := 0
		if dens-math.Min(float64(dis), float64(len(nodes)-dis))/float64(maxDis) >= 0 {
			theta = 1
		}
		return prop*dens + (1-prop)*float64(theta)
	}
	density := float64(ctx.Int(utils.PeerNumFlag.Name)) / float64(len(nodes)-1)
	maxDist := len(nodes) / 2
	for i := 0; i < len(nodes); i++ {
		for j := i + 1; j < len(nodes); j++ {
			randNum := float64(rand.Intn(100)) / float64(100)
			if randNum < getPij(density, 0.25, maxDist, i, j) {
				nodes[i].Peers = append(nodes[i].Peers, nodes[j].Address)
				nodes[j].Peers = append(nodes[j].Peers, nodes[i].Address)

			}
		}
	}
}
