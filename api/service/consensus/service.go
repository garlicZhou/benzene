package consensus

import (
	msg_pb "benzene/api/proto/message"
	"benzene/consensus"
	"benzene/core/types"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/rpc"
)

// Service is the consensus service.
type Service struct {
	blockChannel chan *types.Block // The channel to receive new blocks from Node
	consensus    *consensus.Consensus
	stopChan     chan struct{}
	stoppedChan  chan struct{}
	startChan    chan struct{}
	messageChan  chan *msg_pb.Message
}

// New returns consensus service.
func New(blockChannel chan *types.Block, consensus *consensus.Consensus, startChan chan struct{}) *Service {
	return &Service{blockChannel: blockChannel, consensus: consensus, startChan: startChan}
}

// StartService starts consensus service.
func (s *Service) StartService() {
	log.Info("[consensus/service] Starting consensus service.")
	s.stopChan = make(chan struct{})
	s.stoppedChan = make(chan struct{})
	s.consensus.Start(s.blockChannel, s.stopChan, s.stoppedChan, s.startChan)
}

// StopService stops consensus service.
func (s *Service) StopService() {
	log.Info("Stopping consensus service.")
	s.stopChan <- struct{}{}
	<-s.stoppedChan
	log.Info("Consensus service stopped.")
}

// NotifyService notify service
func (s *Service) NotifyService(params map[string]interface{}) {}

// SetMessageChan sets up message channel to service.
func (s *Service) SetMessageChan(messageChan chan *msg_pb.Message) {
	s.messageChan = messageChan
}

// APIs for the services.
func (s *Service) APIs() []rpc.API {
	return nil
}
