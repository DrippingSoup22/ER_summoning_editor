#!/bin/sh
# Build the Windows release: dist/ER-Summoning-Editor-windows-amd64-<version>.zip,
# one folder holding the desktop editor, the README and the licences. The
# command line is left out: players need only the window, and anyone who wants
# it builds it with `go build ./cmd/ersummoneditor`. VERSION names the release
# (1.2.3 or v1.2.3).
#
#   VERSION=1.0.0 sh scripts/build-windows.sh
#
# The desktop editor is linked with `-H windowsgui`. Without it the linker marks
# the binary as a console application, and Windows opens a console window
# behind it every time it starts - one that cannot be closed without killing
# the editor. It is invisible on a developer's machine, where the binary is
# launched from a shell that already has one, and immediately obvious to
# anyone who double-clicks it.
#
# CGO off because Windows Gio needs no cgo, which is what lets this build from
# any system, WSL included; the Linux build does need it (scripts/build-linux.sh).
#
# The window and taskbar icon is NOT set from Go. Gio's Windows driver loads it
# out of the executable's own resources - LoadImage(hInst, 1, IMAGE_ICON) in
# os_windows.go - so it has to be a linked Windows resource. The linker picks
# up cmd/ersummoneditorgui/rsrc_windows_amd64.syso automatically because of its
# name, and that file is committed, so an ordinary build needs nothing here.
#
# To change the icon, replace packaging/windows/icon.png and regenerate both:
#
#   go run ./tools/appicon
#   go run github.com/akavel/rsrc@v0.10.2 -ico packaging/windows/app.ico \
#       -o cmd/ersummoneditorgui/rsrc_windows_amd64.syso -arch amd64
#
# rsrc is not a dependency of this module and is deliberately not vendored:
# it runs twice a year, and adding a build-time tool to go.mod to save typing a
# command that long is a bad trade.
set -eu
cd "$(dirname "$0")/.."

version=${VERSION:-}
version=${version#v}
case "$version" in
    [0-9]*.[0-9]*.[0-9]*) ;;
    *) echo "set VERSION to the release's version, such as VERSION=1.0.0" >&2; exit 1 ;;
esac
package="ER-Summoning-Editor-windows-amd64-$version"
bundle="dist/windows-amd64/$package"
archive="dist/$package.zip"
rm -rf "$bundle" "$archive"
mkdir -p "$bundle"

# Pack the art before linking it in, every time.
#
# The desktop binary embeds the icon directory whole - embed variables are not
# dead-code eliminated - so a full-resolution master left in it is 3.5 MB of
# executable for a picture drawn into a cell 34 dp across. Art arrives
# continuously and a manual step in front of it would be missed, so the build
# does it rather than asking. It is idempotent: an icon whose decoded pixels
# already match its master is left byte-for-byte alone. The masters are the
# author's and stay private, so the published folder carries the packed icons
# and has no packer to run.
if [ -d tools/artpack ]; then
    go run ./tools/artpack
fi

# gioui.org/app.ID is where the editor keeps its settings: renaming it would
# silently orphan the choices already saved. -buildvcs=false keeps the commit
# out of the binary, so a build from the published source matches this one.
CGO_ENABLED=0 GOOS=windows GOARCH=amd64 go build -trimpath -buildvcs=false \
    -ldflags "-H windowsgui -s -w -X gioui.org/app.ID=io.github.DrippingSoup22.ERSummoningEditor" \
    -o "$bundle/ERSummoningEditor.exe" ./cmd/ersummoneditorgui
cp README.md LICENSE THIRD_PARTY_NOTICES.md "$bundle/"

go run github.com/DrippingSoup22/ER_shared/releasezip -root "$bundle" -out "$archive"
echo "built $archive"
