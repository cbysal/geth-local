package emu

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"

	"github.com/ethereum/go-ethereum/common"
)

const CONFIG_JSON = "config.json"

var ErrAddrNotFound = errors.New("Address not found!")

type Config struct {
	Period        uint64
	MinTxInterval uint64
	MaxTxInterval uint64
	Nodes         map[common.Address]*Node
	Latency       uint64
	Bandwidth     uint64
}

var Global Config

func GetAddrById(id string) (common.Address, error) {
	for addr, node := range Global.Nodes {
		if fmt.Sprintf("emu%06d", node.Identity) == id {
			return addr, nil
		}
	}
	return common.Address{}, ErrAddrNotFound
}

func LoadConfig(dataDir string) error {
	configPath := path.Join(dataDir, CONFIG_JSON)
	bytes, err := os.ReadFile(configPath)
	if err != nil {
		return err
	}
	return json.Unmarshal(bytes, &Global)
}

func StoreConfig(dataDir string) error {
	jsonString, err := json.MarshalIndent(Global, "", "  ")
	if err != nil {
		return err
	}
	configPath := path.Join(dataDir, CONFIG_JSON)
	return os.WriteFile(configPath, jsonString, 0744)
}
