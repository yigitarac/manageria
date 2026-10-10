package engine

import "testing"

// DEF-3 (T-021): out of possession the holding block obeys a vertical compactness
// budget. Lines (same-depth neighbours) may ride at most compactGap beyond the
// line beneath them, measured forward from the last line of resistance — the
// pocket between the lines is where arriving support receives. Deep laggards are
// never lifted, and an in-possession block is never squeezed.
func TestDefensiveBlockCompactnessBudget(t *testing.T) {
	s := &simState{}
	s.tactics[0].Pressing = 0      // every outfielder holds (no chasers) for the fixture
	s.tactics[0].Mentality = 3     // keep blockShift near zero so targets track anchors
	s.tactics[0].DefensiveLine = 2 //
	s.ballX, s.ballY = 0.5, 0.5
	s.owner = 1 // the opponents have the ball — team 0 defends

	// Stretched block: one straggling recovery run at 0.10, back four clustered
	// at 0.20–0.23, midfield dangling at 0.50, front two camping at 0.80.
	s.players[1] = PlayerState{X: 0.10, Y: 0.5}
	for i := 2; i <= 5; i++ {
		s.players[i] = PlayerState{X: 0.20 + 0.01*float64(i-2), Y: 0.5}
	}
	for i := 6; i <= 8; i++ {
		s.players[i] = PlayerState{X: 0.50, Y: 0.5}
	}
	s.players[9] = PlayerState{X: 0.80, Y: 0.5}
	s.players[10] = PlayerState{X: 0.80, Y: 0.5}

	checks := []struct {
		name string
		idx  int
		tx   float64
		want float64
	}{
		{"back line top is the reference line (unmoved)", 2, 0.22, 0.22},
		{"midfield pulled to the first budget ceiling", 6, 0.50, 0.23 + compactGap},
		{"front line rides the second ceiling", 9, 0.80, 0.23 + 2*compactGap},
		{"deep laggard keeps his depth", 1, 0.12, 0.12},
	}
	for _, c := range checks {
		if got := s.compactDepth(0, c.idx, c.tx); absf(got-c.want) > 0.001 {
			t.Fatalf("%s: compactDepth(0, %d, %.2f) = %.3f, want %.3f", c.name, c.idx, c.tx, got, c.want)
		}
	}

	// Mirrored for team 1 (his attack runs toward x=0): same stretched shape,
	// mirrored coordinates; the mid slot contracts toward HIS goal (x grows).
	s.tactics[1].Pressing = 0
	s.players[12] = PlayerState{X: 0.90, Y: 0.5}
	for i := 2; i <= 5; i++ {
		s.players[11+i] = PlayerState{X: 0.80 - 0.01*float64(i-2), Y: 0.5}
	}
	for i := 6; i <= 8; i++ {
		s.players[11+i] = PlayerState{X: 0.50, Y: 0.5}
	}
	s.players[20] = PlayerState{X: 0.20, Y: 0.5}
	s.players[21] = PlayerState{X: 0.20, Y: 0.5}
	if got := s.compactDepth(1, 6, 0.50); absf(got-(0.77-compactGap)) > 0.001 {
		t.Fatalf("away midfield: compactDepth = %.3f, want %.3f", got, 0.77-compactGap)
	}

	// The target hook: squeezed while defending, untouched in possession.
	s.anchors[0][6] = [2]float64{0.50, 0.5}
	ps := PlayerState{X: 0.50, Y: 0.5}
	s.owner = 0
	tx, _ := s.target(0, 6, &ps)
	if tx < 0.40 {
		t.Fatalf("in-possession block squeezed to %.3f", tx)
	}
	s.owner = 1
	tx, _ = s.target(0, 6, &ps)
	if want := 0.23 + compactGap; tx > want+0.001 {
		t.Fatalf("defending block left dangling at %.3f (want <= %.2f)", tx, want)
	}
}
