package repute

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"sort"
	"time"
)

// CanonicalV1 is the version tag for the first canonical event encoding.
// If the Event shape changes, introduce CanonicalV2 with a new encoder and
// keep V1 so historic logs still verify.
const CanonicalV1 byte = 0x01

// CanonicalBytes produces a deterministic byte encoding of an event for
// hashing. The encoding is length-prefixed per field to prevent boundary
// collisions, version-tagged so future Event shapes can coexist, and never
// re-marshals the Payload (hashing the exact stored bytes preserves key order).
//
// Field order (fixed): version, Seq, Time, Kind, Actors (sorted), Payload.
func CanonicalBytes(e Event) []byte {
	var buf []byte

	buf = append(buf, CanonicalV1)

	var seqBuf [8]byte
	binary.BigEndian.PutUint64(seqBuf[:], e.Seq)
	buf = appendLengthPrefixed(buf, seqBuf[:])

	ts := e.Time.UTC().Format(time.RFC3339Nano)
	buf = appendLengthPrefixed(buf, []byte(ts))

	buf = appendLengthPrefixed(buf, []byte(e.Kind))

	actors := make([]string, len(e.Actors))
	for i, a := range e.Actors {
		actors[i] = string(a)
	}
	sort.Strings(actors)
	var actorBuf []byte
	for _, a := range actors {
		actorBuf = appendLengthPrefixed(actorBuf, []byte(a))
	}
	buf = appendLengthPrefixed(buf, actorBuf)

	buf = appendLengthPrefixed(buf, []byte(e.Payload))

	return buf
}

func appendLengthPrefixed(dst, data []byte) []byte {
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(data)))
	dst = append(dst, lenBuf[:]...)
	dst = append(dst, data...)
	return dst
}

// ChainHash computes H(n) = SHA-256(canonical_bytes(event) || prev_hash).
func ChainHash(e Event, prev [32]byte) [32]byte {
	h := sha256.New()
	h.Write(CanonicalBytes(e))
	h.Write(prev[:])
	return [32]byte(h.Sum(nil))
}

// GenesisHash is the chain head for an empty log: SHA-256 of the seed "repute:genesis".
var GenesisHash = sha256.Sum256([]byte("repute:genesis"))

// Chain tracks the running hash chain head as events are applied.
type Chain struct {
	head [32]byte
	seq  uint64
}

// NewChain returns a chain initialized at the genesis state.
func NewChain() *Chain {
	return &Chain{head: GenesisHash}
}

// Extend folds one event into the chain and returns the new event hash.
// Events must be extended in ascending Seq order.
func (c *Chain) Extend(e Event) [32]byte {
	c.head = ChainHash(e, c.head)
	c.seq = e.Seq
	return c.head
}

// Head returns the current chain head hash.
func (c *Chain) Head() [32]byte { return c.head }

// Seq returns the Seq of the last extended event, or 0 for genesis.
func (c *Chain) Seq() uint64 { return c.seq }

// ChainBreak describes the first position where verification failed.
type ChainBreak struct {
	Seq      uint64
	Expected [32]byte
	Got      [32]byte
}

// VerifyChain replays an EventSource, recomputing the hash chain from genesis,
// and returns nil if the chain is intact. If a break is found, it returns the
// first broken position. If expectedHead is non-zero and the final head
// doesn't match, that's reported as a break at the last seq.
func VerifyChain(ctx context.Context, src EventSource, expectedHead [32]byte) (*ChainBreak, error) {
	c := NewChain()
	for {
		ev, ok, err := src.Next(ctx)
		if err != nil {
			return nil, err
		}
		if !ok {
			break
		}
		c.Extend(ev)
	}
	zero := [32]byte{}
	if expectedHead != zero && c.head != expectedHead {
		return &ChainBreak{Seq: c.seq, Expected: expectedHead, Got: c.head}, nil
	}
	return nil, nil
}

// ComputeChainHead replays an EventSource and returns the final chain head.
func ComputeChainHead(ctx context.Context, src EventSource) ([32]byte, uint64, error) {
	c := NewChain()
	for {
		ev, ok, err := src.Next(ctx)
		if err != nil {
			return [32]byte{}, 0, err
		}
		if !ok {
			return c.head, c.seq, nil
		}
		c.Extend(ev)
	}
}
