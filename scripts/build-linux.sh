#!/bin/sh
# Build the Linux release on Linux (WSL will do):
# dist/ER-Summoning-Editor-linux-amd64-<version>.zip, one folder holding the
# desktop editor, its icon, the README and the licences. The command line is
# left out, as scripts/build-windows.sh explains.
#
#   VERSION=1.0.0 sh scripts/build-linux.sh
#
# Linux Gio needs cgo, so this cannot cross-build: it needs gcc, pkg-config and
# the Wayland, X11, xkbcommon, EGL/GLES, libffi, Xcursor, Xfixes and Vulkan
# development packages. Built on Ubuntu 24.04 the editor needs glibc 2.34 or
# newer. Its file dialogs need zenity, matedialog or qarma at run time.
set -eu
cd "$(dirname "$0")/.."

if [ "$(go env GOOS)" != linux ]; then
    echo "the Linux editor must be built on Linux" >&2
    exit 1
fi
version=${VERSION:-}
version=${version#v}
case "$version" in
    [0-9]*.[0-9]*.[0-9]*) ;;
    *) echo "set VERSION to the release's version, such as VERSION=1.0.0" >&2; exit 1 ;;
esac
package="ER-Summoning-Editor-linux-amd64-$version"
archive="dist/$package.zip"
# Staged on Linux's own file system, so the programs keep their executable bit
# in the archive even when the source sits on a Windows drive.
staging=$(mktemp -d)
trap 'rm -rf "$staging"' EXIT
bundle="$staging/$package"
mkdir -p dist "$bundle"

# The art, as scripts/build-windows.sh packs it.
if [ -d tools/artpack ]; then
    go run ./tools/artpack
fi

CGO_ENABLED=1 go build -trimpath -buildvcs=false \
    -ldflags "-s -w -X gioui.org/app.ID=io.github.DrippingSoup22.ERSummoningEditor" \
    -o "$bundle/ERSummoningEditor" ./cmd/ersummoneditorgui
install -m 644 packaging/windows/icon.png "$bundle/ERSummoningEditor.png"
install -m 644 README.md LICENSE THIRD_PARTY_NOTICES.md "$bundle/"

rm -f "$archive"
go run github.com/DrippingSoup22/ER_shared/releasezip -root "$bundle" -out "$archive"
echo "built $archive"
