# Examples

Reference files. **Nothing here is loaded by the site or the build.**

`GenerateTeamIndex.mjs` only reads `BSRData/Teams/<group>/*.json`, so files in this
folder are never indexed, never shipped, and never validated — which is why they can
carry `$comment` keys that a real team file shouldn't.

---

## `team-goal-rotations.example.json`

How to give a comp a rotation per button in the Team Builder's
**"What would you like to optimize for?"** picker — including the nested Co-op bosses.

### The one rule

**One button = one entry in `p2w-rotations` / `f2p-rotations`.**

The buttons are built from the data, not from a list somewhere. Write a rotation with
`"goal": "events"` and the Events button lights up for that comp. Write nothing and it
renders greyed out and unclickable. There is no separate place to register a button.

### The four goals

They always all render, in this order:

| `goal` | Button | Nested? |
|---|---|---|
| `general` | General | no |
| `coop-boss` | Co-op Bosses | **yes** |
| `events` | Events | no |
| `frenzy-feast` | Frenzy Feast | no |

A rotation with no `goal` counts as `general`.

### The nested one

`coop-boss` is the only goal that opens a second row. Each `coop-boss` rotation also
needs a `boss` id saying which button in that row reaches it:

`ranuganga` · `yammy-llargo` · `sajin-komamura` · `ashisougi-jizo`

Write one entry per boss you have a rotation for. The rest stay greyed. Bosses are
independent — one play-tested boss lights up that boss *and* the Co-op parent, while
the other three stay locked.

`boss` is **required** on `coop-boss` entries and **forbidden** on every other goal.
`GenerateTeamIndex.mjs` fails the build if you get that wrong, rather than letting a
typo turn into a silently missing button.

### Greyed and locked

A button is greyed out **and unclickable** when there's no play-tested rotation behind
it — which covers both cases:

- no rotation written for that goal at all, or
- every rotation for it is flagged `"unverified": true`

`unverified` is for rotations you've written but not yet run. The button behaves as if
it were missing and an "Unverified" chip shows next to the rotation name. **Delete the
flag and the button goes live** — that's the whole promotion step.

### P2W and F2P are independent

The two lists are separate. The toggle swaps which one is showing, and the goal buttons
are rebuilt from whichever is active. A goal written only under `p2w-rotations` is lit
on P2W and greyed on F2P. Put an entry in both lists to have it live on both routes.

### Videos

Every rotation can carry its own `video`, so the embed changes as you move between goal
buttons *and* between boss buttons. Resolution order:

1. the selected rotation's own `video`
2. the comp's route-level `p2w-video` / `f2p-video`
3. nothing — the embed hides

Leave `video` as `""` to fall back.

### Lineup variants

`lineup` stacks on top of goals. The UI narrows to the chosen goal first, then picks the
lineup variant inside it — so you can have "Events, when Rangiku replaces Mayuri". List
only the slugs that *differ* from `members`. An entry without `lineup` is the default
for its goal.

### After editing

```
node BSRData/GenerateTeamIndex.mjs
```

Then sync `BSRData/Teams` → `Web/Public/Teams` (Build.ps1 step 3 does this).

### Renaming a boss

Labels live in one place: `BOSS_CATALOG` in `Web/Source/Lib/RotationGoals.ts`. Change the
label there and every button on the site follows. Changing an `id` means also updating
the matching `boss` values in the team JSONs.
