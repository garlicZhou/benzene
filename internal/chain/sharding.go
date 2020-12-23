package chain

import (
	"benzene/consensus/engine"
	"benzene/core/types"
)

type EngineImpl struct {

}

func New() *EngineImpl {
	return &EngineImpl{}
}

// Engine is an algorithm-agnostic consensus engine.
var Engine = &EngineImpl{}

// VerifyHeader checks whether a header conforms to the consensus rules of the bft engine.
// Note that each block header contains the bls signature of the parent block
func (e *EngineImpl) VerifyHeader(chain engine.ChainHeaderReader, header *types.Header, seal bool) error {
	return e.verifyHeader(chain, header, nil)
}

// VerifyHeaders is similar to VerifyHeader, but verifies a batch of headers. The
// method returns a quit channel to abort the operations and a results channel to
// retrieve the async verifications (the order is that of the input slice).
func (e *EngineImpl) VerifyHeaders(chain engine.ChainHeaderReader, headers []*types.Header, seals []bool) (chan<- struct{}, <-chan error) {
	abort := make(chan struct{})
	results := make(chan error, len(headers))

	go func() {
		for i, header := range headers {
			err := e.verifyHeader(chain, header, headers[:i])

			select {
			case <-abort:
				return
			case results <- err:
			}
		}
	}()
	return abort, results
}

// verifyHeader checks whether a header conforms to the consensus rules.The
// caller may optionally pass in a batch of parents (ascending order) to avoid
// looking those up from the database. This is useful for concurrently verifying
// a batch of new headers.
func (e *EngineImpl) verifyHeader(chain engine.ChainHeaderReader, header *types.Header, parents []*types.Header) error {
	return nil
}