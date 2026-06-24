// Regenerates Index.json (and Glossary.json) for the site. Run with:
//
//	cd "Data Source/curate" && go run .
//
// NOTE: this program no longer merges any curated fields into the per-character
// ../Data/<slug>.json files — those are owned solely by the scraper (which only
// creates JSON for NEW characters). Curate rebuilds Index.json and Glossary.json
// from whatever per-character/per-term JSON already exists.
//
// Teams.json AND Banners.json are hand-maintained: neither is generated or
// modified here (or by the scraper). Edit Data/Teams.json and Data/Banners.json
// directly — data miners own Banners.json entirely. (Banners.json used to be
// regenerated from a hardcoded literal here, which clobbered hand edits on
// every curate run; that has been removed.) Whether a banner in Banners.json
// is shown as live, upcoming, or archived is computed by the site purely from
// its startDate/endDate vs. today — see classifyBanners in Web/Source/Lib/Banner.ts.
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
	// characters). Curate is limited to regenerating Index.json and Glossary.json
	// from whatever per-character/per-term JSON exists.
	//
	// Teams.json and Banners.json are NOT generated here. They are hand-maintained
	// files — edit Data/Teams.json and Data/Banners.json directly. Curate (and the
	// scraper) deliberately leave them untouched; regenerateIndex also skips them.

	// Regenerate Index.json so it picks up releaseDate (and any new chars).
	if err := regenerateIndex(dataDir); err != nil {
		fmt.Printf("  ! index regen: %v\n", err)
	} else {
		fmt.Println("  + Index.json regenerated (with releaseDate)")
	}

	// Aggregate the per-term glossary files into a single Glossary.json the site
	// fetches at runtime. Drop a new Data/Glossary/<Term>.json and it's picked up.
	if n, err := regenerateGlossary(dataDir); err != nil {
		fmt.Printf("  ! glossary regen: %v\n", err)
	} else {
		fmt.Printf("  + Glossary.json regenerated (%d term(s))\n", n)
	}
}

// regenerateGlossary scans dataDir/Glossary/*.json (one file per term:
// {"term": "...", "definition": "..."}) and writes a combined dataDir/Glossary.json
// mapping term -> definition. The filename is the fallback term name. This is the
// glossary the website loads to make in-game terms hoverable in ability text.
func regenerateGlossary(dataDir string) (int, error) {
	glossDir := filepath.Join(dataDir, "Glossary")
	files, err := os.ReadDir(glossDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil // no glossary folder yet — nothing to do
		}
		return 0, err
	}
	out := map[string]string{}
	for _, f := range files {
		if f.IsDir() || filepath.Ext(f.Name()) != ".json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(glossDir, f.Name()))
		if err != nil {
			continue
		}
		var entry struct {
			Term       string `json:"term"`
			Definition string `json:"definition"`
		}
		if err := json.Unmarshal(raw, &entry); err != nil {
			continue
		}
		term := entry.Term
		if term == "" {
			term = f.Name()[:len(f.Name())-len(".json")]
		}
		out[term] = entry.Definition
	}
	buf, err := json.MarshalIndent(out, "", "  ")
	if err != nil {
		return 0, err
	}
	buf = append(buf, '\n')
	return len(out), os.WriteFile(filepath.Join(dataDir, "Glossary.json"), buf, 0644)
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
		if name == "Index.json" || name == "Stamps.json" || name == "Teams.json" || name == "Banners.json" || name == "Glossary.json" {
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
