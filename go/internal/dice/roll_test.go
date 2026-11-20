package dice

import (
	"math/rand"
	"strings"
	"testing"
)

// The notation a DM writes is the notation the engine has to read: attack bonuses, damage dice and plain constants.
func TestParseRollReadsTheNotationsTheGameUses(t *testing.T) {
	cases := []struct {
		notation string
		modifier int
		dice     int
	}{
		{"d20", 0, 1},
		{"1d20+4", 4, 1},
		{"1d20-1", -1, 1},
		{"2d6+3", 3, 1},
		{"1d4+1d6+2", 2, 2},
		{"1d8", 0, 1},
		{"+5", 5, 0},
	}
	for _, c := range cases {
		terms, modifier, _, err := ParseRoll(c.notation)
		if err != nil {
			t.Fatalf("%s: unexpected error %v", c.notation, err)
		}
		if len(terms) != c.dice {
			t.Fatalf("%s: expected %d dice terms, got %d", c.notation, c.dice, len(terms))
		}
		if modifier != c.modifier {
			t.Fatalf("%s: expected modifier %d, got %d", c.notation, c.modifier, modifier)
		}
	}
}

// Guessing at an unreadable notation would put a number in front of the table that nothing can verify.
func TestParseRollRefusesWhatItCannotRead(t *testing.T) {
	for _, bad := range []string{"", "d", "1d", "two d six", "1d20+", "0d6", "1d0", "1d20-1d4"} {
		if _, _, _, err := ParseRoll(bad); err == nil {
			t.Fatalf("%q should not be read as a roll", bad)
		}
	}
}

// Every face has to be inside the die: a roll the table can check is the whole point of moving dice out of the model.
func TestRollStaysInsideTheDice(t *testing.T) {
	rng := rand.New(rand.NewSource(7))
	for i := 0; i < 500; i++ {
		roll, err := RollNotation("2d6+3", rng)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(roll.Dice) != 1 || len(roll.Dice[0]) != 2 {
			t.Fatalf("expected two dice, got %v", roll.Dice)
		}
		total := 3
		for _, face := range roll.Dice[0] {
			if face < 1 || face > 6 {
				t.Fatalf("a d6 came up %d", face)
			}
			total += face
		}
		if roll.Total != total {
			t.Fatalf("total %d does not match the faces %v plus 3", roll.Total, roll.Dice[0])
		}
	}
}

func TestRollDetailReadsLikeTheTable(t *testing.T) {
	roll := Roll{
		Notation: "1d20+4",
		Dice:     [][]int{{9}},
		sizes:    []int{20},
		Modifier: 4,
		Total:    13,
	}
	if got := roll.Detail(); got != "d20(9)+4 = 13" {
		t.Fatalf("unexpected detail %q", got)
	}

	neg := Roll{Notation: "1d20-1", Dice: [][]int{{11}}, sizes: []int{20}, Modifier: -1, Total: 10}
	if got := neg.Detail(); got != "d20(11)-1 = 10" {
		t.Fatalf("unexpected detail %q", got)
	}

	damage := Roll{Notation: "1d4+4", Dice: [][]int{{3}}, sizes: []int{4}, Modifier: 4, Total: 7}
	if got := damage.Detail(); !strings.HasPrefix(got, "d4(3)+4 = 7") {
		t.Fatalf("unexpected detail %q", got)
	}
}

// A natural 20 and a natural 1 are read from the die, not from the total: a +9 cannot turn a 20 into a 19.
func TestNaturalIsTheDieFace(t *testing.T) {
	roll := Roll{Dice: [][]int{{20}}, sizes: []int{20}, Modifier: 5, Total: 25}
	if got := roll.Natural(); got != 20 {
		t.Fatalf("expected natural 20, got %d", got)
	}
}
