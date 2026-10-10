package engine

import "testing"

// DEF-6: a distant block can restrict space without creating an immediate
// challenge. The old long-tailed value exceeded the emergency threshold here.
func TestChallengePressureExcludesDistantDefenders(t *testing.T) {
	s := &simState{}
	s.players[0] = PlayerState{X: 0.5, Y: 0.5}
	s.ballX, s.ballY = 0.5, 0.5
	s.tactics[0].Mentality = 3
	s.tactics[1].Pressing = 2
	s.onPitch[0].Attr.Composure = 10
	for i := 11; i < 22; i++ {
		s.players[i] = PlayerState{X: 0.9, Y: 0.6} // 0.5 Manhattan distance
		s.onPitch[i].Attr.WorkRate = 10
	}
	if got := s.pressure(0, 0); got <= 2.2 {
		t.Fatalf("broader block pressure %.2f, want >2.2 for the regression fixture", got)
	}
	if got := s.challengePressure(0, 0); got != 0 {
		t.Fatalf("distant defenders create immediate challenge %.2f", got)
	}
	if got := s.situation(0, 0).challenge; got != 0 {
		t.Fatalf("carrier situation inherits distant challenge %.2f", got)
	}
	hasHold := func() bool {
		plays := s.buildPlays(0, 0, s.situation(0, 0), []passOption{{to: 1, dist: 0.1, gain: -0.1}})
		for _, p := range plays {
			if p.kind == playHold {
				return true
			}
		}
		return false
	}
	if hasHold() {
		t.Fatal("distant block creates a shielding option")
	}

	s.players[11] = PlayerState{X: 0.53, Y: 0.5}
	if got := s.challengePressure(0, 0); got <= 2.2 {
		t.Fatalf("near defender challenge %.2f, want >2.2", got)
	}
	if !hasHold() {
		t.Fatal("near defender does not create a shielding option")
	}
	s.cards[11].Off = true
	if got := s.challengePressure(0, 0); got != 0 {
		t.Fatalf("sent-off defender creates challenge %.2f", got)
	}
}
