package quorum

import (
	"encoding/json"

	bls_core "github.com/harmony-one/bls/ffi/go/bls"
	"github.com/harmony-one/harmony/crypto/bls"

	"benzene/consensus/votepower"
	"github.com/ethereum/go-ethereum/common"

	"benzene/internal/utils"
	"github.com/ethereum/go-ethereum/log"
	bls_cosi "github.com/harmony-one/harmony/crypto/bls"
	"github.com/harmony-one/harmony/numeric"
)

type uniformVoteWeight struct {
	DependencyInjectionWriter
	DependencyInjectionReader
	SignatureReader
}

// Policy ..
func (v *uniformVoteWeight) Policy() Policy {
	return SuperMajorityVote
}

// AddNewVote ..
func (v *uniformVoteWeight) AddNewVote(
	p Phase, pubKeys []*bls_cosi.PublicKeyWrapper,
	sig *bls_core.Sign, headerHash common.Hash,
	height, viewID uint64) (*votepower.Ballot, error) {
	pubKeysBytes := make([]bls.SerializedPublicKey, len(pubKeys))
	for i, pubKey := range pubKeys {
		pubKeysBytes[i] = pubKey.Bytes
	}
	return v.submitVote(p, pubKeysBytes, sig, headerHash, height, viewID)
}

// IsQuorumAchieved ..
func (v *uniformVoteWeight) IsQuorumAchieved(p Phase) bool {
	r := v.SignersCount(p) >= v.TwoThirdsSignersCount()
	log.Info("Quorum details", "phase", p.String(),"signers-count", v.SignersCount(p), "threshold", v.TwoThirdsSignersCount(),
		"participants", v.ParticipantsCount())
	return r
}

// IsQuorumAchivedByMask ..
func (v *uniformVoteWeight) IsQuorumAchievedByMask(mask *bls_cosi.Mask) bool {
	if mask == nil {
		return false
	}
	threshold := v.TwoThirdsSignersCount()
	currentTotalPower := utils.CountOneBits(mask.Bitmap)
	if currentTotalPower < threshold {
		const msg = "[IsQuorumAchievedByMask] Not enough voting power: need %+v, have %+v"
		log.Warn(msg, threshold, currentTotalPower)
		return false
	}
	const msg = "[IsQuorumAchievedByMask] have enough voting power: need %+v, have %+v"
	log.Debug(msg, threshold, currentTotalPower)
	return true
}

// QuorumThreshold ..
func (v *uniformVoteWeight) QuorumThreshold() numeric.Dec {
	return numeric.NewDec(v.TwoThirdsSignersCount())
}

// IsAllSigsCollected ..
func (v *uniformVoteWeight) IsAllSigsCollected() bool {
	return v.SignersCount(Commit) == v.ParticipantsCount()
}

/*func (v *uniformVoteWeight) SetVoters(
	subCommittee *shard.Committee, epoch *big.Int,
) (*TallyResult, error) {
	// NO-OP do not add anything here
	return nil, nil
}*/

func (v *uniformVoteWeight) String() string {
	s, _ := json.Marshal(v)
	return string(s)
}

func (v *uniformVoteWeight) MarshalJSON() ([]byte, error) {
	type t struct {
		Policy       string   `json:"policy"`
		Count        int      `json:"count"`
		Participants []string `json:"committee-members"`
	}
	keysDump := v.Participants()
	keys := make([]string, len(keysDump))
	for i := range keysDump {
		keys[i] = keysDump[i].Bytes.Hex()
	}

	return json.Marshal(t{v.Policy().String(), len(keys), keys})
}

func (v *uniformVoteWeight) AmIMemberOfCommitee() bool {
	pubKeyFunc := v.MyPublicKey()
	if pubKeyFunc == nil {
		return false
	}
	identity, _ := pubKeyFunc()
	everyone := v.Participants()
	for _, key := range identity {
		for i := range everyone {
			if key.Object.IsEqual(everyone[i].Object) {
				return true
			}
		}
	}
	return false
}

func (v *uniformVoteWeight) ResetPrepareAndCommitVotes() {
	v.reset([]Phase{Prepare, Commit})
}

func (v *uniformVoteWeight) ResetViewChangeVotes() {
	v.reset([]Phase{ViewChange})
}