package engine

import "testing"

func TestStrikerDefensiveRecoveryTarget(t *testing.T) {
	for _, tc := range []struct {
		name         string
		team         int
		x, y         float64
		wantX, wantY float64
	}{
		{name: "home", team: 0, x: 0.12, y: 0.08, wantX: 0.30, wantY: 0.20},
		{name: "away", team: 1, x: 0.88, y: 0.92, wantX: 0.70, wantY: 0.80},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &simState{owner: int8(1 - tc.team)}
			s.onPitch[tc.team*11+9].Pos = PosST
			x, y := s.defensiveRoleTarget(tc.team, 9, tc.x, tc.y)
			if x != tc.wantX || y != tc.wantY {
				t.Fatalf("target = (%.2f, %.2f), want (%.2f, %.2f)", x, y, tc.wantX, tc.wantY)
			}
			centralX := 0.18
			if tc.team == 1 {
				centralX = 0.82
			}
			x, y = s.defensiveRoleTarget(tc.team, 9, centralX, 0.5)
			if x != centralX || y != 0.5 {
				t.Fatalf("central defensive outlet = (%.2f, %.2f), want tactical freedom", x, y)
			}
			s.restart.Kind = RestartCorner
			x, y = s.defensiveRoleTarget(tc.team, 9, tc.x, tc.y)
			if x != tc.x || y != tc.y {
				t.Fatalf("corner target = (%.2f, %.2f), want freedom to defend the set piece", x, y)
			}
		})
	}
}
