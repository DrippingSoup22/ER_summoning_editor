package application

// fixedHosts are the Ashes whose summon the game resolves without the params:
// a formation authored for one, or a conversion of one, is never delivered.
// See HostFixedError.
var fixedHosts = map[int32]bool{256000: true}

// FixedHost reports whether an Ash's formation is beyond a save's reach. A
// frontend offers no edit to it.
func FixedHost(goodsFamily int32) bool { return fixedHosts[goodsFamily] }

// refuseFixed is the guard every path that rewrites or re-points a host runs.
func refuseFixed(goodsFamily int32, name string) error {
	if fixedHosts[goodsFamily] {
		return &HostFixedError{Name: name, GoodsFamily: goodsFamily}
	}
	return nil
}
