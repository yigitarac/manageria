package liveplay

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"
	"testing"
)

// T-055 frozen disabled baseline (isolated prototype evidence).
//
// The digests below were captured from the PRISTINE tree (1365ecb) BEFORE any
// T-055 runtime edit; see /tmp/opencode/t055-baseline-capture.log. They pin the
// exact disabled JSON shape/state of every matrix case (initial/intermediate/
// final), so the opt-in shell cannot silently alter the legacy path. No expected
// value here was generated from the changed runtime.

// t055Digest is the SHA-256 of the canonical JSON of v.
func t055Digest(t *testing.T, v any) string {
	t.Helper()
	blob, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal digest: %v", err)
	}
	sum := sha256.Sum256(blob)
	return hex.EncodeToString(sum[:])
}

// t055NativeScene is the plain live-pass release->run scene (1 runner).
func t055NativeScene(t *testing.T, seed uint64) *Sim {
	t.Helper()
	sim := mustSim(t, LivePassConfig(), seed)
	sim.SetRunners([]Runner{InertialRunner(0.66, 0.50, 0.30, 0.50, DefaultMotionParams())})
	sim.ReleasePass(PassParams{StartX: 0.30, StartY: 0.50, TargetX: 0.60, TargetY: 0.50, SpeedMPS: 14.0, Passer: -1})
	return sim
}

// t055SampledScene is the sampled-local watcher scene (toucher + near/far local).
func t055SampledScene(t *testing.T, seed uint64) *Sim {
	t.Helper()
	sim := mustSim(t, ScheduledLivePassConfig(100), seed)
	toucher := InertialRunner(0.50, 0.50, 0.50, 0.50, DefaultMotionParams())
	nearX := 0.50 - 1.5/PitchLengthM
	near := t050Local(nearX, 0.50, nearX, 0.50, 3.0, 300, true)
	far := t050Local(0.10, 0.10, 0.10, 0.10, 0.5, 300, true)
	sim.SetRunners([]Runner{toucher, near, far})
	sim.ReleasePass(t052InteriorPass(2.0, 0.0))
	return sim
}

// t055StopOutScene is a single slow (stop) or fast (out) live ball.
func t055StopOutScene(t *testing.T, out bool) *Sim {
	t.Helper()
	sim := mustSim(t, LivePassConfig(), 55)
	sim.SetRunners([]Runner{t052Sensing(t, 0.95, 0.05, 0.01)})
	if out {
		sim.SetBall(Ball{Mode: BallRolling, X: 0.90, Y: 0.50, VXM: 30.0})
	} else {
		sim.SetBall(Ball{Mode: BallRolling, X: 0.50, Y: 0.50, VXM: 1.0})
	}
	return sim
}

// t055TieScene is the exact mirrored-chord slot tie (disabled: lowest slot wins).
func t055TieScene(t *testing.T) *Sim {
	t.Helper()
	sim := mustSim(t, LivePassConfig(), 3)
	off := 0.5 / PitchWidthM
	r0 := InertialRunner(0.50, 0.50-off, 0.50, 0.50-off, DefaultMotionParams())
	r1 := InertialRunner(0.50, 0.50+off, 0.50, 0.50+off, DefaultMotionParams())
	sim.SetRunners([]Runner{r0, r1})
	sim.SetBall(Ball{Mode: BallRolling, X: 0.50 - 0.9/PitchLengthM, Y: 0.50, VXM: 2.0})
	for len(t052Touches(sim)) == 0 && sim.ClockMS() < 4000 {
		sim.Step()
	}
	return sim
}

// t055NativeInitialJSON is the exact expected disabled Snapshot JSON at the
// native scene's clock 0, pinning that NO new zero key (integrated/playerId)
// appears on the disabled path.
const t055NativeInitialJSON = `{"cfg":{"substepMs":20,"decelMps2":0.6,"livePass":true},"clockMs":0,"ball":{"mode":1,"x":0.3,"y":0.5,"vxM":14,"vyM":0},"runners":[{"x":0.66,"y":0.5,"vxM":0,"vyM":0,"reachM":0,"motion":1,"intentX":0.3,"intentY":0.5,"topSpeedMps":8.5,"accelMps2":4,"decelMps2":4.5,"lateralAccelMps2":3}],"contactMs":-1,"contactRunner":-1,"rng":[4204209062293533097,10330944102538159132,3830118112478979541,8535791464090941865],"events":[{"seq":0,"clockMs":0,"kind":0,"runner":-1,"ballX":0.3,"ballY":0.5,"vxM":14}],"eventSeq":1,"immunityUntil":400,"releaseRunner":-1,"firstTouchMs":-1,"firstTouchRunner":-1}`

// TestT055FrozenDisabledNativeInitial pins the exact initial disabled JSON and
// confirms no new key leaked into the disabled shape.
func TestT055FrozenDisabledNativeInitial(t *testing.T) {
	sim := t055NativeScene(t, 7)
	blob, err := json.Marshal(sim.Snapshot())
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(blob) != t055NativeInitialJSON {
		t.Fatalf("native initial JSON changed:\n got %s\nwant %s", blob, t055NativeInitialJSON)
	}
	for _, key := range []string{`"integrated"`, `"playerId"`, `"journal"`, `"snapshotVersion"`} {
		if strings.Contains(string(blob), key) {
			t.Fatalf("disabled snapshot leaked new key %s: %s", key, blob)
		}
	}
	t.Logf("T-055 frozen native.initial exact JSON ok; no new key present")
}

// TestT055FrozenDisabledDigests pins the full JSON snapshot/events/samples/RNG
// digests of every matrix case at its initial/intermediate/final clocks.
func TestT055FrozenDisabledDigests(t *testing.T) {
	type cap struct {
		label               string
		snap, ev, samp, rng string
	}
	check := func(c cap, sim *Sim) {
		t.Helper()
		if got := t055Digest(t, sim.Snapshot()); got != c.snap {
			t.Fatalf("%s @%d snapshot digest %s != %s", c.label, sim.ClockMS(), got, c.snap)
		}
		if got := t055Digest(t, sim.Events()); got != c.ev {
			t.Fatalf("%s @%d events digest %s != %s", c.label, sim.ClockMS(), got, c.ev)
		}
		if got := t055Digest(t, sim.Samples()); got != c.samp {
			t.Fatalf("%s @%d samples digest %s != %s", c.label, sim.ClockMS(), got, c.samp)
		}
		if got := t055Digest(t, sim.RNGState()); got != c.rng {
			t.Fatalf("%s @%d rng digest %s != %s", c.label, sim.ClockMS(), got, c.rng)
		}
	}

	// native
	n := t055NativeScene(t, 7)
	check(cap{"native", "8a298dfb315a3c7780c96590e287bc0d2949fcf46aee12829467df49f06bd6cd", "84fdcaea5606fed3b968d6f5ab4b6dc406d348c352de0873632d76228b343d56", "74234e98afe7498fb5daf1f36ac2d78acc339464f950703b8c019892f982b90b", "ce8292b7c01f8972d9db734becc5cfef1afab78324873dc30f87e980474f85ee"}, n)
	for i := 0; i < 5; i++ {
		n.Step()
	}
	check(cap{"native", "0f22c6ca9c06fe4ed11c47f902a7607cd2e2286e853a661e5ab634204dab453d", "065a35dcf748a124d7a368b8bc72d455749caf33d4716d460bdb1c8a9b44a81f", "8c5893fd6084d51a6bd2b31d22b78560facc50c5be5aea8427285b7950ffeabf", "ce8292b7c01f8972d9db734becc5cfef1afab78324873dc30f87e980474f85ee"}, n)
	for n.ClockMS() < 2000 {
		n.Step()
	}
	check(cap{"native", "2d5028745ee5a0ab496895ec9814b5b2d05153a68508c2d7085ee7717d636ff2", "46b8dc29b67151f76275d20267b42fd012c183f8e2b916a3b861da4aec7fba91", "34e502bf81764375af37e76def9fbf49d9419cae5a52dc83f99cde6fa152b986", "ce8292b7c01f8972d9db734becc5cfef1afab78324873dc30f87e980474f85ee"}, n)

	// sampled local
	s := t055SampledScene(t, 9)
	check(cap{"sampled", "32b56d777d492bf9f83353861e5c7c5a71d5f6bc8a5ddc659e3e46cfd1410aee", "9d49b43fc1f3cecc4a3b0569b480802235acd6864059cfafa8429266def5bb67", "74234e98afe7498fb5daf1f36ac2d78acc339464f950703b8c019892f982b90b", "fb85c6927cd084f8e24ab10c1360ccfa98995ccaef9784a35bc7a15b685a1869"}, s)
	for s.ClockMS() < 20 {
		s.Step()
	}
	check(cap{"sampled", "bc118c6361ed1a51375f1b1131df66a5eff9069fc14123977d0669b1848b4edd", "979886cc595c67d39dfe41c25f11a124370c45e9d996a47dc343bc9f37040962", "d402ed070810246885351ccc1f037f4f87c240aef07ef567018febc8e41da3b9", "fb85c6927cd084f8e24ab10c1360ccfa98995ccaef9784a35bc7a15b685a1869"}, s)
	for s.ClockMS() < 60 {
		s.Step()
	}
	check(cap{"sampled", "769160a4cc466af86447b8f9e7c36c09e56b53ec49c0edb3ba5caaed47ee78c4", "979886cc595c67d39dfe41c25f11a124370c45e9d996a47dc343bc9f37040962", "b81a88fc936f37bcf801face03508e8e88a87ec432aeddc8391c989643cc49b7", "fb85c6927cd084f8e24ab10c1360ccfa98995ccaef9784a35bc7a15b685a1869"}, s)

	// control + held + second release
	c := t053Build(t, t053ControlSeed, true)
	check(cap{"control", "94ce00b52aa7404515e0637f929f8119affd2879ccfc93d19564eb759c1cd55e", "9d49b43fc1f3cecc4a3b0569b480802235acd6864059cfafa8429266def5bb67", "74234e98afe7498fb5daf1f36ac2d78acc339464f950703b8c019892f982b90b", "de4d0b6f0e3389b75e075fb0a8fcfedf408dd69c0031705664465f490e3ca37e"}, c)
	t053StepToWithMid(c, 20, t053MidSecondRelease)
	check(cap{"control.contact", "5dc32a8ab28e889ca01d636c25a4cdc2e84603f286db5921adba85aaab2c89e1", "30ef92bbd6880a8bbcbf95f20f783d0844b337b8f925ff5c5a1e9261d4fbef7f", "0b9b371eabebd4dded527b62be5bd7fc7ab2c31c670d17229c2d2b36676cf056", "de4d0b6f0e3389b75e075fb0a8fcfedf408dd69c0031705664465f490e3ca37e"}, c)
	t053StepToWithMid(c, 200, t053MidSecondRelease)
	check(cap{"control.held", "b481b1c74e84fe3e5c38fee6beb2b3a84fb5d11645034de2424aa77faf3f55df", "30ef92bbd6880a8bbcbf95f20f783d0844b337b8f925ff5c5a1e9261d4fbef7f", "2c776d15213add2638f8fb224c808ff5465c1915b20f4d41179c22d5f1b9f292", "de4d0b6f0e3389b75e075fb0a8fcfedf408dd69c0031705664465f490e3ca37e"}, c)
	t053StepToWithMid(c, 1020, t053MidSecondRelease)
	check(cap{"control.second", "b9eac0cec9915667fcdb54d503c964490a2fdb2d37560dbd080f75dda4f281fc", "1db22f96e71c2139d4819193b9bcadf309fc0536b998f3ed81a7a25e8c66e314", "9c227af27077205ab5eae5099142fd0ca581ebf0e91bedc92fae5ef397899e92", "faee44d1a9bce05f38fccd4cb49e43079f57ddaa1bd6777a5f941803e66aa3de"}, c)

	// deflect + cooldown
	d := t053Build(t, t053DeflectSeed, false)
	check(cap{"deflect", "16241c9f89fbac90e95c7d66a372c39cc92a40844e700555f216bdf957a04d44", "7b6678a16ed79d70de36a4f865b5aa5b8f8552a9c98ee57ceea8efb433c2e1ff", "74234e98afe7498fb5daf1f36ac2d78acc339464f950703b8c019892f982b90b", "9411085c1b0013460ebf588f0807236f779b9328e13dd42ec954a3a97e78c253"}, d)
	t053StepToWithMid(d, 20, nil)
	check(cap{"deflect.pre", "e3e91902ddda6e2fe46bbe30e71b37387d6bd6d6078d572b4f9fe01adb5dba12", "7b6678a16ed79d70de36a4f865b5aa5b8f8552a9c98ee57ceea8efb433c2e1ff", "afe0df613750f6459801b9761f59fba12f33e917ce15a8983396e371d7d4a12a", "9411085c1b0013460ebf588f0807236f779b9328e13dd42ec954a3a97e78c253"}, d)
	t053StepToWithMid(d, 1100, nil)
	check(cap{"deflect.cooldown", "0da2a20db5eb97564d6d20e08dc40502e4c14fd7de78269169787d4593f5ab4a", "4c8208806b7737d46b3bd660873a3389429190a9b7e46e6abe0de7e2fc62ae54", "cec9b5e53a90750c8a61a47308a741237c4a756cfafe048982209ff8fd3cfdde", "77cc5a9b804a52b57268f78d973231e3433587a69050c1a527ec1fa50813fbb4"}, d)

	// stop / out
	st := t055StopOutScene(t, false)
	for st.ClockMS() < 2000 {
		st.Step()
	}
	check(cap{"stop", "7d763791aedf47290fe7433dab5890b5b820c2b5779087ec5a88738acf00e75e", "56382aca8172573aca363a3ba859122973994ec856e2335f2274610f50919c66", "814bf7ac2284b21dfc0041bff38736320dfb7c271479e77d7ec55d545fe3e2a6", "ea257eb03e0e94aea6185b728597eb45dee34087ca480a00b6c34b399cfc3e17"}, st)
	ou := t055StopOutScene(t, true)
	for ou.ClockMS() < 400 {
		ou.Step()
	}
	check(cap{"out", "98b155bb3142df331cb6b052029f253e77da39b311dba423bb7dc63a42dcd110", "d4642a698408f488f1e6bc2adcb5004e95315d0dc1c16bcf9134323494925b25", "442ab2ed1d27183d83e6d1f30b3ff6223efbd3b93c1bb9910e162c6eaa51e4c5", "ea257eb03e0e94aea6185b728597eb45dee34087ca480a00b6c34b399cfc3e17"}, ou)

	// exact slot tie
	tie := t055TieScene(t)
	check(cap{"tie", "1e55c6c4b4f71771c1f5b3b8d73edf72ab2bd4e67d745b29e0808ac5e35ede77", "8c2882e558870dcd4fadf365e0224e557ca3c68d166aa6d49ee08e4a83d23c5b", "fc31be077342e47777121fcdce9adfa887808965d2b02c507edbd6f338259cc0", "b2b76b13e795df82b047058d92da3eb32ccf82154fc2425864e6bc7f874501c5"}, tie)
	if len(t052Touches(tie)) == 0 || t052Touches(tie)[0].Runner != 0 {
		t.Fatalf("disabled tie winner slot %+v, want 0", t052Touches(tie))
	}
	t.Logf("T-055 frozen disabled digests: native/sampled/control/deflect/stop/out/tie byte-identical to pristine1365ecb; disabled tie keeps slot 0")
}
