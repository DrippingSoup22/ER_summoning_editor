package savecore

import "github.com/DrippingSoup22/ER_shared/regulation"

// The six tables this product opens. regulation.OpenTable asserts each one's
// type, data version and stride on every open. The actor tables are outside the
// patch write surface - docs/SAFETY.md rule 1 permits inserting a clone into
// them and nothing else - but an insertion rebuilds the whole table, so their
// geometry is checked like every other. SpEffectVfxParam holds one field of one
// row: whether a summon renders in the white spirit look (see vfxFields).
var (
	goodsLayout    = regulation.MustLayout("EquipParamGoods")
	buddyLayout    = regulation.MustLayout("BuddyParam")
	stoneLayout    = regulation.MustLayout("BuddyStoneParam")
	npcLayout      = regulation.MustLayout("NpcParam")
	npcThinkLayout = regulation.MustLayout("NpcThinkParam")
	vfxLayout      = regulation.MustLayout("SpEffectVfxParam")
)
