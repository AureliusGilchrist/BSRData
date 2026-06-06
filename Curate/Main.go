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

// Boundary is one of a character's six Boundary unlocks. The curated
// boundariesBySlug map below OVERRIDES the scraped boundary text in each JSON
// so all six always render with consistent wording. Icons are not listed here
// — they're derived from the slug + level ("/Images/<slug>/boundary-<n>.png").
type Boundary struct {
	// Boundary level, 1..6.
	Level int `json:"level"`
	// Optional title shown above the description (B3/B5 are usually unnamed).
	Name string `json:"name,omitempty"`
	// Effect text shown on the character page.
	Description string `json:"description"`
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
	"ichigo-white": "SSR", "ulquiorra-resurreccion": "SSR",

	"grimmjow": "SR+",
	"ulquiorra": "SR+",

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
	"ichigo-white": "S",
	"ulquiorra-resurreccion": "S",

	"kenpachi":     	"S",
	"grimmjow":         "A",
	"ichigo-bankai":    "S",
	"tosen":            "A",

	"yachiru":          "B",
	"momo":             "B",
	"rangiku":          "B",
	"yoruichi": 		"B",
	"ulquiorra":        "B",

	"ikkaku":   		"C",
	"byakuya":        	"C",
	"rukia":            "C",
	"komamura": 		"C",
	"ichigo-shikai":  	"C",

	"chad":           	"D",
	"ichigo-initial": 	"D",
	"orihime":        	"D",
	"nemu":           	"D",
	"renji":          	"D",
	"ururu":          	"D",
	"uryu":           	"D",
}

// boundariesBySlug is the authoritative, hand-curated Boundary text for every
// character. When a slug is present here it OVERRIDES whatever the scraper
// wrote into <slug>.json so all six boundaries always render with consistent
// wording. Each entry's icon is derived from the slug + level, so only the
// level / name / description are listed. Edit the text here, then re-run curate
// to push it into the JSONs (and Index regen). Empty descriptions are
// placeholders — fill them in as the wiki/data surfaces the effect text.
var boundariesBySlug = map[string][]Boundary{
	// --- New characters: empty placeholders until the wiki surfaces text. ---
	"ichigo-white": {
		{Level: 1, Description: ""},
		{Level: 2, Description: ""},
		{Level: 3, Description: ""},
		{Level: 4, Description: ""},
		{Level: 5, Description: ""},
		{Level: 6, Description: ""},
	},
	"ulquiorra": {
		{Level: 1, Description: ""},
		{Level: 2, Description: ""},
		{Level: 3, Description: ""},
		{Level: 4, Description: ""},
		{Level: 5, Description: ""},
		{Level: 6, Description: ""},
	},
	"ulquiorra-resurreccion": {
		{Level: 1, Description: ""},
		{Level: 2, Description: ""},
		{Level: 3, Description: ""},
		{Level: 4, Description: ""},
		{Level: 5, Description: ""},
		{Level: 6, Description: ""},
	},
	"aizen": {
		{Level: 1, Name: "Perception of Power", Description: "Enters Complete Suppression upon entering the battle. When Aizen releases a backline Battlefield Skill while under Complete Suppression, he gains 50% Battlefield Skill Energy."},
		{Level: 2, Name: "As Expected", Description: "All characters in the team deal 20% more Spirit DMG."},
		{Level: 3, Description: "Increases Level of Basic Attack, Technique, Ultimate, Counterattack, and Battlefield Skill by 2."},
		{Level: 4, Name: "Don't Let Me Down", Description: "Aizen's basic attacks, techniques, and counterattacks ignore 50% of the enemy's DEF. Each stack of Ravage applied on the enemy further increases 5% of DEF ignored. When Aizen is switched in through a backline Battlefield Skill, he can release a counterattack. This effect lasts until Aizen is switched to the backline again. Cooldown: 20 second(s). Increases the cap of Ravage to 3 stacks."},
		{Level: 5, Description: "Increases Level of Basic Attack, Technique, Ultimate, Counterattack and Battlefield Skill by 2."},
		{Level: 6, Name: "Under Control", Description: "Upon releasing a Battlefield Skill, the current Crit Rate is multiplied by 2. Any excess Crit Rate converts into Crit DMG for that attack at a 1:1 ratio."},
	},
	"byakuya": {
		{Level: 1, Description: "When Byakuya releases the Ultimate, he instantly gains 30% Spiritual Pressure."},
		{Level: 2, Description: "When in the Bankai state, each time Byakuya uses a technique, he will gain 1 stack of Determination based on the number of successful hits landed. Each stack of Determination increases all of Byakuya's damage by 2% for 12 seconds and stacks up to 14 times. When Determination reaches more than 8 stacks, Byakuya gains an additional 40% Critical DMG."},
		{Level: 3, Description: "Increases Level of Basic Attack, Technique, Ultimate, Counterattack, and Battlefield Skill by 2."},
		{Level: 4, Description: "Techniques deal 100% more damage."},
		{Level: 5, Description: "Increases Level of Basic Attack, Technique, Ultimate, Counterattack, and Battlefield Skill by 2."},
		{Level: 6, Description: "When Byakuya uses his technique in the Bankai state, his Spiritual Pressure will be consumed 50% slower."},
	},
	"chad": {
		{Level: 1, Description: ""},
		{Level: 2, Description: ""},
		{Level: 3, Description: ""},
		{Level: 4, Description: ""},
		{Level: 5, Description: ""},
		{Level: 6, Description: ""},
	},
	"gin": {
		{Level: 1, Name: "Devouring Mouth", Description: "Gains 3 Technique Points immediately when entering battle. Fully restores Technique Points after releasing the Ultimate."},
		{Level: 2, Name: "Manhunt", Description: "For every 1 Technique Point consumed, the Crit DMG of the next Ultimate is increased by 35%, stacking up to 6 times. Hitting the enemy with the final strike of the Basic Attack reduces Technique Points recovery time by 2 seconds."},
		{Level: 3, Description: ""},
		{Level: 4, Name: "Icy Demeanor", Description: "Impale DMG increases by 100%."},
		{Level: 5, Description: ""},
		{Level: 6, Name: "Cell Dissolution", Description: "The Ultimate inflicts Snake Venom on the enemy, dealing DMG equal to 50% of ATK every second. Snake Venom explodes after 12 seconds, dealing further DMG equal to 40% of the total DMG dealt by Gin during this 12-second period, up to 2500× of Gin's current ATK. While charging a technique, an additional wide-area Thrust attack is released without interrupting the charge (up to 600% of ATK based on charge time)."},
	},
	"grimmjow": {
		{Level: 1, Name: "Total Annihilation", Description: "Technique can be released 1 additional time. After Grimmjow releases his Ultimate and enters Form of Destruction, switching him to the backline will stop Ultimate Energy from draining. Each point of Spiritual Pressure Reserve doubles Grimmjow's Strike DMG bonus."},
		{Level: 2, Name: "Battle Thirst", Description: "The Ultimate, Gran Rey Cero, and Enhanced Basic Attack, Form of Destruction, ignore 60% of the enemy's DEF."},
		{Level: 3, Description: ""},
		{Level: 4, Name: "Strongest Cero", Description: "Increases the damage dealt by Grimmjow's Ultimate, Gran Rey Cero, and Enhanced Basic Attack, Form of Destruction, by 50%. When releasing Ultimate, Gran Rey Cero, additionally inflicts 5 stacks of Destruction Mark to the enemy."},
		{Level: 5, Description: ""},
		{Level: 6, Name: "Duel of Equals", Description: "Each time Destruction Mark is detonated, inflicts 1 stack of Battle Mark on the enemy, increasing the damage they take from Grimmjow's Ultimate, Gran Rey Cero, and Enhanced Basic Attack, Form of Destruction, by 7.5%. Lasts 20 seconds, stacking up to 10 times."},
	},
	"grimmjow-pantera": {
		{Level: 1, Name: "Evolutionary Path", Description: "The passive, Bare Killing Intent, increases the maximum stack of Killing Intent to 10. Each stack also increases the damage of the next Special Attack, Pounce, by 5%."},
		{Level: 2, Name: "Spare No One", Description: "When Pounce hits an enemy, it immediately deals Lacerate DMG. The Prey Mark applied to the enemy also reduces their Strike Resistance by 10%."},
		{Level: 3, Description: ""},
		{Level: 4, Name: "Worth the Hunt", Description: "When Prey Mark applied by Pounce ends, it deals an additional round of damage to the enemy equal to 15% of the total Strike DMG the target took during Prey Mark, up to a maximum of 3000× Grimmjow's current ATK."},
		{Level: 5, Description: ""},
		{Level: 6, Name: "I Will Be King", Description: "Grimmjow enters Destructive Urge immediately after battle starts. Lacerate Points accumulation rate increased by 40%. Each Lacerate trigger grants 8% All-Skill Mastery and 20% Crit DMG for 15 seconds, stacking up to 3 times."},
	},
	"ichigo-bankai": {
		{Level: 1, Name: "Sun That Seals the Sky", Description: "After using a technique or counterattack, the critical rate of the special attack increases by 30% for 6 seconds, and can be stacked up to 2 times. Each time a special attack lands a critical hit, the cooldown of the technique is reduced by 0.5 seconds. This activates once every 0.5 seconds."},
		{Level: 2, Name: "Rising Black Moon", Description: "Increases Getsuga Mark duration by 3 seconds. Each time Ichigo hits an enemy affected by Getsuga Mark, all damage dealt by him increases by 3% for 5 seconds, stacking up to 5 times. When Ichigo is in the Hollowfied state, can be stacked up to 10 times."},
		{Level: 3, Description: "Increases Level of Basic Attack, Technique, Ultimate and Counterattack by 2."},
		{Level: 4, Name: "Soul Convergence", Description: "Ichigo's counterattack deals 100% more damage."},
		{Level: 5, Description: "Increases Level of Basic Attack, Technique, Ultimate and Counterattack by 2."},
		{Level: 6, Name: "Edge of Black & White", Description: "When Ichigo launches a special attack, it will grant a quick charge if one is not already present. Otherwise, it will be consumed instantly, increasing the damage of the special attack by an additional 100%."},
	},
	"ichigo-initial": {
		{Level: 1, Description: ""},
		{Level: 2, Description: ""},
		{Level: 3, Description: ""},
		{Level: 4, Description: ""},
		{Level: 5, Description: ""},
		{Level: 6, Description: ""},
	},
	"ichigo-shikai": {
		{Level: 1, Description: ""},
		{Level: 2, Description: ""},
		{Level: 3, Description: ""},
		{Level: 4, Description: ""},
		{Level: 5, Description: ""},
		{Level: 6, Description: ""},
	},
	"ikkaku": {
		{Level: 1, Description: ""},
		{Level: 2, Description: ""},
		{Level: 3, Description: ""},
		{Level: 4, Description: ""},
		{Level: 5, Description: ""},
		{Level: 6, Description: ""},
	},
	"kenpachi": {
		{Level: 1, Name: "Wandering Beast", Description: "Spiritual Pressure gained from releasing Special Attacks is increased by 100%, and the damage dealt by Special Attacks is increased by 50%."},
		{Level: 2, Name: "The Name of Kenpachi", Description: "For every 1% of Ailment DMG Bonus Kenpachi has, his Crit Rate increases by 0.5%, up to 30%."},
		{Level: 3, Description: ""},
		{Level: 4, Name: "Fearless Demon", Description: "Increases the damage dealt by all Sever Attacks by 30%."},
		{Level: 5, Description: ""},
		{Level: 6, Name: "Driven to the Edge", Description: "Each time an on-field enemy is inflicted with Cleave, Kenpachi's Crit DMG and Ailment Mastery are increased by 30% and 20%, respectively, for 30 seconds, stacking up to 2 times."},
	},
	"kisuke": {
		{Level: 1, Name: "Vanished Mystery", Description: "Upon gaining Reishi Analysis, gains 10% extra Slash DMG Bonus."},
		{Level: 2, Name: "Sinless Sin", Description: "Enemies near Portable Gigai receive 15% more DMG for 6s. Taking Explosion DMG from Portable Gigai refreshes the duration."},
		{Level: 3, Description: ""},
		{Level: 4, Name: "Carefree", Description: "Reduces the cooldown of the Ultimate by 5s."},
		{Level: 5, Description: ""},
		{Level: 6, Name: "Benihime's Scream", Description: "Each stack of Reishi Analysis increases Urahara's Crit Rate by an extra 10% and Crit DMG by an extra 10%."},
	},
	"komamura": {
		{Level: 1, Description: ""},
		{Level: 2, Description: ""},
		{Level: 3, Description: ""},
		{Level: 4, Description: ""},
		{Level: 5, Description: ""},
		{Level: 6, Description: ""},
	},
	"mayuri": {
		{Level: 1, Name: "Seeker's Heart", Description: "Excitement can now stack up 4 times."},
		{Level: 2, Name: "Collapsed Functionality", Description: "Increases the Spiritual Pressure Charge Rate by 50% and the efficiency of inflicting Poison Scale to enemies by 100%. Increases the damage bonuses for Superhuman Potion and Double-Barreled by an extra 5%."},
		{Level: 3, Description: ""},
		{Level: 4, Name: "Absolute Madness", Description: "Increases the entire team's Ailment DMG Bonus equal to 20% of Mayuri Kurotsuchi's Ultimate Charge Rate, up to 50%."},
		{Level: 5, Description: ""},
		{Level: 6, Name: "Depletion Engraftment", Description: "Increases the Spiritual Pressure Reduction effect of Flesh Bomb and Double-Barreled when hitting enemies by an extra 50%. Increases the damage dealt by 10% when hitting enemies affected by Spiritual Pressure Dissipation."},
	},
	"momo": {
		{Level: 1, Name: "Distant Feelings", Description: "The maximum number of Insight stacks increases to 6."},
		{Level: 2, Name: "Blade of Truth", Description: "When Hinamori takes fatal DMG, she immediately heals 50% of max HP instead of becoming incapacitated, and gains 3 stacks of Insight. Triggers once every 180 seconds."},
		{Level: 3, Description: "Increases Level of Basic Attack, Technique, Ultimate and Counterattack by 2."},
		{Level: 4, Name: "Steadfast", Description: "Increases DMG dealt by the special attack Bombardment by 100%."},
		{Level: 5, Description: "Increases Level of Basic Attack, Technique, Ultimate and Counterattack by 2."},
		{Level: 6, Name: "Dancing Plum Blossoms", Description: "The explosion damage from igniting a Kido Snare increases to the number of Insight stacks x10%. Reaching maximum Insight stacks increases Explosion DMG by an extra30%."},
	},
	"nelliel": {
		{Level: 1, Name: "Reckoning Time", Description: "When releasing the Ultimate, Praise, Gamuza, immediately gains 3 points of Battle Will. Each time any Battlefield Skill is released, Nelliel gains Reckoning for 8 seconds, during which all Battlefield Skills deal 50% increased damage. All characters in the team deal 20% more Thrust DMG."},
		{Level: 2, Name: "Rebirth Amid Calamity", Description: "During Resurreccion, releasing the Technique: Verde Whirl, Special Attacks: Lance Execution, or landing the final hit of counterattack: Heroic Stomp grants 1 stack of Harass, increasing the entire team's Crit DMG by 20% for 30 seconds, stacking up to 4 times."},
		{Level: 3, Description: ""},
		{Level: 4, Name: "You've Got This", Description: "When any teammate triggers a counterattack, Nelliel's Crit Rate and Critical DMG are increased by 20% and 40%, respectively, for 30 seconds. Unstackable."},
		{Level: 5, Description: ""},
		{Level: 6, Name: "The Return of Gamuza", Description: "Releasing Cero Sincretico enables an immediate counterattack. During Resurreccion, the first hit of Lanzador Verde costs 0 Ultimate Energy and does not end the Resurreccion state. It also refreshes Heroic Stomp and maxes out Battle Will."},
	},
	"nemu": {
		{Level: 1, Description: ""},
		{Level: 2, Description: ""},
		{Level: 3, Description: ""},
		{Level: 4, Description: ""},
		{Level: 5, Description: ""},
		{Level: 6, Description: ""},
	},
	"orihime": {
		{Level: 1, Description: ""},
		{Level: 2, Description: ""},
		{Level: 3, Description: ""},
		{Level: 4, Description: ""},
		{Level: 5, Description: ""},
		{Level: 6, Description: ""},
	},
	"rangiku": {
		{Level: 1, Name: "Dust Settled", Description: "Every time Ash Blade hits an enemy, Rangiku gains 2.5% Ultimate Energy."},
		{Level: 2, Name: "Cat's Helping Paw", Description: "Haineko's Assist grants teammates extra Ailment DMG Bonus equal to 30% of Rangiku's Ailment DMG Bonus."},
		{Level: 3, Description: ""},
		{Level: 4, Name: "Scratch Marks", Description: "Every time Ash Blade hits an enemy, subsequent Ash Blade DMG increases by 20% for 15 seconds. Stacks up to 10 times."},
		{Level: 5, Description: ""},
		{Level: 6, Name: "Sandstorm", Description: "The drag range for techniques is expanded, and their DMG is increased by 200%."},
	},
	"renji": {
		{Level: 1, Description: ""},
		{Level: 2, Description: ""},
		{Level: 3, Description: ""},
		{Level: 4, Description: ""},
		{Level: 5, Description: ""},
		{Level: 6, Description: ""},
	},
	"rukia": {
		{Level: 1, Name: "Butterflies From Hell", Description: "Increases the Freeze Points inflicted by the Battlefield Skill Icy Sword Wave by an extra 25."},
		{Level: 2, Name: "Rain of Repentance", Description: "Increases DMG dealt by the Battlefield Skill Icy Sword Wave by 30%. Triggering the Battlefield Skill Chill Release's combo attack grants a 66% chance to release a second Ice Spike and a 33% chance to release a third."},
		{Level: 3, Description: ""},
		{Level: 4, Name: "Noble Ascension", Description: "Increases Rukia's Spirit DMG by 20%."},
		{Level: 5, Description: ""},
		{Level: 6, Name: "Snow of Dance", Description: "Releases the Battlefield Skill Chill Release when the battle begins. When an enemy triggers Breaking Ice or is within range of the Battlefield Skill Chill Release, their Spirit Resistance is reduced by 15% for 20s."},
	},
	"soi-fon": {
		{Level: 1, Name: "Beyond Admiration", Description: "Reduces the cooldown of Ultimate by 10 seconds and increases Ultimate Energy Charge Rate by 100%. Increases Soi Fon's Crit Rate by 15%."},
		{Level: 2, Name: "Unforgivable Betrayal", Description: "Increases All-Skill Mastery by 40%."},
		{Level: 3, Description: ""},
		{Level: 4, Name: "Don't Slow Me Down", Description: "Reduces the Strike Resistance of enemies inflicted with Execution Target by an extra 20% and increases all damage they receive by 15%."},
		{Level: 5, Description: ""},
		{Level: 6, Name: "Flashy Ambush", Description: "When any character in the team lands a critical hit, an extra Sting is triggered, dealing damage equal to 500% of Soi Fon's ATK. Cooldown: 4 seconds."},
	},
	"szayelaporro": {
		{Level: 1, Name: "Aligned Interests", Description: "When releasing a Special Attack, if there are 6 stacks of Intel Control, the entire team gains an additional 10% Spirit DMG Bonus for 20 seconds. Increases Ultimate Charge Rate by 40%."},
		{Level: 2, Name: "A Guest of Honor", Description: "Each time a Special Attack is released, enemies hit have their Spirit Resistance reduced by 10% for 20 seconds."},
		{Level: 3, Description: ""},
		{Level: 4, Name: "Grand Entrance", Description: "Increases Ultimate DMG by 600%, up to 1800 times of Szayelaporro's ATK. The buff provided by the passive, Research Expert, is increased by an extra 5%."},
		{Level: 5, Description: ""},
		{Level: 6, Name: "How Unfortunate", Description: "Increases the Crit Rate of all Special Attacks and counterattacks by 80% and their damage dealt by 50%. Each release also increases the entire team's counterattack damage by 200% for 20 seconds."},
	},
	"tosen": {
		{Level: 1, Description: ""},
		{Level: 2, Description: ""},
		{Level: 3, Description: ""},
		{Level: 4, Description: ""},
		{Level: 5, Description: ""},
		{Level: 6, Description: ""},
	},
	"toshiro": {
		{Level: 1, Name: "In the Name of Snow", Description: "When entering Bankai state, immediately gains 3 Ice Blossoms. All damage dealt by Toshiro increases by 25%."},
		{Level: 2, Name: "Dive", Description: "Increases Critical Rate by 20% for 8 seconds after releasing a technique or counterattack. Stacks up to 2 times."},
		{Level: 3, Description: "Increases Level of Basic Attack, Technique, Ultimate and Counterattack by 2."},
		{Level: 4, Name: "Talent", Description: "Increases all technique damage by 150%."},
		{Level: 5, Description: "Increases Level of Basic Attack, Technique, Ultimate and Counterattack by 2."},
		{Level: 6, Name: "Throne of Frost", Description: "Instantly fills Spiritual Pressure to maximum at the start of battle. The Ultimate Hyoten Hyakkaso gains extra Critical DMG equal to current Build Up stacks x40%. Each Build Up stack consumed increases basic attack damage by 60% for 8 seconds."},
	},
	"ururu": {
		{Level: 1, Description: ""},
		{Level: 2, Description: ""},
		{Level: 3, Description: ""},
		{Level: 4, Description: ""},
		{Level: 5, Description: ""},
		{Level: 6, Description: ""},
	},
	"uryu": {
		{Level: 1, Description: ""},
		{Level: 2, Description: ""},
		{Level: 3, Description: ""},
		{Level: 4, Description: ""},
		{Level: 5, Description: ""},
		{Level: 6, Description: ""},
	},
	"yachiru": {
		{Level: 1, Name: "Innocent Smile", Description: "Yachiru gains 1% Ultimate Energy every second while Spectating."},
		{Level: 2, Name: "Ever With the Blade", Description: "Inspiration grants an extra 8% Slash DMG Bonus."},
		{Level: 3, Description: ""},
		{Level: 4, Name: "Bloodstain", Description: "Yachiru's Ultimate Energy Charge Rate increases by 30%."},
		{Level: 5, Description: ""},
		{Level: 6, Name: "Distant Name", Description: "When Yachiru is Spectating, the on-field character's ATK increases by an extra 12%."},
	},
	"yoruichi": {
		{Level: 1, Name: "Hidden in Night", Description: "Yoruichi gains 20% Spiritual Pressure whenever she dodges an attack."},
		{Level: 2, Name: "Tenshiheiso", Description: "Increases the Lightning Seal stack limit to 8."},
		{Level: 3, Description: ""},
		{Level: 4, Name: "Disguise", Description: "Increases DMG dealt by the special attack Hakuda - Stomping Strike by 100%."},
		{Level: 5, Description: ""},
		{Level: 6, Name: "Pose of Flash Goddess", Description: "Reduces Yoruichi's Perfect Dodge cooldown to 8s. Every Perfect Dodge increases Crit DMG by 30% for 15s, stacking up to 3 times."},
	},
}

// Note: release dates are no longer hard-coded here. The scraper (../Main.go)
// extracts the authoritative Global date from the Spanish Fandom wiki on every
// run and writes it to each character's JSON. This file only consumes that
// value via the merge loop below.

// ---------------------------------------------------------------------------
// Per-character curated data.

var curation = map[string]CharacterCuration{
	// --- New characters. Stop-boundary defaults follow the rarity rules in
	// CharacterCuration's doc comment; stamps/rotations stay empty until the
	// build data is known, so curate only fills rarity/tier/boundaries here. ---
	"ichigo-white": {
		StopBoundaryP2W: 6,
		StopBoundaryF2P: 0,
		BestStamp: BestStamp{
			SetName:         "Mindscape Encroachment (3)",
			MainStats:       "1: Slash DMG Bonus > Atk%  •  2: Crit DMG (W5/C5) > Crit Rate •  3: Atk% > Ult Charge Rate",
			SubStatPriority: "Crit DMG > Crit Rate (25% for W5/C5) > Atk% > Ult Charge Rate > Flat Atk",
			Notes:           "Aim for ~119% Ult Charge Rate to align Ultimate with team buffs.",
		},
		Rotations: []Rotation{
			{
				Name: "Opener (Cleaving Throw)",
				Steps: []string{
					"Start with Slash Full Assault's Technique",
					"Swap to White → Special Attack",
					"Technique",
					"Enter Ultimate whenever available",
					"Swap to another character (White will continuously shoot Getsuga Tensho whenever you hit the enemy)",
					"Use the Ultimate Finisher whenever it's charged",
				},
				Notes: "White's Special Attack provides a manual counter. ",
				Tags:  []string{"Burst", "Boss"},
            },
	},

	"ulquiorra": {
		StopBoundaryP2W: 3,
		StopBoundaryF2P: 3,
	},
	"ulquiorra-resurreccion": {
		StopBoundaryP2W: 2,
		StopBoundaryF2P: 1,
	},

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
			SetName:         "Becoming the King (3)",
			MainStats:       "1: Strike DMG Bonus  •  2: Crit Rate  •  3: Atk%",
			SubStatPriority: "ATK% > Crit Rate (50%) > Crit DMG > Flat Atk > Ailment%",
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
			SetName:         "Sample Collection (3)",
			MainStats:       "1: ATK%  •  2: Crit Rate  •  3: Ult Charge Rate",
			SubStatPriority: "Ult Charge Rate (228%)> Crit Rate > Ailment% > ATK% > Crit DMG",
			Notes:           "S-tier Support — poison can crit, so stack crit rate after obtaining 228% Ult Charge Rate.",
		},
		Rotations: []Rotation{
			{Name: "Poison Setup", Steps: []string{"Ultimate", "Technique", "Swap out — poison ticks off-field"}, Notes: "His best damage comes off-field — apply poison and rotate.", Tags: []string{"Sustain", "Boss"}},
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
			SetName:         "Stealth Force (3)",
			MainStats:       "1: ATK% > Strike DMG Bonus  •  2: Crit DMG > ATK%  •  3: Ailment% > ATK%",
			SubStatPriority: "Ailment% > Crit DMG > Crit Rate (25%) > Atk% > Flat Atk",
			Notes:           "Two-rotation Tactician — the second deletes bosses.",
		},
		Rotations: []Rotation{
			{Name: "Nigeki Kessatsu", Steps: []string{"Battlefield → Manual Counter → Technique", "Use Strike Full Assault to trigger Perfect Dodge", "Swap back using Windmill → Ultimate"}, Notes: "Soi Fon swaps must be done at intervals of 10 Battlefield Skill Energy.", Tags: []string{"Burst", "Boss"}},
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

// normalizeRole maps any of the scraper's role-string variants
// ("Full Assault", "Full Assault/DPS", "Full Assault / DPS", "DPS", etc.) to a
// canonical RoleClass value: "Assault" | "Tactician" | "Support". Returns the
// trimmed input unchanged if it doesn't match a known class.
func normalizeRole(s string) string {
	r := strings.ToLower(strings.TrimSpace(s))
	switch {
	case strings.Contains(r, "tactician"):
		return "Tactician"
	case strings.Contains(r, "support"):
		return "Support"
	case strings.Contains(r, "assault") || strings.Contains(r, "dps"):
		return "Assault"
	default:
		return strings.TrimSpace(s)
	}
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
		// Boundaries: boundariesBySlug is the authoritative, hand-curated
		// boundary text. When present it OVERRIDES the scraped boundaries so all
		// six always render. Icons are derived from the slug + level.
		if bs, ok := boundariesBySlug[slug]; ok && len(bs) > 0 {
			out := make([]map[string]any, 0, len(bs))
			for _, b := range bs {
				m := map[string]any{
					"level":       b.Level,
					"description": b.Description,
					"icon":        fmt.Sprintf("/Images/%s/boundary-%d.png", slug, b.Level),
				}
				if b.Name != "" {
					m["name"] = b.Name
				}
				out = append(out, m)
			}
			obj["boundaries"] = out
		}
		// Role normalization: the scraper writes a few variants for the same
		// class — "Full Assault", "Full Assault/DPS", "Full Assault / DPS" all
		// mean the canonical "Assault" role. Collapse every variant to the
		// canonical RoleClass value ("Assault" / "Tactician" / "Support") so the
		// site's filters, ROLE_LABEL, and ROLE_KANJI lookups all match.
		if rs, ok := obj["role"].(string); ok {
			switch normalizeRole(rs) {
			case "Assault":
				obj["role"] = "Assault"
			case "Tactician":
				obj["role"] = "Tactician"
			case "Support":
				obj["role"] = "Support"
			}
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
