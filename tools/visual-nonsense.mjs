#!/usr/bin/env node
// Visual-nonsense meter (the eye's accountant): quantifies what a viewer FEELS
// wrong — frozen statues and jumping dots — so fixes and regressions are numbers,
// not vibes. Usage: node tools/visual-nonsense.mjs <dump.json>

import { readFileSync } from "node:fs";

const dump = JSON.parse(readFileSync(process.argv[2] ?? "web/public/samples/match-42.json", "utf8"));

let frozenWindows = 0;
let outfieldFrozen = 0;
let outfieldWindows = 0;
let returnWindows = 0;
let windows = 0;
let maxJump = 0;
let frozenWorst = { slot: -1, window: "", travel: Infinity };

for (let s = 0; s < dump.keyframes.length - 20; s += 20) {
  // 60 s window (20 keyframes @ 3 s)
  for (let slot = 0; slot < 22; slot++) {
    windows++;
    if (slot !== 0 && slot !== 11) outfieldWindows++;
    const a = dump.keyframes[s].players[slot];
    const b = dump.keyframes[s + 20]?.players[slot] ?? a;
    const displacement = Math.hypot(b.x - a.x, b.y - a.y);
    let travel = 0;
    for (let k = s + 1; k <= s + 20; k++) {
      const from = dump.keyframes[k - 1].players[slot];
      const to = dump.keyframes[k].players[slot];
      travel += Math.hypot(to.x - from.x, to.y - from.y);
    }
    if (displacement < 0.02 && travel >= 0.02) returnWindows++;
    if (travel < 0.02) {
      frozenWindows++;
      if (slot !== 0 && slot !== 11) outfieldFrozen++;
      if (travel < frozenWorst.travel) {
        frozenWorst = { slot, window: `${Math.round(dump.keyframes[s].tMs / 1000)}s`, travel };
      }
    }
  }
}
for (let i = 1; i < dump.keyframes.length; i++) {
  for (let slot = 0; slot < 22; slot++) {
    const a = dump.keyframes[i - 1].players[slot];
    const b = dump.keyframes[i].players[slot];
    maxJump = Math.max(maxJump, Math.hypot(b.x - a.x, b.y - a.y));
  }
}

console.log(`VISUAL NONSENSE — keyframes ${dump.keyframes.length} · windows ${windows}`);
console.log(`  true statue-windows   : ${frozenWindows} (${((100 * frozenWindows) / windows).toFixed(1)}% of all windows)`);
console.log(`  outfield statue-windows : ${outfieldFrozen} (${((100 * outfieldFrozen) / outfieldWindows).toFixed(1)}% of outfield windows)`);
console.log(`  returned-to-mark windows: ${returnWindows} (moved, then returned near their start)`);
console.log(`  stillest soul         : slot ${frozenWorst.slot} @ ${frozenWorst.window} (travel ${frozenWorst.travel.toFixed(3)})`);
console.log(`  max 3s jump           : ${maxJump.toFixed(3)} (norm units${maxJump > 0.2 ? " — TELEPORT-LIKE" : ""})`);
console.log(`target: outfield statue% < 20, maxJump < 0.15`);
