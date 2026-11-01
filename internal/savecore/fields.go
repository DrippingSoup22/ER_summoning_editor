package savecore

// The shape of the three tables this product writes: which field lives at
// which offset, how wide it is, and what range a value in it may hold. Every
// validator checks against this and every writer writes through it, so a field
// this file does not name is a field the product cannot touch.

import (
	"fmt"
	"math"
)

const (
	goodsRefIDDefaultOffset = 4
	// consumeMP and consumeHP are the resource cost, both s16. Measured at
	// runtime: a positive field is both the minimum
	// the player must hold and the exact amount spent, so cost and requirement
	// are one field and not two. When both are positive the game uses AND
	// logic - either shortfall blocks the summon, and a successful use spends
	// both. 0/-1, 0/0 and -1/-1 are all free; the product writes vanilla's
	// 0/-1 spelling.
	goodsConsumeMPOffset = 128
	goodsConsumeHPOffset = 130
	// The range 1.17 actually holds, which is narrower than s16. -1 is the
	// unset spelling and 660, Mimic Tear's HP price, is the largest shipped
	// value.
	minGoodsConsume = -1
	maxGoodsConsume = 9999
	// Multiplayer flags are outside the write surface. These two facts are used
	// only by the bounded state reader and unchanged-byte assertions.
	goodsFlagsOffset     = 72
	goodsEnableMultiMask = byte(1 << 4)
	buddyAppearOffset    = 36
	// The stance pair. disablePCTargetShare and pcFollowType decide whether a
	// summoned formation follows the player or holds the ground it was called
	// on, and every shipped row spells them together: 140 rows at 0/0, 28 at
	// 1/1, two at 0/2, and none that mixes them.
	//
	// 0/0 written over Putrid Corpse Ashes row 21500000, whose shipped value
	// is 1/1, makes its summon follow on foot and warp back.
	// The identification run authored into the same host without writing
	// either field, inherited 1/1, and got a formation that would not follow.
	// One host, two runs, opposite results, these two fields the only
	// difference.
	buddyTargetShareOffset = 37
	buddyFollowTypeOffset  = 38
	// npcPlayerInitParamId names a CharaInitParam row - a human character with
	// its own stats, equipment and face - and when it is set the game builds
	// that person INSTEAD of the creature npcParamId names. Twelve shipped
	// rows carry one: Mimic Tear's second row, the five Puppets, and the DLC
	// NPC Ashes down to Jolán and Anna.
	//
	// Which is why an authored row has to clear it. Leaving it alone was the
	// defect the identification run found: ten hosts delivered the host's own
	// person at the requested count, and the one slot that came out right was
	// Mimic Tear's first row, the only row of the twelve blocks whose value is
	// already -1.
	buddyPlayerInitOffset = 84
	// The animation a summon arrives with. 1830 on 157 of the 170 shipped rows
	// and the ordinary spirit generation; the thirteen exceptions are the
	// human Ashes above, which arrive as a person does, plus Mimic Tear's own
	// 1831. Writing the ordinary one keeps a creature from arriving on a
	// person's animation, and changes nothing on the 157.
	buddyGenerateAnimOffset = 88

	// Where in the formation the actor stands, and which way it faces. Floats,
	// unlike everything else on the row: lateral metres, forward metres and
	// degrees. The shipped rows stand within x -1.8..1.8 and z -1.5..1.5, and
	// face -130..90 degrees.
	// The bounds a placement is held to.
	//
	// The angle is the field's own range. The offset is not a measurement:
	// nothing has been seen failing at any distance - actors placed at up to
	// 25 metres arrive where they are put - so the bound is wide enough to be
	// found by testing and narrow enough that a typo is still refused rather
	// than written.
	buddyOffsetLimit = 1000
	buddyAngleLimit  = 180

	buddyXOffset      = 24
	buddyZOffset      = 28
	buddyYAngleOffset = 32
	// cloneRowOffset and cloneRowCeiling bound the summon-side namespace. They
	// must equal formation.CloneOffset and its ceiling, which own the rule;
	// savecore does not import that package, so a test asserts the two agree
	// rather than a comment asking.
	//
	// The upper bound is not decoration. NpcParam holds 766 rows at or above
	// the offset, but 651 of them sit in the 5xxxxxxxx band, which is a
	// different ID scheme entirely - research calls those quest and NPC actors,
	// never a summon target. Exactly 115 are in 1xxxxxxxx, and those 115 are
	// exactly the rows carrying teamType 47, Spirit Summon.
	cloneRowOffset  = int32(100000000)
	cloneRowCeiling = int32(200000000)
	// The clone BANDS (formation.CloneBands, which owns the
	// rule; a test asserts the two agree): one creature on several recipes
	// is several clones, the n-th at source + n x cloneRowOffset. Band 1 is the
	// namespace above; bands 2-4 and 6-20 hold no row of the game (measured on
	// 11711000), and band 5 is the game's own other scheme and is never
	// touched. A clone is named by its KEY - source + (band-1) x offset - so
	// its row is key + cloneRowOffset in every band.
	// The summon-side copy band, the model's (formation.SummonCopyRow) written
	// twice like the offset and checked once: a shipped shell within
	// summonCopyWindow of cloneRowOffset copies to summonCopyFirst + its
	// distance, one within the window of dlcShellFirst to summonCopySplit +
	// its distance. The band sits above every clone a world row can have
	// (world rows end at 95090000).
	summonCopyFirst  = int32(199000000)
	summonCopySplit  = int32(199500000)
	summonCopyWindow = int32(100000)
	dlcShellFirst    = int32(150000000)
	// spiritVfxRow is the only SpEffectVfxParam row this product may write, and
	// it is a constant rather than an argument for the same reason a clone
	// destination is: it makes "the colour switch cannot reach another visual
	// effect" true by construction instead of by review.
	//
	// It is the row spEffect 295000 delivers, and 295000 is the companion
	// effect every summon carries. Measured on 11701000: of 11,354
	// SpEffectParam rows exactly one names vfx 57000, and it is 295000.
	spiritVfxRow = int32(57000)
	// spiritPhantomProfile is PhantomParam 200, shipped name "Spirit Summon" -
	// the white look. noPhantomProfile is the game's own spelling for no
	// overwrite at all, carried by 1,457 of the 1,513 vfx rows, and writing it
	// is what lets a summon render in the creature's own colours.
	//
	// The pointer is written rather than PhantomParam 200's colours because 200
	// has a second caller: vfx 51220, reached by unrelated effect 15032.
	// Editing the palette would change that too; editing the pointer cannot.
	spiritPhantomProfile = int64(200)
	noPhantomProfile     = int64(0)
	// neutralPhantomProfile is PhantomParam 410, and it is what the off
	// position writes now. Clearing the field to 0 shows the creature's own
	// colours and ALSO puts a red "Host of Fingers" plate over every summon:
	// no RoleParam row carries teamType 47, so a summon falls back to row 0,
	// and the overwrite had been suppressing that fallback. 410 multiplies
	// nothing and adds nothing - diffMulColor 255/255/255 at alpha 1, with
	// edge, front, light and glow all zero - so it keeps an overwrite live
	// while rendering the creature as itself. Seen on the console.
	neutralPhantomProfile = int64(410)
)

// vfxFields is the whole SpEffectVfxParam write surface: one field.
//
// phantomParamOverwriteId names the PhantomParam profile a visual effect
// applies. PhantomParam 200 does not repaint the model - its diffMulColor is
// 255/255/255 at alpha 0.9, a ninety percent multiplier on the creature's own
// diffuse, under a white rim and a thin dark overlay - which is why a summon
// looks white or not depending on how saturated the creature already is.
var vfxFields = map[string]paramField{
	"phantomParamOverwriteId": {
		offset: 128, kind: "s32", bounded: true,
		// The range spans the three profiles the switch may name. It is the
		// coarse guard; the exact allow-list is enforced on the manifest in
		// rowrules.go, which is what stops a value inside the range but not
		// one of the three.
		low: float64(noPhantomProfile), high: float64(neutralPhantomProfile),
	},
}

var stoneFields = map[string]stoneField{
	"eliminateTargetEntityId":         {8, "u32", 0},
	"summonedEventFlagId":             {12, "u32", 0},
	"activateRange":                   {28, "u16", 65535},
	"overwriteReturnRange":            {30, "s16", -1},
	"overwriteActivateRegionEntityId": {32, "u32", 0},
	"warnRegionEntityId":              {36, "u32", 0},
}

type stoneField struct {
	offset int
	kind   string
	want   float64
}

// goodsFields is the whole EquipParamGoods write surface.
//
// Three fields and two writers. A conversion writes refId_default and nothing
// else; a cost edit writes the pair and nothing else; a reset restores all
// three, because one changed field is reason enough to put the row back and a
// row restored halfway is a row in a state the game never shipped.
var goodsFields = map[string]paramField{
	"refId_default": {offset: goodsRefIDDefaultOffset, kind: "s32", bounded: true, low: 1, high: math.MaxInt32},
	"consumeMP":     {offset: goodsConsumeMPOffset, kind: "s16", bounded: true, low: minGoodsConsume, high: maxGoodsConsume},
	"consumeHP":     {offset: goodsConsumeHPOffset, kind: "s16", bounded: true, low: minGoodsConsume, high: maxGoodsConsume},
}

// buddyFields are the BuddyParam fields a reset restores.
//
// A conversion writes one of them, appearOnAroundSekihi, to a constant. An
// authored formation writes the ones buddyAuthoredFields names. A reset has to
// put back whatever either one changed, so the restorable set is the union and
// not the conversion's own single field.
var buddyFields = map[string]paramField{
	"triggerSpEffectId":    {offset: 4, kind: "s32"},
	"npcParamId":           {offset: 8, kind: "s32"},
	"npcThinkParamId":      {offset: 12, kind: "s32"},
	"appearOnAroundSekihi": {offset: buddyAppearOffset, kind: "u8", bounded: true, low: 0, high: 1},
	// The stance pair and the two fields below it are bounded to what 1.17
	// actually holds, not to their storage. A value outside it is a save this
	// product did not write and did not measure.
	"disablePCTargetShare": {offset: buddyTargetShareOffset, kind: "u8", bounded: true, low: 0, high: 1},
	"pcFollowType":         {offset: buddyFollowTypeOffset, kind: "u8", bounded: true, low: 0, high: 2},
	"npcPlayerInitParamId": {offset: buddyPlayerInitOffset, kind: "s32"},
	"generateAnimId":       {offset: buddyGenerateAnimOffset, kind: "s32"},
	// Placement. Bounded to what internal/editor/buddypatch.go in the research
	// repository already validates rather than to the shipped envelope: the
	// envelope is four metres across, and a Furnace Golem placed inside it
	// arrives on top of the player. Nothing outside the envelope has been
	// seen, so a caller writing there is running a test, and the bound is
	// what stops a test becoming a coordinate nobody can reach.
	"x_offset": {offset: buddyXOffset, kind: "f32", bounded: true, low: -buddyOffsetLimit, high: buddyOffsetLimit},
	"z_offset": {offset: buddyZOffset, kind: "f32", bounded: true, low: -buddyOffsetLimit, high: buddyOffsetLimit},
	"y_angle":  {offset: buddyYAngleOffset, kind: "f32", bounded: true, low: -buddyAngleLimit, high: buddyAngleLimit},
	// The reinforcement table: which special effect the summon carries at each
	// of the Ash's eleven upgrade levels. Shipped as 290000+level on every
	// formation the audit checks, and writable so that a formation can be
	// scaled to the creature it delivers rather than to the summon ladder.
	// The offsets are the ones formations.go already reads them at.
	"dopingSpEffect_lv0":  {offset: buddyDopingOffset + 0*4, kind: "s32"},
	"dopingSpEffect_lv1":  {offset: buddyDopingOffset + 1*4, kind: "s32"},
	"dopingSpEffect_lv2":  {offset: buddyDopingOffset + 2*4, kind: "s32"},
	"dopingSpEffect_lv3":  {offset: buddyDopingOffset + 3*4, kind: "s32"},
	"dopingSpEffect_lv4":  {offset: buddyDopingOffset + 4*4, kind: "s32"},
	"dopingSpEffect_lv5":  {offset: buddyDopingOffset + 5*4, kind: "s32"},
	"dopingSpEffect_lv6":  {offset: buddyDopingOffset + 6*4, kind: "s32"},
	"dopingSpEffect_lv7":  {offset: buddyDopingOffset + 7*4, kind: "s32"},
	"dopingSpEffect_lv8":  {offset: buddyDopingOffset + 8*4, kind: "s32"},
	"dopingSpEffect_lv9":  {offset: buddyDopingOffset + 9*4, kind: "s32"},
	"dopingSpEffect_lv10": {offset: buddyDopingOffset + 10*4, kind: "s32"},
}

// buddyAuthoredFields are the fields a formation row must state for itself.
// Authoring writes exactly these unless it is also scaling the row, and a reset
// writes them as part of the whole.
//
// # Why this list is stated rather than grown
//
// It used to be four - the trigger, the actor, its think row, and the origin
// flag - and everything else on the row was inherited from whatever the host
// shipped. The comment for that read "a row left alone keeps the one it has",
// which was written about the reinforcement ladder, where it is right, and
// quietly generalised to the whole row, where it is not.
//
// An authored formation is a claim about what a slot delivers. Every field
// carrying part of that answer and left unwritten is a way for the host to
// leak into the result, and three leaks were found three different ways: the
// mount pair from the data, the stance pair from a console run that ended with
// a formation standing still, and npcPlayerInitParamId from a run that spent
// forty-one subjects delivering the host's own person instead of the creature
// asked for.
//
// A set defined by what nobody thought about is what produced all three. So
// this list is stated, buddyInheritedFields states the rest with the reason it
// may be inherited, and TestEveryBuddyFieldIsAccountedFor holds a byte of the
// shipped row against one or the other. Adding a field means deciding.
var buddyAuthoredFields = map[string]paramField{
	"triggerSpEffectId": buddyFields["triggerSpEffectId"],
	"npcParamId":        buddyFields["npcParamId"],
	"npcThinkParamId":   buddyFields["npcThinkParamId"],
	// A conversion clears this one and a reset restores it, so a row put back
	// without it would keep a conversion's origin fix after the conversion is
	// gone.
	"appearOnAroundSekihi": buddyFields["appearOnAroundSekihi"],
	// The stance pair: whether the formation follows or holds its ground. Both
	// are written because the shipped rows never separate them, and stated
	// rather than defaulted because holding position is a thing a player may
	// want on purpose - see archive/history/editor/IDENTIFICATION-RUN.md, N9.
	"disablePCTargetShare": buddyFields["disablePCTargetShare"],
	"pcFollowType":         buddyFields["pcFollowType"],
	// Cleared, always. This is the field that decides whether the row summons
	// a creature at all, and there is no authored formation for which building
	// the host's own person instead is the right answer.
	"npcPlayerInitParamId": buddyFields["npcPlayerInitParamId"],
	// So a creature does not arrive on a person's animation.
	"generateAnimId": buddyFields["generateAnimId"],
	// Where each actor stands. Stated rather than inherited, and the reason is
	// the same one every other entry in this list carries: an inserted row is a
	// byte copy of its template, so leaving placement unwritten gave every
	// inserted slot the template's spot and a host that ships one row put its
	// whole formation on one point. The decode run met that as actors standing
	// inside one another.
	"x_offset": buddyFields["x_offset"],
	"z_offset": buddyFields["z_offset"],
	"y_angle":  buddyFields["y_angle"],
}

// buddyInheritedFields are the rest of the shipped row: what an authored
// formation deliberately does not write, and why each one is safe to leave.
//
// "Safe" means something different per entry and the reason is the point. Read
// it before moving anything out of here, and write the reason when moving
// anything in.
var buddyInheritedFields = map[string]inheritedField{
	// Package-build flags. They select what ships in a regional build and mean
	// nothing at runtime.
	"disableParam_NT":        {0, 1, "a build flag, not runtime behaviour"},
	"disableParamReserve1":   {1, 3, "padding"},
	"npcParamId_ridden":      {16, 4, "-1 on all 170 shipped rows, so there is nothing to inherit; TestNoShippedBuddyRowNamesARiddenActor measures that, and docs/HAZARDS.md records what it costs a mounted actor"},
	"npcThinkParamId_ridden": {20, 4, "the other half of the same pair, and -1 on all 170 for the same reason"},
	"reserve1":               {39, 1, "padding"},
	// Not padding, whatever the pinned Paramdex implies by stopping short of
	// it: 169 of the 170 shipped rows carry SpEffectParam 1410 here, and 1410
	// is a perception effect - eye_angX 90, eye_angY 180, sightSearchAdd 15.
	// Only row 0, the all -1 template, differs. Left inherited rather than
	// authored because what it does has not been seen.
	"perceptionEffect": {92, 4, "SpEffectParam 1410 on 169 of 170 rows; a sight cone, not padding"},
	// The row is 160 bytes and the pinned Paramdex describes 96. Upstream
	// Paramdex stops in the same place and the Spirit Battler's longer def
	// names the remainder Unk1..Unk17, so none of this is documented anywhere.
	// Three of those bytes are now measured and they are effect slots:
	//
	//	96   SpEffectParam 1420 on 169 of 170 rows. NOT a bare marker: the row
	//	     carries
	//	     maxHpRate 0.5 with effectEndurance -1, and research's effects.csv
	//	     has it as "Buddy universal max-HP halving ... applies to every
	//	     summon including converted ones". Whether it REACHES a summoned
	//	     actor is open - research proved the neighbouring slot at 92 does
	//	     not - and the two records cannot both be right. If it does reach,
	//	     every summon in the game runs at half max HP. One seat settles it:
	//	     read a summon's max HP against its world row.
	//	100  SpEffectParam 19390 on exactly the eleven person-carrying
	//	     summons, -1 on everything else
	//	132  SpEffectParam 1400, which carries vfxId 11300 and PhantomParam
	//	     203, a red-rimmed profile
	//
	// They stay inherited. Knowing which table a field points into is not
	// knowing what writing it would do, and AGENTS.md's rule for undocumented
	// bytes is to preserve them exactly - which writing only named offsets
	// already does.
	"undocumented": {96, 64, "beyond every published schema; three effect slots measured inside it, all preserved exactly"},
}

// inheritedField is one field an authored row leaves alone, and the reason.
type inheritedField struct {
	offset, width int
	why           string
}

// paramField is one field of a table this product both reads and writes. It
// carries no recipe target the way a stone field does: the operations that use
// it write values from a record rather than to a constant.
type paramField struct {
	offset int
	kind   string
	// bounded narrows the accepted range to low..high where the shipped data is
	// narrower than the storage. appearOnAroundSekihi is a flag: 0 on 150 rows
	// and 1 on 20, measured on 11701000, where no third value exists. The cost
	// pair is bounded to the range the regulation holds rather than to s16.
	bounded   bool
	low, high float64
}

// buddyDopingFields are the eleven reinforcement levels, projected out of
// buddyFields so the two cannot disagree about offsets or widths.
var buddyDopingFields = func() map[string]paramField {
	fields := make(map[string]paramField, dopingLevels)
	for level := 0; level < dopingLevels; level++ {
		name := fmt.Sprintf("dopingSpEffect_lv%d", level)
		field, known := buddyFields[name]
		if !known {
			panic("buddyFields is missing " + name)
		}
		fields[name] = field
	}
	return fields
}()

// storageRange is the inclusive range a field's width can hold. A recorded
// value outside it means the snapshot and this table disagree about the
// layout, which must fail rather than be silently truncated on the way in.
func storageRange(kind string) (float64, float64) {
	switch kind {
	case "s32":
		return math.MinInt32, math.MaxInt32
	case "u32":
		return 0, math.MaxUint32
	case "u16":
		return 0, math.MaxUint16
	case "s16":
		return math.MinInt16, math.MaxInt16
	case "u8":
		return 0, math.MaxUint8
	case "f32":
		// Every f32 field this product writes is bounded far inside the type,
		// so the storage range is the honest outer limit and never the one
		// used. Stated anyway, because a field kind with no range would read
		// as zero-width to storageRange's callers.
		return -math.MaxFloat32, math.MaxFloat32
	}
	return 0, 0
}

func (f stoneField) valueRange() (float64, float64) { return storageRange(f.kind) }

// valueRange is the storage range unless the shipped data is narrower, in
// which case the narrower one is the honest bound: a value the regulation
// never holds is a snapshot fault, not a field this product should write.
func (f paramField) valueRange() (float64, float64) {
	if f.bounded {
		return f.low, f.high
	}
	return storageRange(f.kind)
}

// cloneBands are formation.CloneBands, written twice and checked once.
var cloneBands = []int32{1, 2, 3, 4, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20}

// cloneKeySource is the world row a clone key copies, and whether key is one:
// a positive key whose last eight digits name a row and whose band is a clone
// band.
func cloneKeySource(key int32) (int32, bool) {
	if key <= 0 {
		return 0, false
	}
	source, band := key%cloneRowOffset, key/cloneRowOffset+1
	if source == 0 {
		return 0, false
	}
	for _, allowed := range cloneBands {
		if band == allowed {
			return source, true
		}
	}
	return 0, false
}

// inCloneBand reports whether row sits in a clone band: the rows this product
// may insert, write and remove, and no others.
//
// Band 1 is the whole summon namespace as it always was - FromSoftware ships
// summon rows there that are no clone of anything, the person shell at
// 100000000 among them - and a higher band holds only clones of a world row.
func inCloneBand(row int32) bool {
	// The character bodies are cut out: the character write owns them
	// (characters.go), and no clone list, removal or switch may reach one.
	if row >= characterBodyFloor && row < characterBodyCeiling {
		return false
	}
	if row >= cloneRowOffset && row < cloneRowCeiling {
		return true
	}
	if row < cloneRowOffset {
		return false
	}
	_, ok := cloneKeySource(row - cloneRowOffset)
	return ok
}
