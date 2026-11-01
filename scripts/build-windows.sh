#!/bin/sh
# Cross-build the desktop editor for Windows.
#
# This exists for one flag. Without `-H windowsgui` the linker marks the binary
# as a console application, and Windows opens a console window behind it every
# time it starts - one that cannot be closed without killing the editor. It is
# invisible on a developer's machine, where the binary is launched from a shell
# that already has one, and immediately obvious to anyone who double-clicks it.
#
# CGO off because Windows Gio needs no cgo, which is what makes cross-building
# from WSL work at all; the Linux and macOS builds do need it and are not this
# script's business.
#
# `scripts/release.sh` packages this binary too, and repeats the flags rather
# than calling this script, so a release does not depend on another script
# having been run first. This one stays because a developer building the editor
# to look at it should not have to run a release.
# The window and taskbar icon is NOT set from Go. Gio's Windows driver loads it
# out of the executable's own resources - LoadImage(hInst, 1, IMAGE_ICON) in
# os_windows.go - so it has to be a linked Windows resource. The linker picks
# up cmd/ersummoneditorgui/rsrc_windows_amd64.syso automatically because of its
# name, and that file is committed, so an ordinary build needs nothing here.
#
# To change the icon, replace packaging/windows/icon.png and regenerate both:
#
#   go run ./tools/appicon
#   go run github.com/akavel/rsrc@v0.10.2 -ico packaging/windows/app.ico #       -o cmd/ersummoneditorgui/rsrc_windows_amd64.syso -arch amd64
#
# rsrc is not a dependency of this module and is deliberately not vendored:
# it runs twice a year, and adding a build-time tool to go.mod to save typing a
# command that long is a bad trade.
set -eu

# The file name is the tool's name, not the package's, and it is written the way
# a person writes it. cmd/ersummoneditorgui is an import path and stays as it is;
# what a player double-clicks in Explorer should say what it is, in words, and
# match the title bar it opens.
#
# The spaces are deliberate and the ldflags ID below is deliberately NOT changed
# with them: gioui.org/app.ID is where the app keeps its settings, so renaming it
# would silently orphan the font and icon-size choices already saved.
OUTPUT=${1:-"dist/ER Summoning Editor.exe"}
mkdir -p "$(dirname "$OUTPUT")"

# Pack the art before linking it in, every time.
#
# The desktop binary embeds the icon directory whole - embed variables are not
# dead-code eliminated - so a full-resolution master left in it is 3.5 MB of
# executable for a picture drawn into a cell 34 dp across. That is not a
# hypothetical: 93 of them accumulated unnoticed while other work was going on,
# and five more landed within the hour of clearing them out. Art arrives
# continuously and a manual step in front of it will be missed again, so the
# build does it rather than asking.
#
# It is idempotent: an icon whose decoded pixels already match its master is
# left byte-for-byte alone, so an ordinary build leaves the tracked icons
# untouched. It compares pixels, not bytes, because Go's PNG encoding changes
# between releases - the shipped set was packed by go1.25 and go1.27 encodes
# the same pixels differently. It is not fast: it decodes every master, which
# is about a minute on the full roster, and that is the price of not having to
# remember to run it. The masters are the author's and stay private, so the
# published folder carries the packed icons and has no packer to run.
if [ -d tools/artpack ]; then
    go run ./tools/artpack
fi

CGO_ENABLED=0 GOOS=windows GOARCH=amd64 \
    go build -trimpath \
    -ldflags "-H windowsgui -s -w -X gioui.org/app.ID=io.github.DrippingSoup22.ERSummoningEditor" \
    -o "$OUTPUT" ./cmd/ersummoneditorgui

echo "built $OUTPUT"
