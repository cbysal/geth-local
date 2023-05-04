package emu

import (
	"github.com/ethereum/go-ethereum/common"
)

type Node struct {
	Identity uint64
	Address  common.Address
	Peers    []common.Address
}
