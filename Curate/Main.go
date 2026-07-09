// Regenerates Index.json for the site. Run with:
//
//	cd "Data Source/curate" && go run .
//
// NOTE: this program no longer merges any curated fields into the per-character
// ../Data/<slug>.json files — those are owned solely by the scraper (which only
// creates JSON for NEW characters). Curate is limited to rebuilding Index.json
// from whatever per-character JSON already exists.
//
// Teams.json, Banners.json, AND Glossary.json are all hand-maintained: none of
// them are generated or modified here (or by the scraper). Edit them directly.
// Glossary.json is keyed per character slug ({"<slug>": {"Term": "definition"}})
// — there's no longer a per-term Data/Glossary/ folder feeding it. (Banners.json
// and Glossary.json used to be regenerated from a hardcoded literal / that
// folder on every curate run, which clobbered hand edits each time; both
// regeneration steps have been removed.) Whether a banner in Banners.json is
// shown as live, upcoming, or archived is computed by the site purely from its
// startDate/endDate vs. today — see classifyBanners in Web/Source/Lib/Banner.ts.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

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
	// characters). Curate is limited to regenerating Index.json from whatever
	// per-character JSON exists.
	//
	// Teams.json, Banners.json, and Glossary.json are NOT generated here. They
	// are hand-maintained files — edit them directly. Curate (and the scraper)
	// deliberately leave them untouched; regenerateIndex also skips them.

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
	// Previous index, keyed by slug — a character whose JSON currently fails
	// to parse keeps its old entry instead of vanishing from the site. Hand
	// edits have shipped broken JSON more than once (aizen, ulquiorra), and
	// silently dropping the character is far worse than serving slightly
	// stale index fields until the file is fixed.
	prev := map[string]indexEntry{}
	if raw, err := os.ReadFile(filepath.Join(dataDir, "Index.json")); err == nil {
		var prevEntries []indexEntry
		if err := json.Unmarshal(raw, &prevEntries); err == nil {
			for _, e := range prevEntries {
				prev[e.Slug] = e
			}
		}
	}
	keepPrev := func(name, reason string) *indexEntry {
		slug := name[:len(name)-len(".json")]
		if e, ok := prev[slug]; ok {
			fmt.Printf("  ! %s: %s — keeping previous index entry\n", name, reason)
			return &e
		}
		fmt.Printf("  ! %s: %s — no previous entry to keep, character will be MISSING from the index\n", name, reason)
		return nil
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
		if name == "Index.json" || name == "Stamps.json" || name == "Teams.json" || name == "Banners.json" || name == "Glossary.json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dataDir, name))
		if err != nil {
			if e := keepPrev(name, "unreadable: "+err.Error()); e != nil {
				entries = append(entries, *e)
			}
			continue
		}
		var d map[string]any
		if err := json.Unmarshal(raw, &d); err != nil {
			if e := keepPrev(name, "invalid JSON: "+err.Error()); e != nil {
				entries = append(entries, *e)
			}
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
