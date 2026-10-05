package rng

import (
	"testing"
)

// Golden vectors frozen at implementation time. They exist to catch accidental
// changes to the algorithm or seeding — bump them deliberately if behaviour changes.
func TestSeedingGoldenVectors(t *testing.T) {
	t.Parallel()

	r := New(42)
	want := []uint64{
		0xd0764d4f4476689f,
		0x519e4174576f3791,
		0xfbe07cfb0c24ed8c,
		0xb37d9f600cd835b8,
		0xcb231c3874846a73,
	}
	for i, w := range want {
		if got := r.Uint64(); got != w {
			t.Fatalf("draw %d = %#016x, want %#016x", i, got, w)
		}
	}
}

func TestStateRoundTrip(t *testing.T) {
	t.Parallel()

	a := New(7)
	for i := 0; i < 10; i++ {
		a.Uint64()
	}
	restored, err := FromState(a.State())
	if err != nil {
		t.Fatalf("FromState: %v", err)
	}
	for i := 0; i < 25; i++ {
		if x, y := a.Uint64(), restored.Uint64(); x != y {
			t.Fatalf("draw %d after restore = %#016x, want %#016x", i, y, x)
		}
	}
}

func TestFromStateRejectsZeroState(t *testing.T) {
	t.Parallel()

	if _, err := FromState(State{}); err != ErrZeroState {
		t.Fatalf("FromState(zero) error = %v, want %v", err, ErrZeroState)
	}
}

func TestFloat64Range(t *testing.T) {
	t.Parallel()

	r := New(3)
	sum := 0.0
	const n = 10_000
	for i := 0; i < n; i++ {
		f := r.Float64()
		if f < 0 || f >= 1 {
			t.Fatalf("Float64 = %f out of [0,1)", f)
		}
		sum += f
	}
	mean := sum / n
	if mean < 0.47 || mean > 0.53 {
		t.Fatalf("Float64 mean = %f, want ~0.5", mean)
	}
}

func TestIntnBounds(t *testing.T) {
	t.Parallel()

	r := New(9)
	const bound = 7
	counts := make([]int, bound)
	for i := 0; i < 10_000; i++ {
		v := r.Intn(bound)
		if v < 0 || v >= bound {
			t.Fatalf("Intn(%d) = %d out of range", bound, v)
		}
		counts[v]++
	}
	for v, c := range counts {
		if c < 1000 {
			t.Fatalf("Intn(%d) bucket %d has only %d hits (suspiciously uniformity-broken)", bound, v, c)
		}
	}
	if got := r.Intn(0); got != 0 {
		t.Fatalf("Intn(0) = %d, want 0", got)
	}
}

func TestDistinctSeedsDiverge(t *testing.T) {
	t.Parallel()

	a, b := New(1), New(2)
	same := 0
	for i := 0; i < 100; i++ {
		if a.Uint64() == b.Uint64() {
			same++
		}
	}
	if same > 2 {
		t.Fatalf("different seeds collided %d/100 times", same)
	}
}
