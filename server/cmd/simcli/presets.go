package main

// Tactic preset matrix shared by the balance audit (`-audit`) and the
// pressured-retention lens (`-retention`): one source of truth so every lens
// probes the exact same manager instructions.

import "github.com/yigitarac/manageria/server/internal/engine"

type preset struct {
	name string
	mut  func(*engine.MatchInput)
}

// set mutates the home tactics; only non-nil knobs move.
func set(ment, line, passing *int8, press, tempo, width *int8, mark, tackle *int8, counter *bool) func(*engine.MatchInput) {
	return func(in *engine.MatchInput) {
		t := &in.Home.Tactics
		if ment != nil {
			t.Mentality = *ment
		}
		if line != nil {
			t.DefensiveLine = *line
		}
		if passing != nil {
			t.PassingStyle = *passing
		}
		if press != nil {
			t.Pressing = *press
		}
		if tempo != nil {
			t.Tempo = *tempo
		}
		if width != nil {
			t.Width = *width
		}
		if mark != nil {
			t.Marking = *mark
		}
		if tackle != nil {
			t.Tackling = *tackle
		}
		if counter != nil {
			t.CounterAttack = *counter
		}
	}
}

// setAway mirrors `set` for the visiting side (counter probes arm both benches).
func setAway(ment, line, passing *int8, press, tempo, width *int8, mark, tackle *int8, counter *bool) func(*engine.MatchInput) {
	return func(in *engine.MatchInput) {
		t := &in.Away.Tactics
		if ment != nil {
			t.Mentality = *ment
		}
		if line != nil {
			t.DefensiveLine = *line
		}
		if passing != nil {
			t.PassingStyle = *passing
		}
		if press != nil {
			t.Pressing = *press
		}
		if tempo != nil {
			t.Tempo = *tempo
		}
		if width != nil {
			t.Width = *width
		}
		if mark != nil {
			t.Marking = *mark
		}
		if tackle != nil {
			t.Tackling = *tackle
		}
		if counter != nil {
			t.CounterAttack = *counter
		}
	}
}

func i8(v int8) *int8 { return &v }
func b(v bool) *bool  { return &v }

// presetMatrix is the audit's home-tactic sweep (each row vs a default opponent)
// plus the counter-matchup probes (names carry "_vs_").
func presetMatrix() []preset {
	return []preset{
		{"default", func(*engine.MatchInput) {}},
		{"defensive", set(i8(1), i8(1), i8(1), nil, nil, nil, nil, nil, nil)},
		{"attacking", set(i8(5), i8(3), i8(3), nil, nil, nil, nil, nil, nil)},
		{"high_press", set(nil, nil, nil, i8(3), nil, nil, nil, nil, nil)},
		{"low_block_counter", set(i8(1), i8(1), i8(3), i8(1), nil, nil, nil, nil, b(true))},
		{"gegenpress", set(i8(4), nil, nil, i8(3), i8(4), nil, nil, nil, b(true))},
		{"tiki_taka", set(i8(3), nil, i8(1), i8(2), i8(2), i8(2), nil, nil, nil)},
		{"wing_play", set(i8(4), nil, i8(2), nil, nil, i8(3), nil, nil, nil)},
		{"direct_long", set(i8(4), nil, i8(3), nil, i8(4), nil, nil, nil, nil)},
		{"deep_park", set(i8(1), i8(1), i8(1), i8(1), i8(1), nil, nil, i8(1), nil)},
		{"ultra_attack", set(i8(5), i8(3), i8(3), i8(3), i8(5), nil, nil, nil, nil)},
		{"hard_press_mark", set(nil, nil, nil, i8(3), nil, nil, i8(2), i8(2), nil)},
		{"narrow_dense", set(i8(3), nil, nil, nil, nil, i8(1), nil, nil, nil)},
		{"fast_breaks", set(i8(4), nil, i8(3), nil, i8(5), nil, nil, nil, b(true))},
		{"tall_target", set(i8(4), nil, i8(3), nil, nil, i8(3), nil, nil, nil)},
		{"high_wire", set(i8(5), i8(3), nil, nil, nil, i8(3), nil, nil, nil)},
		// Counter-matchup probes (names carry "_vs_"): rock-paper-scissors must exist —
		// the counter side plays a prepared counter-punch style and must blunt the aggressor.
		{"ultra_vs_lbc", func(in *engine.MatchInput) {
			set(i8(5), i8(3), i8(3), i8(3), i8(5), nil, nil, nil, nil)(in)
			setAway(i8(1), i8(1), i8(3), i8(1), nil, nil, nil, nil, b(true))(in)
		}},
		{"fast_vs_lbc", func(in *engine.MatchInput) {
			set(i8(4), nil, i8(3), nil, i8(5), nil, nil, nil, b(true))(in)
			setAway(i8(1), i8(1), i8(3), i8(1), nil, nil, nil, nil, b(true))(in)
		}},
		{"tall_vs_hpc", func(in *engine.MatchInput) {
			set(i8(4), nil, i8(3), nil, nil, i8(3), nil, nil, nil)(in)
			setAway(nil, nil, i8(3), i8(3), nil, nil, i8(2), i8(2), b(true))(in)
		}},
	}
}
