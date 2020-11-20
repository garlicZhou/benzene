package p2p

import (
	"context"
	"encoding/binary"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/libp2p/go-libp2p"
	libp2p_crypto "github.com/libp2p/go-libp2p-core/crypto"
	libp2p_host "github.com/libp2p/go-libp2p-core/host"
	libp2p_peer "github.com/libp2p/go-libp2p-core/peer"
	libp2p_peerstore "github.com/libp2p/go-libp2p-core/peerstore"
	libp2p_pubsub "github.com/libp2p/go-libp2p-pubsub"
	ma "github.com/multiformats/go-multiaddr"
	"github.com/pkg/errors"
	"github.com/rs/zerolog"
)

/* 注：
 * 1. 此处的topic变量在本项目中应该表现为分片ID，Harmony不知道是不是
 * 2. host.go里面只有发送方法没有接收方法，接收是在node的main函数中，找到Next方法就有了
 */

type Host interface {
	GetSelfPeer() Peer
	AddPeer(*Peer) error
	GetID() libp2p_peer.ID
	GetP2PHost() libp2p_host.Host
	
	ConnectHostPeer(Peer) error
	PubSub() *libp2p_pubsub.PubSub
	JoinShard(topic string) (*libp2p_pubsub.Topic, error)
	GetShard(topic string) *libp2p_pubsub.Topic
	SendMessageToGroups(groups []string, msg []byte) error
	ListTopic() []string
	ListPeer(topic string) []libp2p_peer.ID
}

/*
 * bls这个库似乎有点问题，测试了一天都没搞好，它跟mcl库又有关联
 * 实在不行就换一个
 */
type Peer struct {
	IP				string
	Port			string
	Addrs			[]ma.Multiaddr
	PeerID			libp2p_peer.ID
}

func NewHost(self *Peer, key libp2p_crypto.PrivKey) (Host, error) {
	listenAddr, err := ma.NewMultiaddr(fmt.Sprintf("/ip4/%s/tcp/%s", self.IP, self.Port))
	if err != nil {
		return nil, errors.Wrapf(err, "cannot create listen multiaddr from port %#v", self.Port)
	}

	ctx := context.Background()
	host, err := libp2p.New(ctx,
		libp2p.ListenAddrs(listenAddr))
	if err != nil {
		return nil, errors.Wrapf(err, "cannot initialize libp2p host")
	}

	pubsub, err := libp2p_pubsub.NewGossipSub(ctx, host)
	if err != nil {
		return nil, errors.Wrapf(err, "cannot initialize libp2p pubsub")
	}

	self.PeerID = host.ID()
	subLogger := zerolog.New(os.Stdout).With().Str("hostID", host.ID().Pretty()).Logger()

	h := &HostV2{
		h:		host,
		pubsub:	pubsub,
		joined:	map[string]*libp2p_pubsub.Topic{},
		self:	*self,
		priKey:	key,
		logger: &subLogger,
	}

	if err != nil {
		return nil, err
	}

	return h, nil
}

/*
 * 主机定义
 */
type HostV2 struct {
	h		libp2p_host.Host
	pubsub	*libp2p_pubsub.PubSub
	joined	map[string]*libp2p_pubsub.Topic
	self	Peer
	priKey	libp2p_crypto.PrivKey
	lock	sync.Mutex
	logger	*zerolog.Logger
}

func (host *HostV2) PubSub() *libp2p_pubsub.PubSub {
	return host.pubsub
}

// 加入某一个分片
// (在node.go中必须加入所有分片，否则无法向该分片收发数据。虽然没啥影响，但就是很不爽)
func (host *HostV2) JoinShard(topic string) (*libp2p_pubsub.Topic, error) {
	host.lock.Lock()
	defer host.lock.Unlock()
	if t, err := host.pubsub.Join(topic); err != nil {
		return nil, errors.Wrapf(err, "cannot join pubsub topic %x", topic)
	} else {
		host.joined[topic] = t
		return t, nil
	}
}

// 取本分片(将用于接收数据)
func (host *HostV2) GetShard(topic string) *libp2p_pubsub.Topic {
	host.lock.Lock()
	defer host.lock.Unlock()
	if t, ok := host.joined[topic]; ok {
		return t
	} else {
		return nil
	}
}

// groups是分片的数组
// 明明有个发送数据，然而接收数据不写成方法，而是在node.go中
// 然而想了想使用libp2p确实应该写在node.go中，但我觉得很不爽
func (host *HostV2) SendMessageToGroups(groups []string, msg []byte) (err error) {
	if len(msg) == 0 {
		return errors.New("cannot send out empty message")
	}

	for _, group := range groups {
		t := host.GetShard(group)
		if t != nil {
			continue
		}

		e := t.Publish(context.Background(), msg)
		if e != nil {
			err = e
			continue
		}
	}

	return err
}

// AddPeer add p2p.Peer into Peerstore
func (host *HostV2) AddPeer(p *Peer) error {
	if p.PeerID != "" && len(p.Addrs) != 0 {
		host.Peerstore().AddAddrs(p.PeerID, p.Addrs, libp2p_peerstore.PermanentAddrTTL)
		return nil
	}

	if p.PeerID == "" {
		host.logger.Error().Msg("AddPeer PeerID is EMPTY")
		return fmt.Errorf("AddPeer error: peerID is empty")
	}

	// reconstruct the multiaddress based on ip/port
	// PeerID has to be known for the ip/port
	addr := fmt.Sprintf("/ip4/%s/tcp/%s", p.IP, p.Port)
	targetAddr, err := ma.NewMultiaddr(addr)
	if err != nil {
		host.logger.Error().Err(err).Msg("AddPeer NewMultiaddr error")
		return err
	}

	p.Addrs = append(p.Addrs, targetAddr)
	host.Peerstore().AddAddrs(p.PeerID, p.Addrs, libp2p_peerstore.PermanentAddrTTL)
	host.logger.Info().Interface("peer", *p).Msg("AddPeer add to libp2p_peerstore")
	return nil
}

// Peerstore returns the peer store
func (host *HostV2) Peerstore() libp2p_peerstore.Peerstore {
	return host.h.Peerstore()
}

// GetID returns ID.Pretty
func (host *HostV2) GetID() libp2p_peer.ID {
	return host.h.ID()
}

// GetSelfPeer gets self peer
func (host *HostV2) GetSelfPeer() Peer {
	return host.self
}

// GetP2PHost returns the p2p.Host
func (host *HostV2) GetP2PHost() libp2p_host.Host {
	return host.h
}

// ListTopic returns the list of topic the node subscribed
// 列举自己可以收到哪些分片的数据
func (host *HostV2) ListTopic() []string {
	host.lock.Lock()
	defer host.lock.Unlock()
	topics := make([]string, 0)
	for t := range host.joined {
		topics = append(topics, t)
	}
	return topics
}

// ListPeer returns list of peers in a topic
func (host *HostV2) ListPeer(topic string) []libp2p_peer.ID {
	host.lock.Lock()
	defer host.lock.Unlock()
	return host.joined[topic].ListPeers()
}

// ConnectHostPeer connects to peer host
func (host *HostV2) ConnectHostPeer(peer Peer) error {
	ctx := context.Background()
	addr := fmt.Sprintf("/ip4/%s/tcp/%s/ipfs/%s", peer.IP, peer.Port, peer.PeerID.Pretty())
	peerAddr, err := ma.NewMultiaddr(addr)
	if err != nil {
		host.logger.Error().Err(err).Interface("peer", peer).Msg("ConnectHostPeer")
		return err
	}

	// AddrInfoFromP2pAddr将Multiaddr变成AddrInfo.
	peerInfo, err := libp2p_peer.AddrInfoFromP2pAddr(peerAddr)
	if err != nil {
		host.logger.Error().Err(err).Interface("peer", peer).Msg("ConnectHostPeer")
		return err
	}
	if err := host.h.Connect(ctx, *peerInfo); err != nil {
		host.logger.Warn().Err(err).Interface("peer", peer).Msg("can't connect to peer")
		return err
	}
	host.logger.Info().Interface("node", *peerInfo).Msg("connected to peer host")
	return nil
}


// ConstructMessage constructs the p2p message as [messageType, contentSize, content]
// 这个代码可能需要改一下，它的messageType好像固定了
func ConstructMessage(content []byte) []byte {
	message := make([]byte, 5+len(content))
	message[0] = 17 // messageType 0x11
	binary.BigEndian.PutUint32(message[1:5], uint32(len(content)))
	copy(message[5:], content)
	return message
}

// AddrList is a list of multiaddress
type AddrList []ma.Multiaddr

// String is a function to print a string representation of the AddrList
func (al *AddrList) String() string {
	strs := make([]string, len(*al))
	for i, addr := range *al {
		strs[i] = addr.String()
	}
	return strings.Join(strs, ",")
}

// StringsToAddrs convert a list of strings to a list of multiaddresses
func StringsToAddrs(addrStrings []string) (maddrs []ma.Multiaddr, err error) {
	for _, addrString := range addrStrings {
		addr, err := ma.NewMultiaddr(addrString)
		if err != nil {
			return maddrs, err
		}
		maddrs = append(maddrs, addr)
	}
	return
}