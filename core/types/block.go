package types

import (
	"benzene/block"
)

type Block struct {
	header              *block.Header
	transactions        Transactions
}