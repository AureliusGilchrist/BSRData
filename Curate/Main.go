// Generates the aggregate Banners.json (and regenerates Index.json) for the
// site. Run with:
//
//   cd "Data Source/curate" && go run .
//
// NOTE: this program no longer merges any curated fields into the per-character
// ../Data/<slug>.json files — those are owned solely by the scraper (which only
// creates JSON for NEW characters). Curate is limited to Banners.json plus
// rebuilding Index.json from whatever per-character JSON already exists.
//
// Teams.json is hand-maintained: it is NOT generated or modified here (or by
// the scraper). Edit Data/Teams.json directly. Banner recommendations are
// community/archetype best-effort guesses — adjust as the meta evolves.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// ---------------------------------------------------------------------------
// Schema mirrors (subset of) Web/Source/Lib/Types.ts.

// Banner is a single summon/rate-up banner. The site shows the `current`
// banner on the home page with a live countdown to EndDate, and lists the
// `upcoming` banners on the /upcoming page.
//
// Dates are plain ISO calendar dates ("YYYY-MM-DD"). The site treats EndDate
// as the END of that day in UTC (i.e. the banner is considered live through
// 23:59:59 UTC on EndDate) when computing the real-time countdown.
type Banner struct {
	// Display name, e.g. "Grimmjow (Pantera) Rate-Up".
	Name string `json:"name"`
	// Featured character slugs (must match a /Data/<slug>.json). The site uses
	// the first slug's art for the banner image and links each to its page.
	Slugs []string `json:"slugs"`
	// ISO calendar dates "YYYY-MM-DD".
	StartDate string `json:"startDate,omitempty"`
	EndDate   string `json:"endDate,omitempty"`
	// Optional explicit banner artwork URL. When empty the site falls back to
	// the featured character's art image.
	Image string `json:"image,omitempty"`
	// Free-form note shown under the banner (optional).
	Note string `json:"note,omitempty"`
	// Where this info came from (optional).
	Source string `json:"source,omitempty"`
}

// Leak is an unconfirmed, datamined/leaked future character. Unlike a Banner,
// a leak does NOT link anywhere — the site only shows who they are plus their
// role and damage type. No art links, no character page. An optional expected
// date drives a "Time until" countdown on the site (there is no end date).
type Leak struct {
	// Character name, e.g. "Ulquiorra Cifer".
	Name string `json:"name"`
	// Role class: "Assault" / "Full Assault / DPS" / "Tactician" / "Support".
	Role string `json:"role,omitempty"`
	// Damage type: "Slash" / "Strike" / "Thrust" / "Spirit".
	DamageType string `json:"damageType,omitempty"`
	// Rarity, if known: "SSR" / "SR+" / "SR" (optional).
	Rarity string `json:"rarity,omitempty"`
	// Optional expected/leaked ISO date "YYYY-MM-DD". When set, the site shows
	// a live "Time until" countdown to the START of that day. There is no end
	// date for leaks — the countdown simply disappears once the day arrives.
	Date string `json:"date,omitempty"`
	// Free-form note (optional), e.g. "datamined, subject to change".
	Note string `json:"note,omitempty"`
}

// BannerData is the shape written to Data/Banners.json.
type BannerData struct {
	Current  *Banner  `json:"current"`
	Upcoming []Banner `json:"upcoming"`
	// Leaks: unconfirmed future characters. Shown on /upcoming with name +
	// role + type only (no links). Filled in by hand.
	Leaks []Leak `json:"leaks"`
}

// ---------------------------------------------------------------------------
// Banners.
//
// Sourced from the Spanish BSR Fandom wiki "Eventos China" timeline:
//   https://bleach-soul-resonance-esp.fandom.com/es/wiki/Eventos_China
// The `current` banner powers the home-page countdown; `upcoming` is shown on
// the /upcoming page.
//
// TO UPDATE: edit `banners` below. When a new rate-up goes live, move the next
// `upcoming` entry into `current` and add the following one to `upcoming`.
// Slugs must match a /Data/<slug>.json file so the site can pull art + link.
var banners = BannerData{
	Current: &Banner{
		Name:      "Ichigo Kurosaki・Inner Hollow Releases!",
		Slugs:     []string{"ichigo-white"},
		StartDate: "2026-06-05",
		EndDate:   "2026-06-26",
		Note:      "First limited Slash Tactic SSR. White debuts in his Shikai as well as Bankai forms.",
	},
	// ---------------------------------------------------------------------
	// UPCOMING — fill these out as new banners are announced. Add the
	// character slug(s), display name, the signature weapon, and the
	// start/end dates (YYYY-MM-DD). Delete the placeholder entries you
	// don't need. Each entry is shown on the /upcoming page in order.
	Upcoming: []Banner{
		{
			Name:      "Ulquiorra Shifar (Base) SR+ Free Unit",
			Slugs:     []string{"ulquiorra"},
			StartDate: "2026-06-05",
			EndDate:   "2026-06-05",
			Note:      "Base form Ulquiorra, and will be free via mail on the 5th of June, 2026 (UTC).",
		},
		{
			Name:      "Ulquiorra Shifar (Resurrección) SSR Rate-Up",
			Slugs:     []string{"ulquiorra-resurreccion"},
			StartDate: "2026-06-26",
			Note:      "Ulquiorra Shifar set to compete against Toshiro as a Spirit Full Assault with his dual forms; Murciélago and Segunda Etapa.",
		},
	},
	// ---------------------------------------------------------------------
	// LEAKS — unconfirmed datamined/leaked characters. These DON'T link to a
	// character page; the site only shows the name, role, and damage type.
	// Fill these in by hand. Delete the placeholders you don't need.
	//   Role:       "Full Assault / DPS" | "Tactician" | "Support"
	//   DamageType: "Slash" | "Strike" | "Thrust" | "Spirit"
	//   Rarity:     "SSR" | "SR+" | "SR" (optional)
	//   Date:       "YYYY-MM-DD" (optional) — drives a "Time until" countdown;
	//               leaks have no end date.
	Leaks: []Leak{
		{
			Name:       "Yammy Llargo",
			Role:       "TBD",
			DamageType: "TBD",
			Rarity:     "TBD",
			Date:       "TBD",
			Note:       "He was added to the game files, however no details upon him were found.",
		},
		{
			Name:       "Nnoitorra Gilga",
			Role:       "TBD",
			DamageType: "TBD",
			Rarity:     "TBD",
			Date:       "TBD",
			Note:       "He was added to the game files, however no details upon him were found.",
		},
		{
			Name:       "Kyouraku Shunsui",
			Role:       "TBD",
			DamageType: "TBD",
			Rarity:     "TBD",
			Date:       "TBD",
			Note:       "He was added to the game files, however no details upon him were found.",
		},
		{
			Name:       "Shuuhei Hisagi",
			Role:       "TBD",
			DamageType: "TBD",
			Rarity:     "TBD",
			Date:       "TBD",
			Note:       "He was added to the game files, however no details upon him were found.",
		},
		{
			Name:       "Yamamoto Genryuusai",
			Role:       "TBD",
			DamageType: "TBD",
			Rarity:     "TBD",
			Date:       "TBD",
			Note:       "He was added to the game files, however no details upon him were found.",
		},
		{
			Name:       "Unohana Yachiru",
			Role:       "TBD",
			DamageType: "TBD",
			Rarity:     "TBD",
			Date:       "TBD",
			Note:       "She was added to the game files, however no details upon her were found.",
		},
	},
}

func main() {
	// Walk up from "BSRData/Curate" to the BSRData root.
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	// Allow running from BSRData root too.
	dataDir := filepath.Join(wd, "..", "Data")
	if _, err := os.Stat(dataDir); err != nil {
		// Try assuming wd is BSRData root.
		dataDir = filepath.Join(wd, "Data")
	}
	if _, err := os.Stat(dataDir); err != nil {
		panic(fmt.Errorf("could not find Data dir: %w", err))
	}
	fmt.Println("Data dir:", dataDir)

	// Per-character JSON merging has been intentionally removed: the scraper is
	// now the sole writer of per-character <slug>.json files (and only for NEW
	// characters). Curate is limited to the aggregate Banners.json and
	// regenerating Index.json from whatever per-character JSON exists.
	//
	// Teams.json is NOT generated here. It is a hand-maintained file — edit
	// Data/Teams.json directly. Curate (and the scraper) deliberately leave it
	// untouched; regenerateIndex also skips it.

	// Write Banners.json (current + upcoming rate-up banners).
	bannersPath := filepath.Join(dataDir, "Banners.json")
	bannersOut, err := json.MarshalIndent(banners, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(bannersPath, bannersOut, 0644); err != nil {
		panic(err)
	}
	fmt.Printf("Wrote banners to %s\n", bannersPath)

	// Regenerate Index.json so it picks up releaseDate (and any new chars).
	if err := regenerateIndex(dataDir); err != nil {
		fmt.Printf("  ! index regen: %v\n", err)
	} else {
		fmt.Println("  + Index.json regenerated (with releaseDate)")
	}
}

// regenerateIndex walks dataDir, reads every per-char JSON, and writes a fresh
// Index.json containing slug/name/role/tier/damageType/rarity/art/releaseDate.
// Sorted by slug for stable diffs — UI sorts on render.
func regenerateIndex(dataDir string) error {
	type indexEntry struct {
		Slug        string `json:"slug"`
		Name        string `json:"name,omitempty"`
		Role        string `json:"role,omitempty"`
		Tier        string `json:"tier,omitempty"`
		DamageType  string `json:"damageType,omitempty"`
		Rarity      string `json:"rarity,omitempty"`
		Art         string `json:"art,omitempty"`
		ReleaseDate string `json:"releaseDate,omitempty"`
	}
	files, err := os.ReadDir(dataDir)
	if err != nil {
		return err
	}
	entries := []indexEntry{}
	for _, f := range files {
		if f.IsDir() {
			continue
		}
		name := f.Name()
		if filepath.Ext(name) != ".json" {
			continue
		}
		if name == "Index.json" || name == "Stamps.json" || name == "Teams.json" || name == "Banners.json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dataDir, name))
		if err != nil {
			continue
		}
		var d map[string]any
		if err := json.Unmarshal(raw, &d); err != nil {
			continue
		}
		getStr := func(k string) string {
			if v, ok := d[k].(string); ok {
				return v
			}
			return ""
		}
		e := indexEntry{
			Slug:        getStr("slug"),
			Name:        getStr("name"),
			Role:        getStr("role"),
			Tier:        getStr("tier"),
			DamageType:  getStr("damageType"),
			Rarity:      getStr("rarity"),
			Art:         getStr("art"),
			ReleaseDate: getStr("releaseDate"),
		}
		if e.Slug == "" {
			e.Slug = name[:len(name)-len(".json")]
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Slug < entries[j].Slug })
	out, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, "Index.json"), out, 0644)
}
