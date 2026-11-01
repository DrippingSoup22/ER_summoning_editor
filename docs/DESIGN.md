# Product design

How the editor's features work, and why they are shaped the way they are. The
write rules are [`SAFETY.md`](SAFETY.md). Every mechanic described here was
read from the game's own data and checked on a PlayStation.

## Layout

| Path | What it is |
|---|---|
| `cmd/ersummoneditor`, `cmd/ersummoneditorgui` | the two frontends: parsing and rendering only |
| `internal/application` | the whole workflow: `Service` (one operation per call) and `Session` (load, stage, commit) |
| `internal/formation` | the authoring model; pure |
| `internal/plan` | the bounded manifest |
| `internal/savecore` | the writer: patch families, the clone recipe, sessions |
| `internal/character` | the character slots: ownership, grants, levels; characters as rows, built or read |
| `internal/items` | what a custom character can carry, generated from the merchant editor's item set |
| `internal/catalog`, `reference/` | the embedded catalogs, generated data, never edited by hand |
| `internal/savefixture` | a synthetic decrypted save for tests |
| `internal/ui/gio` | the desktop editor ([`GUI.md`](GUI.md)) |
| `tools/appicon` | the Windows application icon, for `scripts/build-windows.sh` |

## Conversion

A conversion re-points one Spirit Ash goods family at another formation the
game already ships. The player keeps the host Ash's item, upgrade levels, icon,
cost and displayed name; only the delivered formation changes.

```text
host EquipParamGoods rows -> destination trigger SpEffect -> existing BuddyParam formation
```

It changes references rather than copying actors, so it cannot reach a world
actor. It writes, for each conversion:

1. all eleven host `refId_default` rows, re-pointed at one destination trigger
   (and nothing else in those rows);
2. the six-field monument recipe on all 304 `BuddyStoneParam` rows, and the
   39 sleeping monuments woken (the switches, below), so an Ash
   can be summoned away from a Rebirth Monument;
3. `appearOnAroundSekihi = 0` on the 20 rows that ship it, so monument-origin
   formations spawn at the player.

**One formation for the whole family.** Every shipped goods row points at
`family + its own level` (Lone Wolf +3 is `232003`); the editor reads that
mapping and never computes it, and `trigger - level` recovers the family, which
is how a destination an earlier edit re-pointed is recognised and refused.
Every host row from +0 to +10 gets the same destination, so a converted Ash
delivers the same summon at every level: the result depends on the choice, not
on how far the player upgraded the host. **Giant Rat Ashes is the exception:**
its three formations split the reinforcement table (three rats at +0..+4, four
at +5..+9, five at +10), so no single trigger is safe across all levels and it
keeps its shipped progression.

**The monument recipe is applied as one unit** - the six fields seen
working together. Which of them are individually necessary is not established,
and a smaller recipe would be untested, so it is not trimmed
opportunistically. The other five `BuddyStoneParam` fields (`talkChrEntityId`,
`isSpecial`, `buddyId`, `dopingSpEffectId`, `disableParam_NT`) are what make a
stele a particular stele and are never written.

The monument switch is also a change of its own: authoring writes only its
host's block, so it has nothing to say to the monument table. It is two
switches, each staged and dropped apart: **Wider
summoning area** (`plan.Area`) - the five area fields and
`appearOnAroundSekihi` - and **No once-per-rest limit** (`plan.Rest`) -
`summonedEventFlagId`. One manifest part says where both stand: a switch that
is on rides on one all-row patch, a switch that is off is put back row by row
from the shipped record, so off undoes exactly what on did. A conversion,
which the command line still makes, turns both on as it always did, the
sleeping monuments woken with them, so no save is left between the two
positions.

**Its reach is still the monuments'.** A widened row reaches only from its own
monument, so places far from any - Liurnia's Boilprawn Shack, the Radahn
battlefield, most of the DLC - still refuse. No save edit widens
that: a monument is a character the map places, running a script of its own,
and a save holds neither. What Wider summoning area adds is monuments: 39
characters the maps place running the monument's script have no row, and the
switch wakes them with a row keyed to each, on eight overworld tiles that hold
none - Agheel Lake North and Mistwood Outskirts among them.

## Authoring

Authoring rewrites an Ash's **own** formation, slot by slot, with any creature
the save holds. A formation is a block of `BuddyParam` rows sharing one
trigger, one row per actor, each naming its own actor - so a repeated formation
and a mixed one are the same write.

**The Kristoff Ash is refused as a host**, for authoring and for conversion
(`HostFixedError`): the game resolves that item's summon without the params, so
a block written for it still delivers the shipped Kristoff. The GUI does not list it among the
containers, since nothing in it can be edited there.

- **The host's block is edited in place.** Every upgrade level resolves the
  formation of its +0 trigger, so one `BuddyParam` edit reaches all eleven
  levels and no goods row is written. **Giant Rat Ashes** has three blocks,
  one a range of levels (233000 for +0..+4, 233100 for +5..+9, 233200 for
  +10, each row reinforced only at its own levels), so its formation is
  written into all three and every level summons what was placed
  (`unitsFor`; `formation.Block.Levels` says which levels reach a block).
  The grid shows the +0 block.
- **Slots past the block insert rows**, each anchored on the one before, so the
  block stays one contiguous run; the inserted row copies the block's last row
  and inherits its complete reinforcement table. **Slots short of it retire
  rows** (`triggerSpEffectId = -1`, the game's own inert spelling), written
  back naming the actor the game ships for them. A formation that grows again
  re-fills its retired rows before inserting.
- **One to ten actors.** The game runs ten rows of a longer block and drops the
  rest, so more is refused.
- **Placement** is written per slot (`x_offset`, `z_offset`, `y_angle`). A
  placement is where the summon stands, from the player, and a summon stands
  3.2 m further ahead than its `z_offset` says (`formation.Ahead`, measured on the console), so the row holds z less
  3.2, and a row read back is shown 3.2 m further ahead. A slot with none is
  laid out on a ring of 1.3 m about the point 3.2 m ahead, where the shipped
  four-actor formations stand.
- **A world actor arrives through a clone** (SAFETY rule 1); an actor the game
  already ships as a summon needs none. A person slot names a person shell and
  a `CharaInitParam` row, and inserts nothing.

### The clone rows are decided at Save

Nothing is written until Save, and Save writes the file from the whole
configuration - every container, edited or not (`application/clonealloc.go`):

- one clone row per **recipe** of a creature (its level, share, team, branch,
  weapon-out, presence, marker and strength), shared by every slot in every
  Ash that uses it;
- a row the save already holds that **is** a recipe asked for is reused where
  it stands - proved byte for byte (`savecore.MatchClones`), strength
  included;
- a new recipe takes the lowest free band;
- a clone this editor inserted that no formation row names any more is
  removed by the prune.

FromSoftware's own rows are never pruned, and an Ash card of the game's own
Ashes is delivered on FromSoftware's row as shipped (the Follower; another
level on it needs a second row and is refused until the game is shown to
summon one).

### Effect rows of our own

A clone may carry a copy of a summon effect in place of the shipped one
(`savecore/owneffects.go`): **295001**, the presence 295000 with no phantom
overwrite (`vfxId -1`), which draws the summon in its own colours with every
part of its body (the phantom shader hides parts of some bodies); and
**297001**, the companion marker 297000 at an id no battle script tests, under
which the large Shadow Commoner fights with its own moves (its battle script
disarms it under 297000). Both are part of the
slot's recipe, so two slots of one creature in two looks are two clones. A
character's body in its own colours carries 295001 too (A character can be
summoned, below), every character's body carries **511063**, free casting,
the rows summoning characters climb the characters' ladder, **290251-290259**
(Summon strength), and two creatures' fixes add residents of our own (The
catalog, a creature's composed fix): **14709**, a copy of 14707, and
**20011605** and **20011606**, copies of the bare marker 5011 re-applying the
Consort's markers 20011598 and 20011573 every 0.01 s. Save inserts a row while a clone, body or formation row in
the written file names it and removes it once none does, decided over the whole
save like the prune; the character rows are held while the save holds any
character. The save core refuses a write that would leave an actor or a
formation row naming one the file does not hold.

### Perception: the ladder

Each slot's level is the profile written over its think clone, designed field
by field against FromSoftware's own Spirit Ash rows. Six are offered:

| level | it sees | it behaves like |
|---|---|---|
| Follower | 2 m past the front of its body, all round | **the default.** FromSoftware's own Spirit Ash: it takes the player's target and fights what the player fights |
| Guard | 12 m past its front, 90° | keeps the ground around the player, on its own senses |
| Sentinel | 30 m past its front, 45° | watches ahead, often spotting an enemy first |
| Hunter | 50 m past its front, all round, and a 25 m nose | hunts on its own, off the leash |
| Statue | nothing, and hears nothing | never acts and cannot be provoked; stands where it was called with its weapon out |
| Loner | its own senses, as the game gave them | what an enemy gets: it ignores the player's target |

The rules the code holds:

- **A level owns its reach and its cone; everything that makes a clone a
  companion is shared** (`summonPackage`: the home band, the forget times, the
  lost-target actions, the summon ear, sight after dark, no search). The Loner
  alone keeps the creature's own senses, because it is what an enemy gets.
- **Reach is a constant past the front of the body**, counted from the centre
  with the eye origin at 0, so a large creature is not blind inside its own
  model; the front is the collision radius, or where the head ends on a
  creature clearly longer than wide (`reference/bodies.csv`).
- **No level searches**: a summon that loses its enemy comes back to the
  player. Only the Hunter has a nose beyond its body.
- **The share box** - whether a summon takes the player's target - is a
  package: share on is `disablePCTargetShare 0` with the think clone's
  `targetSys_DmgEffectRate 0`, off is 1 with 100. The writer takes a choice
  on the Follower (on by default), Guard, Sentinel and Hunter, and fixes it
  off on the Statue and the Loner; the product offers it on the Guard,
  Sentinel and Hunter only (`application.ShareOffered`): a Follower notices
  two metres by itself, so off it would fight nothing. The writer keeps the
  Follower's choice for the game's own Ashes shipped with share off (the
  Putrid Corpse, the Giant Rats, Floh, Ogha), which arrive on the Follower.
- **Three levels need a stance**: the Statue holds with share off, the Hunter
  and the Loner wander (the leash is `pcFollowType`, not a think field).
  `Formation.Validate` refuses a contradiction rather than correcting it.
- **An enemy** (a hostile team on the world branch) takes only the Loner (its
  default), the Hunter or the Statue, and never shares.
- `eyes`, `blind` and `world` are test levels, for console experiments; the
  product never offers them.

## Summon strength

A summon is three layers, multiplied:

| Layer | What it is | Who sets it |
|---|---|---|
| Body | the creature's own row: its health and attacks before any region multiplies them | the creature |
| Ash level | the ladder (`dopingSpEffect_lv0..10`) the Ash's formation row ships with | the player's upgrade |
| Role | Helper or Fighter | the player, per creature |

**The Ash level.** A creature's Ash ships the summon ladder `290000-290010`,
which is rung for rung the world's region ladder on health and attack: +1 is
tier `7020`, +5 is `7080` (Altus Plateau), +10 is `7200`, the game's top; only
+8 sits between two tiers. Person Ashes ship a puppet ladder (`290050-290060`:
x6.6 health, x3.16 attack at +10) and the Mimic Tear's copy its own
(`290200-290210`: x3.3 health, attack x1 at every level). Every formation row
this editor writes carries the ladder its Ash ships with - an inserted row, the
one of the row it copies - in both roles, so a +10 creature is the creature at
the game's end wherever it lives. The DLC's own region band
(`20007000-20007130`) is left out by design: its scaling is
artificial, and a DLC creature hits as hard as the others.

**The role.** A world creature is also made strong by its region tier and its
New Game + hook; the clone strips both (`7010-7200` and the DLC's band
resident, `GameClearSpEffectID` -1). On top:

| | Helper | Fighter |
|---|---|---|
| the package `296000` (damage dealt x0.25, taken x0.5, poise damage x0.05), and the two extra cuts some summons carry | on | off |
| a clone's health | 2.7x its world row's | its world row's |
| a FromSoftware summon row's health | as shipped | shipped / 2.7 |

2.7 is the median of FromSoftware's own summons over the world rows they come
from (94 measured; the middle 80% lie between x1.0 and x5.0), so a Helper
survives like the game's own Ashes. `+25%` damage and target priority stay:
the taxes are chosen by meaning, not by id range. A shipped summon row is told
apart from a clone this editor inserted by the shipped record, never by its
number: the DLC's Ashes sit at `150000000` and on, where clone arithmetic would
name Commander Gaius. There is no New Game + layer: a +10 Fighter is the
creature at the game's top tier.

**The role is each creature's.** It is part of the clone's recipe, so a Helper
and a Fighter of one creature are two rows; a new summon is a Helper. A clone
keeps the role it was written with: reopened, the slot shows it, and another
role is another slot of the creature. A creature the catalog does not list
keeps the health its row has, in either role.

**FromSoftware's own summon rows** - the creatures of its Ashes and the person
shells - are never cloned, so a role for one is the row's, for every Ash that
summons it (`application/rowstrength.go`). It works by difference: the Fighter
is the shipped record without the taxes at the shipped health over 2.7, and
the Helper puts the record (`reference/vanilla.json`) back on a row carrying an
earlier Fighter's fingerprint - no package, or the world-scaling effects an
earlier build wrote as a New Game + cycle. Choosing what a row already holds
writes nothing. A reset puts every such row back.

People are summoned on a person shell, a FromSoftware summon row, and follow
its rule.

**The player's characters stand on bodies of their own** (The character slots,
below), four per character: the Helper and the Fighter, each in the Puppets'
coat and in its own colours. So a character's role is chosen slot by slot in
the creature window, like its look (`CharacterBodyAs`), and changing it writes
no character row. The Helper carries the package `296000` and 2.7 times the
character's health; the Fighter neither, the character as played. Its Ash level
is a ladder of the characters' own: +10 is the
character as built (`290050`, the puppet ladder's neutral rung), and each level
below takes a tenth of its health and damage, down to a tenth at +1 and +0 -
rungs `290251-290259`, copies of `290050` with `maxHpRate`, the five attack
rates and `staminaAttackRate` at the share. Defences and poise are not scaled.
A formation row naming a character climbs that ladder, and a row that stops
naming one takes its Ash's back, retired rows included (`plan.BuildAuthor`).

## Cost

`consumeMP` and `consumeHP` are the price: a positive field is both the minimum
the player must hold and the exact amount spent, and two positive fields both
apply. `0/-1`, `0/0` and `-1/-1` all mean free; the product writes the game's
own spelling, `0/-1`. A price is written on all eleven host rows, and a reset
puts it back.

## The character slots

The editor also reads the ten character slots and can give a character an Ash
(`internal/character`). PS4 slots carry no checksums, so a slot write needs no
checksum step (a PC save would).

- **Geometry**: PS4 header `0x70`, slot `0x280000`, ten slots; identity is read
  at offsets from a magic pattern found in the slot. A slot whose magic or name
  cannot be read is shown as empty.
- **A Spirit Ash is goods, and goods allocate nothing.** An inventory record is
  12 bytes - handle, quantity, acquisition index - and a goods handle is
  literally `0xB0000000 | goods id`, with no `GaItem` behind it.
- **Ownership is inventory ∪ storage.** Reading only the inventory would offer
  a second copy of an Ash sitting in the chest.
- **A grant** writes one record into the first empty `CommonItems` slot and
  advances `NextEquipIndex` (the visibility gate: wrong, and the item never
  appears in game) and `NextAcquisitionSortId`. An owned Ash is refused, and
  every check runs before a byte is written.
- **The upgrade level is the goods id**, so changing it rewrites the four-byte
  handle of every record of that Ash, inventory and storage alike. No counter
  moves.
- **A removal** zeroes the record where it stands and recounts the header;
  neither counter moves, so the hole is reused and the file never grows.
- **A grant is one-way:** Reset restores the regulation, and slot data was
  never in its scope. The UI says so behind Reset's ? and the Pending list's.

**A character can be summoned.** Every filled slot is offered as a person on
one card, "Your characters" in the Characters category, named by slot, name,
level and starting class - the only card that changes with the save. Opening a save
reads who the characters are and nothing more; what a character wears is read
at Save, from the slot as it is then, for the characters a formation names
(`internal/character` loadout.go, rows.go). It is copied whole and unjudged
into rows of our own (SAFETY, the character band): a `CharaInitParam` row with
the level, attributes, body, voice, six weapons with affinity and level, ammo,
armour, four talismans, Physick tears, seven spells, quick items and pouch
with counts - save the player's flasks, which the AI never drinks: they give
way to the one NPC flask a built character carries, first in the quick slots; a
`FaceParam` row converted from the slot's FACE record; and a custom weapon for each
weapon carrying an Ash of War of its own; and four bodies - copies of the
person shell (Nepheli Loux Puppet's, which gives the AI) carrying the
character's health and every effect active on it: the ones the slot
saved, one per save category, and an
active Great Rune's (600-650 for Godrick to Malenia). A body's role is its own,
whatever role the shell carries (Summon strength). A save
opened after a character changed weapons rewrites the rows on the next Save;
rows no formation names are removed.

**A character is a puppet or in its own colours, nothing between.** The Puppet shell's coat, `295200`, is the spirit
presence `295000` drawn with the Puppets' own profile, and `295201` adds two
particle effects of theirs. So a character's first body
wears the coat as the shell does, and the next id is the same body in its own
colours: `295001` in place of the coat and none of the particles
(`savecore.CharacterBody`); the two after them are the Fighter's pair. The look
is the body a formation names, chosen in the creature window like a creature's
(`CharacterBodyIn`), and all four bodies are written with every character, so
changing it writes no character row.

**A character can be built.** The character creator's back end
(`internal/character` build.go) makes a character from choices instead of a
slot: a name of up to 16 characters, a base template's face (the creator's ten, `1000-1009` body type A and
`2000-2009` body type B; the Warrior is the plain preset), voice, a starting
class and the eight attributes, six weapons by Standard row, affinity, level and an optional Ash
of War, four armour pieces, four talismans, the four ammunition slots, seven
spells (all `CharaInitParam` holds: `equip_Spell_01` to `07`; a player
memorises up to fourteen), and nine quick items with their counts (1 to 99).
The pouch (`secondaryItem`) is left empty: the AI uses the quick items and
not the pouch.

**Every character drinks and casts as the game's own people do.** The first quick slot always holds one NPC Flask of
Crimson Tears (goods `50203`, half the health), the flask FromSoftware gives its
people and the only one the AI's item judgement knows; the player's flasks
carry no judgement. The flask
limits are the cast's, 3 and 3, and there is no cerulean flask: every
character's body - a custom character's and a save slot's alike - carries
`511063` resident, a copy of the Cerulean Hidden Tear (`511060`) made permanent
and invisible, so its spells and skills cost no FP, while items are still used up and the
one flask stays one drink. The row is written while the save holds any
character's body and removed with the last (`own-effects`, SAFETY). Both work
on the console. How often a character casts a spell is the spell's own: its AI
judgement's kind - one press, taps, or a hold - and a held spell, such as the
player's Comet, is cast rarely, where its NPC copy is cast again and again
(seen on the console). So **every character casts a spell's NPC copy where
the game has one**: the same spell with no requirement, 1 FP and the
judgement FromSoftware gave its own people (56 of the builder's 202 spells
have one). The
builder offers the player spell and the row holds the copy; reading a build
back turns the copy into the spell again (`internal/character`
spellcopies.go). A spell with no copy is written as it is.

It becomes the same rows as a
slot's character, at `2990100 + 10 x n` with its bodies at `199900100 + 10 x n`
and the next three ids, for n from 0 to 9 (SAFETY, the character bands), with what a save would hold
computed: the health from Vigor on the game's curve (`CalcCorrectGraphParam`
row 100, which every baseline character's health matches), the level as the
attributes' sum less 79, and the template's `FaceParam` row copied. Every
choice is checked against the regulation: a weapon must have a row at that
affinity and a level within its upgrade ladder (`ReinforceParamWeapon`, +25 or
+10), an armour piece must fit its slot, arrows go only in an arrow slot and
bolts in a bolt slot (`wepType`), and a talisman, a spell and an item must
exist. An Ash of War must go on the weapon - a weapon that takes one
(`gemMountType` 2) and an Ash whose `canMountWep` bit is the weapon type's -
and the affinity must be one the skill allows: the Ash's own list, the physical
four and its element's, or without one the weapon's own skill's, the list
every Ash carrying that art allows (`internal/character` ashes.go). Standard is
always allowed, and 192 weapons' own skills allow every affinity but Blood and
Occult, as the game's rows say. A requirement is warned of, never refused: a weapon's at its affinity (`properStrength` to
`properLuck`, the one-handed values) and a spell's (`requirementIntellect`,
`requirementFaith`, `requirementLuck`) against the attributes
(`internal/character` requirements.go), the copy's where the spell ships as
its copy - none.

**A custom character is kept until it is deleted.** A build is staged by
number (`StageBuild`) and written by itself, whether or not a formation names
it, and every later Save keeps it; `StageBuildDeletion` removes its rows and
body, refused (`ErrBuildNamed`) while a formation the save will hold summons
it. Opening a save reads every one back into choices (`ReadBuild`: the name,
the face matched to its base template, a custom weapon back into base,
affinity, level and Ash), so the builder opens it as it was made. A reset
deletes them all, and says how many. The name is kept in the row's 26 bytes no
shipped row uses (`pad2` at 238) and only the editor shows it: a name in the
world is game text the regulation does not hold, and the engine ignores those
bytes. The name takes the first 25 and the starting class + 1
the last, zero on a build made before the class (read back as the Vagabond).
A class is `CharaInitParam` rows `3000-3009`, Vagabond to Wretch: the
attributes it starts with (`Choices.Classes`) are the least the builder lets
a character of it have; the row itself does not need them, so `Rows` does not
check them. The builder offers only what
the AI can use: the item list (`internal/items`, generated
from the merchant editor's) keeps every weapon, armour piece and talisman, and
of the spells, goods and Ashes of War those with a use-judgement id - for an
Ash, its art's, which leaves
out No Skill alone.

The counter semantics are SaveForge's, derived there by diffing native saves,
and its regression clauses are restated as this package's tests.

## The catalog

The product embeds its catalog as data in `reference/`, generated from the
game's regulation and map files by the author's research tooling (not part of
this repository) and never edited by hand:

| File | What it is |
|---|---|
| `catalog-ashes.csv` | the 84 Spirit Ash families: eleven goods rows each, trigger families, save ids |
| `catalog-families.csv` | one row per `cXXXX` model family |
| `catalog-actors.csv` | one row per world `NpcParam` row: its family, class, arena tier, granted effects |
| `catalog-hierarchy.csv` | what a player is **shown**: one row per entry, every column a tag |
| `think-turns.csv`, `bodies.csv` | which creatures cannot turn; each model's body on the ground: its collision radius, and a long creature's head and tail |
| `shared-effect-writes.csv` | the shared effect writes and their warnings (SAFETY) |
| `vanilla.json` | the shipped value of every field the editor writes, read out of a save on the newest regulation |

**The hierarchy is one generated file**, and the reader keeps every column as
a tag: a change to categories, cards, types or words reaches the GUI with no
code change. Nine columns are
read by name (`key`, `trunk`, `category`, `card`, `card_name`, `type`,
`variant`, `ships`, `offer`). `offer` is the gate: `ship`,
`ship-with-warning` or `hold` (never offered; a broken row is held). The panel also shows `tag`: the
catalog's words for what is
odd about an entry (`statue`, `extra`, `|`-joined when both), which keeps an oddity on its
own creature's card instead of in a category of its own. A card may live
under two categories. The category `ashes` is the game's own Spirit Ashes, one
card per creature they hold, delivered as shipped.

**A creature's rules** are seven more columns, read into
`application.CreatureRules` by every row an entry holds: `look` (`toggle`, the
player's choice of the ghost or its own colours; `own-colours`, a body the
ghost loses a part of; `own-tint`, a creature whose own tint shows whatever the
look), `look_strip` (a named character's tint, taken off its clone),
`levels`, `level_default` and `marker` (a creature limited to some roles, and
the companion marker its clone carries), `arrival` (the animation it arrives
on where the spirit arrival does not suit it: St. Trina is drawn only on her
30000) and `partner` (a pair: the person who arrives on its spot,
`person:shell:arrival`). Staging refuses what a creature does not allow
(`ErrLookFixed`, `ErrLevelNotOffered`) and writes what it decides. A pair is
one slot and two rows: the mimic tear holds its spot as the Statue and turns
into the Silver Tear person, who arrives beside it, as the Mimic Tear Ash's
two rows do, and plays the slot's role; reopened, the save shows the two rows
it holds.

**A creature's composed fix** rides the actor catalog's `granted` and
`removed`: Midra carries his flaming head
`20010268`, the magic torch-pole Commoner `14709` in place of `14707` (its
torch's magic at an id its script never reads, so it never plays the ritual
that ends its attacks), and the Promised Consort his two phase markers held by
`20011605` and `20011606` (Effect rows of our own).

**The family key is the model, never the name** (the DLC ships its own Deer,
Rat and Fingercreeper). **No catalog file knows what a save holds:** the
session offers a variant only when the loaded save holds both its `NpcParam`
and `NpcThinkParam` row.

**Granted effects** are what the map's event script puts on one placed actor
and no row holds (the Stormhill knight's Bloody Slash is `14469|14475`). A
clone carrying them behaves as the placed actor, so the author delivers them by
default; `BareRow` is the opt-out. A **composite** is a creature the
game never ships, made from rows it does (the combined Ancestor): its adds
fold into the granted set and each names where the game gives it to that
creature, so a composite invents nothing. A **withhold** takes one harmful
grant off one row.

**Curation is not reference.** `curation/` is hand-written judgement, not
synced: the panel reads `pictures.csv` (a variant that shows a sibling's
picture: one that differs only in its moves, or one given its card's
picture), `covers.csv` (the picture each card and entry shows, or a
card's own) and `entities.csv`'s `same_as` (the model a DLC re-issue borrows
its picture from). The pictures are the icons in
`internal/ui/gio/assets/entities/`.

## The frontend boundary

`internal/application` owns catalog resolution, validation, manifest
construction, save mutation and structured results. **No frontend names the
domain stack** (`savecore`, `plan`, `catalog`, `character`, `formation`) or
re-derives its rules; what a frontend needs is re-exported there.
`TestFrontendsStayBehindTheApplicationBoundary` holds every frontend to its own
import set. Every failure a frontend must tell apart is reachable with
`errors.Is` / `errors.As`, never by message text.

It offers two shapes:

- **`Service`**, one whole operation per call (`Ashes`, `Inspect`, `Preview`,
  `Apply`, `Author`, `Reset`) - what a command line wants.
- **`Session`**: load, stage as many changes as you like, commit them in one
  write - what an editor wants. `Apply`, `Author` and `Reset` are one call each
  over a session, so there is one path to the file.

`Inspect` is cheap and only fails on a file that is not a decrypted PS4 save; a
save from another version comes back unsupported rather than failing.
**A preview is binding**: its manifest is the one the write produces, and a
test compares the bytes. Save-touching calls take a `context.Context`;
cancellation is checked between stages, and a cancelled operation publishes
nothing.

The session's rules:

- **A staged change is keyed by what it targets**, so editing one Ash twice
  replaces rather than accumulates, and an undo is a deletion.
- **A reset stages** like anything else and sits beside other changes. It
  puts back every row the editor can write, whichever session wrote it: the
  formations, clones and our own rows, prices and conversions, the shared and
  person writes, an earlier version's colour switch (SAFETY), and the game's
  own summon rows a Fighter left. A grant is the one exception (The character
  slots).
- **A reopened save stages nothing and reads as it was composed**: a held slot
  is named as a drop of the same entry is (a creature by the world rows its
  clone copies, a person by its `CharaInitParam` row), and re-staging a held
  formation beside a new change keeps every clone its slots name.
- **Two changes may share a goods row when they name different fields**; every
  other table is claimed by whole rows. Where two meet, the one that named the
  row wins: a formation over a conversion's blanket recipe, both over a reset.
  An Ash staged as both authored and converted is refused at staging.
- **`PrepareCommit` and `Commit` are separate calls**: a native Save As dialog
  is modal to the OS and not to the app, so the plan is frozen before the
  dialog opens. A successful commit advances the session to the file it wrote.

## Limits

- The health-bar label stays the host Ash's name; display strings are not in
  the save's regulation.
- One live summon is game manager state; the Colosseums and a character's
  ability to summon at all are outside these tables.
- PS4 only, regulation `11701000`, `11711000`, `11721000` or `11730034`.
- No multiplayer guarantee (SAFETY rule 4).
