package plan

// The resource price of a Spirit Ash.
//
// What the two fields mean at runtime was measured on the console, and the
// measurement is why this is one editing surface rather than two. A positive consumeMP or
// consumeHP is *both* the minimum the player must be holding and the exact
// amount the summon spends, so the cost and the requirement to pay it are the
// same number. When both are positive the game applies AND logic: either
// shortfall blocks the Ash, and a successful use spends both.
//
// It also measured that 0/-1, 0/0 and -1/-1 are equivalent free pairs. This
// product writes 0/-1, which is the spelling Giant Rat Ashes ships, so that a
// free Ash made here is byte-identical to the free Ash the game already has.

import (
	"fmt"
	"sort"
)

// OperationCost is the manifest's name for a price edit.
const OperationCost = "cost"

// Price is what one summon costs. A field at or below zero is not spent.
type Price struct {
	FP, HP int32
}

// Free reports whether this price takes nothing from the player.
func (p Price) Free() bool { return p.FP <= 0 && p.HP <= 0 }

// Free is the pair the game ships on its one no-cost family, Giant Rat Ashes.
//
// 0/-1, 0/0 and -1/-1 are equivalent at runtime, so this is a
// spelling choice rather than a behavioural one. Normalising to the shipped
// spelling means a free Ash this product makes is byte-identical to the free
// Ash the game already has, which is one fewer difference for anyone comparing
// an edited save against vanilla.
var Free = Price{FP: 0, HP: -1}

// normalize is where that choice is applied, so every caller gets it and none
// of them has to know the rule.
func (p Price) normalize() Price {
	if p.Free() {
		return Free
	}
	return p
}

// Cost is one host Ash's new price.
//
// The price is per family and not per row. Eleven goods rows carry it, one per
// upgrade level, and no shipped family varies across them - the complete 1.17
// scan found all 84 constant. The player means "this Ash costs less", so the
// eleven rows are an implementation detail this expands into rather than a
// choice to put in front of them.
type Cost struct {
	HostGoodsFamily int32
	HostName        string
	UpgradeLevels   int
	Price           Price
}

// BuildCost assembles the manifest for a set of price changes.
func BuildCost(sourceSHA256 string, costs []Cost) (*Manifest, error) {
	if len(sourceSHA256) != 64 {
		return nil, fmt.Errorf("source_sha256 must be a 64-character SHA-256 digest, got %d characters", len(sourceSHA256))
	}
	if len(costs) == 0 {
		return nil, ErrNoCosts
	}
	manifest := &Manifest{SourceSHA256: sourceSHA256, Operation: OperationCost}
	seen := map[int32]string{}
	ordered := append([]Cost(nil), costs...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].HostGoodsFamily < ordered[j].HostGoodsFamily })

	for _, cost := range ordered {
		if cost.UpgradeLevels <= 0 {
			return nil, fmt.Errorf("%s: upgrade_levels must be positive", cost.HostName)
		}
		if err := cost.Price.Validate(); err != nil {
			return nil, fmt.Errorf("%s: %w", cost.HostName, err)
		}
		price := cost.Price.normalize()
		if previous, clash := seen[cost.HostGoodsFamily]; clash {
			return nil, fmt.Errorf("goods family %d is priced twice, by %s and %s",
				cost.HostGoodsFamily, previous, cost.HostName)
		}
		seen[cost.HostGoodsFamily] = cost.HostName
		for level := 0; level < cost.UpgradeLevels; level++ {
			manifest.GoodsPatches = append(manifest.GoodsPatches, RowPatch{
				RowID: cost.HostGoodsFamily + int32(level),
				Name:  fmt.Sprintf("%s +%d costs %s", cost.HostName, level, price),
				Set: map[string]any{
					"consumeMP": price.FP,
					"consumeHP": price.HP,
				},
			})
		}
	}
	return manifest, nil
}

// MinPrice and MaxPrice bound what a price may be set to.
//
// -1 is the unset spelling, and the ceiling is the one the private research
// writer uses: comfortably above Mimic Tear's 660, the largest the game ships,
// and well inside the s16 the fields are stored in. A number a player could
// type that the field cannot hold is refused here rather than silently
// truncated into a price nobody asked for.
const (
	MinPrice = int32(-1)
	MaxPrice = int32(9999)
)

// Validate reports whether both halves are values the fields can hold. It is
// exported so a frontend can refuse a number while the player is still looking
// at the box they typed it into, rather than at the commit.
func (p Price) Validate() error {
	for _, field := range []struct {
		name  string
		value int32
	}{{"FP", p.FP}, {"HP", p.HP}} {
		if field.value < MinPrice || field.value > MaxPrice {
			return fmt.Errorf("%s price %d is outside %d..%d",
				field.name, field.value, MinPrice, MaxPrice)
		}
	}
	return nil
}

// String is what the manifest records, so an audit reads as a sentence rather
// than as two numbers whose units the reader has to know.
func (p Price) String() string {
	switch {
	case p.Free():
		return "nothing"
	case p.FP > 0 && p.HP > 0:
		return fmt.Sprintf("%d FP and %d HP", p.FP, p.HP)
	case p.HP > 0:
		return fmt.Sprintf("%d HP", p.HP)
	default:
		return fmt.Sprintf("%d FP", p.FP)
	}
}

// ErrNoCosts mirrors ErrNoConversions: a manifest that changes no price is a
// manifest that should not have been built.
var ErrNoCosts = fmt.Errorf("a cost edit needs at least one Ash")
