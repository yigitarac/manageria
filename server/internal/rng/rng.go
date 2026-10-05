// Package rng provides the seeded deterministic PRNG used by the match engine.
//
// Algorithm: xoshiro256++, seeded through SplitMix64. State is serializable so that
// re-simulation from a snapshot is bit-identical to a straight run (see the determinism
// rules in the Match Engine design note).
package rng

import (
	"errors"
	"math/bits"
)

// State is the serializable generator state (4 × uint64).
type State [4]uint64

// ErrZeroState is returned by FromState for the degenerate all-zero state.
var ErrZeroState = errors.New("rng: state must not be all zeros")

// Rand is a xoshiro256++ generator. Construct with New or FromState.
type Rand struct {
	state State
}

// New returns a generator seeded via SplitMix64.
func New(seed uint64) *Rand {
	sm := splitmix64{state: seed}
	st := State{sm.next(), sm.next(), sm.next(), sm.next()}
	r, err := FromState(st)
	if err != nil {
		// Practically unreachable: SplitMix64 output is uniformly distributed.
		panic("rng: degenerate state from seed " + err.Error())
	}
	return r
}

// FromState restores a generator from a serialized state.
func FromState(st State) (*Rand, error) {
	if st == (State{}) {
		return nil, ErrZeroState
	}
	return &Rand{state: st}, nil
}

// State returns the current serializable state.
func (r *Rand) State() State {
	return r.state
}

// Uint64 returns the next pseudo-random uint64.
func (r *Rand) Uint64() uint64 {
	s := &r.state
	result := rotl(s[0]+s[3], 23) + s[0]
	t := s[1] << 17
	s[2] ^= s[0]
	s[3] ^= s[1]
	s[1] ^= s[2]
	s[0] ^= s[3]
	s[2] ^= t
	s[3] = rotl(s[3], 45)
	return result
}

// Float64 returns a float64 in [0, 1) using the top 53 bits (IEEE-exact everywhere).
func (r *Rand) Float64() float64 {
	return float64(r.Uint64()>>11) * (1.0 / (1 << 53))
}

// Intn returns an int in [0, n). Returns 0 for n <= 0 (invalid input never panics).
func (r *Rand) Intn(n int) int {
	if n <= 0 {
		return 0
	}
	return int(r.Uint64n(uint64(n)))
}

// Uint64n returns a uint64 in [0, bound) using Lemire's unbiased multiply-shift.
// Returns 0 for bound == 0.
func (r *Rand) Uint64n(bound uint64) uint64 {
	if bound == 0 {
		return 0
	}
	u := r.Uint64()
	hi, lo := bits.Mul64(u, bound)
	if lo < bound {
		lowBound := -bound % bound
		for lo < lowBound {
			u = r.Uint64()
			hi, lo = bits.Mul64(u, bound)
		}
	}
	return hi
}

func rotl(x uint64, k uint) uint64 {
	return (x << k) | (x >> (64 - k))
}

// splitmix64 seeds the main generator (public-domain reference algorithm).
type splitmix64 struct {
	state uint64
}

func (s *splitmix64) next() uint64 {
	s.state += 0x9E3779B97F4A7C15
	z := s.state
	z = (z ^ (z >> 30)) * 0xBF58476D1CE4E5B9
	z = (z ^ (z >> 27)) * 0x94D049BB133111EB
	return z ^ (z >> 31)
}
