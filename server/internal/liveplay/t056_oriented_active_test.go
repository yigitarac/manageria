package liveplay

import (
	"reflect"
	"testing"
)

// T-056 genuine active22 oriented transition windows: fresh seeded baseline
// outside the timer, all22 moving every step, an oriented turning participant and
// a real control/deflect + second release; three fresh windows identical.
func TestT056OrientedActive22Windows(t *testing.T) {
	for _, control := range []bool{true, false} {
		label, seed := "deflect", uint64(t053DeflectSeed)
		if control {
			label, seed = "control", t053ControlSeed
		}
		var inits, finals []Snapshot
		var journals [][]FactEvent
		for w := 0; w < 3; w++ {
			sim := t056OrientedActive22(t, seed, control)
			if st, _ := sim.IntegratedState(); st.ModelVersion != IntegratedModelVersionOriented {
				t.Fatalf("%s: model %d, want oriented", label, st.ModelVersion)
			}
			inits = append(inits, sim.Snapshot())
			faceBefore := t056Face(t, sim, 0)
			prev := sim.Runners()
			held := 0
			for k := 0; k < t053WindowSteps; k++ {
				t053StepWithMid(sim, control, k)
				cur := sim.Runners()
				t053AssertAllMoved(t, label, prev, cur, k)
				prev = cur
				if sim.Ball().Mode == BallHeld {
					held++
				}
			}
			finals = append(finals, sim.Snapshot())
			journals = append(journals, t055AssertJournalReconciles(t, sim).Journal)
			touches := t052Touches(sim)
			if len(touches) == 0 {
				t.Fatalf("%s: no transition", label)
			}
			if !(touches[0].Frac > 0 && touches[0].Frac < 1) {
				t.Fatalf("%s: transition frac %v not interior", label, touches[0].Frac)
			}
			if control {
				if touches[0].Kind != PassEventControl {
					t.Fatalf("control first touch %s", touches[0].Kind)
				}
				if held == 0 {
					t.Fatal("control: no held substep")
				}
				rel := t051ReleaseClocks(sim.Events())
				if len(rel) != 2 || rel[0] != 0 || rel[1] != t053SecondRelMS {
					t.Fatalf("control releases %v, want [0 %d]", rel, t053SecondRelMS)
				}
			} else if touches[0].Kind != PassEventDeflect {
				t.Fatalf("deflect first touch %s", touches[0].Kind)
			}
			if faceAfter := t056Face(t, sim, 0); faceAfter.FX == faceBefore.FX && faceAfter.FY == faceBefore.FY {
				t.Fatalf("%s: oriented participant never turned", label)
			}
		}
		for w := 1; w < 3; w++ {
			if !reflect.DeepEqual(inits[0], inits[w]) || !reflect.DeepEqual(finals[0], finals[w]) || !reflect.DeepEqual(journals[0], journals[w]) {
				t.Fatalf("%s window %d differs from window 0", label, w)
			}
		}
		t.Logf("T-056 active22 %s: 3 fresh windows identical (init/final + %d journal facts); oriented participant turned; 22/22 moved every step; interior transition", label, len(journals[0]))
	}
}
