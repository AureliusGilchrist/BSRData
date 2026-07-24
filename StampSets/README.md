# Stamp Sets

One JSON file per stamp set — the **source of truth** for the site's stamp-set
bonuses. Kept here (NOT in `BSRData/Data`) on purpose: these are hand-authored
game-rule data, not scraped character data.

Equipping all **three** rollable stamps (Stamp 1 / 2 / 3) of the same set grants
that set's **3-piece bonus** in the damage tools (roster + character damage
breakdown).

Most **SSRs** have their own set. **SR / SR+** have none, and **Kaname Tosen**
(SSR) has none.

## File format

The filename must equal the `id`. The 3-piece bonus is written as **plain text**
— describe it the way the game does, and the app parses it (via
`Lib/KitParse.parseEffectText`, the same engine that reads boundaries and
passives). There are **no** structured `target`/`value` pairs to maintain.

```json
{
  "id": "duty-borne",
  "name": "Duty Borne",
  "ownerSlug": "ichigo-ts",
  "placeholder": false,
  "threePiece": {
    "text": "Increases Thrust DMG by 15%. Each time an on-field enemy is inflicted with Breach, your Breach DMG is increased by 3% and Ultimate DMG by 5%. These effects last 15 seconds and stack up to 10 times."
  }
}
```

- `ownerSlug` — the SSR the set is themed to (display/grouping only).
- `placeholder` — `true` while it's a stand-in.
- `threePiece.text` — the bonus in plain English.

### How the text is read

The parser recognises the game's own phrasings. Tips for clean parses:

- Name the category next to the number: **"Increases Ultimate DMG by 25%"**,
  **"Thrust DMG +15%"**, **"increases all damage dealt by 9%"** (All-Type),
  **"reduces the enemy's All-Type Resistance by 3.6%"** (shred).
- Two categories at once work: **"Technique DMG and Ultimate DMG are increased
  by 25%"** (both +25%) or **"…by 25% and 40%, respectively"** (25% / 40%).
- Named ailments count as ailment DMG: **"Breach DMG +30%"**, **"Wound DMG"**,
  **"Sting DMG"**.
- It reads what you write — **"increased by 3% … stack up to 10 times"** parses
  as **3%**, not 30%. If you want the max-stack total, write the total
  (**"by 30%"**).
- Spelling matters: a typo like `TTechnique` won't be recognised as Technique.

## Regenerate the bundle

After adding/editing a set, recompile the aggregate the web app imports:

```
node BSRData/GenerateStampSets.mjs
```

Writes `Web/Source/Data/StampSets.json`. `Build.ps1` runs this automatically
(step 2c), so a normal build keeps it in sync.
