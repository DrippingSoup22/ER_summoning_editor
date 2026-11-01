package catalog

import (
	"strings"
	"testing"
)

// "unread" - no script in the corpus - is not "no": a nose is added on
// evidence, never on its absence. And a value outside the vocabulary refuses
// the table rather than being read as something.
func TestTheTurnTableReadsOnlyEvidence(t *testing.T) {
	never, err := DecodeThinkTurns(strings.NewReader("think_id,battle_goal,turns\n1,11000,no\n2,999999,unread\n3,431000,yes\n"))
	if err != nil {
		t.Fatal(err)
	}
	if !never[1] || never[2] || never[3] {
		t.Errorf("never = %v, want row 1 alone", never)
	}
	if _, err := DecodeThinkTurns(strings.NewReader("think_id,battle_goal,turns\n1,11000,maybe\n")); err == nil {
		t.Error("a turns value of maybe was accepted")
	}
}
