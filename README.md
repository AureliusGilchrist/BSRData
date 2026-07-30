# BSRData

Data backing the [BSRCalc](https://github.com/AureliusGilchrist/BSRCalc) site
(Bleach: Soul Resonance build/tier calculator).

Contents:
- `Data/` — per-character JSON, plus `Index.json` and `Teams.json`.
- `Images/` — character art and skill icons.
- `Scraper/` — Go program that pulls the Spanish Fandom wiki + writes JSON.
- `Curate/` — Go program that overlays hand-curated fields (rarity, tier,
  recommended stop boundaries, etc.) on top of the scraped data.

## Perfect stamps

Each character JSON carries a `perfectStamps` array: the hand-authored, ideal
Stamp 1/2/3 rolls for that character. Nothing derives them — `bestStamp` is
prose for humans, and turning it into numbers would just be a script guessing.
Write them by hand, then run:

```powershell
node GeneratePerfectStamps.mjs
```

That scaffolds the empty array where it's missing, validates everything you've
written (bad stat key, illegal mainstat for the slot, unspent substat ticks…
all fail loudly), and compiles the aggregate the site bundles. The authoring
format and the full list of rules are in the header comment of
`GeneratePerfectStamps.mjs`. Shape, in brief:

```json
"perfectStamps": [
  {
    "slot": "stamp1",
    "mainStat": "thrustDmg",
    "level": 30,
    "substats": [
      { "stat": "ailmentPct", "ticks": 3 },
      { "stat": "critRate",   "ticks": 0 },
      { "stat": "atkPct",     "ticks": 0 },
      { "stat": "critDmg",    "ticks": 0 }
    ],
    "passive": "overdrive-assault",
    "setId": "duty-borne"
  }
]
```

The site shows them as "Perfect &lt;Character&gt; Stamp &lt;n&gt;" — in the Stamp
Creator's third panel and in My Roster's per-slot pickers. They're equip-only:
players can wear them but never save, edit or delete them.

## Local refresh

```powershell
cd Scraper; go run .
cd ../Curate; go run .
```

## Automated refresh

`.github/workflows/Scrape.yml` runs both Go programs daily at 03:00 UTC and
commits any JSON/image changes back to `main`. The BSRCalc site fetches these
files at runtime from `https://raw.githubusercontent.com/AureliusGilchrist/BSRData/main/`,
so a successful commit here is immediately visible to visitors (raw.githubusercontent
has a ~5 minute CDN TTL).
