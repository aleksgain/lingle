// Package pack loads the language packs and holds the game rules that depend
// on them: scoring a guess, enforcing hard mode, and working out which puzzle
// belongs to a given day.
package pack

import (
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
	"strings"
	"time"
)

// Mark is the result for a single tile.
type Mark string

const (
	Correct Mark = "correct"
	Present Mark = "present"
	Absent  Mark = "absent"
)

// Definition is the dictionary entry shown once a puzzle is over. It is
// compiled into the pack at build time, so no lookup leaves the container.
type Definition struct {
	Gloss    string `json:"gloss"`
	POS      string `json:"pos,omitempty"`
	Accented string `json:"accented,omitempty"`
	Note     string `json:"note,omitempty"`
}

// Pack is one language. The word lists are stored as flat strings of
// fixed-width words; Load splits them on the way in.
type Pack struct {
	Code        string            `json:"code"`
	Name        string            `json:"name"`
	EnglishName string            `json:"englishName"`
	Flag        string            `json:"flag"`
	Dir         string            `json:"dir"`
	Length      int               `json:"length"`
	Tries       int               `json:"tries"`
	Epoch       string            `json:"epoch"`
	Folds       map[string]string `json:"normalize"`
	Keyboard    [][]string        `json:"keyboard"`
	Strings     json.RawMessage   `json:"strings"`

	Definitions map[string]Definition `json:"definitions"`

	RawAnswers string `json:"answers"`
	RawGuesses string `json:"guesses"`

	answers []string
	guesses map[string]struct{}
	epoch   time.Time
}

// Meta is the subset of a pack the browser is allowed to see. The word lists
// are deliberately absent: the whole point of the server is that the client
// never holds the answer.
type Meta struct {
	Code        string            `json:"code"`
	Name        string            `json:"name"`
	EnglishName string            `json:"englishName"`
	Flag        string            `json:"flag"`
	Dir         string            `json:"dir"`
	Length      int               `json:"length"`
	Tries       int               `json:"tries"`
	Normalize   map[string]string `json:"normalize"`
	Keyboard    [][]string        `json:"keyboard"`
	Strings     json.RawMessage   `json:"strings"`
}

func (p *Pack) Meta() Meta {
	return Meta{
		Code: p.Code, Name: p.Name, EnglishName: p.EnglishName, Flag: p.Flag,
		Dir: p.Dir, Length: p.Length, Tries: p.Tries,
		Normalize: p.Folds, Keyboard: p.Keyboard, Strings: p.Strings,
	}
}

// Set is every pack the server knows about, in catalogue order.
type Set struct {
	byCode map[string]*Pack
	order  []string
}

type indexFile struct {
	Languages []struct {
		Code string `json:"code"`
	} `json:"languages"`
}

// Load reads packs/index.json and every pack it names out of fsys.
func Load(fsys fs.FS, dir string) (*Set, error) {
	raw, err := fs.ReadFile(fsys, path.Join(dir, "index.json"))
	if err != nil {
		return nil, fmt.Errorf("reading pack index: %w", err)
	}
	var idx indexFile
	if err := json.Unmarshal(raw, &idx); err != nil {
		return nil, fmt.Errorf("parsing pack index: %w", err)
	}

	set := &Set{byCode: make(map[string]*Pack)}
	for _, entry := range idx.Languages {
		p, err := loadOne(fsys, path.Join(dir, entry.Code+".json"))
		if err != nil {
			return nil, err
		}
		set.byCode[p.Code] = p
		set.order = append(set.order, p.Code)
	}
	if len(set.order) == 0 {
		return nil, fmt.Errorf("no language packs found in %s", dir)
	}
	return set, nil
}

func loadOne(fsys fs.FS, name string) (*Pack, error) {
	raw, err := fs.ReadFile(fsys, name)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", name, err)
	}
	var p Pack
	if err := json.Unmarshal(raw, &p); err != nil {
		return nil, fmt.Errorf("parsing %s: %w", name, err)
	}
	if p.Length <= 0 || p.Tries <= 0 {
		return nil, fmt.Errorf("%s: length and tries must be positive", name)
	}
	if p.epoch, err = time.Parse("2006-01-02", p.Epoch); err != nil {
		return nil, fmt.Errorf("%s: bad epoch: %w", name, err)
	}

	p.answers = chunk(p.RawAnswers, p.Length)
	if len(p.answers) == 0 {
		return nil, fmt.Errorf("%s: answer list is empty", name)
	}
	p.guesses = make(map[string]struct{}, len(p.RawGuesses)/p.Length)
	for _, w := range chunk(p.RawGuesses, p.Length) {
		p.guesses[w] = struct{}{}
	}
	for _, a := range p.answers {
		if _, ok := p.guesses[a]; !ok {
			return nil, fmt.Errorf("%s: answer %q is not an accepted guess", name, a)
		}
	}
	// The raw strings are only needed to build the two lookups above.
	p.RawAnswers, p.RawGuesses = "", ""
	return &p, nil
}

func chunk(s string, width int) []string {
	runes := []rune(s)
	if width <= 0 || len(runes)%width != 0 {
		return nil
	}
	out := make([]string, 0, len(runes)/width)
	for i := 0; i < len(runes); i += width {
		out = append(out, string(runes[i:i+width]))
	}
	return out
}

func (s *Set) Get(code string) (*Pack, bool) { p, ok := s.byCode[code]; return p, ok }
func (s *Set) Codes() []string               { return append([]string(nil), s.order...) }

// Filter narrows the set to the given codes, keeping catalogue order. An empty
// allow-list means "everything".
func (s *Set) Filter(allow []string) *Set {
	if len(allow) == 0 {
		return s
	}
	keep := make(map[string]bool, len(allow))
	for _, c := range allow {
		keep[strings.TrimSpace(c)] = true
	}
	out := &Set{byCode: make(map[string]*Pack)}
	for _, code := range s.order {
		if keep[code] {
			out.byCode[code] = s.byCode[code]
			out.order = append(out.order, code)
		}
	}
	if len(out.order) == 0 {
		return s
	}
	return out
}

func (s *Set) Metas() []Meta {
	out := make([]Meta, 0, len(s.order))
	for _, code := range s.order {
		out = append(out, s.byCode[code].Meta())
	}
	return out
}

// Normalize folds the letters a language treats as equivalent (Russian ё onto
// е) and lowercases. Every comparison in the game runs on normalized text.
func (p *Pack) Normalize(word string) string {
	out := strings.ToLower(strings.TrimSpace(word))
	for from, to := range p.Folds {
		out = strings.ReplaceAll(out, from, to)
	}
	return out
}

func (p *Pack) IsWord(word string) bool {
	_, ok := p.guesses[word]
	return ok
}

func (p *Pack) RuneLen(word string) int { return len([]rune(word)) }

// Day is the puzzle number for the given instant, counted from the pack epoch
// in loc. Days roll over at local midnight.
func (p *Pack) Day(now time.Time, loc *time.Location) int {
	epoch := time.Date(p.epoch.Year(), p.epoch.Month(), p.epoch.Day(), 0, 0, 0, 0, loc)
	local := now.In(loc)
	today := time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, loc)
	return int(today.Sub(epoch).Hours() / 24)
}

// Answer is the word for a given day. The pack's answer list is already in a
// fixed shuffled order, so consecutive days are unrelated and no word repeats
// until the list is exhausted.
func (p *Pack) Answer(day int) string {
	n := len(p.answers)
	i := ((day % n) + n) % n
	return p.answers[i]
}

func (p *Pack) AnswerCount() int { return len(p.answers) }

// DefinitionCount reports how many answers have a dictionary entry.
func (p *Pack) DefinitionCount() int { return len(p.Definitions) }

// Definition returns the dictionary entry for a word, if the pack has one.
func (p *Pack) Definition(word string) *Definition {
	if d, ok := p.Definitions[word]; ok {
		return &d
	}
	return nil
}

// NextRollover is the next local midnight after now.
func NextRollover(now time.Time, loc *time.Location) time.Time {
	local := now.In(loc)
	return time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, loc)
}

// Evaluate scores a guess against the answer. Duplicate letters are handled
// the way the original game does: exact matches are taken first, then each
// remaining letter of the answer can satisfy at most one misplaced tile.
func Evaluate(guess, answer string) []Mark {
	g, a := []rune(guess), []rune(answer)
	if len(g) != len(a) {
		return nil
	}
	marks := make([]Mark, len(g))
	pool := make(map[rune]int, len(a))
	for i := range g {
		if g[i] == a[i] {
			marks[i] = Correct
		} else {
			marks[i] = Absent
			pool[a[i]]++
		}
	}
	for i := range g {
		if marks[i] == Correct {
			continue
		}
		if pool[g[i]] > 0 {
			marks[i] = Present
			pool[g[i]]--
		}
	}
	return marks
}

// HardModeViolation reports whether guess fails to reuse a hint that an
// earlier guess already revealed.
func HardModeViolation(previous []string, guess, answer string) bool {
	g := []rune(guess)
	for _, prev := range previous {
		p := []rune(prev)
		marks := Evaluate(prev, answer)
		required := make(map[rune]int, len(p))
		for i := range p {
			if marks[i] == Correct {
				if i >= len(g) || g[i] != p[i] {
					return true
				}
			}
			if marks[i] != Absent {
				required[p[i]]++
			}
		}
		for r, want := range required {
			got := 0
			for _, c := range g {
				if c == r {
					got++
				}
			}
			if got < want {
				return true
			}
		}
	}
	return false
}
