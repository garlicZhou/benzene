package networkinfo

import (
	"benzene/internal/configs"
	"context"
	"fmt"
	"github.com/ethereum/go-ethereum/log"
	"net"
	"sync"
	"time"

	msg_pb "benzene/api/proto/message"
	"benzene/internal/utils"
	"benzene/p2p"
	"github.com/ethereum/go-ethereum/rpc"
	badger "github.com/ipfs/go-ds-badger"
	coredis "github.com/libp2p/go-libp2p-core/discovery"
	libp2p_peer "github.com/libp2p/go-libp2p-core/peer"
	libp2pdis "github.com/libp2p/go-libp2p-discovery"
	libp2pdht "github.com/libp2p/go-libp2p-kad-dht"
	libp2pdhtopts "github.com/libp2p/go-libp2p-kad-dht/opts"
	madns "github.com/multiformats/go-multiaddr-dns"
	manet "github.com/multiformats/go-multiaddr-net"
	"github.com/pkg/errors"
)

// Service is the network info service.
type Service struct {
	Host        p2p.Host
	Rendezvous  configs.GroupID
	bootnodes   p2p.AddrList
	dht         *libp2pdht.IpfsDHT
	cancel      context.CancelFunc
	stopChan    chan struct{}
	stoppedChan chan struct{}
	peerChan    chan p2p.Peer
	peerInfo    <-chan libp2p_peer.AddrInfo
	discovery   *libp2pdis.RoutingDiscovery
	messageChan chan *msg_pb.Message
	started     bool
}

// ConnectionRetry set the number of retry of connection to bootnode in case the initial connection is failed
var (
	// retry for 30s and give up then
	ConnectionRetry = 15
)

const (
	waitInRetry       = 5 * time.Second
	connectionTimeout = 3 * time.Minute

	minFindPeerInterval = 5    // initial find peer interval during bootstrap
	maxFindPeerInterval = 1800 // max find peer interval, every 30 minutes

	// register to bootnode every ticker
	dhtTicker = 6 * time.Hour

	discoveryLimit = 32
)

// New returns role conversion service.  If dataStorePath is not empty, it
// points to a persistent database directory to use.
func New(
	h p2p.Host, rendezvous configs.GroupID, peerChan chan p2p.Peer,
	bootnodes p2p.AddrList, dataStorePath string,
) (*Service, error) {
	ctx, cancel := context.WithCancel(context.Background())
	var dhtOpts []libp2pdhtopts.Option
	if dataStorePath != "" {
		dataStore, err := badger.NewDatastore(dataStorePath, nil)
		if err != nil {
			return nil, errors.Wrapf(err,
				"cannot open Badger datastore at %s", dataStorePath)
		}
		log.Info("backing DHT with Badger datastore", "dataStorePath", dataStorePath)
		dhtOpts = append(dhtOpts, libp2pdhtopts.Datastore(dataStore))
	}

	dht, err := libp2pdht.New(ctx, h.GetP2PHost(), dhtOpts...)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create DHT")
	}

	return &Service{
		Host:        h,
		dht:         dht,
		Rendezvous:  rendezvous,
		cancel:      cancel,
		stopChan:    make(chan struct{}),
		stoppedChan: make(chan struct{}),
		peerChan:    peerChan,
		bootnodes:   bootnodes,
		discovery:   nil,
		started:     false,
	}, nil
}

// MustNew is a panic-on-error version of New.
func MustNew(
	h p2p.Host, rendezvous configs.GroupID, peerChan chan p2p.Peer,
	bootnodes p2p.AddrList, dataStorePath string,
) *Service {
	service, err := New(h, rendezvous, peerChan, bootnodes, dataStorePath)
	if err != nil {
		panic(err)
	}
	return service
}

// StartService starts network info service.
func (s *Service) StartService() {
	err := s.Init()
	if err != nil {
		log.Error("Service Init Failed", "err", err)
		return
	}
	s.Run()
	s.started = true
}

// Init initializes role conversion service.
func (s *Service) Init() error {
	ctx, cancel := context.WithTimeout(context.Background(), connectionTimeout)
	defer cancel()
	log.Info("Init networkinfo service")

	// Bootstrap the DHT. In the default configuration, this spawns a Background
	// thread that will refresh the peer table every five minutes.
	log.Debug("Bootstrapping the DHT")
	if err := s.dht.Bootstrap(ctx); err != nil {
		return fmt.Errorf("error bootstrap dht: %s", err)
	}

	var wg sync.WaitGroup
	if s.bootnodes == nil {
		// TODO: should've passed in bootnodes through constructor.
		s.bootnodes = p2p.BootNodes
	}

	connected := false
	var bnList p2p.AddrList
	for _, maddr := range s.bootnodes {
		if madns.Matches(maddr) {
			mas, err := madns.Resolve(context.Background(), maddr)
			if err != nil {
				log.Error("Resolve bootnode", "err", err)
				continue
			}
			bnList = append(bnList, mas...)
		} else {
			bnList = append(bnList, maddr)
		}
	}

	for _, peerAddr := range bnList {
		peerinfo, _ := libp2p_peer.AddrInfoFromP2pAddr(peerAddr)
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < ConnectionRetry; i++ {
				if err := s.Host.GetP2PHost().Connect(ctx, *peerinfo); err != nil {
					log.Warn("can't connect to bootnode", "try", i, "err", err)
					time.Sleep(waitInRetry)
				} else {
					log.Info("connected to bootnode", "try", i, "node", *peerinfo)
					// it is okay if any bootnode is connected
					connected = true
					break
				}
			}
		}()
	}
	wg.Wait()

	if !connected {
		return fmt.Errorf("[FATAL] error connecting to bootnodes")
	}

	// We use a rendezvous point "shardID" to announce our location.
	log.Info("Announcing ourselves...", "Rendezvous", string(s.Rendezvous))
	s.discovery = libp2pdis.NewRoutingDiscovery(s.dht)
	libp2pdis.Advertise(ctx, s.discovery, string(s.Rendezvous))
	log.Info("Successfully announced!")

	return nil
}

// Run runs network info.
func (s *Service) Run() {
	defer close(s.stoppedChan)
	if s.discovery == nil {
		log.Error("discovery is not initialized")
		return
	}

	go s.DoService()
}

// DoService does network info.
func (s *Service) DoService() {
	tick := time.NewTicker(dhtTicker)
	defer tick.Stop()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	peerInterval := minFindPeerInterval
	intervalTick := time.NewTicker(time.Duration(peerInterval) * time.Second)
	defer intervalTick.Stop()
	for {
		select {
		case <-s.stopChan:
			return
		case <-tick.C:
			libp2pdis.Advertise(ctx, s.discovery, string(s.Rendezvous))
			log.Info("Successfully announced!", "Rendezvous", string(s.Rendezvous))
		case <-intervalTick.C:
			var err error
			s.peerInfo, err = s.discovery.FindPeers(
				ctx, string(s.Rendezvous), coredis.Limit(discoveryLimit),
			)
			if err != nil {
				log.Error("FindPeers", "err", err)
				return
			}
			if peerInterval < maxFindPeerInterval {
				peerInterval *= 2
				intervalTick.Stop()
				intervalTick = time.NewTicker(time.Duration(peerInterval) * time.Second)
			}

			go s.findPeers(ctx)
		}
	}
}

func (s *Service) findPeers(ctx context.Context) {
	_, cgnPrefix, err := net.ParseCIDR("100.64.0.0/10")
	if err != nil {
		log.Error("can't parse CIDR", "err", err)
		return
	}
	for peer := range s.peerInfo {
		if peer.ID != s.Host.GetP2PHost().ID() && len(peer.ID) > 0 {
			if err := s.Host.GetP2PHost().Connect(ctx, peer); err != nil {
				log.Warn("can't connect to peer node", "err", err, "peer", peer)
				// break if the node can't connect to peers, waiting for another peer
				break
			} else {
				log.Info("connected to peer node", "peer", peer)
			}
			// figure out the public ip/port
			var ip, port string

			for _, addr := range peer.Addrs {
				netaddr, err := manet.ToNetAddr(addr)
				if err != nil {
					continue
				}
				nip := netaddr.(*net.TCPAddr).IP
				if (nip.IsGlobalUnicast() && !utils.IsPrivateIP(nip)) || cgnPrefix.Contains(nip) {
					ip = nip.String()
					port = fmt.Sprintf("%d", netaddr.(*net.TCPAddr).Port)
					break
				}
			}
			p := p2p.Peer{IP: ip, Port: port, PeerID: peer.ID, Addrs: peer.Addrs}
			log.Info("Notify peerChan", "peer", p)
			if s.peerChan != nil {
				s.peerChan <- p
			}
		}
	}

	log.Info("PeerInfo Channel Closed")
}

// StopService stops network info service.
func (s *Service) StopService() {
	log.Info("Stopping network info service")
	defer s.cancel()

	if !s.started {
		log.Info("Service didn't started. Exit")
		return
	}

	s.stopChan <- struct{}{}
	<-s.stoppedChan
	log.Info("Network info service stopped")
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
