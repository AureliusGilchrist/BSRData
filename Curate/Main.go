// Generates the aggregate Teams.json and Banners.json (and regenerates
// Index.json) for the site. Run with:
//
//   cd "Data Source/curate" && go run .
//
// NOTE: this program no longer merges any curated fields into the per-character
// ../Data/<slug>.json files — those are owned solely by the scraper (which only
// creates JSON for NEW characters). Curate is limited to the aggregate
// Teams.json / Banners.json plus rebuilding Index.json from whatever
// per-character JSON already exists. Team/banner recommendations are
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
	// characters). Curate is limited to the aggregate Teams.json / Banners.json
	// and regenerating Index.json from whatever per-character JSON exists.

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
