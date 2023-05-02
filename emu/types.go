package emu

import (
	"github.com/ethereum/go-ethereum/common"
)

type Node struct {
	Identity uint64
	Address  common.Address
	IsMiner  bool
	Peers    []common.Address
}
