package application

// Staging what a Spirit Ash costs.
//
// This is the second editing surface, and it is here to prove the session
// generalises. Nothing in this file adds machinery: a price stages, unstages,
// composes and commits through exactly the calls a formation does, and the only
// thing G2 had to grow to accept it was a fourth operation in the same list as
// the other three. If a plain field had needed a second staging model, the
// session would have been underbuilt, and this is where that would have shown.
//
// A price is per family and not per row. Eleven goods rows carry it, one per
// upgrade level, and the complete 1.17 scan found all 84 families constant
// across them. The player means "this Ash costs less", so the eleven rows are
// something this expands into rather than a choice to put in front of them.

import (
	"fmt"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
	"github.com/DrippingSoup22/ER_summoning_editor/internal/plan"
)

// StagedCost is one Ash's pending price.
type StagedCost struct {
	HostGoodsFamily int32
	HostName        string
	// Was is what the loaded save charges, so a frontend can show the change
	// rather than only the value it is changing to.
	Was, Price plan.Price
	cost       plan.Cost
}

// StageCost records a new price for one Ash and writes nothing.
//
// Host is an exact case-insensitive name or a numeric goods-family ID, exactly
// as a conversion names one. Staging a second price for the same Ash replaces
// the first: the player means one price, not two.
//
// A price may be staged beside a conversion of the same Ash, which is the one
// pair of edits that share a row. They stay disjoint because they write
// different fields of it, and the save core checks that rather than taking this
// layer's word for it.
func (session *Session) StageCost(host string, price plan.Price) (*StagedCost, error) {
	if !session.Loaded() {
		return nil, ErrNoSaveLoaded
	}
	ash, err := catalog.Resolve(session.service.ashes, host)
	if err != nil {
		return nil, &UnknownAshError{Role: "host", Selector: host, cause: err}
	}
	was, known := session.loaded.state.Goods[ash.GoodsFamily]
	if !known {
		return nil, fmt.Errorf("goods row %d for %s: %w", ash.GoodsFamily, ash.Name, ErrRowNotFound)
	}
	// Checked now rather than at the commit, so a number the field cannot hold
	// is refused while the player is still looking at the box they typed it
	// into. Everything else BuildCost checks comes from the catalog and cannot
	// be wrong here.
	if err := price.Validate(); err != nil {
		return nil, fmt.Errorf("%s: %w", ash.Name, err)
	}
	cost := plan.Cost{
		HostGoodsFamily: ash.GoodsFamily,
		HostName:        ash.Name,
		UpgradeLevels:   ash.UpgradeLevels,
		Price:           price,
	}
	staged := &StagedCost{
		HostGoodsFamily: ash.GoodsFamily,
		HostName:        ash.Name,
		Was:             plan.Price{FP: was.ConsumeMP, HP: was.ConsumeHP},
		Price:           price,
		cost:            cost,
	}
	session.costs[ash.GoodsFamily] = staged
	return staged, nil
}

// FreePrice is what an Ash that costs nothing is staged at: the spelling the
// game ships on its one free family (plan.Free).
var FreePrice = plan.Free

// StagedPrice is the price staged for one Ash, by its goods family, and
// whether one is: what a frontend shows in place of the save's while it is.
func (session *Session) StagedPrice(goodsFamily int32) (Price, bool) {
	staged, ok := session.costs[goodsFamily]
	if !ok {
		return Price{}, false
	}
	return staged.Price, true
}

// UnstageCost drops a staged price and reports whether one was there.
//
// It is separate from Unstage because a price and a formation edit are separate
// things a player can be doing to one Ash at once, so dropping one must not
// silently drop the other.
func (session *Session) UnstageCost(goodsFamily int32) bool {
	_, had := session.costs[goodsFamily]
	delete(session.costs, goodsFamily)
	return had
}
