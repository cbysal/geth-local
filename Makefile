# This Makefile is meant to be used by people that do not usually work
# with Go source code. If you know what GOPATH is then you probably
# don't need to bother with make.

.PHONY: ethemu

GOBIN = ./build/bin
GOBUILD = env GO111MODULE=on go build

ethemu:
	$(GOBUILD) -ldflags "-extldflags '-Wl,-z,stack-size=0x800000'" -tags urfave_cli_no_docs -trimpath -v -o ./build/bin/ethemu ./cmd/ethemu
	@echo "Done building."
	@echo "Run \"$(GOBIN)/ethemu\" to launch ethemu."
