// Copyright 2014 The go-ethereum Authors
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

package types

import (
	"bytes"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/trie"
)

type DerivableBase interface {
	Len() int
	GetRlp(i int) []byte
}

// DeriveSha calculates the hash of the trie generated by DerivableList.
func DeriveSha(list ...DerivableBase) common.Hash {
	keybuf := new(bytes.Buffer)
	trie := new(trie.Trie)
	var num uint

	for j := range list {
		for i := 0; i < list[j].Len(); i++ {
			keybuf.Reset()
			rlp.Encode(keybuf, num)
			trie.Update(keybuf.Bytes(), list[j].GetRlp(i))
			num++
		}
	}
	return trie.Hash()
}
