package engine

import "testing"

// T-021 instrumentation: the touch ledger records the local close-down read at
// each touch so pressured retention and unpressed loops become measurable
// outside the engine (simcli -retention). Telemetry only — no action utility
// consumes Touch.Press.
func TestTouchLedgerRecordsCloseDownRead(t *testing.T) {
	s := &simState{}
	s.ballX, s.ballY = 0.5, 0.5
	s.players[0] = PlayerState{X: 0.5, Y: 0.5}
	s.tactics[1].Pressing = 2
	for i := 11; i < 22; i++ {
		s.players[i] = PlayerState{X: 0.9, Y: 0.6} // distant block, no close-down
		s.onPitch[i].Attr.WorkRate = 10
	}
	s.openChain(0, "buildUp")
	s.recordTouch(TouchPass, 0, 1, true)
	if len(s.touches) != 1 {
		t.Fatalf("recorded %d touches, want 1", len(s.touches))
	}
	if got := s.touches[0].Press; got != 0 {
		t.Fatalf("distant block logged a close-down read %.2f, want 0", got)
	}
	s.players[11] = PlayerState{X: 0.53, Y: 0.5}
	s.recordTouch(TouchPass, 0, 1, true)
	if got := s.touches[1].Press; got <= 2.2 {
		t.Fatalf("near marker logged close-down read %.2f, want >2.2", got)
	}
}
