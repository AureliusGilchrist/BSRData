# BSRData

Data backing the [BSRCalc](https://github.com/AureliusGilchrist/BSRCalc) site
(Bleach: Soul Resonance build/tier calculator).

Contents:
- `Data/` — per-character JSON, plus `Index.json` and `Teams.json`.
- `Images/` — character art and skill icons.
- `Scraper/` — Go program that pulls the Spanish Fandom wiki + writes JSON.
- `Curate/` — Go program that overlays hand-curated fields (rarity, tier,
  recommended stop boundaries, etc.) on top of the scraped data.

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
