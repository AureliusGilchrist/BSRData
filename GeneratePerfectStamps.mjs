/**
 * PERFECT STAMPS — scaffold + validate + compile.
 *
 * A character's "perfect stamps" are the hand-authored, theory-crafted ideal
 * Stamp 1/2/3 for that character: the exact mainstat, level, substat spread,
 * passive and set you'd run if the RNG went perfectly. They are NOT derived
 * from anything — `bestStamp` is prose written for humans, and turning it into
 * numbers would be this script guessing. They are written BY HAND into each
 * character's JSON under a top-level `perfectStamps` array.
 *
 * This script does three things and never invents a stamp:
 *
 *   1. SCAFFOLD  — adds an empty `"perfectStamps": []` to any character JSON
 *                  that has no such key, so there's an obvious place to author
 *                  them. Existing arrays are never touched.
 *   2. VALIDATE  — checks every hand-written entry against the game's slot,
 *                  stat, level and substat-budget rules, and fails loudly.
 *                  A typo'd stat key is a build error, not a silent drop.
 *   3. COMPILE   — writes the aggregate the web app bundles synchronously
 *                  (Web/Source/Data/PerfectStamps.json), so the Stamp Creator
 *                  and My Roster can offer every character's perfect stamps
 *                  without fetching 30-odd character JSONs.
 *
 * Run:  node BSRData/GeneratePerfectStamps.mjs
 * Wired into Build.ps1 (step 2d), next to GenerateStampSets.mjs.
 *
 * ---------------------------------------------------------------------------
 * AUTHORING FORMAT — up to three entries, one per rollable slot:
 *
 *   "perfectStamps": [
 *     {
 *       "slot": "stamp1",
 *       "mainStat": "thrustDmg",
 *       "level": 30,
 *       "substats": [
 *         { "stat": "ailmentPct", "ticks": 3 },
 *         { "stat": "critRate",   "ticks": 0 },
 *         { "stat": "atkPct",     "ticks": 0 },
 *         { "stat": "critDmg",    "ticks": 0 }
 *       ],
 *       "passive": "overdrive-assault",
 *       "setId": "duty-borne",
 *       "note": "optional — why this is the ideal roll"
 *     }
 *   ]
 *
 * Rules the validator enforces:
 *   · `slot` is stamp1 / stamp2 / stamp3, at most one entry each.
 *   · `mainStat` must be a mainstat that slot can actually roll.
 *   · `level` must be a real ascension stop (1, 10, 15, 20, 25, 30).
 *   · exactly 4 substats, all distinct, all from the substat pool.
 *   · total `ticks` must equal the freely-spendable budget at that level
 *     (3 at level 30) — a "perfect" stamp has every tick spent. Level 30 also
 *     auto-grants +1 tick to EVERY slot, which is implicit; don't write it.
 *   · `passive` (optional) must be a Lib/StampPassives id.
 *   · `setId` (optional) must be a BSRData/StampSets/<id>.json id.
 *
 * The display name is derived, not authored: "Perfect <Character> Stamp <n>".
 */

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const DATA_DIR = path.join(here, 'Data');
const SETS_DIR = path.join(here, 'StampSets');
const PASSIVES_TS = path.join(here, '..', 'Web', 'Source', 'Lib', 'StampPassives.ts');
const OUT_FILE = path.join(here, '..', 'Web', 'Source', 'Data', 'PerfectStamps.json');

/* --------------------------------------------------------------------------
 * Game rules, mirrored from Web/Source/Lib/StampBuilder.ts.
 * KEEP IN SYNC with that file — it is the source of truth for the app; this
 * copy exists only so the build can reject bad data before it ships.
 * ------------------------------------------------------------------------ */

const ROLLABLE_SLOTS = ['stamp1', 'stamp2', 'stamp3'];

const SLOT_MAINSTATS = {
  stamp1: ['strikeDmg', 'thrustDmg', 'slashDmg', 'spiritDmg', 'atk', 'hp', 'def', 'atkPct', 'hpPct', 'defPct'],
  stamp2: ['critRate', 'critDmg', 'atk', 'hp', 'atkPct', 'hpPct', 'defPct'],
  stamp3: ['ultChargeRate', 'ailmentPct', 'atk', 'def', 'atkPct', 'hpPct', 'defPct'],
};

const SUBSTAT_POOL = [
  'atk', 'hp', 'def', 'atkPct', 'hpPct', 'defPct',
  'critRate', 'critDmg', 'ultChargeRate', 'ailmentPct',
];

const LEVEL_STOPS = [1, 10, 15, 20, 25, 30];
const SUBSTAT_POINTS_PER_STOP = { 1: 0, 10: 0, 15: 1, 20: 1, 25: 1, 30: 0 };
const SUBSTAT_SLOT_COUNT = 4;
const SUBSTAT_MAX_PER_STAT = 4;
/** Level 30 auto-grants this many ticks to every slot on top of the spent ones. */
const SUBSTAT_FORCED_BONUS = 1;
const SUBSTAT_FORCED_LEVEL = 30;

/** Freely-spendable ticks unlocked by the time a piece reaches `level`. */
function budgetAtLevel(level) {
  return LEVEL_STOPS.filter((stop) => stop <= level).reduce(
    (sum, stop) => sum + (SUBSTAT_POINTS_PER_STOP[stop] ?? 0),
    0,
  );
}

/* -------------------------------------------------------------------------- */

/** Known passive ids, read straight out of the catalog so they can't drift. */
function knownPassiveIds() {
  const src = fs.readFileSync(PASSIVES_TS, 'utf8');
  return new Set([...src.matchAll(/^\s{4}id: '([^']+)',$/gm)].map((m) => m[1]));
}

/** Known stamp-set ids — one file per set, filename === id. */
function knownSetIds() {
  return new Set(
    fs.readdirSync(SETS_DIR).filter((f) => f.endsWith('.json')).map((f) => path.basename(f, '.json')),
  );
}

const passiveIds = knownPassiveIds();
const setIds = knownSetIds();
const errors = [];

/**
 * Check one authored entry. Returns the normalised piece, or null when it was
 * rejected (every rejection is recorded in `errors` and fails the run).
 */
function validatePiece(slug, piece, index, seenSlots) {
  const at = `${slug}.json perfectStamps[${index}]`;
  const fail = (msg) => {
    errors.push(`${at}: ${msg}`);
    return null;
  };

  if (!piece || typeof piece !== 'object') return fail('must be an object');
  if (!ROLLABLE_SLOTS.includes(piece.slot)) {
    return fail(`slot "${piece.slot}" — must be one of ${ROLLABLE_SLOTS.join(', ')} (weapon/core stamps are ascension engravings, not rollable pieces)`);
  }
  if (seenSlots.has(piece.slot)) return fail(`duplicate slot "${piece.slot}" — one perfect stamp per slot`);
  seenSlots.add(piece.slot);

  if (!SLOT_MAINSTATS[piece.slot].includes(piece.mainStat)) {
    return fail(`mainStat "${piece.mainStat}" can't roll on ${piece.slot} — allowed: ${SLOT_MAINSTATS[piece.slot].join(', ')}`);
  }
  if (!LEVEL_STOPS.includes(piece.level)) {
    return fail(`level ${piece.level} is not an ascension stop (${LEVEL_STOPS.join(', ')})`);
  }

  const subs = piece.substats;
  if (!Array.isArray(subs) || subs.length !== SUBSTAT_SLOT_COUNT) {
    return fail(`needs exactly ${SUBSTAT_SLOT_COUNT} substats, got ${Array.isArray(subs) ? subs.length : typeof subs}`);
  }
  const seenStats = new Set();
  let spent = 0;
  for (const s of subs) {
    if (!s || !SUBSTAT_POOL.includes(s.stat)) return fail(`substat "${s?.stat}" is not in the substat pool`);
    if (seenStats.has(s.stat)) return fail(`substat "${s.stat}" appears twice — the 4 slots must be distinct`);
    seenStats.add(s.stat);
    const ticks = s.ticks ?? 0;
    const forced = piece.level >= SUBSTAT_FORCED_LEVEL ? SUBSTAT_FORCED_BONUS : 0;
    if (!Number.isInteger(ticks) || ticks < 0 || ticks + forced > SUBSTAT_MAX_PER_STAT) {
      return fail(`substat "${s.stat}" has ${ticks} ticks — with the level-${SUBSTAT_FORCED_LEVEL} auto-tick that exceeds the cap of ${SUBSTAT_MAX_PER_STAT}`);
    }
    spent += ticks;
  }
  const budget = budgetAtLevel(piece.level);
  if (spent !== budget) {
    return fail(`spends ${spent} substat tick(s) but level ${piece.level} unlocks ${budget} — a perfect stamp spends them all`);
  }

  if (piece.passive && !passiveIds.has(piece.passive)) {
    return fail(`unknown passive "${piece.passive}" (see Web/Source/Lib/StampPassives.ts)`);
  }
  if (piece.setId && !setIds.has(piece.setId)) {
    return fail(`unknown setId "${piece.setId}" (see BSRData/StampSets/)`);
  }

  return {
    slot: piece.slot,
    mainStat: piece.mainStat,
    level: piece.level,
    substats: subs.map((s) => ({ stat: s.stat, ticks: s.ticks ?? 0 })),
    ...(piece.passive ? { passive: piece.passive } : {}),
    ...(piece.setId ? { setId: piece.setId } : {}),
    ...(piece.note ? { note: piece.note } : {}),
  };
}

/* -------------------------------------------------------------------------- */

const files = fs
  .readdirSync(DATA_DIR)
  .filter((f) => f.endsWith('.json') && !/^[A-Z]/.test(f)) // skip Index/Teams/Banners/…
  .sort();

const characters = {};
let scaffolded = 0;
let authored = 0;

for (const file of files) {
  const full = path.join(DATA_DIR, file);
  const raw = fs.readFileSync(full, 'utf8');
  const char = JSON.parse(raw);
  const slug = char.slug ?? path.basename(file, '.json');

  // 1. Scaffold — give every character somewhere to author them, once.
  if (!Array.isArray(char.perfectStamps)) {
    char.perfectStamps = [];
    fs.writeFileSync(full, `${JSON.stringify(char, null, 2)}\n`);
    scaffolded += 1;
  }

  // 2. Validate.
  const seenSlots = new Set();
  const pieces = char.perfectStamps
    .map((p, i) => validatePiece(slug, p, i, seenSlots))
    .filter(Boolean);

  // 3. Collect for the aggregate. Characters with nothing authored yet are
  //    omitted entirely, so the app can just check "is this slug present".
  if (pieces.length > 0) {
    characters[slug] = { name: char.name ?? slug, pieces };
    authored += 1;
  }
}

if (errors.length > 0) {
  console.error('PerfectStamps: refusing to compile — fix these first:\n');
  for (const e of errors) console.error(`  · ${e}`);
  process.exit(1);
}

const NOTE =
  'GENERATED by BSRData/GeneratePerfectStamps.mjs from the `perfectStamps` array inside each ' +
  'BSRData/Data/<slug>.json — edit those, not this aggregate. Perfect stamps are HAND-AUTHORED ' +
  'theory-crafted ideal rolls; nothing derives them from bestStamp. The web app bundles this file ' +
  'so Lib/PerfectStamps can resolve every character\'s perfect stamps synchronously. Display names ' +
  'are derived at read time as "Perfect <Character> Stamp <n>". See the header comment in the ' +
  'generator for the authoring format and the rules it enforces.';

fs.writeFileSync(OUT_FILE, `${JSON.stringify({ note: NOTE, characters }, null, 2)}\n`);

const rel = path.relative(path.join(here, '..'), OUT_FILE);
console.log(
  `PerfectStamps: ${authored} character(s) with authored stamps` +
    (scaffolded ? `, scaffolded ${scaffolded} empty perfectStamps array(s)` : '') +
    ` → ${rel}`,
);
