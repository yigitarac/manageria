package liveplay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"
)

// T-057 frozen body-disabled reference. The digests below were captured from the
// ACCEPTED T056 commit 777eff4 BEFORE any T-057 runtime edit (see
// /tmp/opencode/t057-baseline-capture.log; worker archive
// /home/yigit/Documents/Projects/Manageria, planner immutable archive
// /tmp/opencode/t057-reference-review). They pin the never-body state/events/
// samples/RNG incl. real oriented/profile2 control, deflect and arrival scenes.
// Do not regenerate from the implementation under test.

func t057Digest(v any) string {
	blob, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:])
}

func TestT057FrozenDisabledReferences(t *testing.T) {
	type cap struct{ label, snap, ev, samp, rng string }
	check := func(c cap, sim *Sim) {
		t.Helper()
		if got := t057Digest(sim.Snapshot()); got != c.snap {
			t.Fatalf("%s @%d snapshot %s != %s", c.label, sim.ClockMS(), got, c.snap)
		}
		if got := t057Digest(sim.Events()); got != c.ev {
			t.Fatalf("%s @%d events %s != %s", c.label, sim.ClockMS(), got, c.ev)
		}
		if got := t057Digest(sim.Samples()); got != c.samp {
			t.Fatalf("%s @%d samples %s != %s", c.label, sim.ClockMS(), got, c.samp)
		}
		if got := t057Digest(sim.RNGState()); got != c.rng {
			t.Fatalf("%s @%d rng %s != %s", c.label, sim.ClockMS(), got, c.rng)
		}
	}

	// disabled native
	n := mustSim(t, LivePassConfig(), 1)
	n.SetRunners([]Runner{InertialRunner(0.50, 0.50, 0.50, 0.50, DefaultMotionParams())})
	n.ReleasePass(t052InteriorPass(2.0, 0.0))
	check(cap{"disabled-native", "5341d2bbae425a01f136f73688b561c7d4480040c63f122660dccd32e64669e6", "9d49b43fc1f3cecc4a3b0569b480802235acd6864059cfafa8429266def5bb67", "74234e98afe7498fb5daf1f36ac2d78acc339464f950703b8c019892f982b90b", "20f4eaf5b74db83390d5e49ee5e76853cc3a52a3ff63821034fb3c5a4758487c"}, n)
	n.Step()
	check(cap{"disabled-native", "76553f133ed8d4228a44b92399fbee36f881143ae121312b6285a048c44e6e52", "979886cc595c67d39dfe41c25f11a124370c45e9d996a47dc343bc9f37040962", "d402ed070810246885351ccc1f037f4f87c240aef07ef567018febc8e41da3b9", "20f4eaf5b74db83390d5e49ee5e76853cc3a52a3ff63821034fb3c5a4758487c"}, n)

	// integrated model1
	m1 := t055Active22Build(t, t053ControlSeed, true, true)
	check(cap{"integrated-m1", "faff5c7cd2e453d80b1ed6877a25d61b05e7bc937ade422070d3540faf811e7d", "9d49b43fc1f3cecc4a3b0569b480802235acd6864059cfafa8429266def5bb67", "74234e98afe7498fb5daf1f36ac2d78acc339464f950703b8c019892f982b90b", "de4d0b6f0e3389b75e075fb0a8fcfedf408dd69c0031705664465f490e3ca37e"}, m1)
	t053StepToWithMid(m1, 200, t053MidSecondRelease)
	check(cap{"integrated-m1", "939f37b52a9858cea34621a4cf43e7023540ff2fd2f0e93b79252cc2a26ba95a", "30ef92bbd6880a8bbcbf95f20f783d0844b337b8f925ff5c5a1e9261d4fbef7f", "2c776d15213add2638f8fb224c808ff5465c1915b20f4d41179c22d5f1b9f292", "de4d0b6f0e3389b75e075fb0a8fcfedf408dd69c0031705664465f490e3ca37e"}, m1)

	// oriented model2 control + deflect
	oc := t056OrientedActive22(t, t053ControlSeed, true)
	check(cap{"oriented-m2-control", "0a9cc054889af4981e3a35af389adb7bf70f106035a24cd99a92f9cd584f0a7a", "9d49b43fc1f3cecc4a3b0569b480802235acd6864059cfafa8429266def5bb67", "74234e98afe7498fb5daf1f36ac2d78acc339464f950703b8c019892f982b90b", "de4d0b6f0e3389b75e075fb0a8fcfedf408dd69c0031705664465f490e3ca37e"}, oc)
	for oc.ClockMS() < 200 {
		oc.Step()
	}
	check(cap{"oriented-m2-control", "162badcdec2edc46f1334c985a64a0354532f5b392d0506a5537e3bf9c6ed4f9", "30ef92bbd6880a8bbcbf95f20f783d0844b337b8f925ff5c5a1e9261d4fbef7f", "2c776d15213add2638f8fb224c808ff5465c1915b20f4d41179c22d5f1b9f292", "de4d0b6f0e3389b75e075fb0a8fcfedf408dd69c0031705664465f490e3ca37e"}, oc)
	od := t056OrientedActive22(t, t053DeflectSeed, false)
	check(cap{"oriented-m2-deflect", "c1c8dc2c4887af7d19d47e31b28ab49e9752983b332287f39e1dcc507cac848a", "7b6678a16ed79d70de36a4f865b5aa5b8f8552a9c98ee57ceea8efb433c2e1ff", "74234e98afe7498fb5daf1f36ac2d78acc339464f950703b8c019892f982b90b", "9411085c1b0013460ebf588f0807236f779b9328e13dd42ec954a3a97e78c253"}, od)
	for od.ClockMS() < 200 {
		od.Step()
	}
	check(cap{"oriented-m2-deflect", "ab8f46d0ba129e22fff6f7cb860bc2927d832e2c42936b6f277b61f4142e3066", "faf36652077e43b1113d1f4ffeb0c62b660c5d0882b2ce3f574f092e651b6720", "828d9ec26579ff775a4c96be7e15c5d2f57e20a4572f31ee123ab5d92955fed7", "9411085c1b0013460ebf588f0807236f779b9328e13dd42ec954a3a97e78c253"}, od)

	// oriented arrival scene
	ar := t056ArrivalSidecarScene(t, 11)
	check(cap{"oriented-arrival", "1dbdfddd3a8b981d6a1c847f9aa39d2cc457644183ccd83c48c9c8b73ee8c91d", "0dee7c83c6a45543d3c3f1f852fd2e0ef363a8ee5e082fdfd1b67f4f9bf7ca6a", "74234e98afe7498fb5daf1f36ac2d78acc339464f950703b8c019892f982b90b", "84e66eb66108c4117ce5a40f6cacfad8a81d20bac3b5f8e2d7ead90dea28a9b5"}, ar)
	for ar.ClockMS() < 300 {
		ar.Step()
	}
	check(cap{"oriented-arrival", "330321a7cef1b1efc1bd3e2302df382eb9697c622fa89176debc2504111a59a9", "90474f0d98e769070317bb43cf1b62e0200247378f5babee790cb5f69ae0f3d5", "15a00b2e59572b78b49009d198bda92349e69cdc8269c1afb59e519e95db986a", "84e66eb66108c4117ce5a40f6cacfad8a81d20bac3b5f8e2d7ead90dea28a9b5"}, ar)

	t.Logf("T-057 frozen: disabled/integrated-m1/oriented-m2 control+deflect+arrival byte-identical to accepted777eff4")
}
