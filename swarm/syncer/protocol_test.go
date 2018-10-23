// Copyright 2018 The go-ethereum Authors
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

package syncer

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/p2p"
	"github.com/ethereum/go-ethereum/p2p/enode"
	"github.com/ethereum/go-ethereum/swarm/network"
	"github.com/ethereum/go-ethereum/swarm/storage"
)

// TestDispatcherSendChunk tests if the dispatcher.sendChunk is called
// a chunk message appears on the chunkC channel
func TestDispatcherSendChunk(t *testing.T) {
	baseAddr := network.RandomAddr().OAddr
	s := newDispatcher(baseAddr, nil)
	defer s.close()
	chunk := storage.GenerateRandomChunk(100)
	addr := chunk.Address()
	chunkData := chunk.Data()
	go s.sendChunk(chunk)
	timeout := time.NewTimer(100 * time.Millisecond)
	var chmsg *chunkMsg
	select {
	case chmsg = <-s.chunkC:
	case <-timeout.C:
		t.Fatal("timeout waiting for chunk message on channel")
	}
	if !bytes.Equal(chmsg.Addr, addr) {
		t.Fatalf("expected chunk message address %v, got %v", addr, chmsg.Addr)
	}
	if !bytes.Equal(chmsg.Origin, baseAddr) {
		t.Fatalf("expected Origin address %v, got %v", baseAddr, chmsg.Origin)
	}
	if !bytes.Equal(chmsg.Data, chunkData) {
		t.Fatalf("expected chunk message data %v, got %v", chunkData, chmsg.Data)
	}
	if len(chmsg.Challenge) != 32 {
		t.Fatalf("expected challenge nonce to be 32 bytes long, got %v", len(chmsg.Challenge))
	}
}

type prover struct {
	err error
}

// verify checks the POC proof
func (p *prover) Verify(proof, addr []byte, peer enode.ID) error {
	return p.err
}

// getProof provides a proof for a challenge on the data
func (p *prover) GetProof(data, challenge []byte) []byte {
	max := len(data)
	if max > 32 {
		max = 32
	}
	return append(data[:max], challenge...)
}

// TestDispatcherHandleProof tests that if handleProof is called with a proof message
// the proof is verified and
// - if valid the address is pushed to the synced channel
// - if invalid the address is NOT pushed to the synced channel
func TestDispatcherHandleProof(t *testing.T) {
	baseAddr := network.RandomAddr().OAddr
	p := &prover{}
	s := newDispatcher(baseAddr, p)
	defer s.close()
	chunk := storage.GenerateRandomChunk(100)
	addr := chunk.Address()
	proof := p.GetProof(chunk.Data(), newChallenge())
	prmsg := &proofMsg{addr, proof}
	peer := p2p.NewPeer(enode.ID{}, "", nil)
	go s.handleProof(prmsg, peer)
	timeout := time.NewTimer(100 * time.Millisecond)
	var next []byte
	select {
	case next = <-s.receiptsC:
	case <-timeout.C:
		t.Fatal("timeout waiting for chunk message on channel")
	}
	if !bytes.Equal(next, addr) {
		t.Fatalf("expected chunk message address %v, got %v", addr, next)
	}

	p.err = errors.New("oops")
	go s.handleProof(prmsg, peer)
	timeout.Reset(100 * time.Millisecond)
	select {
	case <-s.receiptsC:
		t.Fatal("proof message on channel")
	case <-timeout.C:
	}
}

// TestStorerHandleChunk that if storer.handleChunk is called then the
// chunk gets stored and a proof is created and a proper proof response appears on the
// storer's proofC channel
func TestStorerHandleChunk(t *testing.T) {
	// set up storer
	origin := network.RandomAddr().OAddr
	p := &prover{}
	chunkStore := storage.NewMapChunkStore()
	s := newStorer(chunkStore, p)
	defer s.close()

	// create a chunk message and call handleChunk on it
	chunk := storage.GenerateRandomChunk(100)
	addr := chunk.Address()
	data := chunk.Data()
	peer := p2p.NewPeer(enode.ID{}, "", nil)
	challenge := newChallenge()
	chmsg := &chunkMsg{
		Origin:    origin,
		Addr:      addr,
		Data:      data,
		Challenge: challenge,
	}
	go s.handleChunk(chmsg, peer)
	timeout := time.NewTimer(100 * time.Millisecond)
	var r *proofResponse
	select {
	case r = <-s.proofC:
	case <-timeout.C:
		t.Fatal("timeout waiting for chunk message on channel")
	}
	if _, err := chunkStore.Get(context.TODO(), addr); err != nil {
		t.Fatalf("expected chunk with address %v to be stored in chunkStore", addr)
	}
	if !bytes.Equal(r.to, origin) {
		t.Fatalf("expected proof response indicate proof should be sent to origin %v, got %v", origin, r.to)
	}
	if !bytes.Equal(r.msg.Addr, addr) {
		t.Fatalf("expected proof msg address to be chunk address %v, got %v", addr, r.msg.Addr)
	}
	proof := append(data[:32], challenge...)
	if !bytes.Equal(r.msg.Proof, proof) {
		t.Fatalf("expected (fake) proof to be data[:32]+challenge \n%x, got \n%x", proof, r.msg.Proof)
	}
}
