package dice

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
)

// Roll is one rolled notation, kept whole so it can be read back to the table.
type Roll struct {
	// Notation is what was rolled, normalised: "1d20+4".
	Notation string
	// Dice holds the individual faces of every dice term, in order.
	Dice [][]int
	// Modifier is the sum of the constant terms.
	Modifier int
	// Total is every die plus the modifier: the number the game uses.
	Total int

	// sizes is the die size of each entry in Dice, so Detail can label them without re-parsing.
	sizes []int
}

// Detail reads the roll the way a player says it: "d20(9)+4 = 13".
func (r Roll) Detail() string {
	var b strings.Builder
	for i, faces := range r.Dice {
		size := 0
		if i < len(r.sizes) {
			size = r.sizes[i]
		}
		if i > 0 {
			b.WriteString("+")
		}
		b.WriteString("d")
		b.WriteString(strconv.Itoa(size))
		b.WriteString("(")
		for j, face := range faces {
			if j > 0 {
				b.WriteString(",")
			}
			b.WriteString(strconv.Itoa(face))
		}
		b.WriteString(")")
	}
	if len(r.Dice) > 0 && r.Modifier > 0 {
		b.WriteString("+")
	}
	if len(r.Dice) == 0 || r.Modifier != 0 {
		b.WriteString(strconv.Itoa(r.Modifier))
	}
	b.WriteString(" = ")
	b.WriteString(strconv.Itoa(r.Total))
	return b.String()
}

// Plain is the roll as a table reads it out: the faces the dice showed and the modifier, never the sum. It exists so
// the engine's lines can be copied into prose without the copy turning into an equation.
func (r Roll) Plain() string {
	var b strings.Builder
	for i, faces := range r.Dice {
		size := 0
		if i < len(r.sizes) {
			size = r.sizes[i]
		}
		if i > 0 {
			b.WriteString("+")
		}
		b.WriteString("d")
		b.WriteString(strconv.Itoa(size))
		b.WriteString(" ")
		for j, face := range faces {
			if j > 0 {
				b.WriteString(",")
			}
			b.WriteString(strconv.Itoa(face))
		}
	}
	if r.Modifier != 0 || len(r.Dice) == 0 {
		if len(r.Dice) > 0 {
			b.WriteString(", ")
		}
		if r.Modifier > 0 && len(r.Dice) > 0 {
			b.WriteString("+")
		}
		b.WriteString(strconv.Itoa(r.Modifier))
	}
	return b.String()
}

// Natural is the face of the first die of the first term, which is what a natural 1 or a natural 20 refers to.
func (r Roll) Natural() int {
	if len(r.Dice) == 0 || len(r.Dice[0]) == 0 {
		return 0
	}
	return r.Dice[0][0]
}

type rollTerm struct {
	count int
	faces int
}

// ParseRoll reads standard dice notation.
//
// Accepted: "d20", "1d20+4", "2d6-1", "1d4+1d6+2", a bare constant "3", and surrounding spaces. Dice are added, never
// subtracted, which is the only thing D&D 5e asks for. Anything unreadable is an error rather than a guess: a number
// the table cannot check is worse than a failed roll.
func ParseRoll(notation string) (terms []rollTerm, modifier int, normalised string, err error) {
	raw := strings.TrimSpace(notation)
	if raw == "" {
		return nil, 0, "", fmt.Errorf("empty roll notation")
	}
	compact := strings.ReplaceAll(strings.ReplaceAll(raw, " ", ""), "−", "-")
	compact = strings.ToLower(compact)

	i := 0
	sign := 1
	if compact[0] == '+' || compact[0] == '-' {
		if compact[0] == '-' {
			sign = -1
		}
		i = 1
	}
	for i < len(compact) {
		start := i
		for i < len(compact) && compact[i] != '+' && compact[i] != '-' {
			i++
		}
		token := compact[start:i]
		if token == "" {
			return nil, 0, "", fmt.Errorf("malformed roll notation %q", notation)
		}
		count, faces, isDice, perr := parseTerm(token)
		if perr != nil {
			return nil, 0, "", fmt.Errorf("%s in %q", perr, notation)
		}
		if isDice {
			if sign < 0 {
				return nil, 0, "", fmt.Errorf("subtracting dice in %q is not supported", notation)
			}
			terms = append(terms, rollTerm{count: count, faces: faces})
		} else {
			modifier += sign * count
		}
		if i < len(compact) {
			if compact[i] == '-' {
				sign = -1
			} else {
				sign = 1
			}
			i++
			if i >= len(compact) {
				return nil, 0, "", fmt.Errorf("roll %q ends on a sign", notation)
			}
		}
	}
	if len(terms) == 0 && modifier == 0 {
		return nil, 0, "", fmt.Errorf("roll %q has no dice and no modifier", notation)
	}
	return terms, modifier, compact, nil
}

func parseTerm(token string) (count, faces int, isDice bool, err error) {
	lower := strings.ToLower(token)
	d := strings.Index(lower, "d")
	if d < 0 {
		n, e := strconv.Atoi(lower)
		if e != nil {
			return 0, 0, false, fmt.Errorf("unreadable term %q", token)
		}
		return n, 0, false, nil
	}
	countText, facesText := lower[:d], lower[d+1:]
	count = 1
	if countText != "" {
		n, e := strconv.Atoi(countText)
		if e != nil {
			return 0, 0, false, fmt.Errorf("unreadable dice count in %q", token)
		}
		count = n
	}
	f, e := strconv.Atoi(facesText)
	if e != nil || f < 1 {
		return 0, 0, false, fmt.Errorf("unreadable die size in %q", token)
	}
	if count < 1 || count > 100 {
		return 0, 0, false, fmt.Errorf("dice count out of range in %q", token)
	}
	if f > 1000 {
		return 0, 0, false, fmt.Errorf("die size out of range in %q", token)
	}
	return count, f, true, nil
}

// RollNotation rolls dice notation with the given source of randomness.
func RollNotation(notation string, rng *rand.Rand) (Roll, error) {
	terms, modifier, normalised, err := ParseRoll(notation)
	if err != nil {
		return Roll{}, err
	}
	roll := Roll{Notation: normalised, Modifier: modifier}
	for _, term := range terms {
		faces := make([]int, term.count)
		for i := range faces {
			faces[i] = rng.Intn(term.faces) + 1
			roll.Total += faces[i]
		}
		roll.Dice = append(roll.Dice, faces)
		roll.sizes = append(roll.sizes, term.faces)
	}
	roll.Total += modifier
	return roll, nil
}

// D20 rolls one twenty-sided die plus a modifier.
func D20(modifier int, rng *rand.Rand) Roll {
	notation := "1d20"
	if modifier > 0 {
		notation += "+" + strconv.Itoa(modifier)
	} else if modifier < 0 {
		notation += strconv.Itoa(modifier)
	}
	roll, err := RollNotation(notation, rng)
	if err != nil {
		return Roll{Notation: notation, Total: modifier}
	}
	return roll
}
