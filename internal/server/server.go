// Package server exposes the game over HTTP and serves the web client.
//
// The browser never receives the answer or the word list. It sends a guess and
// gets back marks, so scoring, hard mode and the try limit are all enforced
// here rather than trusted from the client.
package server

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"lingle/internal/pack"
	"lingle/internal/store"
)

const (
	cookieName = "lingle_player"
	cookieTTL  = 10 * 365 * 24 * time.Hour
	maxGuess   = 64 // bytes; a sane cap before we touch the string at all
)

type Config struct {
	Title        string
	DefaultLang  string
	Location     *time.Location
	TestMode     bool
	StaticFS     fs.FS
	Packs        *pack.Set
	Store        *store.Store
	Logger       *slog.Logger
	StaticMaxAge time.Duration
}

type Server struct {
	cfg Config
	mux *http.ServeMux
}

func New(cfg Config) *Server {
	s := &Server{cfg: cfg, mux: http.NewServeMux()}

	s.mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok\n"))
	})
	s.mux.HandleFunc("GET /api/config", s.handleConfig)
	s.mux.HandleFunc("POST /api/game", s.handleGame)
	s.mux.HandleFunc("POST /api/guess", s.handleGuess)
	s.mux.HandleFunc("GET /api/stats", s.handleStats)
	if cfg.TestMode {
		// Enabled only by LINGLE_TEST_MODE, for the end-to-end suite.
		s.mux.HandleFunc("GET /api/_test/answer", s.handleTestAnswer)
		cfg.Logger.Warn("test mode is on: /api/_test/answer will reveal today's word")
	}
	s.mux.Handle("/", s.static())

	return s
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("Content-Security-Policy",
		"default-src 'self'; base-uri 'none'; frame-ancestors 'none'; object-src 'none'")
	s.mux.ServeHTTP(w, r)
}

/* ------------------------------ identity ------------------------------ */

// playerID reads the player's cookie, minting one on first visit. The id is
// opaque and random; it identifies a browser, not a person.
func (s *Server) playerID(w http.ResponseWriter, r *http.Request) (string, error) {
	if c, err := r.Cookie(cookieName); err == nil && validID(c.Value) {
		return c.Value, s.cfg.Store.TouchPlayer(c.Value)
	}

	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	id := hex.EncodeToString(buf)

	http.SetCookie(w, &http.Cookie{
		Name:     cookieName,
		Value:    id,
		Path:     "/",
		Expires:  time.Now().Add(cookieTTL),
		MaxAge:   int(cookieTTL / time.Second),
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		Secure:   isTLS(r),
	})
	return id, s.cfg.Store.TouchPlayer(id)
}

func validID(v string) bool {
	if len(v) != 32 {
		return false
	}
	_, err := hex.DecodeString(v)
	return err == nil
}

// isTLS reports whether the browser reached us over https, directly or via a
// reverse proxy. The Secure cookie flag would break plain-http LAN access.
func isTLS(r *http.Request) bool {
	if r.TLS != nil {
		return true
	}
	return strings.EqualFold(r.Header.Get("X-Forwarded-Proto"), "https")
}

/* ------------------------------ handlers ------------------------------ */

type configResponse struct {
	Title       *string     `json:"title"`
	DefaultLang string      `json:"defaultLang"`
	Languages   []pack.Meta `json:"languages"`
}

func (s *Server) handleConfig(w http.ResponseWriter, r *http.Request) {
	var title *string
	if s.cfg.Title != "" {
		t := s.cfg.Title
		title = &t
	}
	def := s.cfg.DefaultLang
	if _, ok := s.cfg.Packs.Get(def); !ok {
		def = s.cfg.Packs.Codes()[0]
	}
	writeJSON(w, http.StatusOK, configResponse{
		Title:       title,
		DefaultLang: def,
		Languages:   s.cfg.Packs.Metas(),
	})
}

type gameRequest struct {
	Lang     string `json:"lang"`
	HardMode *bool  `json:"hardMode"`
}

type rowView struct {
	Guess string      `json:"guess"`
	Marks []pack.Mark `json:"marks"`
}

type gameResponse struct {
	Lang         string           `json:"lang"`
	Day          int              `json:"day"`
	Puzzle       int              `json:"puzzle"`
	Length       int              `json:"length"`
	Tries        int              `json:"tries"`
	Rows         []rowView        `json:"rows"`
	Status       string           `json:"status"`
	HardMode     bool             `json:"hardMode"`
	Answer       string           `json:"answer,omitempty"`
	Definition   *pack.Definition `json:"definition,omitempty"`
	NextRollover int64            `json:"nextRollover"`
}

func (s *Server) handleGame(w http.ResponseWriter, r *http.Request) {
	var req gameRequest
	if !decode(w, r, &req) {
		return
	}
	p, ok := s.cfg.Packs.Get(req.Lang)
	if !ok {
		writeErr(w, http.StatusBadRequest, "unknown_language")
		return
	}
	id, err := s.playerID(w, r)
	if err != nil {
		s.fail(w, "identify player", err)
		return
	}

	day := p.Day(time.Now(), s.cfg.Location)
	game, err := s.cfg.Store.Game(id, p.Code, day, false)
	if err != nil {
		s.fail(w, "load game", err)
		return
	}

	if req.HardMode != nil && *req.HardMode != game.HardMode {
		switch err := s.cfg.Store.SetHardMode(id, p.Code, day, *req.HardMode); {
		case errors.Is(err, store.ErrGameStarted):
			// Silently keep the current setting; the client shows its own toast.
		case err != nil:
			s.fail(w, "set hard mode", err)
			return
		default:
			game.HardMode = *req.HardMode
		}
	}

	writeJSON(w, http.StatusOK, s.view(p, day, game))
}

type guessRequest struct {
	Lang  string `json:"lang"`
	Guess string `json:"guess"`
}

type guessResponse struct {
	Accepted bool          `json:"accepted"`
	Reason   string        `json:"reason,omitempty"`
	Game     *gameResponse `json:"game"`
}

func (s *Server) handleGuess(w http.ResponseWriter, r *http.Request) {
	var req guessRequest
	if !decode(w, r, &req) {
		return
	}
	p, ok := s.cfg.Packs.Get(req.Lang)
	if !ok {
		writeErr(w, http.StatusBadRequest, "unknown_language")
		return
	}
	id, err := s.playerID(w, r)
	if err != nil {
		s.fail(w, "identify player", err)
		return
	}

	day := p.Day(time.Now(), s.cfg.Location)
	game, err := s.cfg.Store.Game(id, p.Code, day, false)
	if err != nil {
		s.fail(w, "load game", err)
		return
	}

	reject := func(reason string) {
		writeJSON(w, http.StatusOK, guessResponse{
			Accepted: false, Reason: reason, Game: s.view(p, day, game),
		})
	}

	if game.Status != "playing" || len(game.Guesses) >= p.Tries {
		reject("finished")
		return
	}
	if len(req.Guess) > maxGuess {
		reject("bad_length")
		return
	}
	guess := p.Normalize(req.Guess)
	if p.RuneLen(guess) != p.Length {
		reject("bad_length")
		return
	}
	if !p.IsWord(guess) {
		reject("not_in_list")
		return
	}

	answer := p.Answer(day)
	if game.HardMode && pack.HardModeViolation(game.Guesses, guess, answer) {
		reject("hard_mode")
		return
	}

	status := "playing"
	switch {
	case guess == answer:
		status = "won"
	case len(game.Guesses)+1 >= p.Tries:
		status = "lost"
	}
	if err := s.cfg.Store.AppendGuess(id, p.Code, day, guess, status); err != nil {
		s.fail(w, "save guess", err)
		return
	}

	game.Guesses = append(game.Guesses, guess)
	game.Status = status
	writeJSON(w, http.StatusOK, guessResponse{Accepted: true, Game: s.view(p, day, game)})
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	p, ok := s.cfg.Packs.Get(r.URL.Query().Get("lang"))
	if !ok {
		writeErr(w, http.StatusBadRequest, "unknown_language")
		return
	}
	id, err := s.playerID(w, r)
	if err != nil {
		s.fail(w, "identify player", err)
		return
	}
	st, err := s.cfg.Store.Stats(id, p.Code, p.Tries)
	if err != nil {
		s.fail(w, "load stats", err)
		return
	}
	writeJSON(w, http.StatusOK, st)
}

func (s *Server) handleTestAnswer(w http.ResponseWriter, r *http.Request) {
	p, ok := s.cfg.Packs.Get(r.URL.Query().Get("lang"))
	if !ok {
		writeErr(w, http.StatusBadRequest, "unknown_language")
		return
	}
	day := p.Day(time.Now(), s.cfg.Location)
	writeJSON(w, http.StatusOK, map[string]any{"lang": p.Code, "day": day, "answer": p.Answer(day)})
}

/* -------------------------------- views ------------------------------- */

// view renders a game for the client. The answer is included only once the
// game is over.
func (s *Server) view(p *pack.Pack, day int, g *store.Game) *gameResponse {
	rows := make([]rowView, 0, len(g.Guesses))
	answer := p.Answer(day)
	for _, guess := range g.Guesses {
		rows = append(rows, rowView{Guess: guess, Marks: pack.Evaluate(guess, answer)})
	}
	out := &gameResponse{
		Lang: p.Code, Day: day, Puzzle: day + 1,
		Length: p.Length, Tries: p.Tries,
		Rows: rows, Status: g.Status, HardMode: g.HardMode,
		NextRollover: pack.NextRollover(time.Now(), s.cfg.Location).UnixMilli(),
	}
	// The answer and its dictionary entry are withheld until the game is
	// over; releasing either early would hand the player the solution.
	if g.Status != "playing" {
		out.Answer = answer
		out.Definition = p.Definition(answer)
	}
	return out
}

/* -------------------------------- static ------------------------------ */

func (s *Server) static() http.Handler {
	fileServer := http.FileServer(http.FS(s.cfg.StaticFS))
	maxAge := s.cfg.StaticMaxAge
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/api/") {
			writeErr(w, http.StatusNotFound, "not_found")
			return
		}
		if maxAge > 0 && r.URL.Path != "/" && r.URL.Path != "/index.html" {
			w.Header().Set("Cache-Control", "public, max-age="+itoa(int(maxAge.Seconds())))
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		fileServer.ServeHTTP(w, r)
	})
}

/* -------------------------------- helpers ----------------------------- */

func decode(w http.ResponseWriter, r *http.Request, dst any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 4<<10)
	if err := json.NewDecoder(r.Body).Decode(dst); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request")
		return false
	}
	return true
}

func writeJSON(w http.ResponseWriter, code int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(body)
}

func writeErr(w http.ResponseWriter, code int, reason string) {
	writeJSON(w, code, map[string]string{"error": reason})
}

func (s *Server) fail(w http.ResponseWriter, what string, err error) {
	s.cfg.Logger.Error("request failed", "op", what, "err", err)
	writeErr(w, http.StatusInternalServerError, "internal_error")
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var b [20]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	return string(b[i:])
}
