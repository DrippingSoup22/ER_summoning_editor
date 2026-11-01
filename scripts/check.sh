#!/bin/sh
# Format, vet and test.
#
# The one thing this does beyond `go test ./...` is refuse to be quiet about
# what did not run. Every test that measures this product against the
# regulation it actually ships against skips when ER_SAVE is unset,
# and a skip is not a failure, so a suite with all of them skipped exits 0 and
# reads exactly like a suite that passed. That is fine while developing and is
# not fine as a release gate, which is what scripts/release.sh uses this for.
#
# So: the skipped tests are always listed, and with REQUIRE_SAVE=1 a run
# that cannot reach the real regulation fails instead of passing quietly.
set -eu

unformatted=$(gofmt -l ./cmd ./internal ./reference ./tools)
if [ -n "$unformatted" ]; then
    echo "Go files need formatting:"
    echo "$unformatted"
    exit 1
fi

go vet ./...

# Checked before the suite runs, so a release stops in a second rather than
# after a full test run that proved less than it appears to. A path that is set
# but wrong would let every test skip just as silently as one that is unset.
# ER_SAVE is a decrypted PS4 save on a supported regulation: the author's full
# save, whose eight characters some tests expect.
if [ "${REQUIRE_SAVE:-0}" = "1" ]; then
    if [ -z "${ER_SAVE:-}" ]; then
        echo "REQUIRE_SAVE=1 but ER_SAVE is unset." >&2
        echo "The tests that measure against the shipped regulation would skip," >&2
        echo "and this run would pass without having checked the product at all." >&2
        exit 1
    fi
    if [ ! -f "$ER_SAVE" ]; then
        echo "ER_SAVE is set but $ER_SAVE is missing." >&2
        echo "Every real-regulation test would skip. Point it at a decrypted save." >&2
        exit 1
    fi
fi

go test ./...

# A second pass purely to name what was skipped. The results are cached by the
# run above, so this costs almost nothing; -json is used rather than -v so the
# readable output stays the readable output.
skipped=$(go test -json ./... 2>/dev/null \
    | grep '"Action":"skip"' \
    | grep '"Test":' \
    | sed 's/.*"Test":"\([^"]*\)".*/\1/' \
    | sort -u)

if [ -n "$skipped" ]; then
    count=$(printf '%s\n' "$skipped" | wc -l | tr -d ' ')
    echo
    echo "$count test(s) skipped and therefore proved nothing:"
    printf '%s\n' "$skipped" | sed 's/^/  /'
    if [ -z "${ER_SAVE:-}" ]; then
        echo
        echo "ER_SAVE is unset, which is why most of these skipped."
        echo "Set it to a decrypted save to run them."
    fi
fi
