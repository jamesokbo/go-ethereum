package syncer

import (
	"context"

	"github.com/ethereum/go-ethereum/log"
	"github.com/ethereum/go-ethereum/swarm/storage"
)

// Syncer binds together
// - syncdb
// - protocol (dispatcher)
// - pubsub transport layer
type Syncer struct {
	db      *DB    // sync db
	dpClose func() // dispatcher close function
}

// New constructs a Syncer
func New(dbpath string, baseAddr storage.Address, store storage.ChunkStore, prover Prover, ps PubSub) (*Syncer, error) {
	d := newDispatcher(baseAddr, prover).withPubSub(ps)
	db, err := NewDB(dbpath, store, d.sendChunk, d.receiptsC)
	if err != nil {
		return nil, err
	}
	return &Syncer{db: db, dpClose: d.close}, nil
}

// Close closes the syncer
func (s *Syncer) Close() {
	s.db.Close()
	s.dpClose()
}

// Put puts the chunk to storage and inserts into sync index
// currently chunkstore call is syncronous so it needs to
// wrap dbstore
func (s *Syncer) Put(tagname string, chunk storage.Chunk) {
	it := &item{
		Addr:  chunk.Address(),
		Tag:   tagname,
		chunk: chunk,
		state: SPLIT, // can be left explicit
	}
	s.db.tags.Inc(tagname, SPLIT)
	// this put returns with error if this is a duplicate
	err := s.db.chunkStore.Put(context.TODO(), chunk)
	if err == errExists {
		return
	}
	if err != nil {
		log.Error("syncer: error storing chunk: %v", err)
		return
	}
	s.db.Put(it)
}

// NewTag creates a new info object for a file/collection of chunks
func (s *Syncer) NewTag(name string, total int) (*Tag, error) {
	return s.db.tags.New(name, total)
}
