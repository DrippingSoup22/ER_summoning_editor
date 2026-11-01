// Command appicon builds the Windows application icon from a single PNG.
//
// Gio's Windows driver does not take an icon as an option. It loads one out of
// the executable's own resources - LoadImage(hInst, 1, IMAGE_ICON, ...) in
// os_windows.go - so the icon has to be a linked Windows resource and nothing
// else will do. That means a .ico, compiled into a .syso that the Go linker
// picks up automatically because of its name.
//
// This writes the .ico. github.com/akavel/rsrc turns it into the .syso; see
// scripts/build-windows.sh, which does both.
//
// The sizes are the ones Windows actually asks for: 16 in a title bar, 32 in
// the taskbar and Alt-Tab, 48 in Explorer's medium view, 64 and 128 for larger
// views and high-DPI, 256 for the extra-large one. Windows picks the nearest
// and scales, so a missing size is a blurry icon rather than none - but the
// small ones are where blur is most obvious, and they are cheap.
package main

import (
	"bytes"
	"encoding/binary"
	"flag"
	"fmt"
	"image"
	"image/png"
	"os"

	xdraw "golang.org/x/image/draw"
)

var sides = []int{256, 128, 64, 48, 32, 16}

func main() {
	from := flag.String("in", "packaging/windows/icon.png", "the source picture")
	to := flag.String("out", "packaging/windows/app.ico", "the icon to write")
	flag.Parse()

	source, err := read(*from)
	if err != nil {
		fail(err)
	}
	body, err := build(source)
	if err != nil {
		fail(err)
	}
	if err := os.WriteFile(*to, body, 0o644); err != nil {
		fail(err)
	}
	fmt.Printf("wrote %s: %d sizes, %d bytes\n", *to, len(sides), len(body))
}

func read(path string) (image.Image, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	decoded, err := png.Decode(file)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	return decoded, nil
}

// build assembles the ICO: a directory of entries, then the images.
//
// Every entry is a PNG rather than the older device-independent bitmap. That
// is allowed from Windows Vista on, it is what modern icons use, and it keeps
// the alpha channel without the second inverted mask a BMP entry needs.
func build(source image.Image) ([]byte, error) {
	type entry struct {
		side int
		body []byte
	}
	entries := make([]entry, 0, len(sides))
	for _, side := range sides {
		target := image.NewNRGBA(image.Rect(0, 0, side, side))
		// CatmullRom, as everywhere else art is resampled in this project:
		// bilinear minification to 16px loses the shape of the thing.
		xdraw.CatmullRom.Scale(target, target.Bounds(), source, source.Bounds(), xdraw.Src, nil)
		var body bytes.Buffer
		if err := png.Encode(&body, target); err != nil {
			return nil, err
		}
		entries = append(entries, entry{side: side, body: body.Bytes()})
	}

	var out bytes.Buffer
	// ICONDIR: reserved, type 1 (icon), count.
	binary.Write(&out, binary.LittleEndian, uint16(0))
	binary.Write(&out, binary.LittleEndian, uint16(1))
	binary.Write(&out, binary.LittleEndian, uint16(len(entries)))

	// The images start after the directory, which is 6 bytes plus 16 an entry.
	offset := 6 + 16*len(entries)
	for _, one := range entries {
		// A side of 256 is written as 0: the field is one byte and 256 does
		// not fit in it, which is the format's own convention.
		side := byte(one.side)
		out.WriteByte(side)
		out.WriteByte(side)
		out.WriteByte(0)                                    // no colour palette
		out.WriteByte(0)                                    // reserved
		binary.Write(&out, binary.LittleEndian, uint16(1))  // colour planes
		binary.Write(&out, binary.LittleEndian, uint16(32)) // bits per pixel
		binary.Write(&out, binary.LittleEndian, uint32(len(one.body)))
		binary.Write(&out, binary.LittleEndian, uint32(offset))
		offset += len(one.body)
	}
	for _, one := range entries {
		out.Write(one.body)
	}
	return out.Bytes(), nil
}

func fail(err error) {
	fmt.Fprintln(os.Stderr, "appicon:", err)
	os.Exit(1)
}
