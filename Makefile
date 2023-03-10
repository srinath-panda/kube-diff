# ########################################################## #
# Makefile for Golang Project
# Includes cross-compiling, installation, cleanup
# ########################################################## #

# Check for required command tools to build or stop immediately
EXECUTABLES = git go find pwd
K := $(foreach exec,$(EXECUTABLES),\
        $(if $(shell which $(exec)),some string,$(error "No $(exec) in PATH)))

ROOT_DIR:=$(shell dirname $(realpath $(lastword $(MAKEFILE_LIST))))

BINARY=kube-diff
VERSION=1.0.1
BUILD=`git rev-parse HEAD`
PLATFORMS=darwin linux windows
ARCHITECTURES=arm64 amd64

# Setup linker flags option for build that interoperate with variable names in src code
LDFLAGS=-ldflags "-X main.Version=${VERSION} -X main.Build=${BUILD}"

default: build

all: clean build_all install

build:
	go build ${LDFLAGS} -o ./releases/${BINARY}

build_all:
	$(foreach GOOS, $(PLATFORMS),\
	$(foreach GOARCH, $(ARCHITECTURES), $(shell export GOOS=$(GOOS); export GOARCH=$(GOARCH); go build -v -o ./releases/$(BINARY)-$(GOOS)-$(GOARCH))))

install:
	go install ${LDFLAGS}

# Remove only what we've created
clean:
	find ${ROOT_DIR}/releases/ -name '${BINARY}[-?][a-zA-Z0-9]*[-?][a-zA-Z0-9]*' -delete

.PHONY: check clean install build_all all

manual:
	@env GOOS=darwin GOARCH=amd64 go build -o ./releases/kube-diff-darwin-amd64
	@env GOOS=darwin GOARCH=arm64 go build -o ./releases/kube-diff-darwin-arm64
	@env GOOS=linux GOARCH=amd64 go build -o ./releases/kube-diff-linux-amd64

