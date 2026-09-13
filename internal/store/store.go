// Package store is the SQLite persistence layer: who is playing, what they
// have guessed today, and their per-language streaks.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	_ "github.com/mattn/go-sqlite3"
)

type Store struct{ db *sql.DB }

// Game is one player's attempt at one day's puzzle in one language.
type Game struct {
	Guesses  []string
	Status   string // playing | won | lost
	HardMode bool
}

// Stats is a player's record in one language.
type Stats struct {
	Played    int   `json:"played"`
	Wins      int   `json:"wins"`
	Streak    int   `json:"streak"`
	MaxStreak int   `json:"maxStreak"`
	LastDay   *int  `json:"-"`
	Dist      []int `json:"dist"`
}

const schema = `
PRAGMA journal_mode = WAL;
PRAGMA foreign_keys = ON;

CREATE TABLE IF NOT EXISTS players (
    id         TEXT PRIMARY KEY,
    created_at INTEGER NOT NULL,
    last_seen  INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS games (
    player_id   TEXT    NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    lang        TEXT    NOT NULL,
    day         INTEGER NOT NULL,
    guesses     TEXT    NOT NULL DEFAULT '',
    status      TEXT    NOT NULL DEFAULT 'playing',
    hard_mode   INTEGER NOT NULL DEFAULT 0,
    started_at  INTEGER NOT NULL,
    finished_at INTEGER,
    PRIMARY KEY (player_id, lang, day)
);

CREATE TABLE IF NOT EXISTS stats (
    player_id  TEXT    NOT NULL REFERENCES players(id) ON DELETE CASCADE,
    lang       TEXT    NOT NULL,
    played     INTEGER NOT NULL DEFAULT 0,
    wins       INTEGER NOT NULL DEFAULT 0,
    streak     INTEGER NOT NULL DEFAULT 0,
    max_streak INTEGER NOT NULL DEFAULT 0,
    last_day   INTEGER,
    dist       TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (player_id, lang)
);

CREATE INDEX IF NOT EXISTS games_by_day ON games(lang, day);
`

func Open(path string) (*Store, error) {
	dsn := path + "?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on"
	db, err := sql.Open("sqlite3", dsn)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}
	// One writer is plenty for a household, and it removes every chance of
	// SQLITE_BUSY without any retry logic.
	db.SetMaxOpenConns(1)
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("connecting to database: %w", err)
	}
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("applying schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

// TouchPlayer records a player id, creating it on first sight.
func (s *Store) TouchPlayer(id string) error {
	now := time.Now().Unix()
	_, err := s.db.Exec(`
        INSERT INTO players (id, created_at, last_seen) VALUES (?, ?, ?)
        ON CONFLICT(id) DO UPDATE SET last_seen = excluded.last_seen`,
		id, now, now)
	return err
}

// Game returns today's game, creating an empty one if the player has not
// started yet.
func (s *Store) Game(playerID, lang string, day int, hardModeDefault bool) (*Game, error) {
	var g Game
	var joined string
	var hard int
	err := s.db.QueryRow(
		`SELECT guesses, status, hard_mode FROM games
         WHERE player_id = ? AND lang = ? AND day = ?`,
		playerID, lang, day).Scan(&joined, &g.Status, &hard)

	switch {
	case errors.Is(err, sql.ErrNoRows):
		if _, err := s.db.Exec(
			`INSERT INTO games (player_id, lang, day, hard_mode, started_at)
             VALUES (?, ?, ?, ?, ?)`,
			playerID, lang, day, boolToInt(hardModeDefault), time.Now().Unix()); err != nil {
			return nil, err
		}
		return &Game{Status: "playing", HardMode: hardModeDefault}, nil
	case err != nil:
		return nil, err
	}

	g.HardMode = hard == 1
	if joined != "" {
		g.Guesses = strings.Split(joined, "\n")
	}
	return &g, nil
}

// SetHardMode changes the hard-mode flag. It refuses once the player has
// guessed, so nobody can switch it on after seeing hints.
var ErrGameStarted = errors.New("game already started")

func (s *Store) SetHardMode(playerID, lang string, day int, on bool) error {
	res, err := s.db.Exec(
		`UPDATE games SET hard_mode = ?
         WHERE player_id = ? AND lang = ? AND day = ? AND guesses = ''`,
		boolToInt(on), playerID, lang, day)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrGameStarted
	}
	return nil
}

// AppendGuess stores a guess and, when the game ends, folds the result into
// the player's statistics. Both happen in one transaction so a crash cannot
// leave a finished game uncounted or counted twice.
func (s *Store) AppendGuess(playerID, lang string, day int, guess, status string) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback() }()

	var joined string
	if err := tx.QueryRow(
		`SELECT guesses FROM games WHERE player_id = ? AND lang = ? AND day = ?`,
		playerID, lang, day).Scan(&joined); err != nil {
		return err
	}
	if joined == "" {
		joined = guess
	} else {
		joined += "\n" + guess
	}

	var finishedAt any
	if status != "playing" {
		finishedAt = time.Now().Unix()
	}
	if _, err := tx.Exec(
		`UPDATE games SET guesses = ?, status = ?, finished_at = ?
         WHERE player_id = ? AND lang = ? AND day = ?`,
		joined, status, finishedAt, playerID, lang, day); err != nil {
		return err
	}

	if status != "playing" {
		tries := len(strings.Split(joined, "\n"))
		if err := recordResult(tx, playerID, lang, day, status == "won", tries); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func recordResult(tx *sql.Tx, playerID, lang string, day int, won bool, tries int) error {
	st, err := readStats(tx.QueryRow(
		`SELECT played, wins, streak, max_streak, last_day, dist FROM stats
         WHERE player_id = ? AND lang = ?`, playerID, lang))
	if err != nil {
		return err
	}
	if st.LastDay != nil && *st.LastDay == day {
		return nil // already counted
	}

	st.Played++
	if won {
		st.Wins++
		if tries >= 1 && tries <= len(st.Dist) {
			st.Dist[tries-1]++
		}
		if st.LastDay != nil && *st.LastDay == day-1 {
			st.Streak++
		} else {
			st.Streak = 1
		}
		if st.Streak > st.MaxStreak {
			st.MaxStreak = st.Streak
		}
	} else {
		st.Streak = 0
	}
	st.LastDay = &day

	_, err = tx.Exec(`
        INSERT INTO stats (player_id, lang, played, wins, streak, max_streak, last_day, dist)
        VALUES (?, ?, ?, ?, ?, ?, ?, ?)
        ON CONFLICT(player_id, lang) DO UPDATE SET
            played = excluded.played, wins = excluded.wins,
            streak = excluded.streak, max_streak = excluded.max_streak,
            last_day = excluded.last_day, dist = excluded.dist`,
		playerID, lang, st.Played, st.Wins, st.Streak, st.MaxStreak, day, encodeDist(st.Dist))
	return err
}

func (s *Store) Stats(playerID, lang string, tries int) (*Stats, error) {
	st, err := readStats(s.db.QueryRow(
		`SELECT played, wins, streak, max_streak, last_day, dist FROM stats
         WHERE player_id = ? AND lang = ?`, playerID, lang))
	if err != nil {
		return nil, err
	}
	for len(st.Dist) < tries {
		st.Dist = append(st.Dist, 0)
	}
	return st, nil
}

type scanner interface{ Scan(dest ...any) error }

func readStats(row scanner) (*Stats, error) {
	st := &Stats{Dist: make([]int, 6)}
	var dist sql.NullString
	var lastDay sql.NullInt64
	err := row.Scan(&st.Played, &st.Wins, &st.Streak, &st.MaxStreak, &lastDay, &dist)
	if errors.Is(err, sql.ErrNoRows) {
		return st, nil
	}
	if err != nil {
		return nil, err
	}
	if lastDay.Valid {
		d := int(lastDay.Int64)
		st.LastDay = &d
	}
	if dist.Valid && dist.String != "" {
		st.Dist = decodeDist(dist.String)
	}
	return st, nil
}

func encodeDist(d []int) string {
	parts := make([]string, len(d))
	for i, n := range d {
		parts[i] = strconv.Itoa(n)
	}
	return strings.Join(parts, ",")
}

func decodeDist(s string) []int {
	parts := strings.Split(s, ",")
	out := make([]int, len(parts))
	for i, p := range parts {
		out[i], _ = strconv.Atoi(p)
	}
	return out
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
