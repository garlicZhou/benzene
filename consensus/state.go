package consensus

import (
	"benzene/internal/utils"
	"sync"
)

// MaxViewIDDiff limits the received view ID to only 249 further from the current view ID
const MaxViewIDDiff = 249

// State contains current mode and current viewID
type State struct {
	mode    Mode
	modeMux sync.RWMutex

	// current view id in normal mode
	// it changes per successful consensus
	blockViewID uint64
	cViewMux    sync.RWMutex

	// view changing id is used during view change mode
	// it is the next view id
	viewChangingID uint64

	viewMux sync.RWMutex
}

// Mode return the current node mode
func (pm *State) Mode() Mode {
	pm.modeMux.RLock()
	defer pm.modeMux.RUnlock()
	return pm.mode
}

// SetMode set the node mode as required
func (pm *State) SetMode(s Mode) {
	pm.modeMux.Lock()
	defer pm.modeMux.Unlock()
	pm.mode = s
}

// GetCurBlockViewID return the current view id
func (pm *State) GetCurBlockViewID() uint64 {
	pm.cViewMux.RLock()
	defer pm.cViewMux.RUnlock()
	return pm.blockViewID
}

// SetCurBlockViewID sets the current view id
func (pm *State) SetCurBlockViewID(viewID uint64) {
	pm.cViewMux.Lock()
	defer pm.cViewMux.Unlock()
	pm.blockViewID = viewID
}

func createTimeout() map[TimeoutType]*utils.Timeout {
	timeouts := make(map[TimeoutType]*utils.Timeout)
	timeouts[timeoutConsensus] = utils.NewTimeout(phaseDuration)
	timeouts[timeoutBootstrap] = utils.NewTimeout(bootstrapDuration)
	return timeouts
}
