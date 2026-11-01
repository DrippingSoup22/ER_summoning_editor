# Safety rules

Hard requirements on every generated save, and what enforces each one.

## 1. Never modify a world actor

> **Never write a row outside the clone bands.** A world `NpcParam` or
> `NpcThinkParam` row (below `100000000`) is read and never written, inserted
> over or removed.

A summon reaches a world creature through a **clone**: a copy of the world row,
inserted at an address derived from it and written summon-side.

- **Where a clone goes is arithmetic, never an argument.** A creature's n-th
  recipe lives at `source + n × 100000000`, n a clone band (1, 2-4, 6-20;
  `formation.CloneBands`). The game's tables use bands 0 (the world), 1 (its
  summons) and 5 only; band 5 is never touched and band 21 would overflow an
  s32. So a clone's last eight digits are its source in every band, and it
  stays in its own `cXXXX` model namespace by construction. The manifest names
  a clone by its key (`source + (n-1) × 100000000`), never by its row, and
  no API lets a caller name a destination.
- **A shipped summon-side row is copied, never written,** when a slot needs it
  changed (a person on a team): to `199000000 + distance` from `100000000`, or
  `199500000 + distance` from `150000000` (`formation.SummonCopyRow`). The
  copy is the shell byte for byte with `teamType` moved.
- **Modification** is bounded to the clone bands: `validateNpcPatches` refuses
  any other row before a byte is written. The bands hold nothing but clones -
  FromSoftware's 115 summons and the ones this editor inserted - so writing
  there never reaches a world row. A role chosen for one of FromSoftware's
  summon rows (and a reset putting one back) is the one product write here.
- **Removal** takes only a row this editor could have inserted: a clone-band
  row the shipped snapshot does not name. A shipped row is never removable,
  and a world row under no circumstances. A record that names no shipped clone
  rows beside a save holding some is refused rather than read as "the game
  shipped none", which would delete all 115.
- **Untested:** whether the game summons a row above band 1.

**Converting or authoring an Ash changes that Ash and nothing else in the
world.** The item and the enemy are different data: an Ash's goods row points at
a summon-side actor, and the wolves in Limgrave are world rows no Ash
references. Making every wolf a boss would mean writing the wolf's own row,
which this rule forbids.

**Acceptance test:** after any write, every world `NpcParam` and
`NpcThinkParam` row is byte-identical to the input.

### The proofs

- **A patch** is fenced by `verifyBoundedMutation`: the regulation is compared
  byte by byte, and one changed byte outside a declared row aborts the write.
- **An insertion** moves bytes it does not change, so it carries a stronger
  proof (`verifyRowInsertion`, in `shared/regulation`): every row present
  before is byte-identical **by row ID**, the only new IDs are the declared
  ones, nothing disappeared.
- **A removal** (`verifyRowRemoval`): every row not named survives by ID, every
  row named is gone, nothing appeared.
- **A clone** is an insertion followed by a patch of that row; each half keeps
  its proof.
- **The whole commit**: `verifyStructuralStage` walks every table the product
  opens and requires that each row present before is there and unchanged, that
  only declared rows appeared and only declared rows vanished.

### The clone recipe

A bare copy is still not a summon: it would stand on an enemy team with the
enemy's runes and item lots. So every clone is written summon-side as it is
inserted - team 47, rewards and the game-clear hook cleared, the boss-soul bit
dropped, hitbox capped, area scaling (`7000-7699`, `20007000-20007699`)
stripped, `295000`, `296000` and `297000` resident, `isBuddyAI` on the think
half. Each is a property all 115 shipped summons already have
(`clone_real_test.go`), so the recipe is the game's, not this product's. The
perception level written over the think clone is a write inside the inserted
row and nothing else.

## 2. Never write in place

A distinct output path, an atomic write, the result reopened and validated, and
the input's hash confirmed unchanged afterwards. The input save is never
modified. An existing output is refused (`O_EXCL`), and the audit manifest is
written only after the save it describes exists.

## 3. Never write outside a declared range

Every changed byte falls inside a row the manifest declared. A manifest is
pinned to the SHA-256 of the save it was planned against, and is refused on any
other.

### The write surface

Each operation has its own surface, and the manifest validator refuses a field
outside it before the save is opened:

| Operation | Adds rows | Removes rows | Goods | Other |
|---|---|---|---|---|
| `convert` | the woken monuments | the woken monuments | `refId_default` only | the monument switches, as `area` |
| `area` | the woken monuments | the woken monuments | none | `BuddyStoneParam`: each switch that is on on every row (the area's five fields, or `summonedEventFlagId` 0), each that is off put back per row from the shipped record; `appearOnAroundSekihi` on 20 `BuddyParam` rows |
| `author` | yes | no | none | its host's `BuddyParam` block; clones in `NpcParam` / `NpcThinkParam` |
| `cost` | no | no | `consumeMP`, `consumeHP` only | none |
| `reset` | no | yes | restores shipped | restores shipped values per row; removes the woken monuments |
| `prune` | no | creature rows only | none | none |
| `shared-effects` | no | no | none | the closed lists below |
| `characters` | character bands | character bands | none | whole rows of `CharaInitParam`, `FaceParam` and `EquipParamCustomWeapon` at `2990000-2990199`; `NpcParam` bodies at `199900000-199900199` |
| `own-effects` | its fifteen rows | its fifteen rows | none | `SpEffectParam` 295001 (a copy of 295000, `vfxId -1`), 297001 (a copy of 297000), 511063 (a copy of 511060 with `effectEndurance`, `vfxId`, `iconId` and `cycleOccurrenceSpEffectId` -1 and `goodsConsumptionRate` 1), 14709 (a copy of 14707), 20011605 and 20011606 (copies of 5011 with `motionInterval` 0.01 and `cycleOccurrenceSpEffectId` 20011598 and 20011573) and 290251-290259 (copies of 290050 with `maxHpRate`, the five attack rates and `staminaAttackRate` at 0.1 to 0.9), nothing else |

- **Goods is shared by field.** A conversion and a price can be staged on one
  Ash together; each must name its whole field set and nothing outside it.
- **Authoring writes no goods row**, so it cannot reach `enable_multi` even in
  principle: it edits the host's own block, which every upgrade level resolves
  (the Giant Rat's own three blocks, one a range of levels).
- **The character bands** are the player's own characters summoned as people
  (DESIGN, The character slots) and the characters built from choices: rows
  of our own, which nothing in the world references, at `2990000-2990199` in
  three tables and, for each character's four bodies (the Helper and the
  Fighter, each in its shell's Puppet coat and in its own colours, side by
  side), at `199900000-199900199` in `NpcParam` -
  the first hundred a slot's, the second a built character's;
  ids every supported regulation leaves free. The body band sits in band 1 but is cut out of the
  clone bands, so no clone list, prune, reset or strength write reaches it.
  The `characters` operation inserts, rewrites and removes rows there and
  nothing else; no other operation may name one.
- **Only a reset, a prune, the character bands and the own effect rows remove.** A prune removes the creature rows this
  editor inserted that no formation row will name once the commit is written,
  and nothing else.
- A creature row one part removes and another inserts is **replaced** - the
  prune frees a row, a new recipe takes its number - for the two actor tables
  only; removals run first and the structural proof checks the result. A
  formation row is never replaced.

## 4. Never touch a multiplayer gate

The tool never writes `EquipParamGoods.enable_multi`, `NetworkParam`,
`RoleParam`, invasion or session parameters, and offers no option that does.
This binds a reset as much as a conversion: the shipped value stays, whatever it
is (`enable_multi` is 1 on every Spirit Ash row).

**Acceptance test:** every `enable_multi` byte in the output equals the input.

**Can the monument recipe reach session permission?** It rewrites all 304
`BuddyStoneParam` rows, including region fields. Five independent lines say it
cannot: the table has no session field; the three Colosseum arenas' rows are
identical yet only Caelid permits Ashes, so the arena's map data decides;
session permission is not reachable by a save edit; with the
recipe applied a second Ash is still refused while a buddy lives; and Seamless
Co-op has to replace the session layer to allow spirits. This is convergent
inference, not measurement, so **the product makes no multiplayer guarantee and
is documented for solo play.**

## Version matching

A save whose embedded regulation is older than the installed game has it
**replaced wholesale** on load - offline, with no error - which is
indistinguishable from an edit that did nothing. The tool reads the version and
refuses anything but the measured ones (`11701000`, `11711000` and `11721000`,
row-for-row identical, and `11730034`, one Bullet field away, a table the tool
never reads), rather than warning.

- **The woken monuments** are the 39 entities the maps place running the
  monument's script with no `BuddyStoneParam` row (`savecore/monuments.go`):
  each a copy of row 45000100 keyed to its entity (`talkChrEntityId`), with no
  enemy to defeat, no region, and `summonedEventFlagId` at entity + 2000, the
  shipped convention. Wider summoning area writes all 39 afresh and its off
  position, like a reset, removes them; a manifest naming any other row is
  refused.

## Collateral the tool accounts for

The monument switches touch all 304 `BuddyStoneParam` rows, so:

- with Wider summoning area on, the 20 `appearOnAroundSekihi = 1` rows are
  normalised to 0, or four untouched families spawn at a distant monument;
- No once-per-rest limit lifts the restriction for **every** Ash, and the tool
  says so.

## The two exceptions to "nothing else in the world"

**Shared effect writes.** Two catalog entries fight as they do only through
fields of shipped `SpEffectParam` rows the world's own fight reads too: the
combined Ancestor (two attack sets moved off the priority that makes them evict
each other) and Rykard below half health (his great combo rebuilt from his own
markers). The scripts test the effect ids themselves, so there is no
per-creature route.

**Person writes.** Eleven people the game ships unable to fight - Irina, Fia,
Hyetta, Roderika, Seluvis, Tanith, Rya, two Finger Maidens, Boc's human row,
Sage Gowry - are offered armed, with a loadout written into their own shipped
`CharaInitParam` row. They never fight in the world and cannot be hit, so no
world fight reads the change.

Both are fenced four ways, and no fence may be relaxed:

- **The catalog warns** on every such entry, saying what changes in the world.
- **Present only while an Ash holds the entry**, decided at Save over the whole
  save: written while a formation row names it, put back to the shipped value
  once none does - a reset included. A value this editor did not write is never
  "put back" over.
- **A closed surface.** The save core holds every row, field and pair of values
  (`internal/savecore/sharedeffects.go`, `personwrites.go`) and refuses
  anything else; the application refuses to start if the catalog asks for more.
- **Proved like every patch**, and the real-save tests write each field and put
  it back to the shipped regulation byte for byte.

A creature's need met by an effect row **of our own** - a new row nothing in
the world references - is not an exception and needs none. The `own-effects`
operation holds the fifteen there are; a manifest naming any other row is
refused, and a write that would leave an actor or a formation row naming one the
save does not hold is refused after every patch.

**The retired colour switch.** Earlier versions pointed `SpEffectVfxParam`
57000, which `295000` delivers, at a profile with no coat, for the whole save -
reaching the 19 world rows that carry `295000` too, the spectral enemies the
game draws as spirits. The look is each creature's now, on a presence of our
own, so the editor writes 57000 only to put the shipped value back: with the
next change that writes the regulation where an earlier version wrote it, and
beside a reset whatever wrote it.

## What the tool refuses

| Request | Why |
|---|---|
| A formation larger than 10 | the game silently drops the rows past ten |
| A regulation version it was not built for | see above |
| Authoring into a family whose formation varies by upgrade level | it owns several blocks, so some levels would change and others not |
| Authoring into an Ash an earlier conversion re-pointed | its block belongs to another item now; reset first |
| An actor row the save does not hold | nothing to copy |
| One Ash staged as both authored and converted | no order satisfies both |
| Two staged changes that write one row two ways | refused before the save is opened |
| A session manifest with edits of its own | only its parts face an operation's rules |
| Two Ashes live at once, the Colosseums, summoning for a character that cannot | manager state, map data and character state, not the regulation |

**Nothing is written until a commit.** A staged change is an overlay on the
loaded save; a session never committed leaves the disk as it found it, a staged
reset included. A commit carrying several operations is not a relaxed rule set:
every part is validated as the single-operation manifest it is, and the save
core then checks what is true only of the combination.

## Before any release

The release gate is that the product **claims nothing about multiplayer**: it
writes no session field (the save core refuses a goods patch naming one), and
the README says it is built for solo play. The closed-session observation was made
on 2026-11-01 on a PlayStation, in a password-closed session on a second
account, with both monument switches on: a converted Ash and an untouched one
behaved the same, both greyed out from the Furlcalling Finger Remedy on while
hosting and both greyed out as a guest in another world. Public
players are never a test population. If the two ever behave differently, it is
a defect against rule 4 and the release is withdrawn.
