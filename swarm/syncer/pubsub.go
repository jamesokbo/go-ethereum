	package syncer

import (
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/ethereum/go-ethereum/swarm/pss"
)

//
var (
	pssChunkTopic = "SYNC" // pss topic for chunks
	pssProofTopic = "POC"  // pss topic for proofs
)

// PubSub is a Postal Service interface needed to send/receive chunks, send/receive proofs
type PubSub interface {
	Register(topic string, handler func(msg []byte, p *p2p.Peer) error)
	Send(to []byte, topic string, msg []byte) error
}

type Pss struct {
	pss  *pss.Pss
	prox bool
}

func NewPss(p *pss.Pss, prox bool) *Pss {
	return &Pss{
		pss:  p,
		prox: prox,
	}
}

func (p *Pss) Register(topic string, handler func(msg []byte, p *p2p.Peer) error) {
	f := func(msg []byte, peer *p2p.Peer, _ bool, _ string) error {
		return handler(msg, peer)
	}
	h := pss.NewHandler(f).WithRaw()
	if p.prox {
		h = h.WithProxBin()
	}
	pt := pss.BytesToTopic([]byte(topic))
	p.pss.Register(&pt, h)
}

func (p *Pss) Send(to []byte, topic string, msg []byte) error {
	pt := pss.BytesToTopic([]byte(topic))
	log.Warn("Send", "topic", topic, "to", label(to))
	return p.pss.SendRaw(pss.PssAddress(to), pt, msg)
}

// withPubSub plugs in PubSub to the storer to receive chunks and sending proofs
func (s *storer) withPubSub(ps PubSub) *storer {

	// Registers handler on pssChunkTopic that deserialises chunkMsg and calls
	// syncer's handleChunk function
	ps.Register(pssChunkTopic, func(msg []byte, p *p2p.Peer) error {
		var chmsg chunkMsg
		err := rlp.DecodeBytes(msg, &chmsg)
		if err != nil {
			return err
		}
		log.Error("Handler", "chunk", label(chmsg.Addr), "origin", label(chmsg.Origin))
		return s.handleChunk(&chmsg, p)
	})

	// consumes outgoing proof messages and sends them to the originator
	go func() {
		for r := range s.proofC {
			msg, err := rlp.EncodeToBytes(r.msg)
			if err != nil {
				continue
			}
			log.Error("send proof", "addr", label(r.msg.Addr), "to", label(r.to))
			err = ps.Send(r.to[:], pssProofTopic, msg)
			if err != nil {
				log.Warn("unable to send", "error", err)
			}
		}
	}()
	return s
}

func label(b []byte) string {
	return hexutil.Encode(b[:2])
}

func (s *dispatcher) withPubSub(ps PubSub) *dispatcher {

	// Registers handler on pssProofTopic that deserialises proofMsg and calls
	// syncer's handleProof function
	ps.Register(pssProofTopic, func(msg []byte, p *p2p.Peer) error {
		var prmsg proofMsg
		err := rlp.DecodeBytes(msg, &prmsg)
		if err != nil {
			return err
		}
		log.Error("Handler", "proof", label(prmsg.Addr), "self", label(s.baseAddr))
		return s.handleProof(&prmsg, p)
	})

	// consumes outgoing chunk messages and sends them to their destination
	// using neighbourhood addressing
	go func() {
		for c := range s.chunkC {
			msg, err := rlp.EncodeToBytes(c)
			if err != nil {
				continue
			}
			log.Error("send chunk", "addr", label(c.Addr), "self", label(s.baseAddr))
			err = ps.Send(c.Addr[:], pssChunkTopic, msg)
			if err != nil {
				log.Warn("unable to send", "error", err)
			}
		}
	}()

	return s
}
