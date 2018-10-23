package syncer

import (
	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/rlp"
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
		return s.handleChunk(&chmsg, p)
	})

	// consumes outgoing proof messages and sends them to the originator
	go func() {
		for r := range s.proofC {
			msg, err := rlp.EncodeToBytes(r.msg)
			if err != nil {
				continue
			}
			ps.Send(r.to[:], pssProofTopic, msg)
		}
	}()
	return s
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
			ps.Send(c.Addr[:], pssChunkTopic, msg)
		}
	}()

	return s
}
