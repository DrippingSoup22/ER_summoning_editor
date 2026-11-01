// Package savefixture builds a synthetic decrypted PS4 save in memory, so the
// product's save path can be tested without private user data.
//
// It is a deliberately *independent* implementation of the container format.
// It shares no code with internal/savecore and carries its own copy of the
// regulation key, because a fixture produced by the code under test would prove
// only that savecore agrees with itself. Everything savecore reads, this
// package writes; everything savecore writes, read.go reads back. A format
// mistake on either side shows up as a failing test rather than as a save the
// game rejects.
//
// It is an ordinary package rather than a _test.go helper because both
// internal/savecore and internal/application need it, and Go will not let a
// test helper cross a package boundary. Nothing in cmd/ imports it, so it is
// never linked into the shipped binary.
package savefixture

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"encoding/binary"
	"fmt"
	"math"
	"slices"
	"unicode/utf16"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
	"github.com/klauspost/compress/zstd"
)

// This is a second, independent copy of the constant in savecore. If either is
// ever corrupted, a fixture built here stops loading there.
var regulationKey = []byte{
	0x99, 0xBF, 0xFC, 0x36, 0x6A, 0x6B, 0xC8, 0xC6,
	0xF5, 0x82, 0x7D, 0x09, 0x36, 0x02, 0xD6, 0x76,
	0xC4, 0x28, 0x92, 0xA0, 0x1C, 0x20, 0x7F, 0xB0,
	0x24, 0xD3, 0xAF, 0x4E, 0x49, 0x3F, 0xEF, 0x99,
}

const (
	ps4HeaderSize        = 0x70
	slotSize             = 0x280000
	numSlots             = 10
	userdata10Size       = 0x60000
	userdata11HeaderSize = 0x10
	aesIVSize            = 16
	dcxHeaderSize        = 76

	paramHeaderSize  = 0x40
	binderHeaderSize = 0x40
	binderEntrySize  = 0x24

	// Field offsets the product writes, repeated here so a fixture row can be
	// seeded with a realistic starting value.
	goodsRefIDDefaultOffset = 4
	goodsConsumeMPOffset    = 128
	goodsConsumeHPOffset    = 130
	goodsFlagsOffset        = 72
	goodsEnableMultiMask    = byte(1 << 4)
	buddyAppearOffset       = 36
	buddyDopingOffset       = 40
	cloneRowOffset          = int32(100000000)
)

// BuddyDopingLevels is the size of the dopingSpEffect_lv0..lv10 table, and
// BuddyDopingBase the first row of the 290000..290010 reinforcement series that
// 83 of the 84 shipped families carry. Both measured on 11701000.
const (
	BuddyDopingLevels = 11
	BuddyDopingBase   = int32(290000)
)

// BuddyRowsPerTrigger is the fixture's convention for which block a row belongs
// to: row 23200000 belongs to trigger 232000. It builds well-formed blocks out
// of a row list without carrying a second table beside it.
//
// It is a convention and not a claim about the regulation. The shipped table
// does relate the two this way for most rows, but 36 of its 170 carry a trigger
// their own ID does not predict - measured by
// TestAShippedBlockIsAContiguousRunUnlessItSummonsNothing - which is exactly
// why the product reads a block by grouping on the trigger rather than by
// deriving an ID range.
const BuddyRowsPerTrigger = int32(100)

// BuddySummonActor is the actor every fixture block row names. It is one of the
// clone rows the fixture's NpcParam and NpcThinkParam carry, so a block read
// from the fixture names an actor that exists - which is what the real table
// does, and what an authoring compile has to be able to assume it might.
const BuddySummonActor = int32(140700000)

// ps4Magic opens every decrypted PS4 save; the readers tell it from a PC save by it.
var ps4Magic = []byte{0xCB, 0x01, 0x9C, 0x2C}

// UserData11Offset is where the encrypted regulation begins in a PS4 save.
const UserData11Offset = ps4HeaderSize + numSlots*slotSize + userdata10Size

// StoneFieldOffsets are the six expanded-area fields, by offset and width, so a
// fixture can seed them and a test can read them back.
var StoneFieldOffsets = map[string]struct {
	Offset int
	Bits   int
}{
	"eliminateTargetEntityId":         {8, 32},
	"summonedEventFlagId":             {12, 32},
	"activateRange":                   {28, 16},
	"overwriteReturnRange":            {30, 16},
	"overwriteActivateRegionEntityId": {32, 32},
	"warnRegionEntityId":              {36, 32},
}

// BuddyFieldOffsets are the restorable BuddyParam fields, by offset and width,
// so a fixture can seed them and a test can read them back. Like every other
// offset here it is written out independently of savecore on purpose.
//
// The eleven reinforcement fields joined the four when authoring learned to
// write them: a save whose ladder the product can change is a save whose ladder
// a reset has to put back, and both halves need a fixture that carries the
// shipped values rather than a convention.
//
// The stance pair, npcPlayerInitParamId and generateAnimId joined for the same
// reason. A name this map does not hold is silently skipped when the snapshot
// is written over a row, so a field missing here is a fixture that claims to be
// vanilla and holds a zero where the game ships something else - which is a
// reset test that passes for the wrong reason.
var BuddyFieldOffsets = map[string]struct {
	Offset int
	Bits   int
}{
	"triggerSpEffectId":    {4, 32},
	"npcParamId":           {8, 32},
	"npcThinkParamId":      {12, 32},
	"appearOnAroundSekihi": {buddyAppearOffset, 8},
	"disablePCTargetShare": {37, 8},
	"pcFollowType":         {38, 8},
	"npcPlayerInitParamId": {84, 32},
	"generateAnimId":       {88, 32},
	"dopingSpEffect_lv0":   {buddyDopingOffset + 0*4, 32},
	"dopingSpEffect_lv1":   {buddyDopingOffset + 1*4, 32},
	"dopingSpEffect_lv2":   {buddyDopingOffset + 2*4, 32},
	"dopingSpEffect_lv3":   {buddyDopingOffset + 3*4, 32},
	"dopingSpEffect_lv4":   {buddyDopingOffset + 4*4, 32},
	"dopingSpEffect_lv5":   {buddyDopingOffset + 5*4, 32},
	"dopingSpEffect_lv6":   {buddyDopingOffset + 6*4, 32},
	"dopingSpEffect_lv7":   {buddyDopingOffset + 7*4, 32},
	"dopingSpEffect_lv8":   {buddyDopingOffset + 8*4, 32},
	"dopingSpEffect_lv9":   {buddyDopingOffset + 9*4, 32},
	"dopingSpEffect_lv10":  {buddyDopingOffset + 10*4, 32},
	// Placement, and the only three fields on the row that are floats. Bits is
	// 32 for them too - the width is the same - so a writer has to look at
	// Float as well, which is why it exists rather than being inferred from a
	// name.
	"x_offset": {24, 32},
	"z_offset": {28, 32},
	"y_angle":  {32, 32},
}

// BuddyFloatFields are the entries of BuddyFieldOffsets whose 32 bits are an
// f32 rather than an integer. A writer that ignores this seeds 1.3 as the
// integer 1, which is a fixture claiming to be vanilla while holding a
// placement the game never ships.
var BuddyFloatFields = map[string]bool{"x_offset": true, "z_offset": true, "y_angle": true}

// SeedBuddyRow is what a shipped BuddyParam row holds before this product
// touches it, and it is exported because a test that wants one field different
// has to start from the other fields being right. Replacing the whole Init and
// setting one field would leave a save whose every block was blank, which is
// not the save any test means to describe.
//
// Each row carries the flag every monument-origin row ships with, the trigger
// whose block it belongs to, an actor that exists in this save's actor tables,
// and the reinforcement series a shipped block carries. Authoring reads a block
// before it edits one, so a fixture whose blocks were blank could not be
// authored into at all.
func SeedBuddyRow(rowID int32, row []byte) {
	row[buddyAppearOffset] = 1
	binary.LittleEndian.PutUint32(row[BuddyFieldOffsets["triggerSpEffectId"].Offset:], uint32(rowID/BuddyRowsPerTrigger))
	binary.LittleEndian.PutUint32(row[BuddyFieldOffsets["npcParamId"].Offset:], uint32(BuddySummonActor))
	binary.LittleEndian.PutUint32(row[BuddyFieldOffsets["npcThinkParamId"].Offset:], uint32(BuddySummonActor))
	// A creature, not a person, arriving on the ordinary spirit animation and
	// following the player: what 140 of the 170 shipped rows say. The zero
	// value would be a row that follows but summons CharaInitParam row 0 on no
	// animation at all, which is not a shape the game ships.
	noPerson, ordinaryAnimation := int32(-1), int32(1830)
	binary.LittleEndian.PutUint32(row[BuddyFieldOffsets["npcPlayerInitParamId"].Offset:], uint32(noPerson))
	binary.LittleEndian.PutUint32(row[BuddyFieldOffsets["generateAnimId"].Offset:], uint32(ordinaryAnimation))
	for level := 0; level < BuddyDopingLevels; level++ {
		binary.LittleEndian.PutUint32(row[buddyDopingOffset+4*level:], uint32(BuddyDopingBase+int32(level)))
	}
}

// Table describes one PARAM to synthesise.
type Table struct {
	// Entry is the binder entry name, such as "EquipParamGoods.param".
	Entry string
	// ParamType and DataVersion must match what the product asserts, unless a
	// test is deliberately provoking the schema guard.
	ParamType   string
	DataVersion int16
	// RowSize is the stride the product derives from consecutive rows.
	RowSize int
	RowIDs  []int32
	// Init seeds one row before it is written. It receives a slice exactly
	// RowSize long.
	Init func(rowID int32, row []byte)
	// Stagger inserts a gap before the final row, breaking the uniform stride
	// the product requires. Only a negative test should set it.
	Stagger int
	// Format is the PARAM header shape byte at 0x2D. Zero means the shape
	// Elden Ring ships: 0x80 puts the type string behind an offset and 0x04
	// makes row data offsets 64-bit. A test may set it to reach the other
	// branch of the product's header parser.
	Format byte
}

// Options describes a whole synthetic save.
type Options struct {
	RegulationVersion uint32
	Tables            []Table
	// CapacitySlack is spare encrypted room beyond the compressed regulation,
	// mirroring the headroom a real save carries.
	CapacitySlack int
	// IV is fixed so a fixture is byte-reproducible across runs.
	IV []byte
	// DCXFormat is the four-character compression tag. Empty means "ZSTD"; a
	// test may set another to check the product refuses what it cannot read.
	DCXFormat string
}

// DefaultParamFormat is the header shape Elden Ring ships.
const DefaultParamFormat = byte(0x84)

// paramLayout resolves a header shape byte into the three facts that depend on
// it. It is written out separately from the product's parser on purpose: the
// two must agree, and a test is how that gets noticed.
func paramLayout(format byte) (rowTableOffset, rowEntrySize int, typeByOffset bool) {
	rowEntrySize = 12
	if format&0x04 != 0 {
		rowEntrySize = 24
	}
	position := 12
	if format&0x80 != 0 {
		typeByOffset = true
		position += 4 + 8 + 0x14
	} else {
		position += 0x20
	}
	position += 4
	if (format&0x01 != 0 && format&0x02 != 0) || format&0x04 != 0 {
		position += 16
	}
	return position, rowEntrySize, typeByOffset
}

// GoodsFamilies are the Ash families a default fixture carries, with the
// trigger each upgrade level points at before any conversion. They are real
// catalog values so a test reads like the product it exercises.
//
// The value is one trigger per level rather than one per family because a goods
// row's refId_default names the formation, not the level of it, and Giant Rat
// Ashes is the shipped proof: a fixture that gave it a single trigger could not
// tell a tool that reads the mapping from one that assumes it.
var GoodsFamilies = map[int32][]int32{
	232000: levelSeries(232000), // Lone Wolf Ashes
	240000: levelSeries(240000), // Wandering Noble Ashes
	// The two families whose BuddyParam blocks this fixture also carries, so
	// that an authored formation has a host whose block is really there. Every
	// other family here has goods rows and no block, which is enough to convert
	// but not to author.
	215000:  levelSeries(215000),   // Putrid Corpse Ashes, four rows
	244000:  levelSeries(244000),   // Land Squirt Ashes, three rows
	2201000: levelSeries(20201000), // Bloodfiend Hexer's Ashes
	2207000: levelSeries(20207000), // Messmer Soldier Ashes
	// Giant Rat Ashes, the one family whose formation changes as it is
	// upgraded: three rats at +0..+4, four at +5..+9 and five at +10. The
	// formation family changes while the level offset keeps counting, which is
	// why +5 is 233105 and not 233100.
	233000: {
		233000, 233001, 233002, 233003, 233004,
		233105, 233106, 233107, 233108, 233109,
		233210,
	},
}

// levelSeries is the shape every shipped family uses: the formation family plus
// the upgrade level. A fixture that repeated one trigger on all eleven rows
// would let a tool that ignores the level pass.
func levelSeries(family int32) []int32 {
	triggers := make([]int32, LevelRows)
	for level := range triggers {
		triggers[level] = family + int32(level)
	}
	return triggers
}

// LevelRows is how many upgrade rows a Spirit Ash family occupies.
const LevelRows = 11

// SpiritCompanionEffects are the resident effects every shipped summon carries.
// Named here so a fixture's SpEffectParam holds them, independently of the copy
// savecore writes into a clone.
var SpiritCompanionEffects = [3]int32{295000, 296000, 297000}

// actorTables builds NpcParam and NpcThinkParam holding the given clone rows
// and the world rows they were cloned from.
//
// docs/SAFETY.md rule 1 permits inserting a clone into these two and nothing
// else, and a snapshot reads them so that a reset can tell the rows the game
// shipped from the rows the editor added. Clone insertion also opens them, so
// their geometry here is the shipped one — NpcParam is 736 bytes at data
// version 9 and NpcThinkParam 228 at version 2 — rather than something merely
// large enough to hold a row ID.
//
// A world row is seeded as what it is: an enemy that pays out, scales to its
// region and is too big to stand beside the player. Everything the summon-side
// recipe writes therefore starts at a value it must move, and a recipe that
// skipped a field could not pass by coincidence.
//
// A SHIPPED clone row carries the three Spirit Summon companion effects, in the
// third resident run. That was left zeroed while nothing read it, and it is
// seeded now because a summon row's role is read from it: 296000 is on every
// one of the 115 rows the real regulation ships, so a fixture without it is a
// fixture where every summon reads as a Fighter. The run is the third because
// that is where FromSoftware puts its own - see npcResidentEffectOffsets.
func actorTables(npcClones, thinkClones []int32) []Table {
	build := func(entry, paramType string, dataVersion int16, rowSize int, clones []int32, init func(int32, []byte)) Table {
		seen := map[int32]bool{}
		var rowIDs []int32
		add := func(rowID int32) {
			if rowID > 0 && !seen[rowID] {
				seen[rowID] = true
				rowIDs = append(rowIDs, rowID)
			}
		}
		for _, clone := range clones {
			add(clone - cloneRowOffset)
			add(clone)
		}
		for _, world := range CloneableWorldActors {
			add(world)
		}
		slices.Sort(rowIDs)
		return Table{Entry: entry, ParamType: paramType, DataVersion: dataVersion, RowSize: rowSize, RowIDs: rowIDs, Init: init}
	}
	return []Table{
		build("NpcParam.param", "NPC_PARAM_ST", 9, 736, npcClones, seedActor),
		build("NpcThinkParam.param", "NPC_THINK_PARAM_ST", 2, 228, thinkClones, nil),
	}
}

// CloneableWorldActors are world rows the fixture carries with no clone beside
// them, which is the only situation clone insertion exists for: an actor the
// save cannot yet summon. There are two so that inserting both has to place the
// second header after the first, rather than after a row that shipped.
//
// They are rows the shipped catalog actually holds - two of Malenia's, one in a
// scaled region and one in none - rather than invented numbers. That matters
// now that authoring can scale a summon to the creature it delivers: a slot
// naming a row no catalog knows gets no scaling, and a fixture made of those
// would test the switch by never turning it on.
var CloneableWorldActors = []int32{21200056, 21201000}

// NpcWorldActor is what a fixture world actor holds before the summon-side
// recipe runs. Every value is one the recipe must move, so a recipe that
// skipped a field could not pass by coincidence, and a test can name what a
// field moved away from. Like every other offset here it is written out
// independently of savecore on purpose.
var NpcWorldActor = struct {
	HitHeightOffset, HitRadiusOffset      int
	GetSoulOffset, GameClearOffset        int
	ItemLotEnemyOffset, ItemLotMapOffset  int
	TeamTypeOffset, SoulByBossFlagsOffset int
	AreaScalingSlotOffset, KeptSlotOffset int
	DlcScalingSlotOffset                  int
	HitHeight, HitRadius                  float32
	GetSoul, GameClear                    int32
	ItemLotEnemy, ItemLotMap              int32
	AreaScalingEffect, KeptEffect         int32
	DlcScalingEffect                      int32
	TeamType, SoulByBossFlags             byte
}{
	HitHeightOffset: 20, HitRadiusOffset: 24,
	GetSoulOffset: 44, GameClearOffset: 108,
	ItemLotEnemyOffset: 48, ItemLotMapOffset: 52,
	TeamTypeOffset: 307, SoulByBossFlagsOffset: 349,
	// The first resident slot of the first run and the first of the second, so
	// a recipe that only walked one run is visible either way.
	AreaScalingSlotOffset: 76, KeptSlotOffset: 376,
	// The second slot of the first run carries a Shadow of the Erdtree tier,
	// stripped like the base band.
	DlcScalingSlotOffset: 80,
	HitHeight:            12, HitRadius: 3,
	GetSoul: 3000, GameClear: 4000,
	ItemLotEnemy: 100100, ItemLotMap: 100200,
	AreaScalingEffect: 7100, KeptEffect: 310000,
	DlcScalingEffect: 20007040,
	TeamType:         6, SoulByBossFlags: 0b1001,
}

// shippedCloneSlotOffsets are three slots of the third resident run, where the
// companion effects go.
var shippedCloneSlotOffsets = [3]int{488, 492, 496}

// seedActor seeds one NpcParam row.
func seedActor(rowID int32, row []byte) {
	if rowID >= cloneRowOffset {
		for index, effect := range SpiritCompanionEffects {
			binary.LittleEndian.PutUint32(row[shippedCloneSlotOffsets[index]:], uint32(effect))
		}
		return
	}
	seed := NpcWorldActor
	binary.LittleEndian.PutUint32(row[seed.HitHeightOffset:], math.Float32bits(seed.HitHeight))
	binary.LittleEndian.PutUint32(row[seed.HitRadiusOffset:], math.Float32bits(seed.HitRadius))
	for offset, value := range map[int]int32{
		seed.GetSoulOffset: seed.GetSoul, seed.GameClearOffset: seed.GameClear,
		seed.ItemLotEnemyOffset: seed.ItemLotEnemy, seed.ItemLotMapOffset: seed.ItemLotMap,
		seed.AreaScalingSlotOffset: seed.AreaScalingEffect, seed.KeptSlotOffset: seed.KeptEffect,
		seed.DlcScalingSlotOffset: seed.DlcScalingEffect,
	} {
		binary.LittleEndian.PutUint32(row[offset:], uint32(value))
	}
	row[seed.TeamTypeOffset] = seed.TeamType
	row[seed.SoulByBossFlagsOffset] = seed.SoulByBossFlags
}

// Defaults describes a save close enough to a real one that the product's
// guards all pass against it.
// ShippedSummonClones are the summon-side rows this fixture carries as the
// game ships them: one per Ash the fixture can summon, each locked behind the
// balance package.
var ShippedSummonClones = []int32{140700000, 143111300, 136610000}

func Defaults() Options {
	goods := Table{
		Entry: "EquipParamGoods.param", ParamType: "EQUIP_PARAM_GOODS_ST",
		DataVersion: 3, RowSize: 176,
		Init: func(rowID int32, row []byte) {
			// Seed the shipped trigger and multiplayer bit. Conversions change
			// only the trigger; tests assert this bit survives byte-identically.
			binary.LittleEndian.PutUint32(row[goodsRefIDDefaultOffset:], uint32(vanillaTriggerFor(rowID)))
			row[goodsFlagsOffset] |= goodsEnableMultiMask
		},
	}
	for _, family := range sortedInt32Keys(GoodsFamilies) {
		for level := 0; level < LevelRows; level++ {
			goods.RowIDs = append(goods.RowIDs, family+int32(level))
		}
	}

	buddy := Table{
		Entry: "BuddyParam.param", ParamType: "BUDDY_PARAM_ST",
		DataVersion: 2, RowSize: 160,
		Init: SeedBuddyRow,
	}
	buddy.RowIDs = append(buddy.RowIDs, plan.MonumentOriginRows1_17...)

	stone := Table{
		Entry: "BuddyStoneParam.param", ParamType: "BUDDY_STONE_PARAM_ST",
		DataVersion: 2, RowSize: 64,
		Init: func(_ int32, row []byte) {
			// Values that differ from every target the recipe writes, so an
			// unapplied field cannot pass by coincidence.
			binary.LittleEndian.PutUint32(row[8:], 1000)
			binary.LittleEndian.PutUint32(row[12:], 2000)
			binary.LittleEndian.PutUint16(row[28:], 40)
			binary.LittleEndian.PutUint16(row[30:], 50)
			binary.LittleEndian.PutUint32(row[32:], 3000)
			binary.LittleEndian.PutUint32(row[36:], 4000)
		},
	}
	for row := 0; row < 8; row++ {
		stone.RowIDs = append(stone.RowIDs, int32(row))
	}

	effect := Table{
		Entry: "SpEffectParam.param", ParamType: "SP_EFFECT_PARAM_ST",
		DataVersion: 4, RowSize: 912,
	}
	// One SpEffect row per distinct trigger any family points at, so the
	// product's dangling-reference check passes for every conversion between
	// them. A PARAM cannot hold the same row ID twice, hence the dedupe.
	seen := map[int32]bool{}
	for _, family := range sortedInt32Keys(GoodsFamilies) {
		for _, trigger := range GoodsFamilies[family] {
			if !seen[trigger] {
				seen[trigger] = true
				effect.RowIDs = append(effect.RowIDs, trigger)
			}
		}
	}
	// The three effects that make a summon a summon. An inserted clone names
	// them, and the product refuses to write a slot that would resolve to
	// nothing, so a fixture without them could not be cloned into.
	effect.RowIDs = append(effect.RowIDs, SpiritCompanionEffects[:]...)
	slices.Sort(effect.RowIDs)

	actors := actorTables(ShippedSummonClones, ShippedSummonClones)
	return Options{
		RegulationVersion: 11701000,
		Tables:            append([]Table{goods, buddy, stone, effect, vfxTable(), charaInitTable(), faceTable(), customWeaponTable()}, actors...),
		CapacitySlack:     64 << 10,
		IV:                bytes.Repeat([]byte{0x5A}, aesIVSize),
	}
}

func vanillaTriggerFor(rowID int32) int32 {
	for family, triggers := range GoodsFamilies {
		if rowID >= family && rowID < family+LevelRows {
			return triggers[rowID-family]
		}
	}
	return 0
}

// Build assembles the complete synthetic save.
func Build(options Options) ([]byte, error) {
	if len(options.IV) != aesIVSize {
		return nil, fmt.Errorf("fixture IV must be %d bytes, got %d", aesIVSize, len(options.IV))
	}
	binder, err := buildBinder(options.Tables)
	if err != nil {
		return nil, err
	}
	encoder, err := zstd.NewWriter(nil, zstd.WithEncoderLevel(zstd.SpeedBestCompression), zstd.WithEncoderCRC(false))
	if err != nil {
		return nil, err
	}
	stream := encoder.EncodeAll(binder, nil)
	if err := encoder.Close(); err != nil {
		return nil, err
	}

	plaintextLen := roundUp(dcxHeaderSize+len(stream)+options.CapacitySlack, aes.BlockSize)
	plaintext := make([]byte, plaintextLen)
	format := options.DCXFormat
	if format == "" {
		format = "ZSTD"
	}
	if len(format) != 4 {
		return nil, fmt.Errorf("DCX format tag must be four characters, got %q", format)
	}
	writeDCXHeader(plaintext, format, len(binder), len(stream))
	copy(plaintext[dcxHeaderSize:], stream)

	block, err := aes.NewCipher(regulationKey)
	if err != nil {
		return nil, err
	}
	ciphertext := make([]byte, plaintextLen)
	cipher.NewCBCEncrypter(block, options.IV).CryptBlocks(ciphertext, plaintext)

	save := make([]byte, UserData11Offset+userdata11HeaderSize+aesIVSize+plaintextLen)
	copy(save, ps4Magic)
	userdata11 := save[UserData11Offset:]
	copy(userdata11[:4], " GER")
	binary.LittleEndian.PutUint32(userdata11[8:12], options.RegulationVersion)
	copy(userdata11[userdata11HeaderSize:], options.IV)
	copy(userdata11[userdata11HeaderSize+aesIVSize:], ciphertext)
	return save, nil
}

// writeDCXHeader fills the four fields the product reads and the magic strings
// that make the block recognisable; the remaining words are not consulted by
// any reader here and are left zero.
func writeDCXHeader(header []byte, format string, decompressed, compressed int) {
	copy(header[0:4], "DCX\x00")
	binary.BigEndian.PutUint32(header[4:8], 0x11000)
	binary.BigEndian.PutUint32(header[8:12], 0x18)
	binary.BigEndian.PutUint32(header[12:16], 0x24)
	copy(header[0x18:0x1C], "DCS\x00")
	binary.BigEndian.PutUint32(header[28:32], uint32(decompressed))
	binary.BigEndian.PutUint32(header[32:36], uint32(compressed))
	copy(header[0x24:0x28], "DCP\x00")
	copy(header[40:44], format)
	copy(header[0x44:0x48], "DCA\x00")
	binary.BigEndian.PutUint32(header[0x48:0x4C], 8)
}

func buildBinder(tables []Table) ([]byte, error) {
	type blob struct {
		name string
		body []byte
	}
	blobs := make([]blob, 0, len(tables))
	for _, table := range tables {
		body, err := buildParam(table)
		if err != nil {
			return nil, err
		}
		// A real regulation names entries by their full game path. Using one
		// here exercises the product's suffix match rather than only the exact
		// name comparison.
		blobs = append(blobs, blob{name: `N:\GR\data\Param\param\GameParam\` + table.Entry, body: body})
	}

	nameStart := binderHeaderSize + len(blobs)*binderEntrySize
	names := make([]byte, 0, 64*len(blobs))
	nameOffsets := make([]int, len(blobs))
	for index, item := range blobs {
		nameOffsets[index] = nameStart + len(names)
		for _, unit := range utf16.Encode([]rune(item.name)) {
			names = binary.LittleEndian.AppendUint16(names, unit)
		}
		names = append(names, 0, 0)
	}

	// Entry allocations are contiguous and 16-aligned, and the last one ends at
	// the archive end with no trailing padding. That is how the shipped
	// regulation is laid out, and growing an entry depends on it: an entry that
	// does not own the space up to its neighbour cannot be enlarged in place.
	dataStart := roundUp(nameStart+len(names), 16)
	offsets := make([]int, len(blobs))
	position := dataStart
	for index, item := range blobs {
		offsets[index] = position
		position += len(item.body)
		if index+1 < len(blobs) {
			position = roundUp(position, 16)
		}
	}

	binder := make([]byte, position)
	copy(binder[0:4], "BND4")
	binary.LittleEndian.PutUint32(binder[0x0C:0x10], uint32(len(blobs)))
	binary.LittleEndian.PutUint64(binder[0x10:0x18], binderHeaderSize)
	binary.LittleEndian.PutUint64(binder[0x20:0x28], binderEntrySize)
	for index, item := range blobs {
		base := binderHeaderSize + index*binderEntrySize
		binary.LittleEndian.PutUint32(binder[base+4:base+8], 0xFFFFFFFF)
		binary.LittleEndian.PutUint64(binder[base+8:base+16], uint64(len(item.body)))
		// Stored, not compressed: the shipped regulation repeats the size here.
		binary.LittleEndian.PutUint64(binder[base+16:base+24], uint64(len(item.body)))
		binary.LittleEndian.PutUint32(binder[base+24:base+28], uint32(offsets[index]))
		binary.LittleEndian.PutUint32(binder[base+28:base+32], uint32(index))
		binary.LittleEndian.PutUint32(binder[base+32:base+36], uint32(nameOffsets[index]))
		copy(binder[offsets[index]:], item.body)
	}
	copy(binder[nameStart:], names)
	return binder, nil
}

// buildParam writes the PARAM shape the product parses: a 0x40 header with the
// type string reached by offset, a long-offset row table, then contiguous rows
// whose uniform stride is the row size.
func buildParam(table Table) ([]byte, error) {
	if len(table.RowIDs) < 2 {
		return nil, fmt.Errorf("%s needs at least two rows for a stride to exist", table.Entry)
	}
	if table.RowSize <= 0 {
		return nil, fmt.Errorf("%s needs a positive row size", table.Entry)
	}
	if len(table.RowIDs) > 0xFFFF {
		return nil, fmt.Errorf("%s has more rows than the 16-bit row count holds", table.Entry)
	}

	format := table.Format
	if format == 0 {
		format = DefaultParamFormat
	}
	rowTableOffset, rowEntrySize, typeByOffset := paramLayout(format)

	// The shipped order, measured on 11701000: the row table is followed
	// immediately by the row bodies, and every string — the type name first,
	// then one name per row — sits after them. Building the strings in front of
	// the bodies instead would still parse, which is exactly why it would be a
	// bad fixture: row insertion has to relocate this region, and a fixture that
	// puts it out of harm's way would witness nothing.
	rowCount := len(table.RowIDs)
	dataStart := rowTableOffset + rowCount*rowEntrySize
	bodyRegion := rowCount*table.RowSize + table.Stagger
	stringsOffset := dataStart + bodyRegion

	strings := []byte(nil)
	typeBytes := append([]byte(table.ParamType), 0)
	if typeByOffset {
		strings = append(strings, typeBytes...)
	}
	// Every shipped row carries a name. Rows here do too, so that the offsets
	// pointing into this region are real and have to survive being moved.
	nameOffsets := make([]int, rowCount)
	for index, rowID := range table.RowIDs {
		nameOffsets[index] = stringsOffset + len(strings)
		strings = append(strings, encodeUTF16(fmt.Sprintf("%s_%d", table.ParamType, rowID))...)
	}

	blob := make([]byte, stringsOffset+len(strings))
	blob[0x2D] = format
	binary.LittleEndian.PutUint16(blob[8:10], uint16(table.DataVersion))
	binary.LittleEndian.PutUint16(blob[10:12], uint16(rowCount))
	binary.LittleEndian.PutUint32(blob[0:4], uint32(stringsOffset))
	binary.LittleEndian.PutUint64(blob[0x30:0x38], uint64(dataStart))
	copy(blob[stringsOffset:], strings)
	if typeByOffset {
		binary.LittleEndian.PutUint64(blob[0x10:0x18], uint64(stringsOffset))
	} else {
		// The older shape carries the type inline in a fixed 0x20 field.
		if len(typeBytes) > 0x20 {
			return nil, fmt.Errorf("%s: type %q does not fit the inline field", table.Entry, table.ParamType)
		}
		copy(blob[12:12+0x20], typeBytes)
	}

	for index, rowID := range table.RowIDs {
		base := rowTableOffset + index*rowEntrySize
		offset := dataStart + index*table.RowSize
		if index == rowCount-1 {
			offset += table.Stagger
		}
		binary.LittleEndian.PutUint32(blob[base:base+4], uint32(rowID))
		if rowEntrySize == 24 {
			binary.LittleEndian.PutUint64(blob[base+8:base+16], uint64(offset))
			binary.LittleEndian.PutUint64(blob[base+16:base+24], uint64(nameOffsets[index]))
		} else {
			binary.LittleEndian.PutUint32(blob[base+4:base+8], uint32(offset))
		}
		if table.Init != nil {
			table.Init(rowID, blob[offset:offset+table.RowSize])
		}
	}
	return blob, nil
}

func encodeUTF16(value string) []byte {
	units := utf16.Encode([]rune(value))
	encoded := make([]byte, 0, len(units)*2+2)
	for _, unit := range units {
		encoded = append(encoded, byte(unit), byte(unit>>8))
	}
	return append(encoded, 0, 0)
}

func roundUp(value, multiple int) int {
	if remainder := value % multiple; remainder != 0 {
		return value + multiple - remainder
	}
	return value
}

// SpiritVfxRow and SpiritPhantomProfile are the colour switch's one row and its
// shipped value. Written out independently of savecore like every other offset
// here, and the row is present so a fixture can be asked which way the switch
// is - a save without it cannot be opened at all.
const (
	SpiritVfxRow          = int32(57000)
	SpiritPhantomProfile  = int32(200)
	vfxPhantomFieldOffset = 128
)

// charaInitTable is CharaInitParam at its shipped geometry - CHARACTER_INIT_PARAM,
// version 2, 320 bytes - which the product opens to read the person writes'
// fields (savecore/personwrites.go). It holds two class templates and none of
// the armed people: a save reads as having no person written, which is what an
// untouched save says, and the real-save tests are where the people's own rows
// are measured. Two rows, for the stride.
func charaInitTable() Table {
	return Table{
		Entry: "CharaInitParam.param", ParamType: "CHARACTER_INIT_PARAM",
		DataVersion: 2, RowSize: 320,
		RowIDs: []int32{3000, 3001},
	}
}

// faceTable and customWeaponTable are FaceParam and EquipParamCustomWeapon
// at their shipped geometry, which the product opens to write a player's
// character as a person (savecore/characters.go). Each holds two shipped-like
// rows on either side of the character band, so an inserted character row
// lands between them as it does in the real tables, which both run past it.
func faceTable() Table {
	return Table{
		Entry: "FaceParam.param", ParamType: "FACE_PARAM_ST",
		DataVersion: 2, RowSize: 240,
		RowIDs: []int32{1005, 3000000},
	}
}

func customWeaponTable() Table {
	return Table{
		Entry: "EquipParamCustomWeapon.param", ParamType: "EQUIP_PARAM_CUSTOM_WEAPON_ST",
		DataVersion: 1, RowSize: 16,
		RowIDs: []int32{10, 3000000},
	}
}

// vfxTable is the one SpEffectVfxParam row this product reads and writes,
// seeded with the profile the game ships.
func vfxTable() Table {
	return Table{
		Entry: "SpEffectVfxParam.param", ParamType: "SP_EFFECT_VFX_PARAM_ST",
		DataVersion: 1, RowSize: 164,
		// Two rows, because a PARAM's stride is derived from the gap between
		// the first two and a table with one row has no stride to read. The
		// second is also the more useful shape to test against: it carries 0,
		// the no-overwrite value 1,457 of the 1,513 shipped rows carry, so a
		// write that reached the wrong row would be visible rather than
		// indistinguishable from the right one.
		RowIDs: []int32{SpiritVfxRow, SpiritVfxRow + 1},
		Init: func(rowID int32, row []byte) {
			if rowID != SpiritVfxRow {
				return
			}
			binary.LittleEndian.PutUint32(row[vfxPhantomFieldOffset:], uint32(SpiritPhantomProfile))
		},
	}
}
