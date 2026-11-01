module github.com/DrippingSoup22/ER_summoning_editor

// Floor set by the GUI dependencies: Gio v0.10.x needs >= 1.24. The command
// line itself uses nothing newer than 1.22 and would still build under it.
go 1.25.0

require (
	gioui.org v0.10.1
	github.com/DrippingSoup22/ER_shared v0.2.0
	github.com/klauspost/compress v1.17.11
	github.com/ncruces/zenity v0.10.14
	golang.org/x/exp/shiny v0.0.0-20250408133849-7e4ce0ab07d0
	golang.org/x/image v0.44.0
	golang.org/x/text v0.40.0
)

require (
	gioui.org/shader v1.0.8 // indirect
	github.com/akavel/rsrc v0.10.2 // indirect
	github.com/dchest/jsmin v0.0.0-20220218165748-59f39799265f // indirect
	github.com/go-text/typesetting v0.3.4 // indirect
	github.com/josephspurrier/goversioninfo v1.4.1 // indirect
	github.com/randall77/makefat v0.0.0-20210315173500-7ddd0e42c844 // indirect
	golang.org/x/net v0.48.0 // indirect
	golang.org/x/sys v0.39.0 // indirect
)
