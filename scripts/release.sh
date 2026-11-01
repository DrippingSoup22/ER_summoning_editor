#!/bin/sh
# Build reproducible release archives.
#
# Three things this does that a plain `go build` loop does not:
#
#   - it refuses to package anything scripts/check.sh rejects, so a failing
#     suite cannot produce a downloadable binary, and it demands the real
#     regulation while doing so - a suite whose real-regulation tests all
#     skipped exits 0 and looks exactly like one that passed, which is fine
#     while developing and is not a release gate;
#   - it builds every target twice and compares SHA-256 itself, rather than
#     claiming determinism and leaving a maintainer to verify it;
#   - it normalises archive metadata, so the archive reproduces too and not
#     only the binary inside it.
#
# Archives are .tar.gz for every target, including Windows: Windows 10 1803 and
# later ship bsdtar, so `tar -xf` works there with nothing installed. Requiring
# `zip` would make this script fail on a clean machine for no gain.
set -eu

VERSION=${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}
OUTPUT_DIR=${OUTPUT_DIR:-dist}

# Any fixed instant will do. What matters is that an archive does not carry the
# moment it was built, which would make two identical builds differ.
EPOCH=1700000000

TARGETS=${TARGETS:-"windows/amd64 linux/amd64 darwin/arm64 darwin/amd64"}

# Read from the source so a release cannot claim a version the code does not
# enforce.
REGULATION=$(sed -n 's/^var supportedRegulationVersions = \[\]uint32{\(.*\)}$/\1/p' \
    internal/application/application.go)
if [ -z "$REGULATION" ]; then
    echo "could not read supportedRegulationVersions from the source" >&2
    exit 1
fi

# REQUIRE_SAVE makes check.sh refuse a run it cannot measure the product
# with. Without it a release could be cut on a suite that proved the fixture
# works and nothing else. See scripts/check.sh.
echo "==> checking"
REQUIRE_SAVE=1 sh scripts/check.sh

rm -rf "$OUTPUT_DIR"
mkdir -p "$OUTPUT_DIR"

# The art, before anything is linked, where the masters are. See
# scripts/build-windows.sh for why this is the build's job and not a step
# somebody is asked to remember.
if [ -d tools/artpack ]; then
    go run ./tools/artpack
fi
work=$(mktemp -d)
trap 'rm -rf "$work"' EXIT

# CGO off and -trimpath so the binary carries no build-machine paths;
# -buildvcs=false so it carries no commit or dirty flag either. Both are what
# make the double build below agree.
build() {
    goos=$1
    goarch=$2
    package=$3
    ldflags=$4
    destination=$5
    CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
        go build -trimpath -buildvcs=false -ldflags="$ldflags" \
        -o "$destination" "$package"
}

# Build one binary twice, refuse to package it unless both agree, and print its
# digest. This is the claim the script exists to make rather than assert, so it
# is applied to every binary that goes in an archive and not only the first.
reproducible() {
    goos=$1
    goarch=$2
    package=$3
    ldflags=$4
    output=$5
    build "$goos" "$goarch" "$package" "$ldflags" "$work/first"
    build "$goos" "$goarch" "$package" "$ldflags" "$work/second"
    first=$(sha256sum "$work/first" | cut -d" " -f1)
    second=$(sha256sum "$work/second" | cut -d" " -f1)
    if [ "$first" != "$second" ]; then
        echo "$package for $goos/$goarch is not reproducible: $first vs $second" >&2
        exit 1
    fi
    mv "$work/first" "$output"
    rm -f "$work/second"
    echo "$first"
}

# The desktop editor ships on Windows only, and that is a constraint rather
# than a decision. Windows Gio needs no cgo, so it cross-builds from anywhere;
# Linux and macOS Gio need cgo and the native development packages, which
# cannot be cross-built and would make this script fail on a clean machine.
#
# -H windowsgui is the flag scripts/build-windows.sh exists for: without it the
# linker marks the binary as a console application and Windows opens a console
# window behind the editor that cannot be closed without killing it. It is
# repeated here rather than shared, because a release must not depend on
# another script having been run first.
GUI_LDFLAGS="-H windowsgui -s -w -X gioui.org/app.ID=io.github.DrippingSoup22.ERSummoningEditor"

for target in $TARGETS; do
    goos=${target%/*}
    goarch=${target#*/}
    name="ersummoneditor"
    [ "$goos" = "windows" ] && name="ersummoneditor.exe"

    echo "==> $goos/$goarch"
    stage="$work/stage-$goos-$goarch/ersummoneditor-$VERSION-$goos-$goarch"
    mkdir -p "$stage"
    first=$(reproducible "$goos" "$goarch" ./cmd/ersummoneditor "-s -w" "$stage/$name")

    gui_line="desktop editor      not in this archive (needs cgo on $goos)"
    if [ "$goos" = "windows" ]; then
        gui=$(reproducible "$goos" "$goarch" ./cmd/ersummoneditorgui \
            "$GUI_LDFLAGS" "$stage/ER Summoning Editor.exe")
        gui_line="desktop editor      ER Summoning Editor.exe, sha256 $gui"
    fi
    cp README.md LICENSE THIRD_PARTY_NOTICES.md "$stage/"
    mkdir -p "$stage/docs"
    cp docs/*.md "$stage/docs/"

    cat > "$stage/BUILD-INFO.txt" <<INFO
ER Summoning Editor $VERSION
target              $goos/$goarch
built with          $(go version | cut -d' ' -f3)
regulations         $REGULATION
command line        $name, sha256 $first
$gui_line

This build edits decrypted PlayStation saves whose embedded regulation is one
of $REGULATION. A save from any other game version is refused, not warned
about.

Extract with: tar -xf <archive>
Windows 10 1803 and later have tar built in; no extra tool is needed.

The tool is built and tested for solo play and makes no multiplayer guarantee.
See docs/SAFETY.md.
INFO

    # Sorted names, one fixed timestamp, no owner identity, and gzip -n so the
    # compressed stream carries no filename or mtime of its own.
    archive="$OUTPUT_DIR/ersummoneditor-$VERSION-$goos-$goarch.tar.gz"
    tar --sort=name --mtime="@$EPOCH" --owner=0 --group=0 --numeric-owner \
        -cf - -C "$work/stage-$goos-$goarch" . | gzip -n -9 > "$archive"
done

( cd "$OUTPUT_DIR" && sha256sum ./*.tar.gz > SHA256SUMS )

echo
echo "==> $OUTPUT_DIR"
cat "$OUTPUT_DIR/SHA256SUMS"
