// Hand-curated rotations, swappable members, best stamps, and recommended
// stop-boundaries for every character. Run with:
//
//   cd "Data Source/curate" && go run .
//
// Reads each ../Data/<slug>.json, merges in the curated fields, and
// writes Teams.json. All recommendations are community/archetype best-effort
// guesses since no public source covers all 29 chars — adjust as the meta
// evolves.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ---------------------------------------------------------------------------
// Schema mirrors (subset of) Web/Source/Lib/Types.ts.

type Rotation struct {
	Name  string   `json:"name"`
	Steps []string `json:"steps"`
	Notes string   `json:"notes,omitempty"`
	Tags  []string `json:"tags,omitempty"`
}

type BestStamp struct {
	SetName         string `json:"setName"`
	MainStats       string `json:"mainStats,omitempty"`
	SubStatPriority string `json:"subStatPriority,omitempty"`
	Notes           string `json:"notes,omitempty"`
}

type TeamMember struct {
	Slug       string   `json:"slug"`
	Name       string   `json:"name"`
	Role       string   `json:"role,omitempty"`
	DamageType string   `json:"damageType,omitempty"`
	Notes      []string `json:"notes,omitempty"`
	Swappable  []string `json:"swappable,omitempty"`
}

type Team struct {
	ID            string       `json:"id"`
	Name          string       `json:"name"`
	Rank          int          `json:"rank,omitempty"`
	Tier          string       `json:"tier,omitempty"`
	Difficulty    string       `json:"difficulty,omitempty"`
	Content       string       `json:"content,omitempty"`
	Members       []TeamMember `json:"members"`
	RotationNotes string       `json:"rotationNotes,omitempty"` // deprecated — kept for back-compat
	RotationP2W   string       `json:"rotationP2W,omitempty"`
	RotationF2P   string       `json:"rotationF2P,omitempty"`
	Source        string       `json:"source,omitempty"`
}

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
	// Featured signature weapon/light-cone name, if any (optional).
	Weapon string `json:"weapon,omitempty"`
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
// role and damage type. No dates, no art links, no character page.
type Leak struct {
	// Character name, e.g. "Ulquiorra Cifer".
	Name string `json:"name"`
	// Role class: "Assault" / "Full Assault / DPS" / "Tactician" / "Support".
	Role string `json:"role,omitempty"`
	// Damage type: "Slash" / "Strike" / "Thrust" / "Spirit".
	DamageType string `json:"damageType,omitempty"`
	// Rarity, if known: "SSR" / "SR+" / "SR" (optional).
	Rarity string `json:"rarity,omitempty"`
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

// CharacterCuration holds per-character merged fields.
type CharacterCuration struct {
	StopBoundary    int // legacy / back-compat (mirrors P2W)
	StopBoundaryP2W int // value-optimal ceiling assuming dupes are available
	StopBoundaryF2P int // practical F2P ceiling. Realistic gacha pull math:
	//   SSR meta-defining (Aizen/Toshiro/Kisuke/Bankai/Nelliel): 2 — worth saving
	//   SSR standard: 1 — expect ~1 copy per banner; B1 is the realistic ceiling
	//   SSR niche/legacy (Komamura/Tosen): 0 — don't dupe-invest, use the base unit
	//   SR+: 3 — limited rarity, mid-rate pulls
	//   SR: 5 — copies drop frequently, near-max is feasible
	//   SR free/starter (Ichigo-Initial/Momo): 6 — given by story / free banners
	BestStamp       BestStamp
	Rotations       []Rotation
}

// rarityBySlug fixes a scraper gap: the Spanish wiki pages don't include the
// File:Ssr.png / File:Sr.png markers the scraper looks for, so most characters
// land in JSON with no `rarity` field and the UI falls back to "SR". This map
// is the authoritative roster rarity used to fill in missing values.
var rarityBySlug = map[string]string{
	// SSR — full meta roster
	"aizen": "SSR", "byakuya": "SSR", "gin": "SSR", "grimmjow-pantera": "SSR",
	"ichigo-bankai": "SSR", "ikkaku": "SSR", "kenpachi": "SSR", "kisuke": "SSR",
	"mayuri": "SSR", "nelliel": "SSR", "soi-fon": "SSR", "komamura": "SSR",
	"szayelaporro": "SSR", "toshiro": "SSR", "yoruichi": "SSR", "tosen": "SSR",
	
	"grimmjow": "SR+",

	"rangiku": "SR", "yachiru": "SR", "rukia": "SR",
	"chad": "SR", "ichigo-initial": "SR", "ichigo-shikai": "SR",
	"momo": "SR", "nemu": "SR", "orihime": "SR",
	"renji": "SR", "ururu": "SR", "uryu": "SR",
}

// tierBySlug is the authoritative tier-list ranking shown on the Tier List page
// and on character cards. The scraper does not derive this from the wiki; edit
// this map by hand to re-rank a character. Valid values: "S+", "S", "A", "B", "C".
var tierBySlug = map[string]string{
	"aizen":	        "S",
	"gin": 	    	    "S",
	"kisuke":      	 	"S",
	"mayuri":       	"S",
	"nelliel":      	"S",
	"soi-fon":      	"S",
	"toshiro":          "S",
	"grimmjow-pantera": "S",
	"szayelaporro": 	"S",

	"kenpachi":     	"A",
	"grimmjow":         "A",
	"ichigo-bankai":    "A",
	"tosen":            "A",

	"yachiru":          "B",
	"momo":             "B",
	"rangiku":          "B",
	"yoruichi": 		"B",

	"ikkaku":   		"C",
	"rukia":            "C",
	"komamura": 		"C",
	"uryu":           	"D",
	"ichigo-shikai":  	"C",

	"chad":           	"D",
	"ichigo-initial": 	"D",
	"orihime":        	"D",
	"nemu":           	"D",
	"renji":          	"D",
	"ururu":          	"D",
	"byakuya":        	"C",
}

// Note: release dates are no longer hard-coded here. The scraper (../Main.go)
// extracts the authoritative Global date from the Spanish Fandom wiki on every
// run and writes it to each character's JSON. This file only consumes that
// value via the merge loop below.

// ---------------------------------------------------------------------------
// Per-character curated data.

var curation = map[string]CharacterCuration{
	"aizen": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 1,
		BestStamp: BestStamp{
			SetName:         "Immeasurable Gap (4) + Spirit DMG (2)",
			MainStats:       "1: Spirit DMG Bonus > Atk%  •  2: Crit Rate > Crit DMG  •  3: Atk%",
			SubStatPriority: "Crit DMG > Crit Rate > Atk% > Ult Charge Rate > Flat Atk",
			Notes:           "Aim for ~119% Ult Charge Rate to align Kyokasuigetsu windows with team buffs. Crit DMG scales hardest because of the B6 Crit→CritDMG conversion.",
		},
		Rotations: []Rotation{
			{
				Name: "Opener (Kyokasuigetsu Burst)",
				Steps: []string{
					"Start with support to apply buffs",
					"Aizen battlefield: Technique (Fated Demise) → Basic Attack (Hado 63: Raikoho branch)",
					"Build to 3 Pressure Stacks → Battlefield",
					"Enter Ultimate: Kyokasuigetsu whenever available",
					"Spam Exposed Weakspot basics for stack consumption",
				},
				Notes: "Open with the support so Aizen enters with Suppression already filling. Save Ultimate until enemy is locked, then dump Hado 90.",
				Tags:  []string{"Burst", "Boss"},
			},
			{
				Name: "Sustained / Off-field",
				Steps: []string{
					"Backline while ally builds Suppression Gauge",
					"Swap-in counter (B4) for free counterattack",
					"Backline Battlefield Skill at Complete Suppression",
					"Return to backline, repeat",
				},
				Notes: "B4 lets him counter on swap-in — perfect for stall fights. Each backline BFS while Complete Suppression buffs the next Hado 90 by 37.5% (Dustwring).",
				Tags:  []string{"Sustain", "PvP"},
			},
		},
	},

	"byakuya": {
		StopBoundaryP2W: 0,
		StopBoundaryF2P: 0,
		BestStamp: BestStamp{
			SetName:         "Senbonzakura (4) + Crit (2)",
			MainStats:       "1: Slash DMG Bonus  •  2: Crit Rate / Crit DMG  •  3: Atk%",
			SubStatPriority: "Crit Rate > Crit DMG > Atk% > Ult Charge Rate > Atk",
			Notes:           "Tactician hybrid — keep enough Ult Charge Rate (~110%) so Senbonzakura petals refresh between waves.",
		},
		Rotations: []Rotation{
			{
				Name: "Petal Burst Opener",
				Steps: []string{
					"Front-line entry → Basic ×3",
					"Technique (Scatter, Senbonzakura)",
					"Ultimate at full charge",
					"Battlefield Skill while petals stack",
					"Swap out to retain petal field",
				},
				Notes: "Layer the petal field early — damage scales while he's off-field, making him perfect for rotations.",
				Tags:  []string{"Burst"},
			},
			{
				Name: "Off-field Pressure",
				Steps: []string{
					"Open with petal field, swap to main DPS",
					"Swap back for counter / refresh petals",
				},
				Tags: []string{"Sustain"},
			},
		},
	},

	"chad": {
		StopBoundaryP2W: 5,
		StopBoundaryF2P: 1,
		BestStamp: BestStamp{
			SetName:         "Brazo Derecho (4) + Strike (2)",
			MainStats:       "1: Strike DMG Bonus  •  2: Crit DMG  •  3: Atk%",
			SubStatPriority: "Atk% > Crit DMG > Crit Rate > Atk",
			Notes:           "F2P Assault — pure raw damage substats. Atk% scales his arm-charge basics best.",
		},
		Rotations: []Rotation{
			{
				Name: "Brazo Combo",
				Steps: []string{"Basic ×3 (charge arm)", "Technique", "Ultimate", "Counter on incoming hit"},
				Tags:  []string{"Sustain"},
			},
		},
	},

	"gin": {
		StopBoundaryP2W: 4,
		StopBoundaryF2P: 2,
		BestStamp: BestStamp{
			SetName:         "Shinso (4) + Thrust (2)",
			MainStats:       "1: Atk% > Thrust DMG Bonus •  2: Crit Rate  •  3: Ailment%",
			SubStatPriority: "Crit Rate > Ailment% > Atk% > Crit DMG > Flat Atk > Ult Charge Rate",
			Notes:           "Pure Ailment DPS — Ailment% is king once Crit Rate hits ~70%.",
		},
		Rotations: []Rotation{
			{
				Name: "Pierce Opener",
				Steps: []string{
					"Tactician (Nelliel/Aizen) opener",
					"Gin: Basic ×2 → Technique (Shinso extension)",
					"Ultimate (Kamishini no Yari) at burst window",
					"Battlefield Skill on cooldown",
				},
				Notes: "Gin's Weapon Ascension 3 is crucial to the unit's overall damage. Prioritize upgrading the weapon over boundaries after B2.",
				Tags:  []string{"Burst", "Boss"},
			},
		},
	},

	"grimmjow": {
		StopBoundaryP2W: 5,
		StopBoundaryF2P: 3,
		BestStamp: BestStamp{
			SetName:         "Pantera (4) + Strike (2)",
			MainStats:       "1: Strike DMG Bonus  •  2: Crit Rate  •  3: Atk%",
			SubStatPriority: "Crit DMG > Crit Rate > Atk% > Atk",
			Notes:           "SR Assault — invest only to ascend5, then prefer the SSR Pantera form.",
		},
		Rotations: []Rotation{
			{
				Name: "Rip Combo",
				Steps: []string{"Basic ×3", "Technique (Desgarrón)", "Counter on incoming", "Ultimate"},
				Tags:  []string{"Sustain"},
			},
		},
	},

	"grimmjow-pantera": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 0,
		BestStamp: BestStamp{
			SetName:         "Pantera (4) + Strike (2)",
			MainStats:       "1: Strike DMG Bonus  •  2: Crit Rate  •  3: Atk%",
			SubStatPriority: "ATK% > Crit Rate > Crit DMG > Flat Atk",
			Notes:           "B6 unlocks the second Resurrección burst — worth the full investment.",
		},
		Rotations: []Rotation{
			{
				Name: "Resurrección Burst",
				Steps: []string{
					"Swap-in with full ult charge",
					"Technique → Basic ×3 (rage stacks)",
					"Ultimate (Pantera)",
					"Battlefield Skill while resurrected",
				},
				Tags: []string{"Burst", "Boss"},
			},
			{
				Name: "Rage Sustain",
				Steps: []string{"Counter-bait → Technique → Basic combo", "Refresh rage stacks before Ult"},
				Tags:  []string{"Sustain", "PvP"},
			},
		},
	},

	"ichigo-bankai": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 1,
		BestStamp: BestStamp{
			SetName:         "Tensa Zangetsu (4) + Slash (2)",
			MainStats:       "1: Slash DMG Bonus > ATK%  •  2: Crit Rate  •  3: Atk%",
			SubStatPriority: "Crit Rate > Crit DMG > Atk% > Flat Atk",
			Notes:           "S+ DPS — Crit DMG scaling is uncapped, push Crit Rate to ~70% first.",
		},
		Rotations: []Rotation{
			{
				Name: "Getsuga Burst",
				Steps: []string{
					"Buffer (Urahara/Momo) front, applies buffs",
					"Swap Ichigo in (carries buffs)",
					"Technique → Basic ×3 (Getsuga charges)",
					"Ultimate (Getsuga Tensho)",
					"Battlefield Skill on Resonance peak",
				},
				Notes: "Drop the Ultimate inside the buffer's Battlefield Skill window for max scaling.",
				Tags:  []string{"Burst", "Boss"},
			},
			{
				Name: "Counter Sustain",
				Steps: []string{"Counter on incoming hit (B4 swap-in counter)", "Basic ×3 → Technique", "Repeat"},
				Tags:  []string{"Sustain", "PvP"},
			},
		},
	},

	"ichigo-initial": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 6,
		BestStamp: BestStamp{
			SetName:         "Substitute Shinigami (4) + Slash (2)",
			MainStats:       "1: Slash DMG Bonus  •  2: Crit Rate  •  3: Atk%",
			SubStatPriority: "Atk% > Crit DMG > Crit Rate > Atk",
			Notes:           "Starter character — useful early, replace with Shikai/Bankai once available.",
		},
		Rotations: []Rotation{
			{Name: "Basic Combo", Steps: []string{"Basic ×3", "Technique", "Ultimate", "Counter"}, Tags: []string{"Sustain"}},
		},
	},

	"ichigo-shikai": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 6,
		BestStamp: BestStamp{
			SetName:         "Zangetsu (4) + Slash (2)",
			MainStats:       "1: Slash DMG Bonus  •  2: Crit DMG  •  3: Atk%",
			SubStatPriority: "Crit DMG > Atk% > Crit Rate > Atk",
			Notes:           "Stepping stone to Bankai form — invest just enough to clear story.",
		},
		Rotations: []Rotation{
			{Name: "Getsuga Combo", Steps: []string{"Basic ×3", "Technique (Getsuga)", "Ultimate", "Battlefield Skill"}, Tags: []string{"Sustain"}},
		},
	},

	"ikkaku": {
		StopBoundaryP2W: 5,
		StopBoundaryF2P: 1,
		BestStamp: BestStamp{
			SetName:         "Hozukimaru (4) + Thrust (2)",
			MainStats:       "1: Thrust DMG Bonus  •  2: Crit DMG  •  3: Atk%",
			SubStatPriority: "Crit DMG > Crit Rate > Atk% > Atk",
			Notes:           "S-tier Assault — counter-focused, prioritize Crit DMG once Crit Rate hits ~60%.",
		},
		Rotations: []Rotation{
			{
				Name: "Counter Burst",
				Steps: []string{"Block / counter on incoming attack", "Basic ×3 (stance build)", "Technique → Ultimate (Bankai)", "Battlefield Skill"},
				Notes: "Each counter builds stance — burst when stance is maxed for the Bankai damage multiplier.",
				Tags:  []string{"Burst", "Boss"},
			},
		},
	},

	"kenpachi": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 2,
		BestStamp: BestStamp{
			SetName:         "Nozarashi (4) + Slash (2)",
			MainStats:       "1: ATK%  •  2: Crit DMG  •  3: Atk%",
			SubStatPriority: "Crit DMG > Crit Rate > Atk% > Ailment%",
			Notes:           "Tank-DPS hybrid — HP% sub can sub in once survivability dips.",
		},
		Rotations: []Rotation{
			{Name: "Battle Lust", Steps: []string{"Take damage to charge", "Basic ×3", "Technique", "Ultimate (eye-patch off)", "Battlefield Skill"}, Notes: "His damage scales with missing HP — let him take a few hits before bursting.", Tags: []string{"Burst", "Boss"}},
		},
	},

	"kisuke": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 2,
		BestStamp: BestStamp{
			SetName:         "Benihime (4) + Atk (2)",
			MainStats:       "1: Atk%  •  2: Crit Rate  •  3: Ult Charge Rate",
			SubStatPriority: "Ult Charge Rate > Crit Rate > Atk% > Crit DMG",
			Notes:           "S+ Support — Ult Charge Rate to keep his team-wide ATK% buff on permanent uptime.",
		},
		Rotations: []Rotation{
			{Name: "Buff Loop", Steps: []string{"Enter front-line", "Technique (apply blood mist)", "Ultimate (team ATK buff)", "Swap to main DPS", "Swap back on cooldown to refresh"}, Notes: "His buff window IS the team's burst window — line up DPS Ult inside it.", Tags: []string{"Sustain", "Boss"}},
		},
	},

	"komamura": {
		StopBoundaryP2W: 4,
		StopBoundaryF2P: 0,
		BestStamp: BestStamp{
			SetName:         "Tenken (4) + Strike (2)",
			MainStats:       "1: Strike DMG Bonus  •  2: Crit DMG  •  3: HP% > Atk%",
			SubStatPriority: "Atk% > Crit DMG > HP% > Crit Rate",
			Notes:           "Bruiser Assault — HP scales his giant strikes, but Atk% still leads.",
		},
		Rotations: []Rotation{
			{Name: "Giant Slam", Steps: []string{"Basic ×3", "Technique (giant arm)", "Ultimate (Bankai – Kokujo Tengen Myo'o)", "Battlefield Skill"}, Tags: []string{"Burst"}},
		},
	},

	"mayuri": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 2,
		BestStamp: BestStamp{
			SetName:         "Konjiki Ashisogi (4) + Spirit (2)",
			MainStats:       "1: Thrust DMG Bonus  •  2: Crit Rate  •  3: Atk%",
			SubStatPriority: "Ult Charge Rate > Atk% > Crit DMG > Crit Rate",
			Notes:           "S-tier debuffer/poison — poison ticks off Atk%, so stack it.",
		},
		Rotations: []Rotation{
			{Name: "Poison Setup", Steps: []string{"Technique (apply poison/decay)", "Basic ×3 (stack debuff)", "Ultimate (Bankai)", "Swap out — poison ticks off-field"}, Notes: "His best damage comes off-field — apply poison and rotate.", Tags: []string{"Sustain", "Boss"}},
		},
	},

	"momo": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 1,
		BestStamp: BestStamp{
			SetName:         "Tobiume (4) + Spirit (2)",
			MainStats:       "1: Spirit DMG Bonus  •  2: Crit Rate  •  3: Ult Charge Rate",
			SubStatPriority: "Ult Charge Rate > Atk% > Crit Rate > Crit DMG",
			Notes:           "Only SR Support that hits S — Ult Charge Rate is non-negotiable to keep her buff up.",
		},
		Rotations: []Rotation{
			{Name: "Hado Buff Loop", Steps: []string{"Front-line: Technique (Hado 31)", "Ultimate (team Spirit DMG buff)", "Swap to Aizen/Toshiro", "Return to refresh ult"}, Notes: "Best pair with Aizen — her bond gives him extra crit conversion.", Tags: []string{"Sustain"}},
		},
	},

	"nelliel": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 2,
		BestStamp: BestStamp{
			SetName:         "Gamuza (4) + Thrust (2)",
			MainStats:       "1: ATK%  •  2: Crit Rate  •  3: Ult Charge Rate",
			SubStatPriority: "Crit Rate > Ult Charge Rate > Atk% > Crit DMG > Ailment%",
			Notes:           "S-tier Tactician — applies vulnerability that amplifies the on-field DPS.",
		},
		Rotations: []Rotation{
			{Name: "Battlefield Setup", Steps: []string{"Open: Battlefield", "Technique", "Swap to support (Mayuri Ultimate then technique)", "Swap to DPS for the setup phase"}, Notes: "Her ultimate applies a typing shred — burst after using it.", Tags: []string{"Burst", "Boss"}},
		},
	},

	"nemu": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 0,
		BestStamp: BestStamp{
			SetName:         "Synthetic Body (4) + Atk (2)",
			MainStats:       "1: Atk%  •  2: Crit DMG  •  3: Atk%",
			SubStatPriority: "Atk% > Crit DMG > Crit Rate > HP%",
			Notes:           "Niche Support — only invest as a Mayuri pair.",
		},
		Rotations: []Rotation{
			{Name: "Backup Loop", Steps: []string{"Technique", "Ultimate (Mayuri synergy)", "Swap"}, Tags: []string{"Sustain"}},
		},
	},

	"orihime": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 0,
		BestStamp: BestStamp{
			SetName:         "Shun Shun Rikka (4) + Atk (2)",
			MainStats:       "1: Atk%  •  2: Crit Rate  •  3: HP% > Ult Charge Rate",
			SubStatPriority: "Ult Charge Rate > Atk% > HP% > Crit DMG",
			Notes:           "Healer / shield Support — Ult Charge keeps shields permanent.",
		},
		Rotations: []Rotation{
			{Name: "Shield Loop", Steps: []string{"Ultimate (Santen Kesshun shield)", "Technique (heal)", "Swap to DPS", "Return on shield cooldown"}, Tags: []string{"Sustain", "PvP"}},
		},
	},

	"rangiku": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 6,
		BestStamp: BestStamp{
			SetName:         "Haineko (4) + Thrust (2)",
			MainStats:       "1: Thrust DMG Bonus  •  2: Crit DMG  •  3: Atk%",
			SubStatPriority: "Crit DMG > Crit Rate > Atk% > Atk",
			Notes:           "Ash-cloud DPS Support — pairs with Toshiro for spirit-team synergy.",
		},
		Rotations: []Rotation{
			{Name: "Ash Field", Steps: []string{"Technique (Haineko cloud)", "Basic ×3", "Ultimate", "Swap"}, Tags: []string{"Sustain"}},
		},
	},

	"renji": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 5,
		BestStamp: BestStamp{
			SetName:         "Zabimaru (4) + Slash (2)",
			MainStats:       "1: Slash DMG Bonus  •  2: Crit DMG  •  3: Atk%",
			SubStatPriority: "Atk% > Crit DMG > Crit Rate > Atk",
			Notes:           "B-tier Assault — solid until Bankai/Bansho form drops.",
		},
		Rotations: []Rotation{
			{Name: "Whip Combo", Steps: []string{"Basic ×3", "Technique (whip extension)", "Ultimate", "Counter"}, Tags: []string{"Sustain"}},
		},
	},

	"rukia": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 5,
		BestStamp: BestStamp{
			SetName:         "Sode no Shirayuki (4) + Spirit (2)",
			MainStats:       "1: Spirit DMG Bonus  •  2: Crit DMG  •  3: Atk%",
			SubStatPriority: "Crit DMG > Crit Rate > Atk% > Ult Charge Rate",
			Notes:           "S-tier Tactician — freeze-control opens long burst windows.",
		},
		Rotations: []Rotation{
			{Name: "Freeze Burst", Steps: []string{"Open: Technique (Some no Mai)", "Basic ×3 (frost stacks)", "Ultimate (Tsugi no Mai – Hakuren)", "Battlefield Skill on frozen target"}, Notes: "Hold burst until enemy is frozen — damage on frozen targets is amplified.", Tags: []string{"Burst", "Boss"}},
		},
	},

	"soi-fon": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 1,
		BestStamp: BestStamp{
			SetName:         "Suzumebachi (4) + Strike (2)",
			MainStats:       "1: ATK% > Strike DMG Bonus  •  2: Crit DMG > ATK%  •  3: Atk%",
			SubStatPriority: "Crit DMG > Crit Rate > Ailment% > Atk% > Flat Atk",
			Notes:           "Two-hit execute Tactician — the second sting deletes bosses.",
		},
		Rotations: []Rotation{
			{Name: "Nigeki Kessatsu", Steps: []string{"Basic ×3 → Technique (mark target with Homonka)", "Swap out, let mark persist", "Swap back: Ultimate (second strike → execute)"}, Notes: "Mark must be applied first — DON'T fire Ult until you see the Homonka stack.", Tags: []string{"Burst", "Boss"}},
		},
	},

	"szayelaporro": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 2,
		BestStamp: BestStamp{
			SetName:         "Fornicarás (4) + Spirit (2)",
			MainStats:       "1: ATK%  •  2: Crit DMG > ATK%  •  3: ATK%",
			SubStatPriority: "Atk% > Flat ATK > Crit DMG > Crit Rate > Ultimate Charge Rate",
			Notes:           "Easiest unit to build. Minimum stats required: 5280 ATK and 175% Crit DMG.",
		},
		Rotations: []Rotation{
			{Name: "Debuff Cycle", Steps: []string{"Technique", "Swap to Tactical unit (Aizen)", "Get a manual counter", "Counter with Szayelaporro", "Use Ultimate"}, Tags: []string{"Sustain", "Boss"}},
		},
	},

	"tosen": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 1,
		BestStamp: BestStamp{
			SetName:         "Suzumushi (4) + Thrust (2)",
			MainStats:       "1: Thrust DMG Bonus  •  2: Crit DMG  •  3: Atk%",
			SubStatPriority: "Crit DMG > Crit Rate > Atk% > Atk",
			Notes:           "Stealth Tactician — burst on the silence window.",
		},
		Rotations: []Rotation{
			{Name: "Suzumushi Setup", Steps: []string{"Technique (silence / stealth)", "Basic ×3", "Ultimate (Bankai)", "Battlefield Skill"}, Notes: "His silence prevents enemy counters — burst your whole team inside it.", Tags: []string{"Burst", "PvP"}},
		},
	},

	"toshiro": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 2,
		BestStamp: BestStamp{
			SetName:         "Hyorinmaru (4) + Spirit (2)",
			MainStats:       "1: Spirit DMG Bonus  •  2: Crit DMG  •  3: Atk%",
			SubStatPriority: "Crit DMG > Crit Rate > Atk% > Flat Atk > Ult Charge Rate",
			Notes:           "S+ DPS — the spirit-team carry. Push Crit Rate to 100% (based on your weapon stamp) then dump into Crit DMG.",
		},
		Rotations: []Rotation{
			{
				Name: "Frost Burst",
				Steps: []string{
					"Szayel / Momo open with technique",
					"Swap Toshiro in",
					"Technique (Sennen Hyoro – frost stacks)",
					"Battlefield skill",
					"Manual Counter with Toshiro",
					"Spam basic attacks",
				},
				Notes: "Toshiro wants frost stacks applied before burst — Technique first, then dump.",
				Tags:  []string{"Burst", "Boss"},
			},
			{
				Name: "Sustained Frost",
				Steps: []string{"Basic combo → counter", "Refresh Technique on cooldown", "Battlefield Skill on every charge"},
				Tags:  []string{"Sustain"},
			},
		},
	},

	"ururu": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 5,
		BestStamp: BestStamp{
			SetName:         "Cannon (4) + Spirit (2)",
			MainStats:       "1: Spirit DMG Bonus  •  2: Crit DMG  •  3: Atk%",
			SubStatPriority: "Atk% > Crit DMG > Crit Rate > Atk",
			Notes:           "A-tier Support — cannon-damage scales off Atk.",
		},
		Rotations: []Rotation{
			{Name: "Cannon Cycle", Steps: []string{"Technique (cannon charge)", "Ultimate", "Swap"}, Tags: []string{"Sustain"}},
		},
	},

	"uryu": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 5,
		BestStamp: BestStamp{
			SetName:         "Quincy Bow (4) + Thrust (2)",
			MainStats:       "1: Thrust DMG Bonus  •  2: Crit DMG  •  3: Atk%",
			SubStatPriority: "Crit DMG > Crit Rate > Atk% > Atk",
			Notes:           "Ranged Tactician — arrow stacks scale with Crit DMG.",
		},
		Rotations: []Rotation{
			{Name: "Arrow Volley", Steps: []string{"Basic ×3 (build Reishi)", "Technique (Heilig Pfeil)", "Ultimate (Licht Regen)", "Counter"}, Tags: []string{"Sustain"}},
		},
	},

	"yachiru": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 5,
		BestStamp: BestStamp{
			SetName:         "Sanbe Kawaki (4) + Slash (2)",
			MainStats:       "1: Slash DMG Bonus  •  2: Atk%  •  3: Ult Charge Rate",
			SubStatPriority: "Ult Charge Rate > Atk% > Crit DMG > Crit Rate",
			Notes:           "A-tier Support — buffs adjacent assault chars (esp. Kenpachi).",
		},
		Rotations: []Rotation{
			{Name: "Buff Hop", Steps: []string{"Technique (buff applied)", "Ultimate", "Swap to Kenpachi"}, Notes: "She's basically a Kenpachi battery — pair them.", Tags: []string{"Sustain"}},
		},
	},

	"yoruichi": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 1,
		BestStamp: BestStamp{
			SetName:         "Shunko (4) + Strike (2)",
			MainStats:       "1: Strike DMG Bonus  •  2: Crit Rate  •  3: Atk%",
			SubStatPriority: "Crit DMG > Crit Rate > Atk% > Ult Charge Rate",
			Notes:           "S-tier Assault — high mobility, Shunko stance multiplies damage.",
		},
		Rotations: []Rotation{
			{Name: "Shunko Burst", Steps: []string{"Technique (Shunko on)", "Basic ×3 (rapid strikes)", "Ultimate", "Battlefield Skill", "Counter on incoming"}, Notes: "Burst window is short — pre-buff with Urahara before opening.", Tags: []string{"Burst", "Boss"}},
		},
	},
}

// ---------------------------------------------------------------------------
// Curated team comps. Composition principles, in priority order:
//   1. Newest SSRs anchor each comp (Aizen is the latest, so he headlines
//      multiple — most recent units tend to define the meta).
//   2. Damage-type cohesion where possible — Spirit/Slash/Thrust/Strike trios
//      get bond + set-stamp synergy. Mixed comps only when it strictly upgrades
//      tier (e.g. a S+ off-type carry beats an A on-type one).
//   3. Tier stacking — pair S+ with S+/S whenever the type allows.
//   4. Deep swap lists — every slot lists SSR > SR+ > SR alternates so SRs
//      (Momo, Ichigo-shikai, free units) always have a home.
//
// Tiers used here mirror the official wiki tier list (S+, S, A, B).

var teams = []Team{
	// 1. Top 3 per Hideoutgacha — all-Spirit apex. Toshiro carries, Aizen lays
	//    universal buffs/debuffs, Szayelaporro stacks Spirit/Ailment debuffs.
	//    Momo (FREE SR Spirit Support) is the canonical F2P swap.
	{
		ID:         "spirit-apex",
		Name:       "Spirit Apex — Toshiro / Aizen / Szayelaporro",
		Rank:       3,
		Tier:       "S",
		Difficulty: "Medium",
		Content:    "Boss / Trial Tower / Co-op",
		Members: []TeamMember{
			{Slug: "toshiro", Name: "Toshiro Hitsugaya", Role: "Assault", DamageType: "Spirit",
				Notes: []string{
					"S+ Spirit DPS — main damage",
					"Frost stack detonator inside Aizen's pressure window",
				},
				Swappable: []string{"ichigo-bankai", "rukia"}},
			{Slug: "aizen", Name: "Sosuke Aizen", Role: "Tactician", DamageType: "Spirit",
				Notes: []string{
					"S+ Tactician — newest SSR, team-wide ATK + Ravage debuff",
					"Bond gives Toshiro extra crit conversion",
				},
				Swappable: []string{"rukia", "byakuya"}},
			{Slug: "szayelaporro", Name: "Szayelaporro Granz", Role: "Support", DamageType: "Spirit",
				Notes: []string{
					"SSR Spirit Support — Ailment-Resistance shred + team Spirit DMG buff",
					"Espada bond pairs into the Spirit trio's debuff window",
				},
				Swappable: []string{"momo", "kisuke", "orihime", "ururu"}},
		},
		RotationP2W:   "Szayelaporro (SSR) Ailment-Res shred → Aizen pressure & team ATK buff → Toshiro Bankai burst inside both debuff windows. Triple-SSR all-Spirit; highest Spirit ceiling per Hideoutgacha's Top 3.",
		RotationF2P:   "Sub Momo (FREE SR Spirit) for Szayelaporro — same Spirit trio, Momo's element buff replaces Szayel's debuff. Toshiro and Aizen remain required SSRs; everything else has a free swap.",
	},

	// 2. All-Slash — S+ Slash DPS (Ichigo Bankai) + S Slash Tactician (Byakuya) +
	//    S+ Slash Support (Kisuke). Four Slash members in the swap pool.
	{
		ID:         "slash-storm",
		Name:       "Slash Storm — Bankai / Byakuya / Kisuke",
		Rank:       2,
		Tier:       "S",
		Difficulty: "Easy",
		Content:    "Boss / Story / Co-op",
		Members: []TeamMember{
			{Slug: "ichigo-bankai", Name: "Ichigo Kurosaki (Bankai)", Role: "Assault", DamageType: "Slash",
				Notes: []string{
					"S+ Slash DPS — Getsuga burst",
					"Carries buffs through swap-in",
				},
				Swappable: []string{"toshiro", "ichigo-shikai", "ichigo-initial", "renji"}},
			{Slug: "byakuya", Name: "Byakuya Kuchiki", Role: "Tactician", DamageType: "Slash",
				Notes: []string{
					"S Slash Tactician — petal field amps Slash damage",
					"Damage persists off-field",
				},
				Swappable: []string{"rukia", "tosen"}},
			{Slug: "kisuke", Name: "Kisuke Urahara", Role: "Support", DamageType: "Slash",
				Notes: []string{
					"S+ Slash Support — team ATK% buff (best support in game)",
					"Slash element matches both DPS and Tactician",
				},
				Swappable: []string{"yachiru", "momo", "orihime"}},
		},
		RotationP2W:   "Kisuke (SSR) team ATK% buff → Byakuya petal field → Ichigo Bankai Ult inside both windows. Pure all-SSR Slash trio.",
		RotationF2P:   "Swap Kisuke for Momo (FREE SR) or Yachiru (SR+) for the Support slot — buff Ichigo's Bankai before the petal field detonates. Still all-Slash on the front-line; you just lose the team-wide ATK% ceiling.",
	},

	// 3. Top 1 per Hideoutgacha — all-Thrust apex. Gin's Impale + Snake Venom
	//    DoT, Nelliel covers counters/heal/shield + 80% Crit DMG team buff, Mayuri
	//    amps Ailment DMG and stacks team Mastery debuffs.
	{
		ID:         "thrust-pierce",
		Name:       "Pierce Lance — Gin / Nelliel / Mayuri",
		Rank:       1,
		Tier:       "S",
		Difficulty: "Hard",
		Content:    "Co-op / Trial Tower / Frenzy Fest / Events",
		Members: []TeamMember{
			{Slug: "gin", Name: "Gin Ichimaru", Role: "Assault", DamageType: "Thrust",
				Notes: []string{
					"S Thrust DPS — continuous damage via Impale & Snake Venom",
					"Self-buff loop, fastest rotation in the game",
				},
				Swappable: []string{"ikkaku", "uryu"}},
			{Slug: "nelliel", Name: "Nelliel Tu Odelschwanck", Role: "Tactician", DamageType: "Thrust",
				Notes: []string{
					"S Thrust Tactician — counter mechanic, shield & heal",
					"Team Ailment-Res shred + 80% team Crit DMG via Cero Sincretico",
				},
				Swappable: []string{"tosen", "uryu", "soi-fon"}},
			{Slug: "mayuri", Name: "Kurotsuchi Mayuri", Role: "Support", DamageType: "Thrust",
				Notes: []string{
					"S Thrust Support — team Ailment DMG + Ailment Mastery buff",
					"+20% team Damage Dealt and All-Skills Mastery during his window",
				},
				Swappable: []string{"szayelaporro", "rangiku", "nemu"}},
		},
		RotationP2W:   "Maintain Gin's poison stacks → time Nelliel's Cero Sincretico inside Mayuri's debuff window → Gin Kamishini no Yari on the stacked-debuff target. Triple-SSR all-Thrust, Hideoutgacha's Top 1.",
		RotationF2P:   "Sub Nemu (FREE SR Strike) or Rangiku (SR Thrust) for Mayuri — keep Gin + Nelliel as the core; you lose Mayuri's team-wide Ailment amp but Nelliel's Crit DMG buff still carries the burst. Tosen (SSR) or Uryu (SR) sub the Tactician slot if Nelliel is missing.",
	},

	// 4. Top 2 per Hideoutgacha — Soi Fon's two-hit execute paired with the
	//    Pantera form's Resurrección burst, with Mayuri's Ailment amp covering
	//    the Strike pair off-element.
	{
		ID:         "strike-rush",
		Name:       "Strike Apex — Soi Fon / Grimmjow (Pantera) / Mayuri",
		Rank:       2,
		Tier:       "S",
		Difficulty: "Hard",
		Content:    "Boss / Trial Tower / PvP",
		Members: []TeamMember{
			{Slug: "soi-fon", Name: "Soi Fon", Role: "Tactician", DamageType: "Strike",
				Notes: []string{
					"S Strike Tactician — Homonka mark + Nigeki Kessatsu execute",
					"Setup for Grimmjow's Resurrección burst window",
				},
				Swappable: []string{"yoruichi", "komamura"}},
			{Slug: "grimmjow-pantera", Name: "Grimmjow (Pantera)", Role: "Assault", DamageType: "Strike",
				Notes: []string{
					"SSR Strike Assault — Resurrección rage burst",
					"Espada faction bond with Mayuri amplifies the kill window",
				},
				Swappable: []string{"grimmjow", "yoruichi", "kenpachi"}},
			{Slug: "mayuri", Name: "Kurotsuchi Mayuri", Role: "Support", DamageType: "Thrust",
				Notes: []string{
					"S Thrust Support — Ailment DMG + Mastery team buff",
					"Off-element but the +20% team Damage Dealt scales any DPS",
				},
				Swappable: []string{"szayelaporro", "nemu", "rangiku"}},
		},
		RotationP2W:   "Mayuri team Ailment-DMG buff → Soi Fon Homonka mark → Grimmjow-Pantera Resurrección rage burst on the marked + debuffed target → Soi Fon return swap fires Nigeki Kessatsu for the execute. Triple-SSR, Hideoutgacha's Top 2.",
		RotationF2P:   "Sub Nemu (FREE SR Strike) for Mayuri to go pure-Strike — you lose Mayuri's Ailment amp but keep Strike-element bonuses. Grimmjow-base (SR) can replace the Pantera form if you haven't pulled the SSR — Soi Fon and the rotation order stay the same.",
	},

	// 5. Top-tier mixed — three S+ together. Off-type but absolute ceiling power.
	//    Used when "best chars" matters more than element synergy.
	{
		ID:         "trinity-splus",
		Name:       "Trinity Apex — Triple S+",
		Rank:       5,
		Tier:       "S",
		Difficulty: "Hard",
		Content:    "End-game / Whale comp",
		Members: []TeamMember{
			{Slug: "ichigo-bankai", Name: "Ichigo Kurosaki (Bankai)", Role: "Assault", DamageType: "Slash",
				Notes: []string{
					"S+ Slash DPS — highest single-target ceiling",
					"Swap to Toshiro for an all-Spirit shift",
				},
				Swappable: []string{"toshiro"}},
			{Slug: "aizen", Name: "Sosuke Aizen", Role: "Tactician", DamageType: "Spirit",
				Notes: []string{
					"S+ newest SSR — universal buff/debuff",
					"Off-element but the buff scales any damage type",
				}},
			{Slug: "kisuke", Name: "Kisuke Urahara", Role: "Support", DamageType: "Slash",
				Notes: []string{
					"S+ Support — team ATK% (matches Ichigo's Slash too)",
					"Best generic support in the game",
				},
				Swappable: []string{"momo"}},
		},
		RotationP2W:   "Kisuke team ATK% → Aizen Ravage debuff + team buff → Ichigo Bankai Getsuga burst. Three S+ SSRs stacked — highest pure ceiling in the game.",
		RotationF2P:   "This comp REQUIRES all three SSRs — there is no F2P version. If even one is missing, run Spirit Apex (#1) or Slash Storm (#2) instead, which both have free-unit substitutes for the Support slot.",
	},

	// 6. Top 4 per Hideoutgacha — Kenpachi anchors with Nelliel's heal/shield
	//    keeping him alive while Mayuri stacks Ailment debuffs. Slash + Thrust
	//    mix; off-element but the support pair amps any DPS.
	{
		ID:         "kenpachi-bruiser",
		Name:       "Bruiser Bash — Kenpachi / Nelliel / Mayuri",
		Rank:       4,
		Tier:       "S",
		Difficulty: "Medium",
		Content:    "Boss / Story / Co-op",
		Members: []TeamMember{
			{Slug: "kenpachi", Name: "Kenpachi Zaraki", Role: "Assault", DamageType: "Slash",
				Notes: []string{
					"S Slash Assault — scales with missing HP",
					"Tank-DPS; Nelliel's shield/heal lets him sit low-HP safely",
				},
				Swappable: []string{"ichigo-bankai", "grimmjow-pantera", "yoruichi"}},
			{Slug: "nelliel", Name: "Nelliel Tu Odelschwanck", Role: "Tactician", DamageType: "Thrust",
				Notes: []string{
					"S Thrust Tactician — counter, shield, heal",
					"80% team Crit DMG via Cero Sincretico amplifies Kenpachi's burst",
				},
				Swappable: []string{"soi-fon", "byakuya", "aizen"}},
			{Slug: "mayuri", Name: "Kurotsuchi Mayuri", Role: "Support", DamageType: "Thrust",
				Notes: []string{
					"S Thrust Support — Ailment DMG + +20% team Damage Dealt",
					"Espada bond into Nelliel completes the buff line",
				},
				Swappable: []string{"yachiru", "szayelaporro", "orihime", "momo"}},
		},
		RotationP2W:   "Mayuri Ailment amp → Nelliel Cero Sincretico (team Crit DMG + Res shred) → Kenpachi lets HP drop, then bursts inside both windows. Triple-SSR, Hideoutgacha's Top 4.",
		RotationF2P:   "Sub Yachiru (SR Slash) or Orihime (FREE SR) for Mayuri — Yachiru's adjacent-Assault buff keeps Kenpachi scaling, Orihime adds shield/heal stacking with Nelliel. Kenpachi + Nelliel are the required SSRs.",
	},

	// 7. Counter Stall PvP — newest SSR (Aizen) abusing his B4 swap-in counter
	//    with the wiki's #1 counter-Assault (Ikkaku) and stall Support (Orihime).
	{
		ID:         "counter-stall",
		Name:       "Counter Stall — Aizen Backline",
		Rank:       7,
		Tier:       "S",
		Difficulty: "Hard",
		Content:    "PvP / Trial Tower",
		Members: []TeamMember{
			{Slug: "aizen", Name: "Sosuke Aizen", Role: "Tactician", DamageType: "Spirit",
				Notes: []string{
					"S+ newest SSR — keep him in backline",
					"B4 fires a counter every time he swaps in (20s CD)",
				}},
			{Slug: "ikkaku", Name: "Ikkaku Madarame", Role: "Assault", DamageType: "Thrust",
				Notes: []string{
					"S Counter Assault — stance scales with counters",
					"Pair perfectly with Aizen swap-in counter chain",
				},
				Swappable: []string{"gin", "yoruichi"}},
			{Slug: "orihime", Name: "Orihime Inoue", Role: "Support", DamageType: "Spirit",
				Notes: []string{
					"A Spirit Support — Santen Kesshun shield + heal",
					"Spirit matches Aizen",
				},
				Swappable: []string{"momo", "kisuke", "ururu"}},
		},
		RotationP2W:   "All three SSRs only. Orihime shields → Ikkaku counter-baits → Aizen swap-in counter (B4) → backline BFS at Complete Suppression. Highest counter ceiling.",
		RotationF2P:   "Sub Momo (FREE SR Spirit) for Orihime if Shun Shun Rikka isn't built — lose the shield but gain Momo's Spirit-DMG buff for Aizen. Ikkaku and Aizen are required, no SR alternative exists for the counter chain.",
	},

	// 8. Frost Lockdown — Spirit-leaning A-tier comp with strong SR/free bench.
	{
		ID:         "frost-control",
		Name:       "Frost Lockdown — Rukia Freeze",
		Rank:       8,
		Tier:       "A",
		Difficulty: "Medium",
		Content:    "PvP / Boss",
		Members: []TeamMember{
			{Slug: "rukia", Name: "Rukia Kuchiki", Role: "Tactician", DamageType: "Spirit",
				Notes: []string{
					"S Spirit Tactician — freeze applier",
					"Frozen target = burst window for any Spirit DPS",
				},
				Swappable: []string{"aizen", "byakuya"}},
			{Slug: "toshiro", Name: "Toshiro Hitsugaya", Role: "Assault", DamageType: "Spirit",
				Notes: []string{
					"S+ Spirit DPS — bonus damage on frozen",
					"Frost stack synergy with Rukia",
				},
				Swappable: []string{"ichigo-bankai"}},
			{Slug: "momo", Name: "Momo Hinamori", Role: "Support", DamageType: "Spirit",
				Notes: []string{
					"FREE SR Spirit Support — pure-element comp",
					"Sub Kisuke for higher ceiling, Momo for F2P",
				},
				Swappable: []string{"kisuke", "szayelaporro", "orihime", "ururu"}},
		},
		RotationP2W:   "Kisuke (SSR) ATK% buff → Rukia freeze → Toshiro Bankai burst on frozen target. All-SSR (Toshiro + Rukia) Spirit comp with Kisuke ATK ceiling.",
		RotationF2P:   "Momo (FREE SR Spirit Support) replaces Kisuke for a pure-Spirit all-element comp. Momo buff → Rukia freeze → Toshiro burst — Toshiro is the only required SSR.",
	},

	// 9. Espada Hollow Faction — faction-bond comp using the SSR Pantera form.
	{
		ID:         "espada-hollow",
		Name:       "Espada Hollow — Pantera Faction",
		Rank:       9,
		Tier:       "A",
		Difficulty: "Medium",
		Content:    "Boss / Trial Tower",
		Members: []TeamMember{
			{Slug: "grimmjow-pantera", Name: "Grimmjow (Pantera)", Role: "Assault", DamageType: "Strike",
				Notes: []string{
					"SSR Strike Assault — Resurrección burst",
					"Espada faction anchor",
				},
				Swappable: []string{"grimmjow", "yoruichi", "kenpachi"}},
			{Slug: "nelliel", Name: "Nelliel Tu Odelschwanck", Role: "Tactician", DamageType: "Thrust",
				Notes: []string{
					"S Thrust Tactician — Espada bond + vulnerability",
				},
				Swappable: []string{"aizen", "soi-fon"}},
			{Slug: "szayelaporro", Name: "Szayelaporro Granz", Role: "Support", DamageType: "Spirit",
				Notes: []string{
					"S Spirit Support — Espada debuff Support",
					"Espada faction bond completes the trio",
				},
				Swappable: []string{"mayuri", "nemu", "momo"}},
		},
		RotationP2W:   "Szayelaporro (SSR) Espada debuff → Nelliel (SSR) lance vulnerability → Grimmjow-Pantera (SSR) Resurrección burst on the double-debuffed target. Triple-SSR Espada faction bond.",
		RotationF2P:   "Sub Mayuri (SSR Thrust) for Szayelaporro to keep the SSR debuff line if Szayel is missing, or fall back to Nemu (FREE SR Strike) — you lose Espada faction bond but Nemu still pairs with Grimmjow's Strike element. Grimmjow-base (SR) replaces the Pantera form.",
	},

	// 10. F2P Story Starter — exclusively SR / free / story-given units. Every
	//     slot lists clear SSR upgrades as the player pulls them.
	{
		ID:         "f2p-starter",
		Name:       "F2P Starter — Story Friendly",
		Rank:       10,
		Tier:       "B",
		Difficulty: "Easy",
		Content:    "Early-game story",
		Members: []TeamMember{
			{Slug: "ichigo-shikai", Name: "Ichigo Kurosaki (Shikai)", Role: "Assault", DamageType: "Slash",
				Notes: []string{
					"Story-given starter — Slash Assault",
					"Upgrade to Ichigo (Bankai) once pulled",
				},
				Swappable: []string{"ichigo-initial", "renji", "chad", "ichigo-bankai"}},
			{Slug: "rukia", Name: "Rukia Kuchiki", Role: "Tactician", DamageType: "Spirit",
				Notes: []string{
					"Accessible Spirit Tactician — freeze utility",
					"One of the best F2P-friendly Tacticians",
				},
				Swappable: []string{"uryu", "tosen", "byakuya", "aizen"}},
			{Slug: "momo", Name: "Momo Hinamori", Role: "Support", DamageType: "Spirit",
				Notes: []string{
					"FREE SR Support — pairs with Rukia for Spirit synergy",
					"Sub in Orihime for shield, Kisuke for ATK% buff",
				},
				Swappable: []string{"orihime", "ururu", "nemu", "kisuke", "yachiru"}},
		},
		RotationP2W:   "This comp is intentionally SR-only — the P2W path is to retire it and run any of #1–#5. As you pull SSRs, swap into Spirit Apex (#1, replace Ichigo-shikai with Ichigo-Bankai, Momo with Aizen, etc.) or Slash Storm (#2).",
		RotationF2P:   "Momo (FREE SR) Spirit-DMG buff → Rukia (SR) freeze → Ichigo-shikai (story-given) Getsuga burst on the frozen target. Every slot is free or pity-able — runnable from day 1.",
	},
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
		Name:      "Grimmjow (Pantera) Rate-Up",
		Slugs:     []string{"grimmjow-pantera"},
		Weapon:    "Pantera",
		StartDate: "2026-05-14",
		EndDate:   "2026-06-04",
		Note:      "Resurrección Grimmjow's debut with his SSR Pantera form. Triple-SSR Strike comp with Soi Fon and Mayuri, plus the new Pantera weapon.",
		Source:    "https://bleach-soul-resonance-esp.fandom.com/es/wiki/Eventos_China",
	},
	// ---------------------------------------------------------------------
	// UPCOMING — fill these out as new banners are announced. Add the
	// character slug(s), display name, the signature weapon, and the
	// start/end dates (YYYY-MM-DD). Delete the placeholder entries you
	// don't need. Each entry is shown on the /upcoming page in order.
	Upcoming: []Banner{
		{
			Name:      "Ichigo Kurosaki・Inner Hollow Rate-Up", // e.g. "Tōshirō Hitsugaya Rate-Up"
			Slugs:     []string{""}, // e.g. "toshiro"
			Weapon:    "Zangetsu", // e.g. "Zangetsu"
			StartDate: "2026-06-05", // YYYY-MM-DD
			EndDate:   "2026-06-26", // YYYY-MM-DD
			Note:      "First limited Slash Tactic SSR. White debuts in his Shikai and Bankai forms.",
		},
		{
			Name:      "Ulquiorra Shifar (Base) SR+ Free Unit by Mail",
			Slugs:     []string{""},
			Weapon:    "Murciélago",
			StartDate: "2026-06-05",
			EndDate:   "2026-06-05",
			Note:      "Base form Ulquiorra, and will be free via mail on the 5th.",
		},
		{
			Name:      "Ulquiorra Shifar (Resurrección) SSR",
			Slugs:     []string{""},
			Weapon:    "Murciélago",
			StartDate: "2026-06-27",
			EndDate:   "2026-07-18",
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
	Leaks: []Leak{
		{
			Name:       "", // e.g. "Ulquiorra Cifer"
			Role:       "", // e.g. "Full Assault / DPS"
			DamageType: "", // e.g. "Spirit"
			Rarity:     "", // e.g. "SSR"
			Note:       "",
		},
		{
			Name:       "",
			Role:       "",
			DamageType: "",
			Rarity:     "",
			Note:       "",
		},
	},
}

// ---------------------------------------------------------------------------
// Merge & write.

// synthesizeBuilds derives a Build[] from the curated BestStamp + boundaries.
// Strategy: the BestStamp.MainStats string is hand-written as
//   "1: <stat>  •  2: <stat>  •  3: <stat>"
// which we split into BuildSetPiece entries. The set name keeps the full
// "<4pc> + <2pc>" composition. We emit two labelled builds when the P2W and
// F2P stop-boundaries differ, so the UI can show distinct ceilings.
func synthesizeBuilds(stamp BestStamp, p2w, f2p int) []map[string]any {
	if stamp.SetName == "" && stamp.MainStats == "" && stamp.SubStatPriority == "" {
		return []map[string]any{}
	}
	pieces := parseMainStats(stamp.MainStats)

	mkBuild := func(label string, boundary int) map[string]any {
		b := map[string]any{
			"label":         label,
			"setName":       stamp.SetName,
			"setPieces":     pieces,
			"subStatPriority": stamp.SubStatPriority,
			"passiveSkills": []string{},
		}
		if boundary > 0 {
			b["range"] = fmt.Sprintf("B0W1C1 - B%dW5C5", boundary)
		}
		return b
	}

	// If the two boundaries are the same, one "Main" build is enough.
	if p2w == f2p || f2p == 0 {
		return []map[string]any{mkBuild("Main", p2w)}
	}
	return []map[string]any{
		mkBuild("P2W (value ceiling)", p2w),
		mkBuild("F2P (practical stop)", f2p),
	}
}

// parseMainStats turns a string like
//   "1: Spirit DMG Bonus > Atk%  •  2: Crit Rate > Crit DMG  •  3: Atk%"
// into typed BuildSetPiece entries. Falls back to slot indices when the
// numeric prefix is missing.
func parseMainStats(in string) []map[string]any {
	if in == "" {
		return []map[string]any{}
	}
	// Split on the bullet glyph the BestStamp strings use to delimit slots.
	// We deliberately don't split on "/" — that's used inside individual
	// stat names like "Crit Rate / Crit DMG".
	parts := strings.Split(in, "•")
	pieces := make([]map[string]any, 0, len(parts))
	for idx, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		slot := idx + 1
		stat := p
		// Strip a leading "<n>:" prefix if present.
		if colon := strings.Index(p, ":"); colon > 0 && colon < 4 {
			prefix := strings.TrimSpace(p[:colon])
			if n := atoi(prefix); n > 0 {
				slot = n
			}
			stat = strings.TrimSpace(p[colon+1:])
		}
		pieces = append(pieces, map[string]any{
			"slot":     slot,
			"mainStat": stat,
		})
	}
	return pieces
}

// atoi parses a small positive int; returns 0 on failure.
func atoi(s string) int {
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			return 0
		}
		n = n*10 + int(r-'0')
	}
	return n
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

	merged := 0
	missing := []string{}
	slugs := make([]string, 0, len(curation))
	for s := range curation {
		slugs = append(slugs, s)
	}
	sort.Strings(slugs)

	for _, slug := range slugs {
		c := curation[slug]
		path := filepath.Join(dataDir, slug+".json")
		raw, err := os.ReadFile(path)
		if err != nil {
			missing = append(missing, slug)
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal(raw, &obj); err != nil {
			fmt.Printf("  ! %s: parse: %v\n", slug, err)
			continue
		}
		// Stop-boundary recommendations (P2W = value ceiling, F2P = practical stop).
		p2w := c.StopBoundaryP2W
		if p2w == 0 {
			p2w = c.StopBoundary // back-compat
		}
		// F2P=0 is meaningful ("don't dupe-invest, use base unit") for niche
		// legacy SSRs (komamura, tosen) — do NOT fall back to P2W here.
		f2p := c.StopBoundaryF2P
		obj["recommendedStopBoundary"] = p2w // legacy field, mirrors P2W
		obj["recommendedStopBoundaryP2W"] = p2w
		obj["recommendedStopBoundaryF2P"] = f2p
		obj["bestStamp"] = c.BestStamp
		// Synthesize a `builds[]` entry from the curated bestStamp when the
		// scraper left it empty OR previously synthesized (= no scraper-provided
		// passiveSkills on any entry). This guarantees every character renders
		// the Builds & Stamps tab with sensible, boundary-aware recommendations
		// and lets us refresh synthesized entries on subsequent curate runs.
		existingBuilds, _ := obj["builds"].([]any)
		hasScraperBuild := false
		for _, b := range existingBuilds {
			if m, ok := b.(map[string]any); ok {
				if ps, ok := m["passiveSkills"].([]any); ok && len(ps) > 0 {
					hasScraperBuild = true
					break
				}
			}
		}
		if !hasScraperBuild {
			obj["builds"] = synthesizeBuilds(c.BestStamp, p2w, f2p)
		}
		// Rarity: rarityBySlug is the authoritative source. Always override
		// whatever the scraper wrote — the wiki pages have inconsistent SR+/SR
		// markers and the map is hand-curated against Hideoutgacha.
		if r, ok := rarityBySlug[slug]; ok {
			obj["rarity"] = r
		}
		// Tier: tierBySlug is the authoritative tier-list ranking. Always
		// override — the scraper has no tier data; edit the map to re-rank.
		if t, ok := tierBySlug[slug]; ok {
			obj["tier"] = t
		}
		// Role normalization: the wiki labels Toshiro and Ichigo (Bankai) as
		// "Full Assault/DPS" but mechanically that's identical to Assault.
		// Collapse the redundant label so filters and counts work cleanly.
		if rs, ok := obj["role"].(string); ok && rs == "Assault" {
			obj["role"] = "Full Assault / DPS"
		}
		// Release date is now written directly by the scraper (extracted from
		// the Spanish wiki's "Release Date" template field, Global value). No
		// override needed here.
		// Convert rotations to []map for consistent JSON output (omit empty tags/notes).
		rots := make([]map[string]any, 0, len(c.Rotations))
		for _, r := range c.Rotations {
			m := map[string]any{"name": r.Name, "steps": r.Steps}
			if r.Notes != "" {
				m["notes"] = r.Notes
			}
			if len(r.Tags) > 0 {
				m["tags"] = r.Tags
			}
			rots = append(rots, m)
		}
		obj["rotations"] = rots

		out, err := json.MarshalIndent(obj, "", "  ")
		if err != nil {
			fmt.Printf("  ! %s: marshal: %v\n", slug, err)
			continue
		}
		if err := os.WriteFile(path, out, 0644); err != nil {
			fmt.Printf("  ! %s: write: %v\n", slug, err)
			continue
		}
		merged++
		fmt.Printf("  + %s (P2W B%d / F2P B%d, %d rotations)\n", slug, p2w, f2p, len(c.Rotations))
	}

	// Write Teams.json.
	teamsPath := filepath.Join(dataDir, "Teams.json")
	teamsOut, err := json.MarshalIndent(teams, "", "  ")
	if err != nil {
		panic(err)
	}
	if err := os.WriteFile(teamsPath, teamsOut, 0644); err != nil {
		panic(err)
	}

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

	fmt.Printf("\nMerged curation into %d/%d characters.\n", merged, len(curation))
	if len(missing) > 0 {
		fmt.Printf("Missing JSON for: %v\n", missing)
	}
	fmt.Printf("Wrote %d teams to %s\n", len(teams), teamsPath)
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
