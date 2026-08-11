/**
 * Seed a full per-goal rotation set onto every comp that doesn't already have
 * one, so the Team Builder's "What would you like to optimize for?" picker
 * offers all four goals — General, the four Co-op Bosses, Events and Frenzy
 * Feast — on BOTH spend routes.
 *
 *   BSRData/Teams/<group>/<team>.json   <- rewritten in place
 *
 * WHAT IS AND ISN'T REAL
 * ----------------------
 * The `general` rotation on each route is the comp's existing, hand-authored
 * one and is left exactly as it is. Everything this script ADDS is a starting
 * point, generated one of two ways:
 *
 *   - Comps that already have `iconSteps` (ts-mono, ulquiorra-mono, and the
 *     hand-authored kenpachi-team which is skipped entirely): the new goals are
 *     TRANSFORMS of that verified rotation — trimmed to the opener for the
 *     short fights, stripped of Ultimates for the sustain route, Counter-laced
 *     for the defensive boss, and so on. The kit knowledge in the original
 *     carries through.
 *   - Comps with prose-only rotations (most of them): there are no iconSteps to
 *     transform, so the new goals are built from the comp's real members and
 *     their roles — carry / second / third — using only skill kinds every
 *     character has (Basic Attack, Technique, Ultimate, Counter, Swap).
 *
 * Either way the result is STRUCTURALLY valid and uses the comp's real cast,
 * but the timings and skill choices have not been confirmed in game. Every
 * generated rotation is therefore stamped `"unverified": true`, which the UI
 * renders as a caution chip next to the rotation name. Drop the flag from the
 * JSON once a rotation has actually been play-tested.
 *
 * Re-running is safe: a goal a comp already carries is never overwritten, so
 * hand-corrected rotations survive.
 *
 * Run:  node BSRData/SeedGoalRotations.mjs
 */

import fs from 'node:fs';
import path from 'node:path';
import { fileURLToPath } from 'node:url';

const here = path.dirname(fileURLToPath(import.meta.url));
const TEAMS = path.join(here, 'Teams');

/** Hand-authored across every goal already — never touch it. */
const SKIP = new Set(['kenpachi-team']);

const BOSSES = [
  ['ranuganga', 'Ranuganga'],
  ['yammy-llargo', 'Yammy Llargo'],
  ['sajin-komamura', 'Sajin Komamura'],
  ['ashisougi-jizo', 'Ashisougi Jizo'],
];

const act = (kind, extra) => ({ kind, ...extra });
const swap = (slug) => ({ kind: 'Swap', slug });
const step = (slug, ...actions) => ({ slug, actions });

/**
 * Orders a comp's three members into the roles the templates speak in:
 * `carry` (the Assault, or the first member when a comp runs none), then the
 * other two in listed order. Every template below is written against A/B/C so
 * it works for mono comps and double-support comps alike.
 */
function cast(team) {
  const ms = team.members.map((m) => m.slug);
  const assaultIdx = team.members.findIndex((m) => m.role === 'Assault');
  const i = assaultIdx >= 0 ? assaultIdx : 0;
  const rest = ms.filter((_, n) => n !== i);
  return { A: ms[i], B: rest[0] ?? ms[i], C: rest[1] ?? rest[0] ?? ms[i] };
}

const nameOf = (team, slug) =>
  team.members.find((m) => m.slug === slug)?.name?.split(' ')[0] ?? slug;

/* ------------------------------------------------------------------ *
 * Templates — used when a comp has no iconSteps to transform.         *
 * `enh` is the Enhanced-tier prefix: P2W gets it, F2P doesn't.        *
 * ------------------------------------------------------------------ */
function templates({ A, B, C }, enh) {
  const E = enh ? 'Enhanced ' : '';
  return {
    'frenzy-feast': [
      step(C, act('Technique'), swap(B)),
      step(B, act('Technique'), swap(A)),
      step(A, act('Technique'), act('Basic Attack', { label: 'x2' }), act(`${E}Technique`)),
      step(A, act('Ultimate')),
      step(A, act('Repeat', { label: 'Step 01' })),
    ],
    ranuganga: [
      step(B, act('Technique'), swap(A)),
      step(A, act('Technique'), act('Basic Attack', { label: 'x2' }), swap(C)),
      step(C, act('Ultimate'), swap(A)),
      step(A, act('Ultimate'), act('Repeat', { label: 'Step 01' })),
    ],
    'yammy-llargo': [
      step(C, act('Ultimate'), swap(B)),
      step(B, act('Technique'), act('Basic Attack', { label: 'x2' }), swap(A)),
      step(A, act('Technique'), act('Basic Attack', { label: 'x3' }), act('Ultimate')),
      step(A, act('Basic Attack', { label: 'x4' }), swap(C)),
      step(C, act('Technique'), act('Ultimate'), swap(A)),
      step(A, act('Ultimate'), act('Repeat', { label: 'Step 01' })),
    ],
    'sajin-komamura': [
      step(B, act('Technique'), act('Counter'), swap(A)),
      step(A, act('Counter'), act('Technique'), swap(C)),
      step(C, act('Technique'), act('Basic Attack', { label: 'x2' }), swap(B)),
      step(B, act(`${E}Counter`), act('Basic Attack', { label: 'x2' }), swap(A)),
      step(A, act(`${E}Technique`), swap(C)),
      step(C, act('Ultimate'), swap(A)),
      step(A, act('Ultimate'), act('Repeat', { label: 'Step 01' })),
    ],
    'ashisougi-jizo': [
      step(C, act('Technique'), act('Ultimate'), swap(B)),
      step(B, act('Technique'), act('Basic Attack', { label: 'x2' }), swap(A)),
      step(A, act('Technique'), act('Basic Attack', { label: 'x3' }), act('Counter'), swap(B)),
      step(B, act('Counter'), swap(A)),
      step(A, act(`${E}Technique`), act('Basic Attack', { label: 'x3' }), swap(C)),
      step(C, act('Technique'), act('Basic Attack', { label: 'x1' }), swap(A)),
      step(A, act('Ultimate'), act('Basic Attack', { label: 'x5' }), act('Ultimate')),
      step(A, act('Repeat', { label: 'Step 01' })),
    ],
    events: [
      step(A, act('Technique'), act('Basic Attack', { label: 'x3' }), swap(B)),
      step(B, act('Technique'), act('Basic Attack', { label: 'x2' }), swap(C)),
      step(C, act('Technique'), act('Basic Attack', { label: 'x1' }), swap(A)),
      step(A, act(`${E}Technique`), act('Counter'), act('Basic Attack', { label: 'x3' }), swap(B)),
      step(A, act('Repeat', { label: 'Step 01' })),
    ],
  };
}

/* ------------------------------------------------------------------ *
 * Transforms — used when the comp HAS a verified iconSteps rotation.  *
 * ------------------------------------------------------------------ */
const clone = (x) => JSON.parse(JSON.stringify(x));
const hasKind = (s, re) => s.actions.some((a) => re.test(a.kind ?? ''));
const REPEAT = (slug) => step(slug, act('Repeat', { label: 'Step 01' }));

function transforms(base, { A }) {
  const S = clone(base);
  const ults = S.filter((s) => hasKind(s, /Ultimate/i));
  const noUlt = S.filter((s) => !hasKind(s, /Ultimate|Battlefield|BFS/i));
  const head = S.slice(0, Math.max(2, Math.ceil(S.length / 4)));
  const half = S.slice(0, Math.ceil(S.length / 2));

  // Defensive route: lead every step with a Counter and stop stacking filler
  // basics, which is the trade the harder boss actually asks for.
  const defensive = clone(S).map((s) => ({
    slug: s.slug,
    actions: [
      act('Counter'),
      ...s.actions.filter((a) => !/Basic Attack/i.test(a.kind ?? '') || !a.label),
    ],
  }));

  return {
    // Buffs, then the first Ultimate window, then loop — no second cycle.
    'frenzy-feast': [...head, ...ults.slice(0, 1), REPEAT(A)],
    // Pure opener: whatever the verified rotation front-loads, then loop.
    ranuganga: [...S.slice(0, 3), REPEAT(A)],
    // Through the mid-fight, then every Ultimate window the rotation has.
    'yammy-llargo': [...half, ...ults, REPEAT(A)],
    'sajin-komamura': [...defensive, REPEAT(A)],
    // The whole verified rotation, looped — the longest fight gets all of it.
    'ashisougi-jizo': [...S, REPEAT(A)],
    // Sustain: drop the Ultimate/Battlefield dumps, keep the repeatable core.
    events: [...(noUlt.length ? noUlt : S), REPEAT(A)],
  };
}

/* ------------------------------------------------------------------ */

const BLURB = {
  'frenzy-feast': (n) =>
    `Front-loads every buff so ${n} swings inside the full window — no second cycle, which is the trade a timed score push wants.`,
  ranuganga: (n) => `Opener only: ${n} should not need a rebuild at this tier.`,
  'yammy-llargo': (n) =>
    `Two cycles — the filler between them exists to get ${n} back to a second Ultimate rather than to add damage.`,
  'sajin-komamura': (n) =>
    `Counter windows over greed. Slower than the General route, but it survives the phase transitions instead of trading ${n} for them.`,
  'ashisougi-jizo': (n) =>
    `The longest of the set — assumes the fight outlives two full Ultimate cycles and that ${n} needs every buff window twice.`,
  events: (n) =>
    `A closed loop with no opener and no Ultimate dump, so it holds up after the burst routes run dry. Bank ${n}'s Ultimate for whatever window the stage gives you.`,
};

const TAGS = {
  'frenzy-feast': ['Burst', 'Speed'],
  ranuganga: ['Boss', 'Burst'],
  'yammy-llargo': ['Boss'],
  'sajin-komamura': ['Boss', 'Defensive'],
  'ashisougi-jizo': ['Boss', 'Sustain'],
  events: ['Sustain'],
};

const LABEL = {
  'frenzy-feast': 'Frenzy Feast',
  events: 'Events',
  ...Object.fromEntries(BOSSES),
};

/** Prose lines mirroring the icon steps, for the text view. */
function proseFor(team, steps) {
  return steps.slice(0, 4).map((s) =>
    `${nameOf(team, s.slug)}: ${s.actions
      .map((a) => (a.kind === 'Swap' ? `swap to ${nameOf(team, a.slug)}` : `${a.kind}${a.label ? ` ${a.label}` : ''}`))
      .join(' → ')}`,
  );
}

function buildRotation(team, key, steps, route) {
  const isBoss = BOSSES.some(([id]) => id === key);
  const carryName = nameOf(team, cast(team).A);
  return {
    name: `${team.name} — ${LABEL[key]}`,
    goal: isBoss ? 'coop-boss' : key,
    ...(isBoss ? { boss: key } : {}),
    tags: TAGS[key],
    unverified: true,
    steps: proseFor(team, steps),
    iconSteps: steps,
    notes:
      `${BLURB[key](carryName)} ` +
      `Generated for the ${route.toUpperCase()} route from this comp's own rotation — not play-tested yet.`,
  };
}

const GOAL_KEYS = ['frenzy-feast', ...BOSSES.map(([id]) => id), 'events'];

let comps = 0;
let added = 0;
for (const g of fs.readdirSync(TEAMS, { withFileTypes: true }).filter((d) => d.isDirectory())) {
  for (const f of fs.readdirSync(path.join(TEAMS, g.name)).filter((x) => x.endsWith('.json'))) {
    const p = path.join(TEAMS, g.name, f);
    const team = JSON.parse(fs.readFileSync(p, 'utf8'));
    if (SKIP.has(team.id)) continue;

    const roles = cast(team);
    let touched = false;

    for (const route of ['p2w', 'f2p']) {
      const key = `${route}-rotations`;
      const list = team[key];
      if (!Array.isArray(list) || !list.length) continue;

      // Which goals this route already covers — never regenerate over them, so
      // a hand-corrected rotation is safe from a re-run.
      const have = new Set(
        list.map((r) => (r.goal === 'coop-boss' ? r.boss : r.goal ?? 'general')),
      );

      const withIcons = list.find((r) => r.iconSteps?.length);
      const built = withIcons
        ? transforms(withIcons.iconSteps, roles)
        : templates(roles, route === 'p2w');

      for (const gk of GOAL_KEYS) {
        if (have.has(gk)) continue;
        list.push(buildRotation(team, gk, built[gk], route));
        added += 1;
        touched = true;
      }

      // Catalog order: General, the four bosses, Events, Frenzy Feast —
      // matching GOAL_CATALOG in Web/Source/Lib/RotationGoals.ts.
      const ORDER = ['general', 'coop-boss', 'events', 'frenzy-feast'];
      const BOSS_ORDER = BOSSES.map(([id]) => id);
      list.sort(
        (a, b) =>
          ORDER.indexOf(a.goal ?? 'general') - ORDER.indexOf(b.goal ?? 'general') ||
          BOSS_ORDER.indexOf(a.boss) - BOSS_ORDER.indexOf(b.boss),
      );
    }

    if (touched) {
      fs.writeFileSync(p, `${JSON.stringify(team, null, 2)}\n`);
      comps += 1;
    }
  }
}

console.log(
  `Seeded ${added} unverified rotation(s) across ${comps} comp(s). ` +
    `kenpachi-team skipped (hand-authored).`,
);
