package rawdb

import (

	"github.com/pkg/errors"
)

// ReadBlockCommitSig retrieves the signature signed on a block.
func ReadBlockCommitSig(db DatabaseReader, blockNum uint64) ([]byte, error) {
	var data []byte
	data, err := db.Get(blockCommitSigKey(blockNum))
	if err != nil {
		// TODO: remove this extra seeking of sig after the mainnet is fully upgraded.
		//       this is only needed for the compatibility in the migration moment.
		data, err = db.Get(lastCommitsKey)
		if err != nil {
			return nil, errors.New("cannot read commit sig for block " + string(blockNum))
		}
	}
	return data, nil
}

// WriteBlockCommitSig ..
func WriteBlockCommitSig(db DatabaseWriter, blockNum uint64, sigAndBitmap []byte) error {
	return db.Put(blockCommitSigKey(blockNum), sigAndBitmap)
}