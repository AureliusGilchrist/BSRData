// Bleach Soul Resonance data generator.
//
// Pulls character data from https://bleachsoulresonance.wiki.gg via the
// MediaWiki API, parses the rendered HTML, downloads images, and writes a
// per-character JSON file under ../Data/.
//
// IMPORTANT: the scraper only ever creates JSON for NEW characters. Any slug
// that already has a ../Data/<slug>.json is skipped entirely on every run, so
// existing per-character data (build/stamp/bond/curated text) is never
// modified. To onboard a new character, add it to slugToEspPage (and
// slugToPage if it has a full English wiki page); the next run scrapes it and
// creates its JSON.
//
// Run from inside the "BSRData/Scraper" folder:
//   go run .
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// backfillAll, when set via -backfill, makes the ESP pass re-run on characters
// that ALREADY have a JSON + images. The run stays strictly additive: image
// downloads skip files that already exist, and mergeEspIntoJSON preserves all
// existing curated text — it only fills in missing skill icons, adds skill
// kinds that were missing (e.g. Gin's Ultimate, Soi Fon's Technique), and
// ensures the Dodge slot exists. Normal runs (no flag) keep the safe default
// of never touching existing characters.
var backfillAll bool

const (
	apiBase   = "https://bleachsoulresonance.wiki.gg/api.php"
	fileBase  = "https://bleachsoulresonance.wiki.gg/wiki/Special:FilePath/"
	cacheDir  = "cache"
	userAgent = "BSR-Datagen/1.0"

	// dodgePlaceholderIcon is a single shared placeholder shown for the "Dodge"
	// ability slot. The game/wiki has no per-character dodge icon, so every
	// character points at this one file until a real icon (and the dodge
	// description, which is filled in by hand) is supplied.
	dodgePlaceholderIcon = "/Images/_shared/skill-dodge.svg"
)

// canonicalKindLabel maps an internal lower-case skill kind to the display
// label used in the JSON (and matched by the per-kind CSS in the web app).
var canonicalKindLabel = map[string]string{
	"basic":     "Basic Attack",
	"technique": "Technique",
	"ultimate":  "Ultimate",
	"counter":   "Counter",
	"dodge":     "Dodge",
}

// canonicalKindOrder is the order skills are written out in.
var canonicalKindOrder = []string{"basic", "technique", "ultimate", "counter", "dodge"}

var (
	dataDir   = filepath.FromSlash("../Data")
	imagesDir = filepath.FromSlash("../Images")
)

// Slugs to wiki page titles. Only characters with a wiki page are listed.
var slugToPage = map[string]string{
	"aizen":         "Sosuke_Aizen",
	"byakuya":       "Byakuya_Kuchiki",
	"ichigo-bankai": "Ichigo_Kurosaki_(Bankai)",
	"momo":          "Momo_Hinamori",
	"toshiro":       "Toshiro_Hitsugaya",
}

// Slugs to Spanish Fandom wiki page titles. Used for every character that
// does NOT have a full English wiki page (24 out of 29). The ESP Fandom hosts
// portrait + skill + boundary + passive icons plus ability text in Spanish.
// Text is translated to English in a separate pass.
var slugToEspPage = map[string]string{
	"aizen":   "Sōsuke Aizen",
	"byakuya": "Kuchiki Byakuya",
	// chad's ESP page is titled plainly "Yasutora Sado" — the "(Chad)" suffix
	// is a 404 (missingtitle), so it never scraped. Corrected.
	"chad":             "Yasutora Sado",
	"gin":              "Ichimaru Gin",
	"grimmjow":         "Grimmjow Jaegerjaques",
	"grimmjow-pantera": "Grimmjow - Resurreción",
	"ichigo-bankai":    "Kurosaki Ichigo (Bankai)",
	"ichigo-shikai":    "Kurosaki Ichigo (Shikai)",
	// The ESP wiki has no "(Initial)" page — the base/initial form lives at the
	// bare "Kurosaki Ichigo" title. Corrected (was a 404).
	"ichigo-initial": "Kurosaki Ichigo",
	"ichigo-white":   "Kurosaki Ichigo (Hollow Interior)",
	"ikkaku":         "Ikkaku Madarame",
	"kenpachi":       "Zaraki Kenpachi",
	"kisuke":         "Urahara Kisuke",
	"komamura":       "Komamura Sajin",
	"ulquiorra": "Ulquiorra Cifer",
	// The Resurrección form's text stays hand-curated in its JSON (the merge is
	// additive and never overwrites populated fields); the ESP wiki gained a
	// dedicated page for it, which we scrape for images. A prior misspelled
	// slug ("ulquiorra-ressurection") produced an empty duplicate stub and has
	// been removed; don't recreate it.
	"ulquiorra-resurreccion": "Ulquiorra Cifer (Resurrección)",
	"mayuri":  "Kurotsuchi Mayuri",
	"momo":    "Hinamori Momo",
	"nelliel": "Nelliel Tu Odelschwanck",
	"nemu":    "Kurotsuchi Nemu",
	"orihime": "Inoue Orihime",
	"rangiku": "Matsumoto Rangiku",
	"renji":   "Abarai Renji",
	// The ESP wiki's only Rukia page is the Shikai form ("Kuchiki Rukia (Shikai)");
	// the bare "Kuchiki Rukia" title 404s. Corrected.
	"rukia":        "Kuchiki Rukia (Shikai)",
	"soi-fon":      "Soi Fon",
	"szayelaporro": "Szayelaporro Granz",
	"tosen":        "Kaname Tōsen",
	"toshiro":      "Tōshirō Hitsugaya",
	"ururu":        "Ururu Tsumugiya",
	"uryu":         "Uryū Ishida",
	"yachiru":      "Yachiru Kusajishi",
	"yoruichi":     "Yoruichi Shihōin",
}

// Universal icon filenames to skip when classifying images on ESP Fandom pages.
// These are role/affiliation/rarity/color/element badges reused across every
// character page and don't represent ability art.
var espUniversalIcons = map[string]bool{
	"Ssr.png": true, "Ssr+.png": true, "Sr.png": true, "Sr+.png": true,
	"R.png": true, "N.png": true,
	"Naranja.png": true, "Amarillo.png": true, "Azul.png": true,
	"Rojo.png": true, "Verde.png": true, "Morado.png": true,
	"Vacio.png": true, "Blanco.png": true,
	"Dpsicon.png": true, "Supporticon.png": true, "Tankicon.png": true,
	"Healericon.png": true,
	"Hollowicon.png": true, "Shiniicon.png": true, "Soulreapericon.png": true,
	"Arrancaricon.png": true, "Quincyicon.png": true, "Fullbringericon.png": true,
	"Nada.png": true,
}

// ---------- types ----------

type apiResp struct {
	Parse struct {
		Title  string   `json:"title"`
		PageID int      `json:"pageid"`
		Text   string   `json:"text"`
		Images []string `json:"images"`
	} `json:"parse"`
}

type Boundary struct {
	Level       int    `json:"level"`
	Name        string `json:"name,omitempty"`
	Description string `json:"description"`
	Icon        string `json:"icon,omitempty"`
}

type Skill struct {
	Name        string `json:"name"`
	Kind        string `json:"kind"`
	Variant     string `json:"variant,omitempty"`
	Description string `json:"description"`
	Icon        string `json:"icon,omitempty"`
}

type Passive struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Icon        string `json:"icon,omitempty"`
}

type Stats struct {
	HP       string `json:"hp,omitempty"`
	ATK      string `json:"atk,omitempty"`
	DEF      string `json:"def,omitempty"`
	CritDmg  string `json:"critDamage,omitempty"`
	CritRate string `json:"critRate,omitempty"`
}

type WikiData struct {
	Rarity      string
	Color       string
	ReleaseDate string
	Limited     string
	Affiliation string
	Role        string
	DamageType  string
	Stats       Stats
	WeaponStats Stats
	Boundaries  []Boundary
	Skills      []Skill
	Passives    []Passive
	ArtFile     string // raw filename, e.g. "Aizen_Art_1.png"
}

// ---------- main ----------

func main() {
	flag.BoolVar(&backfillAll, "backfill", false, "re-run the ESP pass on existing characters to additively backfill missing images/skills")
	flag.Parse()

	mustMk(cacheDir)
	mustMk(imagesDir)

	// Guard against roster drift: warn loudly about any character that the
	// curate step / data already knows about but that has no scraper entry, so
	// brand-new banners/characters can't silently stop being scraped.
	warnMissingScraperEntries()

	// Snapshot which characters already have a JSON BEFORE this run starts. The
	// scraper only creates JSON for NEW characters; any slug already present is
	// skipped so existing per-character data is never modified. Snapshotting up
	// front (instead of an os.Stat per pass) ensures a brand-new character still
	// gets BOTH the English and ESP passes on the same run.
	existing := existingCharacterSlugs()

	slugs := sortedKeys(slugToPage)
	for _, slug := range slugs {
		if existing[slug] {
			log.Printf("[%s] skip — existing character (JSON not modified)", slug)
			continue
		}
		page := slugToPage[slug]
		log.Printf("[%s] new character — processing %s", slug, page)
		if err := processCharacter(slug, page); err != nil {
			log.Printf("[%s] FAIL: %v", slug, err)
			continue
		}
		log.Printf("[%s] OK", slug)
	}

	// Second pass: scrape Spanish Fandom wiki pages for every NEW slug. This is
	// the authoritative source of portrait art for the roster — the English wiki
	// doesn't have art for most characters — plus skill/boundary/passive icons
	// and Spanish ability text. Existing characters are skipped here too.
	for _, slug := range sortedKeys(slugToEspPage) {
		page := slugToEspPage[slug]
		if existing[slug] {
			// JSON already exists — normally we leave it untouched. But re-run
			// the ESP pass (additively) when EITHER the image folder is
			// missing/empty (a prior run's downloads failed after the JSON was
			// committed) OR -backfill was passed to refresh the whole roster.
			// mergeEspIntoJSON preserves all existing curated text.
			if hasImages(slug) && !backfillAll {
				log.Printf("[%s] esp-fandom skip — existing character with images", slug)
				continue
			}
			reason := "missing images"
			if backfillAll {
				reason = "-backfill"
			}
			log.Printf("[%s] esp-fandom backfill (%s) — re-fetching from %s", slug, reason, page)
			if err := processEspFandomCharacter(slug, page); err != nil {
				log.Printf("[%s] esp-fandom backfill FAIL: %v", slug, err)
			}
			continue
		}
		log.Printf("[%s] esp-fandom new character %s", slug, page)
		if err := processEspFandomCharacter(slug, page); err != nil {
			log.Printf("[%s] esp-fandom FAIL: %v", slug, err)
		}
	}

	if err := writeIndex(); err != nil {
		log.Fatalf("write index: %v", err)
	}
	log.Printf("done")
}

// existingCharacterSlugs returns the set of slugs that already have a
// ../Data/<slug>.json file (excluding the aggregate files). The scraper uses
// this to skip characters that already exist: only BRAND-NEW characters get a
// JSON created, and existing per-character JSON is never rewritten.
func existingCharacterSlugs() map[string]bool {
	out := map[string]bool{}
	files, err := os.ReadDir(dataDir)
	if err != nil {
		return out
	}
	skip := map[string]bool{
		"Index.json": true, "Stamps.json": true,
		"Teams.json": true, "Banners.json": true, "Glossary.json": true,
	}
	for _, f := range files {
		name := f.Name()
		if f.IsDir() || filepath.Ext(name) != ".json" || skip[name] {
			continue
		}
		out[strings.TrimSuffix(name, ".json")] = true
	}
	return out
}

// hasImages reports whether ../Images/<slug>/ exists and contains at least one
// file. Used to detect characters whose JSON was committed but whose image
// download failed/was skipped at the time (e.g. a transient fetch error), so
// the ESP-fandom pass can backfill their images without touching curated text.
func hasImages(slug string) bool {
	entries, err := os.ReadDir(filepath.Join(imagesDir, slug))
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			return true
		}
	}
	return false
}

// warnMissingScraperEntries cross-checks the roster the rest of the pipeline
// already knows about (per-character JSON files in ../Data and the slugs that
// appear in Banners.json) against the scraper's own slugToEspPage / slugToPage
// maps. Any slug that has data or a banner reference but no scraper entry is
// reported so a freshly-added character/banner can't silently fall out of the
// scrape. It only logs — it never edits maps or fails the run.
func warnMissingScraperEntries() {
	known := func(slug string) bool {
		if _, ok := slugToEspPage[slug]; ok {
			return true
		}
		_, ok := slugToPage[slug]
		return ok
	}

	missing := map[string]string{} // slug -> where we saw it

	// 1) Every Data/<slug>.json (skip the aggregate files).
	if files, err := os.ReadDir(dataDir); err == nil {
		skip := map[string]bool{
			"Index.json": true, "Stamps.json": true,
			"Teams.json": true, "Banners.json": true, "Glossary.json": true,
		}
		for _, f := range files {
			name := f.Name()
			if f.IsDir() || filepath.Ext(name) != ".json" || skip[name] {
				continue
			}
			slug := strings.TrimSuffix(name, ".json")
			if !known(slug) {
				missing[slug] = "Data/" + name
			}
		}
	}

	// 2) Every slug referenced by the current + upcoming banners.
	if raw, err := os.ReadFile(filepath.Join(dataDir, "Banners.json")); err == nil {
		var bd struct {
			Current *struct {
				Slugs []string `json:"slugs"`
			} `json:"current"`
			Upcoming []struct {
				Slugs []string `json:"slugs"`
			} `json:"upcoming"`
		}
		if json.Unmarshal(raw, &bd) == nil {
			collect := func(slugs []string) {
				for _, s := range slugs {
					s = strings.TrimSpace(s)
					if s != "" && !known(s) {
						if _, seen := missing[s]; !seen {
							missing[s] = "Banners.json"
						}
					}
				}
			}
			if bd.Current != nil {
				collect(bd.Current.Slugs)
			}
			for _, b := range bd.Upcoming {
				collect(b.Slugs)
			}
		}
	}

	if len(missing) == 0 {
		return
	}
	for _, slug := range sortedKeys(missing) {
		log.Printf("WARNING: slug %q (referenced by %s) has no scraper entry — add it to slugToEspPage to scrape it",
			slug, missing[slug])
	}
}

// processRawURLArt downloads a portrait from any direct image URL and writes
// its web path into the slug's JSON `art` field. Used for characters whose
// art lives outside the primary wiki (e.g. Fandom CDN).
func processRawURLArt(slug, rawURL string) error {
	// Derive a sensible extension. Fandom URLs look like
	//   .../File.png/revision/latest?cb=...  → use the ".png" segment.
	ext := ".png"
	if i := strings.Index(rawURL, "."); i > 0 {
		tail := rawURL[i:]
		for _, candidate := range []string{".png", ".jpg", ".jpeg", ".webp", ".gif"} {
			if strings.Contains(strings.ToLower(tail), candidate) {
				ext = candidate
				break
			}
		}
	}
	local := filepath.Join(imagesDir, slug, "art"+ext)
	if err := downloadRawURL(rawURL, local); err != nil {
		return err
	}
	webPath := toWebPath(local)

	jsonPath := filepath.Join(dataDir, slug+".json")
	doc := map[string]interface{}{}
	if data, err := os.ReadFile(jsonPath); err == nil {
		_ = json.Unmarshal(data, &doc)
	}
	if doc["slug"] == nil {
		doc["slug"] = slug
	}
	doc["art"] = webPath
	for _, k := range []string{"skills", "boundaries", "corePassives", "builds", "bondPriority"} {
		if _, ok := doc[k]; !ok {
			doc[k] = []interface{}{}
		}
	}
	if _, ok := doc["stamps"]; !ok {
		doc["stamps"] = map[string]interface{}{
			"weapon": map[string]interface{}{"ascend1": []interface{}{}, "ascend5": []interface{}{}},
			"core":   map[string]interface{}{"ascend1": []interface{}{}, "ascend5": []interface{}{}},
		}
	}
	return writeJSON(jsonPath, doc)
}

// downloadRawURL fetches any image URL (Fandom CDN, etc.) with retries.
func downloadRawURL(rawURL, localPath string) error {
	if _, err := os.Stat(localPath); err == nil {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}
	client := &http.Client{Timeout: 60 * time.Second}
	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 1500 * time.Millisecond)
		}
		req, _ := http.NewRequest("GET", rawURL, nil)
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
		req.Header.Set("Referer", "https://bleach-soul-resonance-esp.fandom.com/")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == 200 {
			f, err := os.Create(localPath)
			if err != nil {
				resp.Body.Close()
				return err
			}
			_, copyErr := io.Copy(f, resp.Body)
			f.Close()
			resp.Body.Close()
			if copyErr != nil {
				return copyErr
			}
			time.Sleep(150 * time.Millisecond)
			return nil
		}
		lastErr = fmt.Errorf("HTTP %d for %s", resp.StatusCode, rawURL)
		resp.Body.Close()
	}
	return lastErr
}

// fetchEspWikitext retrieves the raw wikitext (template source) of an ESP
// Fandom page. Used to read template parameters like "Release Date" that get
// flattened away in the rendered HTML. This deliberately does NOT cache —
// the wikitext payload is tiny and we want the scraper to pick up new banner
// dates on every scheduled re-run.
func fetchEspWikitext(page string) (string, error) {
	u := fmt.Sprintf("%s?action=parse&page=%s&format=json&prop=wikitext&formatversion=2&redirects=1",
		espApiBase, url.QueryEscape(page))
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", userAgent)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	// formatversion=2 returns wikitext as a plain string instead of {"*": "..."}.
	var r struct {
		Parse struct {
			Wikitext string `json:"wikitext"`
		} `json:"parse"`
	}
	if err := json.Unmarshal(body, &r); err != nil {
		return "", err
	}
	return r.Parse.Wikitext, nil
}

// reEspReleaseDate captures the entire "Release Date=...|" parameter value
// from the {{Tarjeta de personaje}} template wikitext. The value typically
// looks like "·China: 14/5/2026<br>·Global: 15/5/2026".
var reEspReleaseDate = regexp.MustCompile(`(?i)\|\s*Release\s*Date\s*=\s*([^|]*)`)

// reEspGlobalDate captures the Global server date in DD/MM/YYYY (or D/M/YYYY)
// from the Release Date field. Falls back to any DD/MM/YYYY-looking date
// elsewhere in the value when no explicit "Global:" label is present.
var (
	reEspGlobalDate = regexp.MustCompile(`(?i)Global[^\d]*(\d{1,2})/(\d{1,2})/(\d{4})`)
	reEspAnyDate    = regexp.MustCompile(`(\d{1,2})/(\d{1,2})/(\d{4})`)
)

// monthAbbr maps a 1-based month number to the three-letter English abbrev
// used by parseReleaseDate() in Web/Source/Lib/Types.ts.
var monthAbbr = [...]string{"", "Jan", "Feb", "Mar", "Apr", "May", "Jun", "Jul", "Aug", "Sep", "Oct", "Nov", "Dec"}

// extractEspReleaseDate parses a wikitext blob, finds the Release Date field
// of the Tarjeta de personaje template, prefers the Global date, and returns
// it normalised to "D MMM YYYY" (e.g. "15 May 2026"). Returns "" if nothing
// usable was found.
func extractEspReleaseDate(wikitext string) string {
	m := reEspReleaseDate.FindStringSubmatch(wikitext)
	if len(m) < 2 {
		return ""
	}
	value := m[1]
	var d, mo, y string
	if gm := reEspGlobalDate.FindStringSubmatch(value); len(gm) == 4 {
		d, mo, y = gm[1], gm[2], gm[3]
	} else if am := reEspAnyDate.FindStringSubmatch(value); len(am) == 4 {
		d, mo, y = am[1], am[2], am[3]
	} else {
		return ""
	}
	moIdx := atoiSafe(mo)
	if moIdx < 1 || moIdx > 12 {
		return ""
	}
	day := atoiSafe(d)
	if day < 1 || day > 31 {
		return ""
	}
	return fmt.Sprintf("%d %s %s", day, monthAbbr[moIdx], y)
}

func atoiSafe(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
}

// ---------- ESP Fandom scraper ----------

const espApiBase = "https://bleach-soul-resonance-esp.fandom.com/es/api.php"

// EspImage is one image found on an ESP Fandom page.
type EspImage struct {
	Filename string // e.g. "Dupe1grimh.png"
	URL      string // absolute static.wikia.nocookie.net URL
}

// EspPage is the result of parsing a Spanish Fandom character page.
type EspPage struct {
	Portrait   EspImage
	Skills     []EspSkill       // ordered: basic, technique, ultimate, counter, bfs
	Boundaries map[int]EspImage // level 1..6
	Passives   []EspImage       // in passive-table order
	// EVERY icon the page has per multi-variant kind, in document order —
	// several kits carry enhanced/second-stage variants beyond the primary
	// icon. Saved as skill-<kind>-<N>.<ext> alongside the primary file.
	TechniqueAll   []EspImage
	UltimateAll    []EspImage
	BattlefieldAll []EspImage
	// Raw Spanish ability text keyed by category for later translation.
	// Keys: "basic", "technique", "ultimate", "counter", "passive-1".. "boundary-1"..
	SpanishText map[string]string
}

// EspSkill pairs a skill kind with its icon and Spanish description.
type EspSkill struct {
	Kind          string // "basic" | "technique" | "ultimate" | "counter"
	Icon          EspImage
	DescriptionES string
}

// fetchEspFandom retrieves and caches the parsed HTML of an ESP Fandom page.
func fetchEspFandom(page string) (string, error) {
	cp := filepath.Join(cacheDir, "esp_"+strings.NewReplacer("(", "", ")", "", " ", "_", "/", "_").Replace(page)+".json")
	if data, err := os.ReadFile(cp); err == nil {
		var r apiResp
		if err := json.Unmarshal(data, &r); err == nil && r.Parse.Text != "" {
			return r.Parse.Text, nil
		}
	}
	u := fmt.Sprintf("%s?action=parse&page=%s&format=json&prop=text&formatversion=2&redirects=1",
		espApiBase, url.QueryEscape(page))
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", userAgent)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	var r apiResp
	if err := json.Unmarshal(body, &r); err != nil {
		return "", err
	}
	if r.Parse.Text == "" {
		return "", fmt.Errorf("empty parse result")
	}
	// Only cache successful, non-empty responses.
	_ = os.WriteFile(cp, body, 0644)
	return r.Parse.Text, nil
}

// Regexes for ESP image extraction & classification.
var (
	reEspImg      = regexp.MustCompile(`(?i)<img[^>]+(?:data-src|src)="(https://static\.wikia\.nocookie\.net/[^"]+)"[^>]*>`)
	reEspFileName = regexp.MustCompile(`/images/[0-9a-f]/[0-9a-f]{2}/([^/?]+\.(?:png|jpg|jpeg|webp|gif))`)
	reEspDupe     = regexp.MustCompile(`(?i)^Dupe\D*(\d)\D*\.(?:png|jpg|jpeg|webp)$`)
	// Basic attack: "Basico*" is the norm, but some pages drop the "co"
	// (Yachiru's is "Basiyachi.png"), so match the looser "Basi" prefix.
	reEspBasico = regexp.MustCompile(`(?i)^Basi\w*\.(?:png|jpg|jpeg|webp)$`)
	// A few characters name their basic attack with the generic "Skill" prefix
	// plus a "bas" marker (Rukia's "Skillrbas.png", Ichigo Shikai's
	// "Skillibas.png"). Detect those so they aren't swept up as techniques.
	reEspSkillBasic = regexp.MustCompile(`(?i)^Skill[\w-]*bas[\w-]*\.(?:png|jpg|jpeg|webp)$`)
	// Technique icons appear under several spellings on the ESP wiki:
	//   "Tecnica*", the typo "Tenica*", abbreviations "Tec*"/"Teq*", and the
	//   generic "Skill*" naming. The bare "Tec" prefix is what was missing —
	//   e.g. Soi Fon's technique is "Tecsoi.png", which never matched before
	//   and so her Technique was silently dropped. `[\w-]` (not just `\w`)
	//   because enhanced variants use hyphenated names: "Skilltoshi2-2.png".
	reEspTecnica   = regexp.MustCompile(`(?i)^(?:Tecnica|Tenica|Tec|Teq|Skill)[\w-]*\.(?:png|jpg|jpeg|webp)$`)
	reEspUlti      = regexp.MustCompile(`(?i)^Ulti(?:mate)?[\w-]*\.(?:png|jpg|jpeg|webp)$`)
	// Battlefield Skill icons: "Campo" (field) names — Campkoma2, Camptose1,
	// Fieldsoi — plus camp-infixed Skill* names (Skillrcamp1, Skillcamishi),
	// which must be checked BEFORE the broad technique pattern that also
	// matches anything starting with "Skill".
	reEspCampo      = regexp.MustCompile(`(?i)^(?:Camp|Field)[\w-]*\.(?:png|jpg|jpeg|webp)$`)
	reEspSkillCampo = regexp.MustCompile(`(?i)^Skill[\w-]*?cam[\w-]*\.(?:png|jpg|jpeg|webp)$`)
	reEspCounter   = regexp.MustCompile(`(?i)^(?:Counter|Contra)[\w-]*\.(?:png|jpg|jpeg|webp)$`)
	reEspPasiva    = regexp.MustCompile(`(?i)^Pasiv?[ac]\w*\.(?:png|jpg|jpeg|webp)$`)
	reEspNada      = regexp.MustCompile(`(?i)^Nada\.(?:png|jpg|jpeg|webp)$`)
	reEspIconArma  = regexp.MustCompile(`(?i)^Iconarma\w*\.(?:png|jpg|jpeg|webp)$`)
	reEspIconNuc   = regexp.MustCompile(`(?i)^Iconnucleo\w*\.(?:png|jpg|jpeg|webp)$`)
	reEspIconSuper = regexp.MustCompile(`(?i)^Iconsuper\d+\.(?:png|jpg|jpeg|webp)$`)
	reEspIconSet   = regexp.MustCompile(`(?i)^(?:Iconset|Iconesta)\w*\.(?:png|jpg|jpeg|webp)$`)
	reEspTCard     = regexp.MustCompile(`(?i)^T[A-Z]\w+\.(?:png|jpg|jpeg|webp)$`)
)

// classifyEspImage returns one of: "portrait", "boundary-N", "skill-basic",
// "skill-technique", "skill-ultimate", "skill-counter", "skill-bfs",
// "passive", or "" to skip.
func classifyEspImage(filename string, alreadySeenPortrait bool) string {
	if espUniversalIcons[filename] {
		return ""
	}
	if reEspNada.MatchString(filename) {
		return ""
	}
	if m := reEspDupe.FindStringSubmatch(filename); m != nil {
		return "boundary-" + m[1]
	}
	// Basic must be checked before technique: "Skill*bas*" names match the
	// broad technique pattern too, but they're really the basic attack.
	if reEspBasico.MatchString(filename) || reEspSkillBasic.MatchString(filename) {
		return "skill-basic"
	}
	// Battlefield before technique for the same reason ("Skillrcamp1" etc.).
	if reEspCampo.MatchString(filename) || reEspSkillCampo.MatchString(filename) {
		return "skill-bfs"
	}
	if reEspTecnica.MatchString(filename) {
		return "skill-technique"
	}
	if reEspUlti.MatchString(filename) {
		return "skill-ultimate"
	}
	if reEspCounter.MatchString(filename) {
		return "skill-counter"
	}
	if reEspPasiva.MatchString(filename) {
		return "passive"
	}
	if reEspIconArma.MatchString(filename) || reEspIconNuc.MatchString(filename) ||
		reEspIconSuper.MatchString(filename) || reEspIconSet.MatchString(filename) ||
		reEspTCard.MatchString(filename) {
		return ""
	}
	if !alreadySeenPortrait {
		return "portrait"
	}
	return ""
}

// parseEspFandomPage walks the page HTML in document order, classifying every
// CDN image and grouping by category.
func parseEspFandomPage(htmlStr string) EspPage {
	out := EspPage{
		Boundaries:  map[int]EspImage{},
		SpanishText: map[string]string{},
	}
	seenFiles := map[string]bool{}
	portraitSet := false
	skillSlotTaken := map[string]bool{}
	// Technique candidates are collected in document order rather than taking
	// only the first match. Several characters (e.g. Gin, Rangiku) have NO
	// dedicated "Ulti*" icon — their ultimate is just the second numbered
	// "Skill*" icon (Skillgin1 = technique, Skillgin2 = ultimate). We resolve
	// those after the scan: first candidate → technique, and if the page has
	// no explicit ultimate, the second candidate is promoted to ultimate.
	var techCands []EspImage

	for _, m := range reEspImg.FindAllStringSubmatch(htmlStr, -1) {
		// HTML attribute values come back with entities escaped (&amp;) which
		// breaks query params like ?cb=...&path-prefix=es. Decode first.
		srcURL := html.UnescapeString(m[1])
		fn := ""
		if fm := reEspFileName.FindStringSubmatch(srcURL); fm != nil {
			fn = fm[1]
		} else {
			continue
		}
		if seenFiles[fn] {
			continue
		}
		seenFiles[fn] = true

		cat := classifyEspImage(fn, portraitSet)
		if cat == "" {
			continue
		}
		img := EspImage{Filename: fn, URL: srcURL}

		switch {
		case cat == "portrait":
			out.Portrait = img
			portraitSet = true
		case strings.HasPrefix(cat, "boundary-"):
			lvl := atoi(strings.TrimPrefix(cat, "boundary-"))
			if lvl >= 1 && lvl <= 6 {
				if _, exists := out.Boundaries[lvl]; !exists {
					out.Boundaries[lvl] = img
				}
			}
		case strings.HasPrefix(cat, "skill-"):
			kind := strings.TrimPrefix(cat, "skill-")
			// Defer technique resolution — see techCands above.
			if kind == "technique" {
				techCands = append(techCands, img)
				continue
			}
			// Multi-variant kinds keep EVERY icon (numbered files), not just
			// the first one that wins the primary slot below.
			if kind == "ultimate" {
				out.UltimateAll = append(out.UltimateAll, img)
			}
			if kind == "bfs" {
				out.BattlefieldAll = append(out.BattlefieldAll, img)
			}
			if skillSlotTaken[kind] {
				continue
			}
			skillSlotTaken[kind] = true
			out.Skills = append(out.Skills, EspSkill{Kind: kind, Icon: img})
		case cat == "passive":
			out.Passives = append(out.Passives, img)
		}
	}

	// Resolve technique / ultimate from the ordered technique candidates.
	if len(techCands) > 0 {
		out.Skills = append(out.Skills, EspSkill{Kind: "technique", Icon: techCands[0]})
		skillSlotTaken["technique"] = true
		rest := techCands[1:]
		// No explicit "Ulti*" icon but a second numbered skill exists → that
		// second icon is the character's ultimate.
		if !skillSlotTaken["ultimate"] && len(techCands) >= 2 {
			out.Skills = append(out.Skills, EspSkill{Kind: "ultimate", Icon: techCands[1]})
			skillSlotTaken["ultimate"] = true
			out.UltimateAll = append(out.UltimateAll, techCands[1])
			rest = techCands[2:]
		}
		out.TechniqueAll = append([]EspImage{techCands[0]}, rest...)
	}

	// Order skills consistently: basic, technique, ultimate, counter, bfs.
	order := map[string]int{"basic": 0, "technique": 1, "ultimate": 2, "counter": 3, "bfs": 4}
	sort.SliceStable(out.Skills, func(i, j int) bool {
		return order[out.Skills[i].Kind] < order[out.Skills[j].Kind]
	})

	// Extract Spanish ability text per section by looking for known table cell
	// labels. The Tarjeta de Personaje template uses these exact labels.
	out.SpanishText["basic"] = extractEspCellText(htmlStr, "Ataque básico")
	out.SpanishText["technique"] = extractEspCellText(htmlStr, "Técnica")
	out.SpanishText["ultimate"] = extractEspCellText(htmlStr, "Ultimate")
	out.SpanishText["counter"] = extractEspCellText(htmlStr, "Contraataque")
	// Dupes 1-6.
	for i := 1; i <= 6; i++ {
		txt := extractEspCellText(htmlStr, fmt.Sprintf("Dupe %d", i))
		if txt != "" {
			out.SpanishText[fmt.Sprintf("boundary-%d", i)] = txt
		}
	}
	return out
}

// extractEspCellText finds a `<td>LABEL</td>` (or `<th>LABEL</th>`) cell and
// returns the plaintext content of the immediately following `<td>` cell.
// Used for parsing the "Tarjeta de personaje" key-value tables.
func extractEspCellText(htmlStr, label string) string {
	// Escape regex meta chars in label, allow whitespace flexibility.
	escLabel := regexp.QuoteMeta(label)
	re := regexp.MustCompile(`(?si)<t[dh][^>]*>\s*` + escLabel + `[\s:]*</t[dh]>\s*<td[^>]*>(.*?)</td>`)
	if m := re.FindStringSubmatch(htmlStr); m != nil {
		return stripHTML(m[1])
	}
	return ""
}

// processEspFandomCharacter scrapes one ESP page, downloads all classified
// images into Web/Public/Images/<slug>/, and merges the image paths plus raw
// Spanish ability text into the slug's JSON (without overwriting existing
// populated English fields).
func processEspFandomCharacter(slug, page string) error {
	htmlStr, err := fetchEspFandom(page)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	ep := parseEspFandomPage(htmlStr)

	// Pull the Global release date out of the template wikitext. Best-effort —
	// if it fails (network blip, page schema change) we leave whatever existing
	// value is already in the JSON untouched.
	releaseDate := ""
	if wt, err := fetchEspWikitext(page); err == nil {
		releaseDate = extractEspReleaseDate(wt)
	} else {
		log.Printf("[%s] esp wikitext failed: %v", slug, err)
	}

	// Download portrait → art.{ext} (only if slug folder doesn't already have art).
	artWebPath := ""
	if ep.Portrait.URL != "" {
		ext := filepath.Ext(ep.Portrait.Filename)
		if ext == "" {
			ext = ".png"
		}
		local := filepath.Join(imagesDir, slug, "art"+ext)
		if err := downloadRawURL(ep.Portrait.URL, local); err != nil {
			log.Printf("[%s] esp art download failed: %v", slug, err)
		} else {
			artWebPath = toWebPath(local)
		}
	}

	// Download boundary icons.
	boundaryPaths := map[int]string{}
	for lvl := 1; lvl <= 6; lvl++ {
		img, ok := ep.Boundaries[lvl]
		if !ok || img.URL == "" {
			continue
		}
		ext := filepath.Ext(img.Filename)
		if ext == "" {
			ext = ".png"
		}
		local := filepath.Join(imagesDir, slug, fmt.Sprintf("boundary-%d%s", lvl, ext))
		if err := downloadRawURL(img.URL, local); err != nil {
			log.Printf("[%s] esp boundary-%d failed: %v", slug, lvl, err)
			continue
		}
		boundaryPaths[lvl] = toWebPath(local)
	}

	// Download skill icons.
	skillPaths := map[string]string{}
	for _, s := range ep.Skills {
		if s.Icon.URL == "" {
			continue
		}
		ext := filepath.Ext(s.Icon.Filename)
		if ext == "" {
			ext = ".png"
		}
		local := filepath.Join(imagesDir, slug, fmt.Sprintf("skill-%s%s", s.Kind, ext))
		if err := downloadRawURL(s.Icon.URL, local); err != nil {
			log.Printf("[%s] esp skill-%s failed: %v", slug, s.Kind, err)
			continue
		}
		skillPaths[s.Kind] = toWebPath(local)
	}

	// Additionally save every EXTRA technique / ultimate / battlefield icon
	// the page has — several kits carry enhanced or second-stage variants
	// beyond the primary — under numbered names: skill-technique-2.png,
	// skill-ultimate-2.png, skill-bfs-3.png, ... The FIRST icon of each kind
	// is the primary (already saved as skill-<kind>.png), so numbering starts
	// at 2 instead of duplicating it as a "-1" file.
	variantSets := []struct {
		kind string
		imgs []EspImage
	}{
		{"technique", ep.TechniqueAll},
		{"ultimate", ep.UltimateAll},
		{"bfs", ep.BattlefieldAll},
	}
	for _, v := range variantSets {
		for i, img := range v.imgs {
			if i == 0 || img.URL == "" {
				continue
			}
			ext := filepath.Ext(img.Filename)
			if ext == "" {
				ext = ".png"
			}
			local := filepath.Join(imagesDir, slug, fmt.Sprintf("skill-%s-%d%s", v.kind, i+1, ext))
			if err := downloadRawURL(img.URL, local); err != nil {
				log.Printf("[%s] esp skill-%s-%d failed: %v", slug, v.kind, i+1, err)
			}
		}
	}

	// Download passive icons.
	passivePaths := []string{}
	for i, p := range ep.Passives {
		if p.URL == "" {
			continue
		}
		ext := filepath.Ext(p.Filename)
		if ext == "" {
			ext = ".png"
		}
		local := filepath.Join(imagesDir, slug, fmt.Sprintf("passive-%d%s", i+1, ext))
		if err := downloadRawURL(p.URL, local); err != nil {
			log.Printf("[%s] esp passive-%d failed: %v", slug, i+1, err)
			continue
		}
		passivePaths = append(passivePaths, toWebPath(local))
	}

	return mergeEspIntoJSON(slug, page, artWebPath, boundaryPaths, skillPaths, passivePaths, ep.SpanishText, releaseDate)
}

// mergeEspIntoJSON merges ESP Fandom data into the slug's JSON. Existing
// populated English fields are preserved. Image paths are added where missing
// or where existing paths point to nothing. Raw Spanish text is stored under
// a `_spanish` sub-object for a follow-up translation pass.
func mergeEspIntoJSON(slug, page, artPath string, boundaryPaths map[int]string, skillPaths map[string]string, passivePaths []string, spanishText map[string]string, releaseDate string) error {
	jsonPath := filepath.Join(dataDir, slug+".json")
	doc := map[string]interface{}{}
	if data, err := os.ReadFile(jsonPath); err == nil {
		_ = json.Unmarshal(data, &doc)
	}
	if doc["slug"] == nil {
		doc["slug"] = slug
	}

	// Art: ESP Fandom is the authoritative source for portrait art (it's the
	// only wiki that has BSR card art for the full roster). Always override.
	if artPath != "" {
		doc["art"] = artPath
	}

	// Release date: the Spanish wiki's Global field is the authoritative source.
	// Overwrite whatever may have been written by an earlier (English-wiki)
	// pass so the roster sort always uses the same normalised "D MMM YYYY".
	if releaseDate != "" {
		doc["releaseDate"] = releaseDate
	}

	// Boundaries: build/merge list. Preserve existing English text where present.
	existingBoundaries := map[int]map[string]interface{}{}
	if old, ok := doc["boundaries"].([]interface{}); ok {
		for _, b := range old {
			if bm, ok := b.(map[string]interface{}); ok {
				if lv, ok := bm["level"].(float64); ok {
					existingBoundaries[int(lv)] = bm
				}
			}
		}
	}
	newBoundaries := []map[string]interface{}{}
	for lvl := 1; lvl <= 6; lvl++ {
		bm := existingBoundaries[lvl]
		if bm == nil {
			bm = map[string]interface{}{"level": lvl}
		}
		// Add icon if missing.
		if cur, _ := bm["icon"].(string); cur == "" {
			if p, ok := boundaryPaths[lvl]; ok {
				bm["icon"] = p
			}
		}
		// Add Spanish text under _spanish key for translation later. Don't
		// overwrite description if already populated.
		key := fmt.Sprintf("boundary-%d", lvl)
		if es := spanishText[key]; es != "" {
			if cur, _ := bm["description"].(string); cur == "" {
				if _, exists := bm["descriptionES"]; !exists {
					bm["descriptionES"] = es
				}
			}
		}
		// Only include in output if it has at least an icon or description.
		if bm["icon"] != nil || bm["description"] != nil || bm["descriptionES"] != nil || existingBoundaries[lvl] != nil {
			newBoundaries = append(newBoundaries, bm)
		}
	}
	if len(newBoundaries) > 0 {
		// Convert to []interface{} for JSON output.
		out := make([]interface{}, len(newBoundaries))
		for i, b := range newBoundaries {
			out[i] = b
		}
		doc["boundaries"] = out
	}

	// Skills: merge by kind. Existing curated skills (name/description/variant)
	// are preserved; we only fill in missing icons and add missing kinds.
	// Any existing skill whose kind is OUTSIDE the canonical set (e.g.
	// "Battlefield Skill") is carried through untouched in extraSkills so it's
	// never dropped.
	kindToSkill := map[string]map[string]interface{}{}
	extraSkills := []interface{}{}
	if old, ok := doc["skills"].([]interface{}); ok {
		for _, s := range old {
			sm, ok := s.(map[string]interface{})
			if !ok {
				continue
			}
			k, _ := sm["kind"].(string)
			k = strings.ToLower(k)
			// Normalize various existing kind labels to our canonical set.
			switch k {
			case "basic attack", "basic":
				k = "basic"
			case "skill", "technique":
				k = "technique"
			case "ultimate":
				k = "ultimate"
			case "counter", "counterattack":
				k = "counter"
			case "dodge", "esquiva", "esquivar":
				k = "dodge"
			}
			if _, isCanonical := canonicalKindLabel[k]; isCanonical {
				kindToSkill[k] = sm
			} else {
				// Unknown/extra kind (Battlefield Skill, etc.) — preserve as-is.
				extraSkills = append(extraSkills, sm)
			}
		}
	}
	for _, kind := range canonicalKindOrder {
		sm := kindToSkill[kind]
		if sm == nil {
			sm = map[string]interface{}{"name": "", "description": ""}
		}
		// Canonicalise the display label / casing.
		sm["kind"] = canonicalKindLabel[kind]
		if iconPath, ok := skillPaths[kind]; ok {
			if cur, _ := sm["icon"].(string); cur == "" {
				sm["icon"] = iconPath
			}
		}
		// The Dodge slot has no scraped icon — point it at the shared
		// placeholder so the card renders. Its text is filled in by hand later.
		if kind == "dodge" {
			if cur, _ := sm["icon"].(string); cur == "" {
				sm["icon"] = dodgePlaceholderIcon
			}
		}
		if es := spanishText[kind]; es != "" {
			if cur, _ := sm["description"].(string); cur == "" {
				if _, exists := sm["descriptionES"]; !exists {
					sm["descriptionES"] = es
				}
			}
		}
		kindToSkill[kind] = sm
	}
	// Battlefield Skill icon: fill it in on the existing curated entry if that
	// entry has no icon yet, or add a stub entry when the wiki page has a
	// battlefield icon and the JSON has no such skill at all. (Battlefield
	// Skill deliberately isn't a canonical kind — curated entries flow through
	// extraSkills untouched — so the icon merge happens here.)
	if bfsIcon, ok := skillPaths["bfs"]; ok && bfsIcon != "" {
		found := false
		for _, e := range extraSkills {
			if em, ok := e.(map[string]interface{}); ok {
				if k, _ := em["kind"].(string); strings.EqualFold(k, "battlefield skill") || strings.EqualFold(k, "bfs") {
					if cur, _ := em["icon"].(string); cur == "" {
						em["icon"] = bfsIcon
					}
					found = true
				}
			}
		}
		if !found {
			extraSkills = append(extraSkills, map[string]interface{}{
				"kind":        "Battlefield Skill",
				"name":        "",
				"description": "",
				"icon":        bfsIcon,
			})
		}
	}

	// Output order: basic, technique, ultimate, counter, [extras e.g.
	// Battlefield Skill], dodge. The Dodge slot is always emitted (it carries
	// the placeholder icon); the other canonical kinds only when they have an
	// icon/description. Extras are preserved verbatim.
	skillsOut := []interface{}{}
	emit := func(kind string) {
		sm, ok := kindToSkill[kind]
		if !ok {
			return
		}
		hasIcon := sm["icon"] != nil && sm["icon"] != ""
		hasDesc := (sm["description"] != nil && sm["description"] != "") || sm["descriptionES"] != nil
		if kind == "dodge" || hasIcon || hasDesc {
			skillsOut = append(skillsOut, sm)
		}
	}
	emit("basic")
	emit("technique")
	emit("ultimate")
	emit("counter")
	skillsOut = append(skillsOut, extraSkills...)
	emit("dodge")
	if len(skillsOut) > 0 {
		doc["skills"] = skillsOut
	}

	// Core passives: existing entries get icons filled in by index.
	existingPassives := []interface{}{}
	if old, ok := doc["corePassives"].([]interface{}); ok {
		existingPassives = old
	}
	for i, p := range passivePaths {
		if i < len(existingPassives) {
			if pm, ok := existingPassives[i].(map[string]interface{}); ok {
				if cur, _ := pm["icon"].(string); cur == "" {
					pm["icon"] = p
				}
				existingPassives[i] = pm
				continue
			}
		}
		existingPassives = append(existingPassives, map[string]interface{}{
			"name":        "",
			"description": "",
			"icon":        p,
		})
	}
	if len(existingPassives) > 0 {
		doc["corePassives"] = existingPassives
	}

	// Stash raw Spanish text for future translation.
	doc["_spanishSource"] = "https://bleach-soul-resonance-esp.fandom.com/es/wiki/" + url.QueryEscape(strings.ReplaceAll(page, " ", "_"))
	if len(spanishText) > 0 {
		doc["_spanishText"] = spanishText
	}

	// Schema defaults.
	for _, k := range []string{"skills", "boundaries", "corePassives", "builds", "bondPriority"} {
		if _, ok := doc[k]; !ok {
			doc[k] = []interface{}{}
		}
	}
	if _, ok := doc["stamps"]; !ok {
		doc["stamps"] = map[string]interface{}{
			"weapon": map[string]interface{}{"ascend1": []interface{}{}, "ascend5": []interface{}{}},
			"core":   map[string]interface{}{"ascend1": []interface{}{}, "ascend5": []interface{}{}},
		}
	}

	return writeJSON(jsonPath, doc)
}

func processCharacter(slug, page string) error {
	html, err := fetchWiki(page)
	if err != nil {
		return fmt.Errorf("fetch: %w", err)
	}
	wd := parseWiki(html)

	// Download art image.
	artPath := ""
	if wd.ArtFile != "" {
		ext := filepath.Ext(wd.ArtFile)
		if ext == "" {
			ext = ".png"
		}
		local := filepath.Join(imagesDir, slug, "art"+ext)
		if err := downloadImage(wd.ArtFile, local); err != nil {
			log.Printf("[%s] art download failed: %v", slug, err)
		} else {
			artPath = toWebPath(local)
		}
	}

	// Download boundary icons.
	for i, b := range wd.Boundaries {
		if b.Icon == "" {
			continue
		}
		local := filepath.Join(imagesDir, slug, fmt.Sprintf("boundary-%d%s", b.Level, filepath.Ext(b.Icon)))
		if err := downloadImage(b.Icon, local); err != nil {
			log.Printf("[%s] boundary icon B%d failed: %v", slug, b.Level, err)
			continue
		}
		wd.Boundaries[i].Icon = toWebPath(local)
	}

	// Download skill icons.
	for i, s := range wd.Skills {
		if s.Icon == "" {
			continue
		}
		local := filepath.Join(imagesDir, slug, fmt.Sprintf("skill-%d%s", i+1, filepath.Ext(s.Icon)))
		if err := downloadImage(s.Icon, local); err != nil {
			log.Printf("[%s] skill icon %d failed: %v", slug, i, err)
			continue
		}
		wd.Skills[i].Icon = toWebPath(local)
	}

	// Download passive icons.
	for i, p := range wd.Passives {
		if p.Icon == "" {
			continue
		}
		local := filepath.Join(imagesDir, slug, fmt.Sprintf("passive-%d%s", i+1, filepath.Ext(p.Icon)))
		if err := downloadImage(p.Icon, local); err != nil {
			log.Printf("[%s] passive icon %d failed: %v", slug, i, err)
			continue
		}
		wd.Passives[i].Icon = toWebPath(local)
	}

	return mergeIntoJSON(slug, page, wd, artPath)
}

// ---------- fetch ----------

func cachePath(page string) string {
	safe := strings.NewReplacer("(", "", ")", "", " ", "_").Replace(page)
	return filepath.Join(cacheDir, safe+".json")
}

func fetchWiki(page string) (string, error) {
	cp := cachePath(page)
	if data, err := os.ReadFile(cp); err == nil {
		var r apiResp
		if err := json.Unmarshal(data, &r); err == nil && r.Parse.Text != "" {
			return r.Parse.Text, nil
		}
	}

	u := fmt.Sprintf("%s?action=parse&page=%s&format=json&prop=text%%7Cimages&formatversion=2",
		apiBase, url.QueryEscape(page))
	req, _ := http.NewRequest("GET", u, nil)
	req.Header.Set("User-Agent", userAgent)
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	_ = os.WriteFile(cp, body, 0644)
	var r apiResp
	if err := json.Unmarshal(body, &r); err != nil {
		return "", err
	}
	return r.Parse.Text, nil
}

// ---------- image download ----------

func downloadImage(wikiFile, localPath string) error {
	if _, err := os.Stat(localPath); err == nil {
		return nil // already downloaded
	}
	if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
		return err
	}
	src := fileBase + url.PathEscape(wikiFile)
	client := &http.Client{Timeout: 60 * time.Second}

	var lastErr error
	for attempt := 0; attempt < 4; attempt++ {
		if attempt > 0 {
			time.Sleep(time.Duration(attempt) * 1500 * time.Millisecond)
		}
		req, _ := http.NewRequest("GET", src, nil)
		req.Header.Set("User-Agent", userAgent)
		req.Header.Set("Accept", "image/avif,image/webp,image/apng,image/svg+xml,image/*,*/*;q=0.8")
		req.Header.Set("Referer", "https://bleachsoulresonance.wiki.gg/")
		resp, err := client.Do(req)
		if err != nil {
			lastErr = err
			continue
		}
		if resp.StatusCode == 200 {
			f, err := os.Create(localPath)
			if err != nil {
				resp.Body.Close()
				return err
			}
			_, copyErr := io.Copy(f, resp.Body)
			f.Close()
			resp.Body.Close()
			if copyErr != nil {
				return copyErr
			}
			// Light throttle between successful downloads.
			time.Sleep(150 * time.Millisecond)
			return nil
		}
		lastErr = fmt.Errorf("HTTP %d for %s", resp.StatusCode, wikiFile)
		resp.Body.Close()
	}
	return lastErr
}

// toWebPath converts a local image path (under Web/Public/) into a browser
// path rooted at "/Images/...".
func toWebPath(localPath string) string {
	p := filepath.ToSlash(localPath)
	idx := strings.Index(p, "/Images/")
	if idx < 0 {
		// try without leading slash
		idx = strings.Index(p, "Images/")
		if idx < 0 {
			return p
		}
		return "/" + p[idx:]
	}
	return p[idx:]
}

// ---------- parse ----------

var (
	reTag        = regexp.MustCompile(`<[^>]+>`)
	reWS         = regexp.MustCompile(`\s+`)
	reFileLink   = regexp.MustCompile(`href="/wiki/File:([^"]+)"`)
	reBoundary   = regexp.MustCompile(`(?s)<article[^>]+id="Boundary_(\d)-\d"[^>]*>(.*?)</article>`)
	reSkillHdr   = regexp.MustCompile(`(?s)<th colspan="2"[^>]*>\s*([^<]+?)\s*</th>`)
	rePassiveRow = regexp.MustCompile(`(?s)<tr>\s*<td[^>]*>(.*?)</td>\s*<th>(.+?)</th>\s*</tr>`)
)

// stripHTML returns plaintext content of an HTML fragment.
func stripHTML(s string) string {
	s = strings.ReplaceAll(s, "<br />", " ")
	s = strings.ReplaceAll(s, "<br/>", " ")
	s = strings.ReplaceAll(s, "<br>", " ")
	s = strings.ReplaceAll(s, "<hr />", " — ")
	s = strings.ReplaceAll(s, "<hr/>", " — ")
	s = strings.ReplaceAll(s, "<hr>", " — ")
	s = reTag.ReplaceAllString(s, "")
	s = html.UnescapeString(s)
	s = reWS.ReplaceAllString(s, " ")
	return strings.TrimSpace(s)
}

// firstFileLink returns the first File:* filename found inside an HTML fragment.
func firstFileLink(s string) string {
	m := reFileLink.FindStringSubmatch(s)
	if m == nil {
		return ""
	}
	// Decode URL-encoded entities like %20 etc.
	if dec, err := url.QueryUnescape(m[1]); err == nil {
		return dec
	}
	return m[1]
}

func parseWiki(htmlText string) WikiData {
	wd := WikiData{}

	// Art image: the FIRST File:*Art* link on the page.
	if m := regexp.MustCompile(`href="/wiki/File:([^"]*Art[^"]*\.(?:png|jpg|jpeg))"`).FindStringSubmatch(htmlText); m != nil {
		wd.ArtFile = m[1]
	} else if m := regexp.MustCompile(`href="/wiki/File:([^"]+\.(?:png|jpg|jpeg))"`).FindStringSubmatch(htmlText); m != nil {
		// Fallback: very first File link.
		wd.ArtFile = m[1]
	}

	// Rarity from File:Ssr.png or File:Sr.png.
	if strings.Contains(htmlText, "/wiki/File:Ssr.png") {
		wd.Rarity = "SSR"
	} else if strings.Contains(htmlText, "/wiki/File:Sr.png") {
		wd.Rarity = "SR"
	}

	// Color (Type) — File:Red.png / Blue.png / Orange.png / Yellow.png in header.
	for _, c := range []string{"Red", "Blue", "Orange", "Yellow"} {
		if strings.Contains(htmlText, "/wiki/File:"+c+".png") {
			wd.Color = c
			break
		}
	}

	// Role + Damage Type + Affiliation row.
	if m := regexp.MustCompile(`(?s)Role:.*?Damage Type:.*?Affiliation:.*?</tr>\s*<tr>(.*?)</tr>`).FindStringSubmatch(htmlText); m != nil {
		cells := splitCells(m[1])
		if len(cells) >= 3 {
			wd.Role = stripHTML(cells[0])
			wd.DamageType = stripHTML(cells[1])
			wd.Affiliation = stripHTML(cells[2])
		}
	}

	// Release Date + Limited.
	if m := regexp.MustCompile(`(?s)Release Date:.*?Limited/Permanent:.*?</tr>\s*<tr>(.*?)</tr>`).FindStringSubmatch(htmlText); m != nil {
		cells := splitCells(m[1])
		if len(cells) >= 4 {
			wd.ReleaseDate = stripHTML(cells[2])
			wd.Limited = stripHTML(cells[3])
		}
	}

	// Stats table (first one with HP/ATK/DEF/Critical Damage/Critical Rate).
	if m := regexp.MustCompile(`(?s)<th[^>]*>\s*Stats\s*</th>.*?<tr>(.*?)</tr>\s*<tr>(.*?)</tr>`).FindStringSubmatch(htmlText); m != nil {
		// m[1] = header row, m[2] = value row
		vals := splitCells(m[2])
		if len(vals) >= 5 {
			wd.Stats.HP = stripHTML(vals[0])
			wd.Stats.ATK = stripHTML(vals[1])
			wd.Stats.DEF = stripHTML(vals[2])
			wd.Stats.CritDmg = stripHTML(vals[3])
			wd.Stats.CritRate = stripHTML(vals[4])
		}
	}

	// Weapon stats — look for a table whose header is "Weapon" with HP/ATK/DEF.
	if m := regexp.MustCompile(`(?s)<th[^>]*>\s*Weapon\s*</th>.*?<tr>(.*?)</tr>\s*<tr>(.*?)</tr>`).FindStringSubmatch(htmlText); m != nil {
		vals := splitCells(m[2])
		if len(vals) >= 3 {
			wd.WeaponStats.HP = stripHTML(vals[0])
			wd.WeaponStats.ATK = stripHTML(vals[1])
			wd.WeaponStats.DEF = stripHTML(vals[2])
		}
	}

	// Boundaries (1..6).
	bmap := map[int]Boundary{}
	for _, m := range reBoundary.FindAllStringSubmatch(htmlText, -1) {
		level := atoi(m[1])
		inner := m[2]
		b := Boundary{
			Level:       level,
			Icon:        firstFileLink(inner),
			Description: stripHTML(inner),
		}
		// Clean leading "—" / pipe artifacts produced by stripHTML.
		b.Description = strings.TrimSpace(strings.TrimPrefix(b.Description, "—"))
		bmap[level] = b
	}
	for lvl := 1; lvl <= 6; lvl++ {
		if b, ok := bmap[lvl]; ok {
			wd.Boundaries = append(wd.Boundaries, b)
		}
	}

	// Skills — between "Abilities" header and "Passives" header.
	reAbilHdr := regexp.MustCompile(`<th[^>]*>\s*Abilities\s*</th>`)
	rePassHdr := regexp.MustCompile(`<th[^>]*>\s*Passives\s*</th>`)
	abilStart := -1
	if loc := reAbilHdr.FindStringIndex(htmlText); loc != nil {
		abilStart = loc[1]
	}
	passStart := -1
	if loc := rePassHdr.FindStringIndex(htmlText); loc != nil {
		passStart = loc[0]
	}
	if abilStart > 0 {
		end := passStart
		if end < 0 {
			end = len(htmlText)
		}
		if end > abilStart {
			wd.Skills = parseSkills(htmlText[abilStart:end])
		}
	}

	// Passives.
	if passStart > 0 {
		wd.Passives = parsePassives(htmlText[passStart:])
	}

	return wd
}

// splitCells splits a table-row inner HTML into <td>/<th> cell contents.
func splitCells(rowHTML string) []string {
	re := regexp.MustCompile(`(?s)<t[dh][^>]*>(.*?)</t[dh]>`)
	ms := re.FindAllStringSubmatch(rowHTML, -1)
	out := make([]string, 0, len(ms))
	for _, m := range ms {
		out = append(out, m[1])
	}
	return out
}

// parseSkills extracts a list of skills from the Abilities region.
// Each skill is introduced by a `<th colspan="2">KIND: NAME</th>` header table,
// followed by a content table containing an icon and a description.
func parseSkills(region string) []Skill {
	// Find all header positions.
	headerRE := regexp.MustCompile(`(?s)<th colspan="2"[^>]*>\s*([^<]+?)\s*</th>`)
	idx := headerRE.FindAllStringSubmatchIndex(region, -1)
	if len(idx) == 0 {
		return nil
	}
	skills := make([]Skill, 0, len(idx))
	for i, m := range idx {
		if i == 0 {
			// First header table is the "Abilities" header itself — skip if matches.
			title := strings.TrimSpace(region[m[2]:m[3]])
			if strings.EqualFold(title, "Abilities") {
				continue
			}
		}
		title := strings.TrimSpace(region[m[2]:m[3]])
		title = stripHTML(title)
		// Determine kind/name from "KIND: NAME"
		kind, name := title, ""
		if c := strings.Index(title, ":"); c > 0 {
			kind = strings.TrimSpace(title[:c])
			name = strings.TrimSpace(title[c+1:])
		}
		// Find body up to next header or end.
		bodyStart := m[1]
		bodyEnd := len(region)
		if i+1 < len(idx) {
			bodyEnd = idx[i+1][0]
		}
		body := region[bodyStart:bodyEnd]
		icon := firstFileLink(body)
		// Take everything inside the first <p>...</p> as description.
		desc := ""
		if pm := regexp.MustCompile(`(?s)<p>(.*?)</p>`).FindStringSubmatch(body); pm != nil {
			desc = stripHTML(pm[1])
		} else {
			desc = stripHTML(body)
		}
		skills = append(skills, Skill{
			Name:        name,
			Kind:        kind,
			Description: desc,
			Icon:        icon,
		})
	}
	return skills
}

// parsePassives extracts passive rows from the Passives table region.
func parsePassives(region string) []Passive {
	// Cut off at "Recommended" section if present.
	if i := strings.Index(region, "Recommended"); i > 0 {
		region = region[:i]
	}
	out := []Passive{}
	for _, m := range rePassiveRow.FindAllStringSubmatch(region, -1) {
		iconCell, body := m[1], m[2]
		icon := firstFileLink(iconCell)
		// Body is "Name<hr />Description". After stripHTML our hr becomes " — ".
		// Use the raw body to split before stripping.
		var name, desc string
		if parts := regexp.MustCompile(`<hr ?/?>`).Split(body, 2); len(parts) == 2 {
			name = stripHTML(parts[0])
			desc = stripHTML(parts[1])
		} else {
			name = stripHTML(body)
		}
		if name == "" {
			continue
		}
		out = append(out, Passive{Name: name, Description: desc, Icon: icon})
	}
	return out
}

// ---------- merge ----------

// mergeIntoJSON loads the existing ../Data/<slug>.json (if any) and
// overwrites wiki-sourced fields, preserving build/stamp/bond data.
func mergeIntoJSON(slug, page string, wd WikiData, artPath string) error {
	jsonPath := filepath.Join(dataDir, slug+".json")
	doc := map[string]interface{}{}
	if data, err := os.ReadFile(jsonPath); err == nil {
		_ = json.Unmarshal(data, &doc)
	}
	if doc["slug"] == nil {
		doc["slug"] = slug
	}
	// Wiki fields (overwrite).
	if wd.Rarity != "" {
		doc["rarity"] = wd.Rarity
	}
	if wd.Color != "" {
		doc["color"] = wd.Color
	}
	if wd.Affiliation != "" {
		doc["affiliation"] = wd.Affiliation
	}
	if wd.Role != "" {
		// Wiki uses "Tactic"; existing JSONs use "Tactician". Normalize.
		role := wd.Role
		if strings.EqualFold(role, "Tactic") {
			role = "Tactician"
		}
		doc["role"] = role
	}
	if wd.DamageType != "" {
		// Wiki uses "Spiritual"; existing JSONs use "Spirit". Normalize.
		dt := wd.DamageType
		if strings.EqualFold(dt, "Spiritual") {
			dt = "Spirit"
		}
		doc["damageType"] = dt
	}
	if wd.ReleaseDate != "" {
		doc["releaseDate"] = wd.ReleaseDate
	}
	if wd.Limited != "" {
		doc["limited"] = wd.Limited
	}
	if hasAnyStat(wd.Stats) {
		doc["stats"] = wd.Stats
	}
	if hasAnyStat(wd.WeaponStats) {
		doc["weaponStats"] = wd.WeaponStats
	}
	if artPath != "" {
		doc["art"] = artPath
	}
	if len(wd.Boundaries) > 0 {
		// Preserve existing boundary "name" values where levels match.
		existing := map[int]string{}
		if old, ok := doc["boundaries"].([]interface{}); ok {
			for _, b := range old {
				if bm, ok := b.(map[string]interface{}); ok {
					if lv, ok := bm["level"].(float64); ok {
						if nm, ok := bm["name"].(string); ok && nm != "" {
							existing[int(lv)] = nm
						}
					}
				}
			}
		}
		for i := range wd.Boundaries {
			if nm := existing[wd.Boundaries[i].Level]; nm != "" {
				wd.Boundaries[i].Name = nm
			}
		}
		doc["boundaries"] = wd.Boundaries
	}
	if len(wd.Skills) > 0 {
		doc["skills"] = wd.Skills
	}
	if len(wd.Passives) > 0 {
		doc["corePassives"] = wd.Passives
	}
	doc["wikiSource"] = "https://bleachsoulresonance.wiki.gg/wiki/" + page

	// Ensure schema-stable fields exist (avoid nulls in TS).
	defaults := map[string]interface{}{
		"skills":       []interface{}{},
		"boundaries":   []interface{}{},
		"corePassives": []interface{}{},
		"stamps":       map[string]interface{}{"weapon": map[string]interface{}{"ascend1": []interface{}{}, "ascend5": []interface{}{}}, "core": map[string]interface{}{"ascend1": []interface{}{}, "ascend5": []interface{}{}}},
		"builds":       []interface{}{},
		"bondPriority": []interface{}{},
	}
	for k, v := range defaults {
		if _, ok := doc[k]; !ok {
			doc[k] = v
		}
	}
	if _, ok := doc["source"]; !ok {
		doc["source"] = nil
	}

	return writeJSON(jsonPath, doc)
}

func hasAnyStat(s Stats) bool {
	return s.HP != "" || s.ATK != "" || s.DEF != "" || s.CritDmg != "" || s.CritRate != ""
}

// ---------- index ----------

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

func writeIndex() error {
	entries := []indexEntry{}
	files, err := os.ReadDir(dataDir)
	if err != nil {
		return err
	}
	for _, f := range files {
		if f.IsDir() || !strings.HasSuffix(f.Name(), ".json") {
			continue
		}
		if f.Name() == "Index.json" || f.Name() == "Stamps.json" || f.Name() == "Teams.json" || f.Name() == "Banners.json" || f.Name() == "Glossary.json" {
			continue
		}
		raw, err := os.ReadFile(filepath.Join(dataDir, f.Name()))
		if err != nil {
			continue
		}
		var d map[string]interface{}
		if err := json.Unmarshal(raw, &d); err != nil {
			continue
		}
		e := indexEntry{
			Slug:        getStr(d, "slug"),
			Name:        getStr(d, "name"),
			Role:        getStr(d, "role"),
			Tier:        getStr(d, "tier"),
			DamageType:  getStr(d, "damageType"),
			Rarity:      getStr(d, "rarity"),
			Art:         getStr(d, "art"),
			ReleaseDate: getStr(d, "releaseDate"),
		}
		if e.Slug == "" {
			e.Slug = strings.TrimSuffix(f.Name(), ".json")
		}
		entries = append(entries, e)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Slug < entries[j].Slug })
	return writeJSON(filepath.Join(dataDir, "Index.json"), entries)
}

// ---------- utils ----------

func writeJSON(path string, v interface{}) error {
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return err
	}
	buf = append(buf, '\n')
	return os.WriteFile(path, buf, 0644)
}

func sortedKeys(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

func mustMk(p string) {
	if err := os.MkdirAll(p, 0755); err != nil {
		log.Fatal(err)
	}
}

func getStr(m map[string]interface{}, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}
