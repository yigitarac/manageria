package liveplay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

// T-056 frozen pre-T056 integrated baseline. The digests below were captured from
// the ACCEPTED T055 commit4bf45aa BEFORE any T-056 motion edit (see
// /tmp/opencode/t056-baseline-capture.log). They pin the non-oriented integrated
// full state/events/samples/RNG, so the oriented branch cannot silently alter the
// existing integrated path. No expected value here was generated from the T-056
// runtime.

// t056Digest is the SHA-256 of the canonical JSON of v (fails the test on error).
func t056Digest(t *testing.T, v any) string {
	t.Helper()
	blob, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal digest: %v", err)
	}
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:])
}

// t056NormSnapshot normalizes ONLY the declared integrated model version, never a
// physical field.
func t056NormSnapshot(s Snapshot) Snapshot {
	if s.Integrated != nil {
		s.Integrated.ModelVersion = 1
	}
	return s
}

func t056FrozenNative(t *testing.T, seed uint64) *Sim {
	t.Helper()
	sim := mustSim(t, LivePassConfig(), seed)
	sim.SetRunners(t055AssignIDs([]Runner{InertialRunner(0.50, 0.50, 0.50, 0.50, DefaultMotionParams())}))
	t055EnableOrFatal(t, sim)
	sim.ReleasePass(t052InteriorPass(2.0, 0.0))
	return sim
}

func t056FrozenTie(t *testing.T) *Sim {
	t.Helper()
	sim := mustSim(t, LivePassConfig(), 3)
	off := 0.5 / PitchWidthM
	r0 := InertialRunner(0.50, 0.50-off, 0.50, 0.50-off, DefaultMotionParams())
	r1 := InertialRunner(0.50, 0.50+off, 0.50, 0.50+off, DefaultMotionParams())
	r0.PlayerID, r1.PlayerID = 2, 1
	sim.SetRunners([]Runner{r0, r1})
	sim.SetBall(Ball{Mode: BallRolling, X: 0.50 - 0.9/PitchLengthM, Y: 0.50, VXM: 2.0})
	t055EnableOrFatal(t, sim)
	t055StepUntilTouch(sim, 4000)
	return sim
}

// TestT056FrozenIntegratedBaseline asserts the accepted T055 integrated digests
// are unchanged after the T-056 motion change.
func TestT056FrozenIntegratedBaseline(t *testing.T) {
	type cap struct{ label, norm, ev, samp, rng string }
	check := func(c cap, sim *Sim) {
		t.Helper()
		if got := t056Digest(t, t056NormSnapshot(sim.Snapshot())); got != c.norm {
			t.Fatalf("%s @%d normalized snapshot digest %s != %s", c.label, sim.ClockMS(), got, c.norm)
		}
		if got := t056Digest(t, sim.Events()); got != c.ev {
			t.Fatalf("%s @%d events digest %s != %s", c.label, sim.ClockMS(), got, c.ev)
		}
		if got := t056Digest(t, sim.Samples()); got != c.samp {
			t.Fatalf("%s @%d samples digest %s != %s", c.label, sim.ClockMS(), got, c.samp)
		}
		if got := t056Digest(t, sim.RNGState()); got != c.rng {
			t.Fatalf("%s @%d rng digest %s != %s", c.label, sim.ClockMS(), got, c.rng)
		}
	}

	n := t056FrozenNative(t, 1)
	check(cap{"native", "e714f1e596af9d7bb3c9faf3dd0e6cbcc4da379565ff2ace0c4eb08817fb83c7", "9d49b43fc1f3cecc4a3b0569b480802235acd6864059cfafa8429266def5bb67", "74234e98afe7498fb5daf1f36ac2d78acc339464f950703b8c019892f982b90b", "20f4eaf5b74db83390d5e49ee5e76853cc3a52a3ff63821034fb3c5a4758487c"}, n)
	n.Step()
	check(cap{"native", "21795ffe9dcac0665e763d745abf8584e2ffb9323587138ca894497ba39d9bb2", "979886cc595c67d39dfe41c25f11a124370c45e9d996a47dc343bc9f37040962", "d402ed070810246885351ccc1f037f4f87c240aef07ef567018febc8e41da3b9", "20f4eaf5b74db83390d5e49ee5e76853cc3a52a3ff63821034fb3c5a4758487c"}, n)

	c := t055Active22Build(t, t053ControlSeed, true, true)
	check(cap{"control", "faff5c7cd2e453d80b1ed6877a25d61b05e7bc937ade422070d3540faf811e7d", "9d49b43fc1f3cecc4a3b0569b480802235acd6864059cfafa8429266def5bb67", "74234e98afe7498fb5daf1f36ac2d78acc339464f950703b8c019892f982b90b", "de4d0b6f0e3389b75e075fb0a8fcfedf408dd69c0031705664465f490e3ca37e"}, c)
	t053StepToWithMid(c, 20, t053MidSecondRelease)
	check(cap{"control", "86b774ed075c6944a05791b3e4bac7e91c4f13b2edd83c2d71de0230660a89f5", "30ef92bbd6880a8bbcbf95f20f783d0844b337b8f925ff5c5a1e9261d4fbef7f", "0b9b371eabebd4dded527b62be5bd7fc7ab2c31c670d17229c2d2b36676cf056", "de4d0b6f0e3389b75e075fb0a8fcfedf408dd69c0031705664465f490e3ca37e"}, c)
	t053StepToWithMid(c, 200, t053MidSecondRelease)
	check(cap{"control", "939f37b52a9858cea34621a4cf43e7023540ff2fd2f0e93b79252cc2a26ba95a", "30ef92bbd6880a8bbcbf95f20f783d0844b337b8f925ff5c5a1e9261d4fbef7f", "2c776d15213add2638f8fb224c808ff5465c1915b20f4d41179c22d5f1b9f292", "de4d0b6f0e3389b75e075fb0a8fcfedf408dd69c0031705664465f490e3ca37e"}, c)
	t053StepToWithMid(c, 980, t053MidSecondRelease)
	check(cap{"control", "bc6b476d25addc953623a8b300e3a97502f62d8833366ff2b72a4bcc85b5865b", "30ef92bbd6880a8bbcbf95f20f783d0844b337b8f925ff5c5a1e9261d4fbef7f", "0cf5d4cb521234d5caff9e51403d76cb12d719ceadf6b24285ad601a7af09eda", "de4d0b6f0e3389b75e075fb0a8fcfedf408dd69c0031705664465f490e3ca37e"}, c)
	t053StepToWithMid(c, 1020, t053MidSecondRelease)
	check(cap{"control", "6a3276c2479bbb4463ee68c7d664e014bb3278675bf30ccaedec5013ebc5d955", "1db22f96e71c2139d4819193b9bcadf309fc0536b998f3ed81a7a25e8c66e314", "9c227af27077205ab5eae5099142fd0ca581ebf0e91bedc92fae5ef397899e92", "faee44d1a9bce05f38fccd4cb49e43079f57ddaa1bd6777a5f941803e66aa3de"}, c)

	d := t055Active22Build(t, t053DeflectSeed, false, true)
	check(cap{"deflect", "8654e78a1a6ab730941203381e3b11644a9947cbd0c85eb88cca7e52a0ef76b5", "7b6678a16ed79d70de36a4f865b5aa5b8f8552a9c98ee57ceea8efb433c2e1ff", "74234e98afe7498fb5daf1f36ac2d78acc339464f950703b8c019892f982b90b", "9411085c1b0013460ebf588f0807236f779b9328e13dd42ec954a3a97e78c253"}, d)
	t053StepToWithMid(d, 20, nil)
	check(cap{"deflect", "bcbe59507adec2fdd7b1037032664d111c532d3b7b399e3538ab68d1c91010dd", "7b6678a16ed79d70de36a4f865b5aa5b8f8552a9c98ee57ceea8efb433c2e1ff", "afe0df613750f6459801b9761f59fba12f33e917ce15a8983396e371d7d4a12a", "9411085c1b0013460ebf588f0807236f779b9328e13dd42ec954a3a97e78c253"}, d)
	t053StepToWithMid(d, 900, nil)
	check(cap{"deflect", "7efafc9ce48a86c70e3f0bc33d9ee9c33c6dc3571893c3e60206ca68b753ec58", "d0c6dcdb0761832083840b78e8826f3f1eceba5c5386d5ea3b12d9182e328fce", "acfe3f9c821df6c907888a06a7f12b261011a52d013bd6c1ebf3796940909ce6", "9411085c1b0013460ebf588f0807236f779b9328e13dd42ec954a3a97e78c253"}, d)
	t053StepToWithMid(d, 1100, nil)
	check(cap{"deflect", "b5b7ef6069612a249c3b4e6791956a2273bb4300997588a8ad702559a8004371", "4c8208806b7737d46b3bd660873a3389429190a9b7e46e6abe0de7e2fc62ae54", "cec9b5e53a90750c8a61a47308a741237c4a756cfafe048982209ff8fd3cfdde", "77cc5a9b804a52b57268f78d973231e3433587a69050c1a527ec1fa50813fbb4"}, d)

	check(cap{"tie", "ffad158c2d3ab5ee3878e4ba0bb8f9ac0a927832798f4edcef1a243ddc1defc6", "e3c737f6ba0d87d2e568933013581fc96f1a9942c6f601923c444b464a15dd46", "0f2175389449121cee0b2be2568e5536e464e9d6514cd52ea64731c4776bd4c3", "b2b76b13e795df82b047058d92da3eb32ccf82154fc2425864e6bc7f874501c5"}, t056FrozenTie(t))

	o := t055IntegratedOutScene(t, 6)
	for o.ClockMS() < 400 {
		o.Step()
	}
	check(cap{"out", "77c4420d4f7175cebd3bf8af34ac3f048f24f2bf798667351e0038dfe88a2668", "c92a202a3be42d8f7900668f8654c346cf6d5d06911130605bd8804c49c6db5b", "42a15357c1d786e04f711e6465e79815944ee21193c19b93996f84bf4c3e5a57", "a52f05e51a2c431a286efc1fbef46d2b03b9655d93d887723ef244ddf75e7b4b"}, o)

	st := t055IntegratedStopScene(t, 3)
	for st.ClockMS() < 2000 {
		st.Step()
	}
	check(cap{"stop", "ef6cddd6e45fb8b216780183bf3304d28cdcfd98348fafc5df78733505f775bf", "d1a9388961b29dd3a48218cc2d3c64148910f02699b6b456c0067c35513ff6e4", "609978172fc943a5e4281d0bef3d2f3d1ec217ad20c2ab5fc1826562d473d3a3", "497995136b1c533bd5175ae11ca4bff97b1393d09fe00b63ba473365c25b46c1"}, st)

	t.Logf("T-056 frozen pre-T056 integrated baseline: native/control/deflect/tie/out/stop full state+events+samples+RNG byte-identical to accepted4bf45aa after the oriented-motion change")
}

// TestT056OrientationAbsentFromIntegratedJSON asserts ONLY the JSON SHAPE of a
// non-oriented integrated snapshot (no `orientation` key leaked). It does not by
// itself prove nil-arithmetic equality; the frozen digests in
// TestT056FrozenIntegratedBaseline pin that the nil branch is byte-identical.
func TestT056OrientationAbsentFromIntegratedJSON(t *testing.T) {
	blob, err := json.Marshal(t055Active22Build(t, t053ControlSeed, true, true).Snapshot())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if s := string(blob); containsSub(s, `"orientation"`) {
		t.Fatalf("non-oriented integrated snapshot leaked an orientation key: %s", s)
	}
	t.Logf("T-056 JSON shape: no orientation key in the non-oriented integrated snapshot (nil arithmetic pinned by the frozen digests, not by this shape check)")
}

// containsSub is a tiny substring helper (avoids importing strings in one test).
func containsSub(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
