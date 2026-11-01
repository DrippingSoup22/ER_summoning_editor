package main

import (
	"testing"
)

func TestParseConversionsRejectsMalformedInput(t *testing.T) {
	for _, input := range [][]string{nil, {"Lone Wolf Ashes"}} {
		if _, err := parseConversions(input); err == nil {
			t.Errorf("input %v was accepted", input)
		}
	}
}
