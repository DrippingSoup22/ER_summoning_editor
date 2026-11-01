package gio

import (
	"testing"

	"gioui.org/op"

	"github.com/DrippingSoup22/ER_summoning_editor/internal/catalog"
)

// One editor frame, warm: the state is loaded and one frame already drawn, so
// what this measures is what a redraw costs rather than what a load does.
func BenchmarkFrame(b *testing.B) {
	state := loadedState(b)
	th := state.Theme()
	gtx := headlessGtx()
	gtx.Ops = new(op.Ops)
	state.Layout(gtx, th)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		gtx.Ops.Reset()
		state.Layout(gtx, th)
	}
}

func BenchmarkIconName(b *testing.B) {
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		iconName("Swordhand of Night Jolán")
	}
}

// A cache hit, which is what every frame after the first one does.
func BenchmarkArtLookupHit(b *testing.B) {
	cache := newAshCache()
	cache.op("Black Knife Tiche", 64)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache.op("Black Knife Tiche", 64)
	}
}

func BenchmarkResetStaged(b *testing.B) {
	state := loadedState(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state.resetStaged()
	}
}

func BenchmarkPendingCount(b *testing.B) {
	state := loadedState(b)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		state.PendingCount()
	}
}

// Filling a cold cache, which is what the first paint of the Ash rail costs.
func BenchmarkArtWarmUp(b *testing.B) {
	ashes, err := catalog.LoadAshes()
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		cache := newAshCache()
		for _, ash := range ashes {
			cache.op(ash.Name, 64)
		}
	}
}
