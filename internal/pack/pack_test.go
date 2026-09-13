package pack

import (
	"testing"
	"time"
)

func marks(s ...Mark) []Mark { return s }

func TestEvaluate(t *testing.T) {
	cases := []struct {
		name, guess, answer string
		want                []Mark
	}{
		{"exact", "crane", "crane", marks(Correct, Correct, Correct, Correct, Correct)},
		{"nothing", "crane", "sulky", marks(Absent, Absent, Absent, Absent, Absent)},
		{
			// Two r's in the guess, two in the answer, one of them placed.
			"array vs radar", "array", "radar",
			marks(Present, Present, Present, Correct, Absent),
		},
		{
			// A second copy of a letter must not be credited when the answer
			// only holds one.
			"аллея vs алмаз", "аллея", "алмаз",
			marks(Correct, Correct, Absent, Absent, Absent),
		},
		{
			"касса vs сосна", "касса", "сосна",
			marks(Absent, Absent, Correct, Present, Correct),
		},
		{
			// Exact matches claim their letter before misplaced ones do.
			"sheer vs herbs", "sheer", "herbs",
			marks(Present, Present, Present, Absent, Present),
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Evaluate(c.guess, c.answer)
			if len(got) != len(c.want) {
				t.Fatalf("got %v, want %v", got, c.want)
			}
			for i := range got {
				if got[i] != c.want[i] {
					t.Fatalf("got %v, want %v", got, c.want)
				}
			}
		})
	}
}

func TestEvaluateRejectsLengthMismatch(t *testing.T) {
	if got := Evaluate("abc", "abcd"); got != nil {
		t.Fatalf("expected nil for mismatched lengths, got %v", got)
	}
}

func TestHardModeViolation(t *testing.T) {
	// slate vs crane -> _ _ a _ e  (a and e are fixed in place)
	prev := []string{"slate"}
	cases := []struct {
		guess string
		want  bool
	}{
		{"crane", false}, // keeps a at 3 and e at 5
		{"plate", false}, // same fixed letters
		{"stale", false}, // different word, same two letters still in place
		{"boots", true},  // drops both
		{"aisle", true},  // e is in place but a has moved off its confirmed spot
	}
	for _, c := range cases {
		if got := HardModeViolation(prev, c.guess, "crane"); got != c.want {
			t.Errorf("HardModeViolation(%q) = %v, want %v", c.guess, got, c.want)
		}
	}
}

func TestHardModeRequiresPresentLetters(t *testing.T) {
	// crane vs mixer -> the e is present but misplaced; a guess that omits it
	// entirely is a violation even though no letter is locked in place.
	// crane vs mixer -> r and e are both present but misplaced, nothing is
	// locked in place. A guess must still carry both letters somewhere.
	prev := []string{"crane"}
	if !HardModeViolation(prev, "spilt", "mixer") {
		t.Error("expected a violation when known-present letters are dropped")
	}
	if !HardModeViolation(prev, "medic", "mixer") {
		t.Error("expected a violation when only one of the two is reused")
	}
	if HardModeViolation(prev, "rebus", "mixer") {
		t.Error("did not expect a violation when both present letters are reused")
	}
}

func TestDayAndAnswerRotation(t *testing.T) {
	loc := time.UTC
	p := &Pack{Length: 5, Tries: 6, answers: []string{"aaaaa", "bbbbb", "ccccc"}}
	var err error
	if p.epoch, err = time.Parse("2006-01-02", "2026-01-01"); err != nil {
		t.Fatal(err)
	}

	day := p.Day(time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC), loc)
	if day != 0 {
		t.Fatalf("epoch day = %d, want 0", day)
	}
	if day := p.Day(time.Date(2026, 1, 4, 0, 0, 0, 0, time.UTC), loc); day != 3 {
		t.Fatalf("day = %d, want 3", day)
	}

	// The list wraps rather than running out.
	if got, want := p.Answer(3), "aaaaa"; got != want {
		t.Fatalf("Answer(3) = %q, want %q", got, want)
	}
	// Dates before the epoch must not panic or index negatively.
	if got := p.Answer(-1); got != "ccccc" {
		t.Fatalf("Answer(-1) = %q, want %q", got, "ccccc")
	}
}

func TestNextRolloverIsLocalMidnight(t *testing.T) {
	loc, err := time.LoadLocation("Europe/Prague")
	if err != nil {
		t.Skip("tzdata unavailable")
	}
	now := time.Date(2026, 6, 15, 22, 30, 0, 0, loc)
	next := NextRollover(now, loc)
	if next.Hour() != 0 || next.Minute() != 0 || next.Day() != 16 {
		t.Fatalf("next rollover = %s, want 2026-06-16 00:00 local", next)
	}
}

func TestNormalizeFoldsLetters(t *testing.T) {
	p := &Pack{Folds: map[string]string{"ё": "е"}}
	if got := p.Normalize("  ЁЖИКИ "); got != "ежики" {
		t.Fatalf("Normalize = %q, want %q", got, "ежики")
	}
}
