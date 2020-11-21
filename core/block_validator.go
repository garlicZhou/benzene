// Copyright 2015 The go-ethereum Authors
// This file is part of the go-ethereum library.
//
// The go-ethereum library is free software: you can redistribute it and/or modify
// it under the terms of the GNU Lesser General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// The go-ethereum library is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
// GNU Lesser General Public License for more details.
//
// You should have received a copy of the GNU Lesser General Public License
// along with the go-ethereum library. If not, see <http://www.gnu.org/licenses/>.

package core

import (
	consensus_engine "benzene/consensus/engine"
	"benzene/core/state"
	"benzene/core/types"
	"benzene/params"
	"github.com/pkg/errors"
)

// BlockValidator is responsible for validating block headers, uncles and
// processed state.
//
// BlockValidator implements Validator.
type BlockValidator struct {
	config *params.ChainConfig // Chain configuration options
	bc     *BlockChain         // Canonical block chain
	engine consensus_engine.Engine    // Consensus engine used for validating
}

// NewBlockValidator returns a new block validator which is safe for re-use
func NewBlockValidator(config *params.ChainConfig, blockchain *BlockChain, engine consensus_engine.Engine) *BlockValidator {
	validator := &BlockValidator{
		config: config,
		engine: engine,
		bc:     blockchain,
	}
	return validator
}

// TODO: how to validate blocks (hongzicong)
// ValidateBody validates the given block's uncles and verifies the block
// header's transaction and uncle roots. The headers are assumed to be already
// validated at this point.
func (v *BlockValidator) ValidateBody(block *types.Block) error {
	return nil
}

// ValidateState validates the various changes that happen after a state
// transition, such as amount of used gas, the receipt roots and the state root
// itself. ValidateState returns a database batch if the validation was a success
// otherwise nil and an error is returned.
func (v *BlockValidator) ValidateState(block *types.Block, statedb *state.DB) error {
	return nil
}

// ValidateHeader checks whether a header conforms to the consensus rules of a
// given engine. Verifying the seal may be done optionally here, or explicitly
// via the VerifySeal method.
func (v *BlockValidator) ValidateHeader(block *types.Block, seal bool) error {
	if block == nil {
		return errors.New("block is nil")
	}
	if h := block.Header(); h != nil {
		return v.engine.VerifyHeader(v.bc, h, true)
	}
	return errors.New("header field was nil")
}

// ValidateHeaders verifies a batch of blocks' headers concurrently. The method returns a quit channel
// to abort the operations and a results channel to retrieve the async verifications
func (v *BlockValidator) ValidateHeaders(chain []*types.Block) (chan<- struct{}, <-chan error) {
	// Start the parallel header verifier
	headers := make([]*types.Header, len(chain))
	seals := make([]bool, len(chain))

	for i, block := range chain {
		headers[i] = block.Header()
		seals[i] = true
	}
	return v.engine.VerifyHeaders(v.bc, headers, seals)
}