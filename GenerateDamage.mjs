// Build a per-character `damage` block from the free-text kit data and write it
// back into every character JSON. Run: `node BSRData/GenerateDamage.mjs [dir]`.
//
// WHAT THIS IS (and is NOT):
//   The game's real damage formula needs enemy DEF/resistance, the elemental
//   affinity triangle, and buff uptimes/cooldowns/energy — NONE of which exist
//   anywhere in this dataset. So this is NOT a validated combat simulator. It
//   is an honest extraction + first-order estimate:
//     • base skill multipliers ("dealing damage equal to 6600% of ATK"),
//     • every damage-category MODIFIER we can recognize across ALL sources
//       (skills, boundaries, core passives, weapon stamps, core stamps),
//       each tagged with its source, its %, and whether it's CONDITIONAL,
//     • a first-order per-skill estimate using base card ATK + the always-on
//       modifiers that apply to that skill. Flagged as an estimate everywhere.
//
// Mirrors the parsing philosophy of Web/Source/Lib/SkillParse.ts (base %ATK)
// and Web/Source/Lib/EngraveEffects.ts (stat/modifier extraction + the
// conditional guard) so the numbers agree with what the site already shows.
import fs from 'node:fs';
import path from 'node:path';

import { listCharacterFiles } from './Lib/Characters.mjs';

// An explicit Characters folder can be passed as argv[2]; otherwise the one
// next to this script is used.
const DIR = process.argv[2];

// ---- Regexes ---------------------------------------------------------------

/** A base "N% of ATK" hit — same shape SkillParse.ts uses. */
const ATK_PCT_RE = /(\d{1,4}(?:\.\d+)?)\s*%\s*of\s+(?:(?:[A-Z][a-zA-Z']*\s+){1,3})?ATK\b/g;

/** A percentage value, thousands separators allowed ("5,000%"). */
const VALUE = '(\\d{1,3}(?:,\\d{3})*(?:\\.\\d+)?|\\d+(?:\\.\\d+)?)\\s*%';

/**
 * Battle-state qualifiers — copied from EngraveEffects.ts. A line containing
 * any of these is a situational buff, so we still record the modifier but flag
 * it `conditional: true` (it must NOT feed the always-on estimate).
 */
const CONDITIONAL_RE =
  /\b(?:when|while|if|during|after|upon|per|each|every|trigger(?:s|ed|ing)?|stack(?:s|ing)?|up\s+to|next|for\s+\d+\s*s(?:econds)?|enem(?:y|ies)|target|team(?:mates?)?|grant(?:s|ed|ing)?|switch|consum\w*|releas\w*|reach\w*|spectat\w*|at\s+max)\b/i;

/** A line only yields a modifier if it reads as a buff/boost, never a base
 * multiplier. Keeps "dealing damage equal to 850% of ATK" out of the buckets. */
const BOOST_RE = /\b(?:increase[sd]?|bonus|boost(?:s|ed)?|raise[sd]?|amplif\w*|enhanc\w*|additional|reduc\w*)\b|\+\s*\d/i;

/**
 * Damage categories, most-specific first so "Ultimate DMG" wins before a bare
 * "DMG" (generic) can match. `specific: false` marks the catch-all buckets.
 */
const CATS = [
  { cat: 'ultimate',  re: 'ult(?:imate)?(?:\\s+skill)?\\s+(?:dmg|damage)', specific: true },
  { cat: 'basic',     re: 'basic(?:\\s+attack)?\\s+(?:dmg|damage)', specific: true },
  { cat: 'technique', re: 'technique\\s+(?:dmg|damage)', specific: true },
  { cat: 'special',   re: 'special(?:\\s+attack)?\\s+(?:dmg|damage)', specific: true },
  { cat: 'counter',   re: 'counter(?:attack)?\\s+(?:dmg|damage)', specific: true },
  { cat: 'slash',     re: 'slash\\s+(?:dmg|damage)(?:\\s+bonus)?', specific: true },
  { cat: 'strike',    re: 'strike\\s+(?:dmg|damage)(?:\\s+bonus)?', specific: true },
  { cat: 'thrust',    re: 'thrust\\s+(?:dmg|damage)(?:\\s+bonus)?', specific: true },
  { cat: 'spirit',    re: 'spirit\\s+(?:dmg|damage)(?:\\s+bonus)?', specific: true },
  { cat: 'ailment',   re: 'ailment\\s+(?:dmg|damage)(?:\\s+bonus)?', specific: true },
  { cat: 'allSkill',  re: 'all[- ]skill\\s+(?:mastery|dmg|damage)', specific: true },
  // Negative lookbehind so "Crit DMG"/"Critical Damage" (a crit stat, handled
  // separately) never counts as a generic damage-category bonus.
  { cat: 'generic',   re: '(?<!crit\\s)(?<!critical\\s)(?:dmg|damage)(?:\\s+dealt|\\s+bonus|\\s+taken)?', specific: false },
];

/** Enemy resistance shred — increases the damage the target takes, so it's a
 * damage source too. Captured separately (keyed by element). */
const RESIST_RE = new RegExp(
  // Allow up to three leading words ("their", "the target's", "enemy's", …)
  // between "reduces" and the element so possessive phrasings are all caught.
  `reduc\\w*\\s+(?:[\\w']+\\s+){0,3}?(slash|strike|thrust|spirit|all|elemental)\\s+resistance\\s+by\\s+${VALUE}`,
  'i',
);

const ELEMENT_CAT = { Slash: 'slash', Strike: 'strike', Thrust: 'thrust', Spirit: 'spirit' };
/** Canonical skill kind → the damage bucket(s) that skill's hits belong to. */
const KIND_CATS = {
  'Basic Attack': ['basic'],
  Technique: ['technique'],
  Ultimate: ['ultimate'],
  Counter: ['counter'],
  Counterattack: ['counter'],
};

// ---- Helpers ---------------------------------------------------------------

const num = (v) => {
  const n = Number(String(v ?? '').replace(/[^0-9.]/g, ''));
  return Number.isFinite(n) ? n : 0;
};
const round = (v) => Math.round(v * 100) / 100;

/** All base "N% of ATK" multipliers in a description, summed. */
function baseHits(description) {
  if (!description) return [];
  const hits = [];
  for (const m of description.matchAll(ATK_PCT_RE)) {
    const pct = Number(m[1]);
    if (Number.isFinite(pct) && pct > 0) hits.push(pct);
  }
  return hits;
}

/** Find a percentage value sitting next to `catRe` in `line`, in either order,
 * ignoring any "N% of ATK" (that's a base multiplier, not a bonus). Returns the
 * number or null. */
function valueNear(line, catRe) {
  for (const re of [
    new RegExp(`${catRe}[^.]{0,40}?${VALUE}`, 'i'),
    new RegExp(`${VALUE}[^.]{0,25}?${catRe}`, 'i'),
  ]) {
    const m = line.match(re);
    if (!m) continue;
    // Reject the base-multiplier phrasing "…N% of ATK".
    const after = line.slice((m.index ?? 0) + m[0].length, (m.index ?? 0) + m[0].length + 8);
    if (/^\s*of\s+/i.test(after) && /of\s+ATK/i.test(line.slice(m.index ?? 0))) continue;
    const v = Number(m[1].replace(/,/g, ''));
    if (Number.isFinite(v) && v > 0) return v;
  }
  return null;
}

/** Every damage-category modifier a single line yields. */
function lineModifiers(line, source) {
  const out = [];
  if (!line || !BOOST_RE.test(line)) {
    // Resistance shred is the one buff that needn't read as an "increase".
    const rm = line ? line.match(RESIST_RE) : null;
    if (rm) {
      out.push({
        category: `${rm[1].toLowerCase()}Resist`,
        value: Number(rm[2].replace(/,/g, '')),
        op: 'resistShred',
        conditional: CONDITIONAL_RE.test(line),
        source,
        text: line.trim(),
      });
    }
    return out;
  }
  let specificMatched = false;
  for (const { cat, re, specific } of CATS) {
    if (cat === 'generic' && specificMatched) continue;
    const value = valueNear(line, re);
    if (value == null) continue;
    out.push({
      category: cat,
      value,
      op: 'dmgBonus',
      conditional: CONDITIONAL_RE.test(line),
      source,
      text: line.trim(),
    });
    if (specific) specificMatched = true;
  }
  const rm = line.match(RESIST_RE);
  if (rm) {
    out.push({
      category: `${rm[1].toLowerCase()}Resist`,
      value: Number(rm[2].replace(/,/g, '')),
      op: 'resistShred',
      conditional: CONDITIONAL_RE.test(line),
      source,
      text: line.trim(),
    });
  }
  return out;
}

/** Split a description into scan-able lines/clauses. */
function toLines(text) {
  return String(text ?? '')
    .split(/\n|(?<=\.)\s+(?=[A-Z])/)
    .map((s) => s.trim())
    .filter(Boolean);
}

// ---- Per-character build ---------------------------------------------------

function buildDamage(c) {
  const totalATK = num(c.stats?.atk) + num(c.weaponStats?.atk);
  const elementCat = ELEMENT_CAT[c.damageType] ?? null;

  // --- base skill multipliers ---
  const skills = [];
  for (const s of c.skills ?? []) {
    const descs = [s.description, ...(s.segments ?? []).map((g) => g.description)].filter(Boolean);
    const hits = descs.flatMap(baseHits);
    if (hits.length === 0) continue;
    skills.push({
      kind: s.kind,
      name: s.name,
      hits,
      totalPct: round(hits.reduce((a, b) => a + b, 0)),
    });
  }

  // --- modifiers from every source ---
  const modifiers = [];
  const add = (text, source) => {
    for (const line of toLines(text)) modifiers.push(...lineModifiers(line, source));
  };
  for (const s of c.skills ?? []) {
    add(s.description, { type: 'skill', kind: s.kind, name: s.name });
    for (const g of s.segments ?? []) add(g.description, { type: 'skill', kind: s.kind, name: g.title ?? s.name });
  }
  for (const b of c.boundaries ?? []) add(b.description, { type: 'boundary', level: b.level, name: b.name });
  for (const p of c.corePassives ?? []) add(p.description, { type: 'corePassive', name: p.name });
  for (const which of ['weapon', 'core']) {
    const set = c.stamps?.[which];
    if (!set) continue;
    for (const tier of [1, 2, 3, 4, 5]) {
      for (const line of set[`ascend${tier}`] ?? []) {
        modifiers.push(...lineModifiers(line, { type: `${which}Stamp`, tier }));
      }
    }
  }

  // --- buckets: aggregate per category ---
  const buckets = {};
  modifiers.forEach((m, i) => {
    const b = (buckets[m.category] ??= { alwaysOnPct: 0, conditionalPct: 0, count: 0, modifierIndexes: [] });
    if (m.conditional) b.conditionalPct += m.value;
    else b.alwaysOnPct += m.value;
    b.count += 1;
    b.modifierIndexes.push(i);
  });
  for (const b of Object.values(buckets)) {
    b.alwaysOnPct = round(b.alwaysOnPct);
    b.conditionalPct = round(b.conditionalPct);
  }

  // --- first-order per-skill estimate ---
  const applies = (kind) => new Set([...(KIND_CATS[kind] ?? []), 'generic', 'allSkill', ...(elementCat ? [elementCat] : [])]);
  const sumBuckets = (cats, key) =>
    [...cats].reduce((sum, cat) => sum + (buckets[cat]?.[key] ?? 0), 0);

  const estimates = skills.map((s) => {
    const cats = applies(s.kind);
    const alwaysOnBonusPct = round(sumBuckets(cats, 'alwaysOnPct'));
    const conditionalBonusPct = round(sumBuckets(cats, 'conditionalPct'));
    const estBaseDamage = totalATK
      ? Math.round((totalATK * s.totalPct / 100) * (1 + alwaysOnBonusPct / 100))
      : null;
    const estPotentialDamage = totalATK
      ? Math.round((totalATK * s.totalPct / 100) * (1 + (alwaysOnBonusPct + conditionalBonusPct) / 100))
      : null;
    return {
      kind: s.kind,
      name: s.name,
      basePct: s.totalPct,
      alwaysOnBonusPct,
      conditionalBonusPct,
      estBaseDamage,
      estPotentialDamage,
    };
  });

  return {
    note: 'First-order estimate only — base card ATK, no stamps/level scaling, no crit, no enemy DEF/resistance, no elemental affinity. Generated by BSRData/GenerateDamage.mjs from free-text kit data.',
    baseAtk: { character: num(c.stats?.atk), weapon: num(c.weaponStats?.atk), total: totalATK },
    element: c.damageType ?? null,
    skills,
    modifiers,
    buckets,
    estimates,
  };
}

// ---- Run -------------------------------------------------------------------

let changed = 0;
const report = [];
for (const { rel: file, file: p } of listCharacterFiles(...(DIR ? [DIR] : []))) {
  const c = JSON.parse(fs.readFileSync(p, 'utf8'));
  if (!Array.isArray(c.skills)) continue; // not a character file
  c.damage = buildDamage(c);
  fs.writeFileSync(p, JSON.stringify(c, null, 2) + '\n');
  changed += 1;
  const m = c.damage.modifiers.length;
  const est = c.damage.estimates.find((e) => e.kind === 'Ultimate');
  report.push(`${file}: ${c.damage.skills.length} skills, ${m} modifiers${est?.estBaseDamage != null ? `, ult≈${est.estBaseDamage}` : ''}`);
}
console.log(report.join('\n'));
console.log(`\n${changed} files updated`);
