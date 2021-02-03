GOPATH:=$(shell go env GOPATH)
export CGO_CFLAGS:="-I$(TOP)/bls/include -I$(TOP)/mcl/include -I/usr/local/opt/openssl/include"
export CGO_LDFLAGS:="-L$(TOP)/bls/lib -L/usr/local/opt/openssl/lib"
export LD_LIBRARY_PATH:="$(TOP)/bls/lib:$(TOP)/mcl/lib:/usr/local/opt/openssl/lib"
export LIBRARY_PATH:="$(LD_LIBRARY_PATH)"
export DYLD_FALLBACK_LIBRARY_PATH:="$(LD_LIBRARY_PATH)"
export GO111MODULE:=on
PKGNAME=harmony
VERSION?=$(shell git tag -l --sort=-v:refname | head -n 1 | tr -d v)
RELEASE?=$(shell git describe --long | cut -f2 -d-)
RPMBUILD=$(HOME)/rpmbuild
DEBBUILD=$(HOME)/debbuild
SHELL:=bash

.PHONY: all libs

all: libs
	bash ./scripts/go_executable_build.sh -S

libs:
	make -C $(GOPATH)/src/github.com/harmony-one/mcl -j8
	make -C $(GOPATH)/src/github.com/harmony-one/bls BLS_SWAP_G=1 -j8