package core

import (
	consensus_engine "benzene/consensus/engine"
	"benzene/core/state"
	"benzene/core/types"
	"benzene/params"
)

// StateProcessor is a basic Processor, which takes care of transitioning
// state from one point to another.
//
// StateProcessor implements Processor.
type StateProcessor struct {
	config *params.ChainConfig     // Chain configuration options
	bc     *BlockChain             // Canonical block chain
	engine consensus_engine.Engine // Consensus engine used for block rewards
}

// NewStateProcessor initialises a new StateProcessor.
func NewStateProcessor(
	config *params.ChainConfig, bc *BlockChain, engine consensus_engine.Engine,
) *StateProcessor {
	return &StateProcessor{
		config: config,
		bc:     bc,
		engine: engine,
	}
}

// TODO: how to process blocks (hongzicong)
// Process processes the state changes according to the Ethereum rules by running
// the transaction messages using the statedb and applying any rewards to both
// the processor (coinbase) and any included uncles.
//
// Process returns the receipts and logs accumulated during the process and
// returns the amount of gas that was used in the process. If any of the
// transactions failed to execute due to insufficient gas it will return an error.
func (p *StateProcessor) Process(
	block *types.Block, statedb *state.DB,
) (
	[]*types.Log, error,
) {
	var (
		allLogs  []*types.Log
	)

	return allLogs, nil
}