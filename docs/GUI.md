# The desktop editor

`cmd/ersummoneditorgui`, built on Gio in `internal/ui/gio`. It is a frontend
over `application.Session` and nothing more: staging, composition, validation
and the commit all live below it (see [`DESIGN.md`](DESIGN.md#the-frontend-boundary)).
**If a file in `internal/ui/gio` starts deciding what an edit means rather than
how it looks, it is in the wrong package.**

## Three views

| View | What it holds |
|---|---|
| **Characters** | the open-file bar, the ten character slots, and which Ashes the chosen character has |
| **Editor** | the containers - every Ash, or the open one on its placement grid - and the spirits catalog |
| **Settings** | the theme, the typeface, full screen, three icon sizes, Wider summoning area, No once-per-rest limit, All free, Reset to vanilla |

The header is the title and the tabs. The footer is on every view, so a staged
change is never out of sight. Before a save is loaded the panels show an empty
state and nothing else. The window opens as a 1280×800 dp window, with a
minimum of 1180×760 dp, the same on every system; Settings' Full screen fills
the whole display with no title bar. `app.Fullscreen` comes after `app.Size` among the
window's options, because they apply in order and Size puts the mode back to
Windowed.

**Esc asks before the editor closes** (`quit.go`): a full-screen window has no
title bar to close it from. Esc goes first to what is open - a dropdown's
list, the creature window, the Pending list, an explanation - and only an Esc
nothing took opens a small dialog: *Close the editor?*, with
what closing loses (changes staged and not saved, a character in the builder
not staged), Cancel, and Close (*Close without saving* when something would be
lost). Esc again or a click outside closes the dialog.

**Design at 125% desktop scaling.** A maximised 1920×1080 Windows screen at the
usual 125% is 1536×826 dp, and the vertical budget is the tight one: about
669 dp of panel under the header and footer. `screenshot_test.go` renders at
1.25 for that reason.

## Characters

**The open-file bar**: a path field, `Load`, `Browse…`, and the open file's
name at the end - the controls and the one label that says what they acted on.
`Browse…` is the usual way; the field reaches a file a picker will not show.
Opening another save over changes not yet saved - staged ones, or a character
in the builder not yet staged - asks first, as closing the editor does.

Under the bar are two columns, each opened by a header line of one fixed
height - title, a muted detail, the count on the right - so their rules line up.
Controls never sit in a header; a column's actions are a toolbar under it.

**The slot chooser** lists the ten slots as rows of one height: the slot
number (as the game's load menu orders them), the name, and `Level N · Class`
under it, which is what tells apart a save's six DrippingSoups. The chosen row
carries the accent edge, as the Editor's lists do; an empty slot reads `Empty`
and cannot be chosen. A save opens on its first character.

**The Ash list** is every Ash in the catalog - the Kristoff Ash included - in
rows with a rule under each, its column count following the icon size. A row is
icon, name, level slider and one button, the last two at the row's right edge
so they line up down the column:

- **an Ash the character holds** is drawn in full, with its level slider and
  `Remove`;
- **an Ash they do not** is veiled, with `Add`, which stages a grant at the
  toolbar's level;
- **a row with a change staged** is lit in the glow.

The button only ever says `Add` or `Remove`, after what the character will hold
once the save is written: pressing `Add` turns it into `Remove`, and pressing
that takes the addition back.

The toolbar holds `Level for every Ash` - it sets every held Ash and the level
later grants use - and `Add the N missing`, which turns into `Remove the
N added`. A slider runs over the eleven levels, chooses the one it is let go
on (a drag stages nothing on its way), and shows the most specific answer: a
staged change, else the save, else the toolbar's level. Sliders replaced
lists, which ran off the screen under a row near the window's foot. Each `Add` raises a
notice that Reset cannot take a granted item back. A character whose storage
box could not be read gets one warning line under the toolbar.

## The Editor

```text
┌──────────────────────────────────────────────────────┬─┬──────────────────────────┐
│ [‹ All Ashes] ▫ Lone Wolf Ashes   FP [55]  HP [0]  ☐ │ │ SPIRITS          105/398 │
│ ┌──────────────────────────────────────────────────┐ │ │ [search…               ] │
│ │ ·  ·  ·  ·  ·  ·  ·  ( 5 m )  ·  ·  ·  ·  ·  ·   │ │┃│ [ashes     ▾][sub     ▾] │
│ │ ·  ·  ·  ·  ·  ·  ▣  ·  ·  ▣  ·  ·  ·  ·  ·  ·   │ │┃│ ▫ Albinauric  ▫ Ancestral│
│ │ ·  ·  ·  ·  ·  ·  ·  ·  ▲  ·  ·  ·  ·  ·  · [+]  │ │ │ ▫ Archer      ▫ Avionette│
│ │ ·  ·  ·  ·  ·  ·  ▣  ·  ·  ▣  ·  ·  ·  ·  · [-]  │ │ │ ▫ …           ▫ …        │
│ │ ·  ·  ·  ·  ·  ·  ·  ·  ·  ·  ·  ·  ·  ·  · [◎]  │ │ │                          │
│ └──────────────────────────────────────────────────┘ │ │                          │
└──────────────────────────────────────────────────────┴─┴──────────────────────────┘
```

Two halves: **the containers** on the left, and **the spirits catalog** on the
right, two cards a row until the player chooses otherwise. **The divider**
between them is dragged to share the width: the catalog keeps at least one
card and the grid a few metres, the catalog holds as many cards a row as its
width does (`catalogCard`), and the width is stored with the settings when let
go.

**The containers** start as every Ash but the Kristoff Ash, whose summon the
game binds itself, as tiles - the item's icon over its name - stretched to fill
each row. A tile with a change staged is lit in the glow; one whose formation
cannot be edited says why in ember (*converted*). Clicking a tile
opens that Ash, and the whole half becomes its formation; `‹ All Ashes` goes
back. Choosing an Ash is an index into the loaded snapshot; loading another
save clears it.

**An open Ash** is a bar - the way back, the Ash's icon and name, a note under
the name when there is one - and under it a strip of the Ash's own options in
two groups (`layoutAshOptions`), side by side, or the grid's under the cost
where the half is too narrow for both:

- **COST**: two fields, FP and HP, validated on every keystroke
  (`plan.Price.Validate`) and staged as soon as both read as a number; typing
  the save's own price back unstages. A click into a field selects what it
  holds (`selectOnFocus`), so a number typed replaces it rather than joining
  it, as in the builder's number fields. The fields show what the Ash will
  cost: a price staged for it, else the save's, the game's -1 (a price the
  Ash does not pay) as 0, which is written back as -1 while it stays 0.
  **Free** beside them makes
  the Ash cost nothing (`application.FreePrice`, the game's own 0 FP / -1
  HP); while it is ticked the two fields only show the price, and unticked
  the Ash is back on its own. It is ticked whenever the Ash will cost
  nothing, however it came to;
- **GRID**: **Show sight**, which draws on the grid what the chosen summon
  notices by itself for the role it has: its eye cone - reach, width and the apex
  behind its centre - and its nose all round. It is the writer's own profile
  (`Session.SightOf`, `savecore.LevelSight`), and the row's own senses for the
  Loner and a row kept as the file holds it. **Turning knobs** shows or hides
  the knobs on the grid; the creature window's facing and the arrow keys turn
  a summon either way. Both are stored in the settings, for every Ash; an
  Ash whose formation cannot be changed has no knobs to offer.

Under the strip is the grid.

**The placement grid** (`placement.go`, drawn by `gridpaint.go`) is the ground
around the player, one cell a metre, 25 m every way: the player facing up,
rings every five metres. The game sets no distance limit - the map does: a
summon travels along a line from the player and stops where something blocks
it - so a far cell is allowed and is the player's risk. +z is ahead, +x to the
right, and a positive facing turns the summon right. A cell is where the summon
will stand: the editor writes its row 3.2 m short, since the game stands a
summon that much further ahead, and shows a save's formations where
they stand - the game's own about 3 m in front of the player.

- **The view** opens on the player with five metres to the panel's nearer
  edge, every time an Ash is opened. The wheel zooms about the pointer, from a
  few cells to the whole ground (a touchpad's pinch arrives as Ctrl and the
  wheel); a drag on empty ground looks around, never further than the ground
  goes. Three buttons in the corner zoom nearer, farther, and home.
- **Every summon stands in one cell**, and the cells are the only places the
  grid offers. A place the save gives between cells, or the ring a slot
  without one stands on (`application.RingPlacement`), is drawn in the nearest
  cell no other summon - and not the player - stands in, and the first edit to
  the formation writes every summon in the cell it is drawn in.
- **A cell** is a tile: dark inside a calm blue frame, a darker gap between
  tiles. The inside and the frame carry different news, so both read at once.
  The inside says what stands there: soft where a body covers it, and
  **yellow where two stand too near each other** - placing them so risks the
  two arriving inside each other. A summon covers its own cell and each cell
  around it that its body fills half of, seen from above, turned with its facing. A body is the creature's look from
  above: its collision circle, or, on a creature the catalog lists, as wide as
  it looks where its collision is far smaller (Miriel's shell) and, if
  clearly longer than wide - a wolf, a horse, a dragon - as long as from
  where its tail ends to where its head ends standing, with what the game
  folds trimmed: a thin tail cut, a curled one no longer than the head
  reaches ahead (`Session.Bodies`). A person or a knight fits its own cell - the
  Crucible Knight's tail comes out only in its attacks; a wolf covers one
  more ahead, a horse about one ahead and two behind, Commander Gaius a band
  three cells wide, a dragon or the Walking Mausoleum tens of metres. The
  warning compares the bodies, not the cells: two closer than a tenth of a
  metre - the player among them, as a person - light the cells both cover and
  their own two, while two with ground between them do not. Height is not
  counted: a rat under the Walking Mausoleum warns too. It warns and never
  refuses. The frame says what the chosen summon notices by itself, with
  *Show sight* on - one summon at a time, so the grid stays readable, and
  none while nobody is chosen: blue for an ally,
  red for an enemy, bright where its eyes see ahead and faint where only its
  nose reaches - all round, so behind it too.
- Each summon is a **token**: a square with its portrait, turned a quarter at a
  time with its facing (upright to 45° either way, on its side to 135°, upside
  down beyond, looking back at the player), and an arrow on its edge for the
  exact facing. The frame is red for an enemy, ember for a summon seen to hurt
  the player, accent and thicker for the chosen one. A hover names it with
  its whole name, variant included, and the name stays while the pointer is
  on its turning knob; nothing is said while it turns.
- **The knob**, round, on a token's edge out along its facing, turns it. It
  shows only while the pointer is on its summon or on it, and while it turns
. *Turning knobs* in the strip hides them all.
- An Ash whose formation cannot be edited is drawn, zoomed and panned, and its
  summons' windows open to be read; nothing else is offered.

**The pointer works the same with a mouse and a touchpad.** A touchpad drags
with a tap and then a second tap held down, which arrives as a click and then
a press. A left click on a catalog card opens it at once, and for the system's double-click time (`platform/clicks`) the
card keeps its place under the pointer - a card above that closes scrolls the
list to match, and scrolling an opened card into view waits - so the second
tap lands on it: a second click leaves it open, and a drag closes it again
and drags it. A custom character opens the builder, which takes the panel,
so its click still waits that time. A left click on a token only
chooses it, which a drag does too, so it waits for nothing. A card is a drag
only once the pointer has moved 6 dp (`dragged`): Gio's `Draggable` says it
is dragging from the press itself.

| To | Mouse | Touchpad | What happens |
|---|---|---|---|
| Choose a summon | click it | tap it | it is chosen alone; nothing opens |
| Pick several | Ctrl-click each, or hold Shift and pass the pointer over them | Ctrl-tap each, or hold Shift and slide over them | a Ctrl-click joins one to the summons picked, chosen so its sight shows, and on a picked one lets it go; with Shift each summon the pointer passes over joins them, no button pressed, and the chosen one stays chosen; a click on the ground lets all go |
| Open a summon's window | right-click it | two-finger tap | the window opens at once on that one summon, which is chosen, keeping those picked with it |
| Move a summon | drag it | tap, then tap and drag | it stands on the cell let go on, and a summon standing there leaves the formation; out of the grid, it is removed. Dragging a picked one moves every one picked as far and the same way - only onto free ground, or nothing moves and the footer says why - and out of the grid removes them all |
| Add a summon | drag a variant to a cell | tap, then tap and drag | it stands there, facing forward: in the slot of a summon already there, which it replaces, or else in the first free slot of ten |
| Add a card or an entry | drag it to a cell | tap, then tap and drag | each of its entries (an entry: each of its variants) in the card's order fills a free slot, replacing nobody: the first on that cell, each next on the nearest free cell where its body keeps clear of every other, a row ahead or behind counting twice a cell aside so they stand in ranks, facing forward; what does not fit in the ten is counted in the footer |
| Open a catalog card | click it | tap it | its entries or movesets open at once in a frame under its row; a later click closes it |
| Turn a summon | drag its knob round, or the arrow keys | tap, then tap and drag the knob | it faces the pointer, in 45° steps, and is chosen; every one picked with it turns as far, each from its own facing |
| Look around | drag the empty ground | tap, then tap and drag | the view moves; a click on the ground lets the summons picked go, and a drag pressed within the double-click time after it picks them again, since a touchpad's drag starts with that tap |
| Zoom | the wheel | pinch, or two-finger scroll | about the pointer |
| Size the catalog | drag the divider | tap, then tap and drag | stored when let go |

The keys: the left and right arrows turn the summons picked 45°, and Esc
closes the creature window and the Pending list - and, with nothing to close,
asks to close the editor; it never lets a summon go (a click on the ground does that). None is taken
while a text field has the keys, and a press on the grid, or a drop on it,
takes the keys back from the field (`takeKeys`): the grid holds no focus of
its own, so without it the arrows would go on moving a field's caret.

**Every change calls `StageFormation(host, slots)` once with the whole
formation**: staging replaces, so there is no incremental slot API. Every slot
is staged in a cell of its own (`cellPlaces`): two summons never share one,
and a second summon on a cell the save gives twice is drawn - and, once the
formation is edited, written - in the nearest free cell. The last summon cannot be removed, and a
refusal is shown in the footer.

**A summon that can hurt the player says so.** Two actors are seen
damaging their own summoner (the Furnace Golem's burning stomp, the Merciless
Chariot); the ally matrix is in the executable, so no save edit can stop it. On
the drop the footer warns in ember, the token's ring stays ember, and the hover
says why. It warns and never refuses.

**The creature window** (`creature.go`), opened by right-clicking a token, is a
modal over the dimmed window, closed by ×, Esc or a click outside it. Like
every dialog it sits a tenth of the window down and keeps its top there
(`modal`), so a click that lengthens or shortens a line moves only what is
under it: a centred dialog would recentre and slide the button just clicked
away from the pointer. Nothing in it changes under the pointer alone. Under
the summon's portrait, name, place and any warning it holds two columns: the
role and the strength in the first, the facing and the three switches in the
second.

- **the role**: six buttons from the Statue to the Loner, three to a row, the
  slot's own lit, with a line under them only for a creature that takes fewer;
- **the strength**: Helper or Fighter, a new summon a Helper. A creature the
  player dropped carries its own, and one of the player's characters the body
  of its role in that slot; on a row of the game's own Ashes or a person's
  shell the choice is the row's, and a line under it says every Ash (or
  person) built on it changes too; a row kept as the save holds it shows the
  strength it carries, fixed;
- **Share aggro**, the share box (`application.ShareOffered`): whether the
  summon takes the player's fights - what the player hits or locks on to, and
  any enemy fighting them - or picks its own with its role's senses. A choice
  on the Guard, the Sentinel and the Hunter, off at first; always on for the
  Follower, which by itself notices two metres; never on the Statue, the
  Loner or an enemy;
- **the facing** (`facing.go`): eight buttons around a needle set the eight
  ways, ahead up as on the grid, the way the summon faces lit; a line beside
  them says it in words. They are the only facings the editor gives - the knob
  and the arrow keys turn in the same 45° steps - and the needle shows a
  facing between two that a formation the save holds may carry. Every summon
  of an Ash that can be changed turns, a creature of the game's own Ashes too;
- **Enemy**: a hostile team on the world branch, playing the Loner;
- **Own colours**: the summon drawn with no spirit coat; off, the game's ghost.
  A switch that cannot change has its padlock and reason on the line under it;
- **Remove from the formation**, beside the refusal the window's last change
  met.

**Explanations are asked for** (`help.go`): the window carries no lines that
explain; a thing that needs explaining has a small ? after its name - the
creature window's fields, the grid's options (by GRID), the Pending list's
title, Settings' Full screen, its three Spirit Ash switches and Reset, and the
builder's Name, Attributes, a requirement and the Ash of War. A click on it
opens what the thing means in a bubble under it, until the ? is clicked
again, another is, the bubble itself is, or Esc, which lets the bubble go
before it closes a dialog or opens the close dialog. Pointing at a ? only
lights it. One bubble shows at a time, and one whose ? the frame no longer
draws - another view, a closed dialog - lets go of itself. A short line beside
a thing says only what state it is in: why it cannot change, what it needs,
what it risks.

Each change stages the whole formation like the grid's, with the stance the
role and share call for (`application.LevelStance`). What the window offers is
decided once a frame (`choices`), and the window draws and acts from that one
answer. What a summon cannot take is drawn recessed and is no control at all:
nothing is laid out under it, so no click reaches it (a disabled Gio control
still takes clicks read elsewhere). A padlock and the reason stand beside it;
on an Ash whose formation cannot be changed, one line under the name says so
for the whole window, and Remove is not offered. A row kept as the file holds it, and a
creature of the game's own Ashes on its shipped row, keep their role, share and
look (another role needs a second row, `ErrAshLevelOwed`); an enemy's role is
the Loner, although the writer would also take the Hunter or the Statue; a
person cannot be an enemy yet, because the world branch needs a clone and a
person's shell is a summon-side row already. A person the game ships has no
look to choose; one of the player's characters has two, the Puppets' coat
(off) or its own colours (on), and since the look is only which of its two
bodies the slot names, a character the file holds changes it too.

**A creature's own rules** come from the catalog (`application.CreatureRules`): a body the ghost loses a part of
(the Elden Beast, the Death Rite Bird, the machines, the Merciless Chariot) is
in its own colours, fixed; a creature with a tint of its own keeps it and has
no look to choose; and the large Shadow Commoner takes only the Statue and the
Hunter, arriving on the Hunter.

**Conversion is not in the GUI.** A player who wants another Ash's formation
drags its spirits onto the grid; `-convert` stays on the command line.

### The spirits catalog

The panel builds from `Session.Hierarchy()` and knows none of its vocabulary:
categories, cards, types and words are the catalog's and arrive with the data.

- **Three depths, opened in place**: a card (a model family under one
  category), then its entries, then their variants. A card opens in a frame
  under its row holding its entries, and an entry inside opens its variants
  in a frame one deeper; the rest of the catalog stays where it was, and a
  second click closes either (over a Back button and
  a grid replaced at each depth). One card is open at a time, one entry in
  it. A depth is skipped where it holds one thing, a card just opened below
  the panel's foot scrolls up to the top, and the frames keep the catalog's
  columns.
- **The filters** stand in one line where they fit (`filterBar`): the
  dropdowns, each as wide as its longest option, then the search taking the
  rest of the line, or a line of its own where less than 180 dp is left. The
  dropdowns come first, so the one just used never moves.
- **The category dropdown** lists the hierarchy's categories in its order,
  capitalised, and opens on the game's own Ashes until the player picks
  another. A category is always chosen: there is no "All categories". **The
  subcategory dropdown** beside it, only where the category has subcategories,
  narrows inside a card (construct, undead, monster, mage, soldier and folk
  under the enemies; major, minor and character under the bosses; quest,
  invader, other and class under the NPCs; mount under wildlife). A
  card sits in one subcategory.
- **Search** narrows the chosen category by every name a player can type; the
  count on the title line says how much of the catalog is showing. Where
  nothing in the category matches, the empty catalog names the categories
  that hold a match and how many each holds.
- **Inside a card a name drops what the card says**: on the Omen card
  "Omen - Greatsword" reads "Greatsword", and a
  variant shows only its own words, "Hooded" under the Axe, or its entry's
  name where it has none; a summon dragged out
  keeps its whole name. On the Soldier, Foot Soldier and Knight cards the
  entries are the factions and the variants their weapons, "Godrick Foot
  Soldier (Dagger, trumpet)".
- **Every depth is a drag source, and places what is under it**: a card each of its entries as its first moveset, an
  entry each of its movesets, a variant itself; opening a card is how one is
  placed. The order is the catalog's. The payload is
  identities (`npc,think,person`, `;`-joined), not pointers.
- **Under a name, words say only what is behind it**: a card's entries, an
  entry's variants, or a variant's HP. **What it is, small icons after them
  say** (`marks.go`): *statue* (a pause sign), *extra*
  (a star: the game never places it), *on foot* (a walker: the world seats it
  on a horse in most of its placements, and nothing seats a summon), *hurts
  you* (an ember triangle) and *note* (an "i": it works, and differs from the
  game). A card or an entry is a statue, an extra or on foot only when all of
  it is, and shows a hurt or a note when any of it does. The note's words show
  when the pointer rests on its icon, in a label growing leftwards; on a card
  whose creatures differ they say to open it. The words are the catalog's
  warning said for a player, and only what is true of that row: its tags and
  its own clauses, and of its model's hazards only those true of every row
  (a large body, a mount, ...); row numbers, animations and asides dropped.
  The legend is the "?" beside SPIRITS. A
  `hold` entry never arrives, and a broken one - its moves do not fit its
  body - is held.
- Portraits come from `assets/entities`, one per variant, named for the row
  it ships (`pictures.go`). An entry and a card show a representative: the
  chosen cover (`curation/covers.csv`), or a card's own picture where one
  is drawn (`card_boss_c3560.png`, the two Godskins), and otherwise the first
  of their variants with a picture. A variant without one borrows the
  sibling's curation gives it (`curation/pictures.csv`), then a picture of
  its own body, then the body `same_as` names; with none, it shows its entry's
  and then its card's. A card without any is a named card, not a hole. A
  creature in an Ash's slot shows the same picture: the character it summons
  where curation names one, its own where the Ash shows a look no other
  picture has (drawn for it), else its body's. An Ashes card, and
  what is under it, shows its Ash's item icon instead (`familyView.Ash`), so a
  creature the game ships as a summon is not taken for the enemy it is drawn
  from.
- **The player's own come last**, after the game's six, in one category,
  **Custom**, of two cards: *Your characters* (the
  save's slots), there when the save has one, then *Custom characters*, there
  even empty. That card always
  opens on its characters, after a tile that adds one; a character - a
  slot's or a custom one - is pictured as its starting class, each card as its first character, the empty Custom card as the
  Vagabond a new character starts as; clicking a custom one opens the builder
  on it. Its card comes from
  `Session.CustomCharacterCard()`, apart from the hierarchy, and is put back
  whenever a build or a deletion is staged or dropped.

### The character builder

Opened from the Custom characters card, it takes the Editor's place, split as
the Editor is. The left half is the character: its name (up to 16 characters;
the card, the bar and the pending list show it, the game never does), body
type, base template and voice, the starting class (the Vagabond for a new
character) and the eight attributes (the level and health follow) in one
column. No attribute takes a value under its class's: one typed under it is
not taken, and the field shows the attribute again once the keys leave it -
any press in the builder takes them, and a press on a field gives them back.
Another class moves attributes still at the old class's to its own, as the
game's creator does, and raises edited ones only where they fall under it;
beside it the equipment as the game's equipment screen lays it
out, slots of a kind side by side: the armaments a grid of the right hand's
three over the left's, the ammunition one of the arrows over the bolts at the
row's end, each under one label since the cells name themselves (R1, B2), then
the seven spells, the
armour beside the four talismans, and the ten quick items, every row ending on
one edge. There is no pouch: the AI uses the quick items. An empty slot shows where it is (R1, A2, Head); a filled one its
item's icon, with a weapon's level or a stack's count in the corner and its
Ash of War small in the other. Under them, the chosen slot: its item, a
weapon's affinity, its list opening upward, and its level on a slider (only
what the save's regulation has for it), its
Ash of War with Choose, a stack's count, Remove. Choose turns the slot to the
weapon's Ash of War: the catalog then lists only the Ashes that go on that
weapon, a click mounts one and turns back to the weapon, and Remove takes it
off. The affinities offered are the skill's - the Ash's, so a lightning Ash
allows Lightning, Sacred and the physical four, or the weapon's own - and an
affinity the new skill does not allow moves to the Ash's own, or to Standard;
a weapon that takes no Ash says so. The right half is the items the chosen
slot takes, with a search and the category and subcategory, laid out and
chosen as the spirits' catalog's are: a category is
always chosen - no "All categories" - and the subcategory has "All
subcategories" first; each dropdown is drawn only where it is a choice (not a
talisman's one category). A slot opens on the category of what it holds,
else the right hand on melee armaments, the left on shields, a spell on the
sorceries. An Ash of War has the search alone: its list is already only what
goes on the weapon. A click puts one in. Spells stay packed from the first slot, as the row holds them. A weapon
or spell whose requirement the attributes do not meet gets an ember border
(a spell cast as its NPC copy has none),
the bar counts them, and the chosen slot says what is missing ("Needs
Strength 10 (has 8)"); the character is built anyway. Nothing stages until Done, which checks the character against the save and says
on the bar what the game cannot build; Cancel drops the draft and Delete stages
the deletion.

## The footer

**Save leads**, with the staged count in its label. `PrepareCommit` runs
**before** the Save As dialog opens, and `Commit` writes that frozen plan. After
a write the editor follows the session to the new file and keeps the
player's place - the open Ash, the builder, the chosen character - since the
file holds the same Ashes and slots. A commit whose reload fails is reported
as written, with the output offered as a fresh load.

**The message line** (`notice.go`) is centred, a size up from body text, and
short: what happened, not why - the ?s say why. A dot before it says its
kind: the accent for news, ember for a warning, red for a refusal, the text
in the same colour for the last two. News fades after six seconds, its last
second fading, so nothing stale is left on screen; a warning or a refusal
stays until the next press anywhere in the window (a pass-through catcher
over everything, read before the frame draws, so the press that leads to a
new message cannot settle it). While a dialog is open the line is drawn over
its scrim - deferred twice, so after the dialog - and every dialog stops
short of the footer (`dialogBottom`), so what a change made in the dialog did
stays readable.

**Pending (N)** opens a modal dialog, like the creature window - the window
dimmed under it, closed by ×, Esc or a click outside, its ? saying that
nothing is written until a save and that Reset leaves the characters' Ashes -
listing every staged
change grouped by the part of the window it was made in:

| section | groups | each change |
|---|---|---|
| **Characters** | one per character | an Ash, with its icon: given at +N, from +N to +M, or taken away - the given first, then the levelled, then the taken |
| **Editor** | one per Ash, with its icon | its formation (how many it summons, each summon once with how many, an enemy marked) and its price (what it will cost, and instead of what) |
| | Custom characters | each one new, changed or deleted, with its level and weapon |
| | The game's own summons | how many rows were made Fighters or Helpers |
| **Spirit Ashes** | - | each monument switch moved, on or off |
| **This save** | - | Reset to vanilla, and what it puts back |

Every change has a `Drop`, and a section or group of more than one a
`Drop all`. A section and a group fold under the arrow beside them, saying how
many changes they hold and keeping their `Drop all` - or, folded over one
change, its `Drop`; what is folded stays folded while the window is open. A
detail wraps rather than being cut short. The list scrolls under the title. It
is built from the session, so anything listed will be written and anything written is
listed.

**Reset to vanilla** is in Settings, not the footer - a whole-save revert does
not belong one click from Save on every view. It stages like anything else and
turns into *Drop the reset*. Staging it is the one staged change that runs on a
worker, because a reset is a diff against the loaded file; an unedited save says
there is nothing to reset.

## Settings

Two columns of cards in the middle of the panel, each card a title over its
rows, every row a name on the left and its control at the right edge, so the
view fits the window without scrolling. On the left **Appearance** - the
theme (below), the font (Lora, or Cinzel for the whole window), Full screen
(off by default, applied at once and kept for the next start) and, under
*Icon sizes*, the Characters page and the Editor's Ash rail at 40-160 dp and
the spirit portraits at 24-96 dp; on the right **Spirit Ashes** - Wider
summoning area, No once-per-rest limit and All free - and under it **This
save** - Reset to vanilla. Everything but the theme, the font and the sizes
carries a ?, since those names cannot say what they do. The two monument
switches show where the open save will stand once saved: moved, they stage a
change like any other, and moved back they drop it (DESIGN, The monument
switch). **All free**
is every Ash's Free box at once (`setAllFree`): on exactly when every Ash of
the open save will cost nothing; switched on, it ticks the boxes not ticked
yet and leaves the rest; switched off, it unticks them all, every Ash back on
its own price, a price typed for one too. Only the one family the game ships
free stays free. It is the open save's state, so it is not stored. The view
scrolls when the window is too short for it. A setting applies on the click and is stored in the user's
configuration directory, never beside the executable; a slider is stored when it
is let go. The Editor's divider is stored the same way, though it lives in the
Editor. Unusable stored settings fall back quietly.

## The look

Five themes (`theme.go`), chosen in Settings and applied to the whole window
at once. The merchant editor's three: **Spirit**, the default - cold, dark
and faint, a tool for calling the dead back, not a shop; **Light**, warm
paper with a slate-blue accent; **Elden Ring**, the game's own menus -
near-black, bronze edges, parchment text and a muted gold. And two of an old
book, taken from the catalog's pictures - crayon on golden parchment, umber
and slate-blue ink: **Parchment**, the page itself, a mid tone that is never
bright, umber text and the drawings' slate blue; and **Leather**, the dark
binding, with the page's golden colour for text and gold leaf for the
accent. Each sets every colour, the grid's too; the grid keeps blue for an
ally and red for an enemy in all five. Spirit's:

```text
colorBg         #0B0E12  window ground: black with blue in it
colorPanelBg    #121821  panels, barely lifted
colorFg         #D6E2E8  ash
colorMuted      #6E7F8C  secondary text, counts
colorDivider    #202A35
colorInputBg    #0E141B  recessed: text fields, controls that are off
colorControl    #1E2836  lifted: a button a player can press
colorAccent     #7FD4E0  spirit flame, as ink: slot edges, selection bars
colorAccentFill #1D5E6E  the same accent as a ground: primary, current tab
colorGlow       #A8ECF2  slot rings, at low alpha
colorWarn       #C9945A  ember: warnings
colorError      #C0473E  refusals
```

**A field recedes, a button lifts, an unavailable button recedes again.** The
pale accent never carries a label: the serif's thin stems antialias to grey on
it, so a primary button is the deep `colorAccentFill` under light text. Cells
touch (`gridGap` 0, a 1 dp border each). Panel titles are 15 sp in the
foreground colour. No motion beyond hover and drop-target state. Lora and
Cinzel are vendored under SIL OFL.

The application icon is the Spirit Calling Bell, compiled into a `.syso` by
`tools/appicon` and `rsrc`; Gio's Windows driver loads it from the executable's
resources.

## Ownership and concurrency

`internal/ui/gio` owns window layout and retained widget state, and renders
session read models. It imports `internal/application`, the platform packages
and `curation` (for portraits) - nothing below the application layer.

`Session` has one owner. Widget state belongs to the frame goroutine; `Load`,
`Commit`, a staged reset and the native dialogs run on a worker, which returns
an immutable snapshot the frame draws. A `busy` flag refuses a second load or
commit while one is in flight, and **no session method is called from the frame
while a worker holds it**.

A redraw costs about 200 µs: the icon cache is keyed on what the caller already
has (name and pixel size), so a warm hit allocates nothing
(`BenchmarkFrame`). Icons are stored at 128 px, twice their default draw size.

## Building and looking

```sh
go run ./cmd/ersummoneditorgui
VERSION=1.0.0 sh scripts/build-windows.sh   # the Windows zip
VERSION=1.0.0 sh scripts/build-linux.sh     # the Linux zip, on Linux
ER_SHOT_DIR=shots go test -tags screenshot -run TestScreenshots ./internal/ui/gio/
```

Windows Gio needs no cgo; Linux Gio does, so the Linux zip is built on Linux.
The Windows script carries `-H windowsgui`, without which Windows opens a
console behind the editor. WSLg reports a scale of 1 while its Windows host
runs at 125%, so under WSL the window is scaled by 1.25
(`cmd/ersummoneditorgui`), window size included; `ER_EDITOR_UI_SCALE` (0.5
to 3) overrides it anywhere. Under WSL the editor also draws through X11
(an empty `WAYLAND_DISPLAY`): through WSLg's Wayland its window stays black.
WSLg's X11 does not honour full screen, so there it opens as a window. The screenshot test renders real frames
through Gio's headless GPU path - a developer tool, skipped where there is no GL
or Vulkan context. A hand-built `layout.Context` must carry a real input source,
or every material widget draws its disabled style. The tests that need a
player's hands (`hands_test.go`) drive the whole window through Gio's router,
finding a control in the semantic tree by what it says, clicking where it is
drawn and typing as the platform types: a test that calls `setRole` cannot see
a button that moves under the pointer, or one that looks open and is not.

## Deliberately absent

Click-to-place, multi-select drag, an undo
history (a staged change is dropped, not inverted), and the audit manifest:
the command line writes one (`-manifest-out`), the GUI does not (no player uses it).
