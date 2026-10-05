// Command simcli simulates sample matches for engine inspection and determinism
// checks. Example: go run ./cmd/simcli -seed 42 -runs 3 -out match.json
package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"os"

	"github.com/yigitarac/manageria/server/internal/engine"
	"github.com/yigitarac/manageria/server/internal/engine/enginetest"
)

func main() {
	seed := flag.Uint64("seed", 42, "match seed")
	runs := flag.Int("runs", 2, "identical re-runs to verify determinism")
	out := flag.String("out", "", "write the match result JSON to this file")
	flag.Parse()

	if *runs < 1 {
		fatal(fmt.Errorf("-runs must be >= 1"))
	}

	in := enginetest.SampleInput(*seed)
	var (
		firstRaw []byte
		result   engine.MatchResult
	)
	for i := 0; i < *runs; i++ {
		res, _, err := engine.Simulate(in)
		if err != nil {
			fatal(err)
		}
		raw, err := json.MarshalIndent(res, "", "  ")
		if err != nil {
			fatal(err)
		}
		if firstRaw == nil {
			firstRaw, result = raw, res
			continue
		}
		if !bytes.Equal(firstRaw, raw) {
			fatal(fmt.Errorf("run %d differs from run 1 — determinism broken", i+1))
		}
	}

	digest, err := enginetest.Digest(result)
	if err != nil {
		fatal(err)
	}

	if *out != "" {
		if err := os.WriteFile(*out, firstRaw, 0o644); err != nil {
			fatal(err)
		}
	}

	fmt.Printf("seed:        %d\n", *seed)
	fmt.Printf("score:       %d-%d\n", result.Score.Home, result.Score.Away)
	fmt.Printf("possession:  %.1f%%-%.1f%%\n", result.Stats.Home.PossessionPct, result.Stats.Away.PossessionPct)
	fmt.Printf("shots:       %d-%d (on target %d-%d)\n",
		result.Stats.Home.Shots, result.Stats.Away.Shots,
		result.Stats.Home.OnTarget, result.Stats.Away.OnTarget)
	fmt.Printf("events:      %d (keyframes %d)\n", len(result.Events), len(result.Keyframes))
	fmt.Printf("digest:      %s\n", digest)
	fmt.Printf("determinism: OK (%d identical runs)\n", *runs)
}

func fatal(err error) {
	fmt.Fprintln(os.Stderr, "simcli:", err)
	os.Exit(1)
}
