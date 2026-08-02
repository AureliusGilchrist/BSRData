// Package charstore is the single source of truth for where per-character JSON
// lives on disk and how the character index is built.
//
// Layout — one file per character, foldered by the YEAR the character released:
//
//	BSRData/Characters/2025/ichigo-bankai.json
//	BSRData/Characters/2026/ichigo-ts.json
//	BSRData/Characters/Index.json           <- generated, see WriteIndex
//
// Many characters share a year folder; the folder is purely an organisational
// bucket, never part of a character's identity. A character is addressed by its
// SLUG alone — Find resolves the slug to whichever year folder holds it — so
// moving a file between years (a corrected release date) never breaks a link.
//
// Both the scraper and the curator go through this package so the two can't
// drift apart on where files live or which ones count as characters.
package charstore

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// DirName is the character folder, relative to the BSRData root.
const DirName = "Characters"

// IndexName is the generated manifest inside DirName.
const IndexName = "Index.json"

// slugRe mirrors the validation the web app re-applies at runtime before a
// slug reaches a URL (Web/Source/Lib/Data.ts `safeSegment`). Enforcing it here
// means a filename can never widen into a path traversal on the client.
var slugRe = regexp.MustCompile(`^[a-z0-9]+(?:-[a-z0-9]+)*$`)

// yearRe matches a four-digit year folder.
var yearRe = regexp.MustCompile(`^\d{4}$`)

// ValidSlug reports whether a slug is safe to use as a path segment.
func ValidSlug(slug string) bool { return slugRe.MatchString(slug) }

// Entry is one row of the generated Characters/Index.json. `Path` is the
// index-relative location of the full record ("2026/ichigo-ts.json") — the web
// app fetches it verbatim rather than guessing a year.
type Entry struct {
	Slug        string `json:"slug"`
	Path        string `json:"path"`
	Year        int    `json:"year,omitempty"`
	Name        string `json:"name,omitempty"`
	Role        string `json:"role,omitempty"`
	Tier        string `json:"tier,omitempty"`
	DamageType  string `json:"damageType,omitempty"`
	Rarity      string `json:"rarity,omitempty"`
	Art         string `json:"art,omitempty"`
	ReleaseDate string `json:"releaseDate,omitempty"`
}

// Dir returns the Characters folder for a BSRData root.
func Dir(root string) string { return filepath.Join(root, DirName) }

// ReleaseYear pulls the four-digit year out of a release date as the scraper
// writes it ("21 Nov 2025"). Returns "" when there is no year to find, which
// callers treat as "leave the file where it is".
func ReleaseYear(releaseDate string) string {
	m := regexp.MustCompile(`\d{4}`).FindString(releaseDate)
	return m
}

// Find resolves a slug to its on-disk path by scanning the year folders.
// Returns "" (and no error) when the character has no file yet.
func Find(root, slug string) (string, error) {
	if !ValidSlug(slug) {
		return "", fmt.Errorf("charstore: unsafe slug %q", slug)
	}
	years, err := years(root)
	if err != nil {
		return "", err
	}
	for _, y := range years {
		p := filepath.Join(Dir(root), y, slug+".json")
		if _, err := os.Stat(p); err == nil {
			return p, nil
		}
	}
	return "", nil
}

// PathFor returns where a character's file SHOULD live given a release date,
// preferring wherever it already lives so an existing file is never duplicated
// into a second year folder by a re-scrape. `year` falls back to "unknown" when
// the release date carries no year, so a character is never silently dropped.
func PathFor(root, slug, releaseDate string) (string, error) {
	if existing, err := Find(root, slug); err != nil {
		return "", err
	} else if existing != "" {
		return existing, nil
	}
	if !ValidSlug(slug) {
		return "", fmt.Errorf("charstore: unsafe slug %q", slug)
	}
	year := ReleaseYear(releaseDate)
	if year == "" {
		year = "unknown"
	}
	return filepath.Join(Dir(root), year, slug+".json"), nil
}

// Load reads a character's JSON as a loose map, along with the path it came
// from. A character with no file yet yields an empty (non-nil) map and "" —
// callers build the document up from there and hand it to Save.
func Load(root, slug string) (map[string]any, string) {
	doc := map[string]any{}
	path, err := Find(root, slug)
	if err != nil || path == "" {
		return doc, ""
	}
	if raw, err := os.ReadFile(path); err == nil {
		_ = json.Unmarshal(raw, &doc)
	}
	return doc, path
}

// Save writes a character document to the year folder its releaseDate calls
// for, creating the folder as needed.
//
// If the character already had a file in a DIFFERENT year folder — the usual
// cause being a first pass that created the file before a release date was
// known, and a later pass that learned one — the old file is removed after the
// new one lands, so a character is never represented twice under two years.
func Save(root, slug string, doc map[string]any) error {
	if !ValidSlug(slug) {
		return fmt.Errorf("charstore: unsafe slug %q", slug)
	}
	existing, err := Find(root, slug)
	if err != nil {
		return err
	}

	releaseDate, _ := doc["releaseDate"].(string)
	year := ReleaseYear(releaseDate)
	if year == "" {
		// No release date to file under. Keep the file where it already is
		// rather than dragging it into "unknown" on every re-scrape.
		if existing != "" {
			return write(existing, doc)
		}
		year = "unknown"
	}

	target := filepath.Join(Dir(root), year, slug+".json")
	if err := write(target, doc); err != nil {
		return err
	}
	if existing != "" && existing != target {
		return os.Remove(existing)
	}
	return nil
}

func write(path string, doc map[string]any) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	buf, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(buf, '\n'), 0o644)
}

// Slugs returns the set of characters that already have a file.
func Slugs(root string) map[string]bool {
	out := map[string]bool{}
	_ = walk(root, func(slug, _ string) error {
		out[slug] = true
		return nil
	})
	return out
}

// years lists the year folders under Characters/, sorted oldest-first.
func years(root string) ([]string, error) {
	ents, err := os.ReadDir(Dir(root))
	if err != nil {
		return nil, err
	}
	var out []string
	for _, e := range ents {
		// "unknown" is a deliberate escape hatch for a character whose release
		// date hasn't been filled in yet — it must still be walked.
		if e.IsDir() && (yearRe.MatchString(e.Name()) || e.Name() == "unknown") {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out, nil
}

// walk visits every character file as (slug, absolute path).
func walk(root string, fn func(slug, path string) error) error {
	ys, err := years(root)
	if err != nil {
		return err
	}
	for _, y := range ys {
		ents, err := os.ReadDir(filepath.Join(Dir(root), y))
		if err != nil {
			return err
		}
		for _, e := range ents {
			name := e.Name()
			if e.IsDir() || filepath.Ext(name) != ".json" {
				continue
			}
			slug := strings.TrimSuffix(name, ".json")
			if !ValidSlug(slug) {
				return fmt.Errorf("charstore: %s/%s — filename must be kebab-case [a-z0-9-]", y, name)
			}
			if err := fn(slug, filepath.Join(Dir(root), y, name)); err != nil {
				return err
			}
		}
	}
	return nil
}

// BuildIndex reads every character file and returns the index rows.
//
// `prev` (an index loaded from a previous run, keyed by slug) is used as a
// fallback for any file that fails to parse: hand edits have shipped broken
// JSON more than once, and serving a slightly stale row beats making the
// character vanish from the site entirely. `onWarn` reports each such case.
func BuildIndex(root string, prev map[string]Entry, onWarn func(string)) ([]Entry, error) {
	if onWarn == nil {
		onWarn = func(string) {}
	}
	entries := []Entry{}
	err := walk(root, func(slug, path string) error {
		rel := filepath.ToSlash(mustRel(Dir(root), path))
		year := 0
		fmt.Sscanf(filepath.Base(filepath.Dir(path)), "%d", &year)

		keepPrev := func(reason string) {
			if e, ok := prev[slug]; ok {
				onWarn(fmt.Sprintf("%s: %s — keeping previous index entry", rel, reason))
				e.Path = rel // the file moved even if we can't read it
				entries = append(entries, e)
				return
			}
			onWarn(fmt.Sprintf("%s: %s — no previous entry, character will be MISSING from the index", rel, reason))
		}

		raw, err := os.ReadFile(path)
		if err != nil {
			keepPrev("unreadable: " + err.Error())
			return nil
		}
		var d map[string]any
		if err := json.Unmarshal(raw, &d); err != nil {
			keepPrev("invalid JSON: " + err.Error())
			return nil
		}
		get := func(k string) string {
			if v, ok := d[k].(string); ok {
				return v
			}
			return ""
		}
		e := Entry{
			Slug:        get("slug"),
			Path:        rel,
			Year:        year,
			Name:        get("name"),
			Role:        get("role"),
			Tier:        get("tier"),
			DamageType:  get("damageType"),
			Rarity:      get("rarity"),
			Art:         get("art"),
			ReleaseDate: get("releaseDate"),
		}
		if e.Slug == "" {
			e.Slug = slug
		}
		if e.Slug != slug {
			return fmt.Errorf("charstore: %s declares slug %q — it must match the filename", rel, e.Slug)
		}
		entries = append(entries, e)
		return nil
	})
	if err != nil {
		return nil, err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Slug < entries[j].Slug })
	return entries, nil
}

// LoadIndex reads the existing Characters/Index.json keyed by slug. A missing
// or unreadable index is not an error — it just yields an empty map.
func LoadIndex(root string) map[string]Entry {
	out := map[string]Entry{}
	raw, err := os.ReadFile(filepath.Join(Dir(root), IndexName))
	if err != nil {
		return out
	}
	var entries []Entry
	if err := json.Unmarshal(raw, &entries); err != nil {
		return out
	}
	for _, e := range entries {
		out[e.Slug] = e
	}
	return out
}

// WriteIndex regenerates Characters/Index.json from the per-character files.
func WriteIndex(root string, onWarn func(string)) (int, error) {
	entries, err := BuildIndex(root, LoadIndex(root), onWarn)
	if err != nil {
		return 0, err
	}
	out, err := json.MarshalIndent(entries, "", "  ")
	if err != nil {
		return 0, err
	}
	if err := os.MkdirAll(Dir(root), 0o755); err != nil {
		return 0, err
	}
	return len(entries), os.WriteFile(filepath.Join(Dir(root), IndexName), out, 0o644)
}

func mustRel(base, target string) string {
	rel, err := filepath.Rel(base, target)
	if err != nil {
		return filepath.Base(target)
	}
	return rel
}
