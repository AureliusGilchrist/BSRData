// Fill missing WEAPON stamp ascend2-4 tiers in every character JSON by
// linearly interpolating the numbers between ascend1 and ascend5, keeping the
// A1 wording ("mimic the text"). Only runs when BOTH A1 and A5 are filled.
// Core stamps are deliberately left at A1+A5 only (site owner's call —
// 2026-07-17). Generated tiers are recorded in an `estimated` array on the
// stamp set so the UI can flag them "(est.)".
import fs from 'node:fs';
import path from 'node:path';

import { listCharacterFiles } from './Lib/Characters.mjs';

// An explicit Characters folder can be passed as argv[2]; otherwise the one
// next to this script is used. There is no aggregate-file skip list any more --
// the folder holds nothing but characters.
const DIR = process.argv[2];
const NUM_RE = /\d{1,3}(?:,\d{3})+(?:\.\d+)?|\d+(?:\.\d+)?/g;

const parseNums = (line) => [...line.matchAll(NUM_RE)].map((m) => Number(m[0].replace(/,/g, '')));

/** v rounded to at most 2 decimals, printed without trailing zeros. */
const fmt = (v) => String(Math.round(v * 100) / 100);

/** a1 line with each number replaced by its tier-t interpolation toward a5. */
function interpLine(l1, l5, t) {
  const n1 = parseNums(l1);
  const n5 = parseNums(l5);
  if (n1.length === 0 || n1.length !== n5.length) return l1; // can't pair — keep A1 text
  let i = 0;
  return l1.replace(NUM_RE, () => {
    const v = n1[i] + ((n5[i] - n1[i]) * (t - 1)) / 4;
    i += 1;
    return fmt(v);
  });
}

let changed = 0;
const report = [];
for (const { rel: file, file: p } of listCharacterFiles(...(DIR ? [DIR] : []))) {
  const c = JSON.parse(fs.readFileSync(p, 'utf8'));
  let touched = false;
  for (const kind of ['weapon']) {
    const s = c.stamps?.[kind];
    const a1 = s?.ascend1;
    const a5 = s?.ascend5;
    if (!Array.isArray(a1) || a1.length === 0 || !Array.isArray(a5) || a5.length === 0) {
      if (Array.isArray(a5) && a5.length) report.push(`${file} ${kind}: skipped (A1 empty)`);
      continue;
    }
    const estimated = [];
    for (const t of [2, 3, 4]) {
      const key = `ascend${t}`;
      if (Array.isArray(s[key]) && s[key].length) continue; // real data — leave alone
      s[key] =
        a1.length === a5.length
          ? a1.map((l, i) => interpLine(l, a5[i], t))
          : a1.slice(); // line counts differ — carry A1 verbatim (conservative)
      estimated.push(t);
    }
    if (estimated.length) {
      s.estimated = estimated;
      touched = true;
      report.push(`${file} ${kind}: estimated A${estimated.join('/A')}${a1.length !== a5.length ? ' (line counts differ — A1 carried)' : ''}`);
    }
  }
  if (touched) {
    // Keep key order stable: ascend1..5 then the rest, matching the scraper's style.
    fs.writeFileSync(p, JSON.stringify(c, null, 2) + '\n');
    changed += 1;
  }
}
console.log(report.join('\n'));
console.log(`\n${changed} files updated`);
