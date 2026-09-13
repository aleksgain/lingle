// Command lingle serves the word game: static client, JSON API, SQLite store.
package main

import (
	"context"
	"errors"
	"io/fs"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	// Embeds the zoneinfo database so LINGLE_TZ and TZ work on any base
	// image, including one with no tzdata package installed.
	_ "time/tzdata"

	"lingle"
	"lingle/internal/pack"
	"lingle/internal/server"
	"lingle/internal/store"
)

var version = "dev" // overridden at build time with -ldflags

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if err := run(logger); err != nil {
		logger.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	addr := env("LINGLE_ADDR", ":8080")
	dbPath := env("LINGLE_DB", "/data/lingle.db")
	defaultLang := env("LINGLE_DEFAULT_LANG", "ru")
	title := env("LINGLE_TITLE", "")
	langs := splitCSV(env("LINGLE_LANGUAGES", ""))
	testMode := truthy(env("LINGLE_TEST_MODE", ""))

	loc := time.Local
	if tz := env("LINGLE_TZ", ""); tz != "" {
		l, err := time.LoadLocation(tz)
		if err != nil {
			return err
		}
		loc = l
	}

	packs, err := pack.Load(lingle.PacksFS, "packs")
	if err != nil {
		return err
	}
	packs = packs.Filter(langs)

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		return err
	}
	db, err := store.Open(dbPath)
	if err != nil {
		return err
	}
	defer func() { _ = db.Close() }()

	staticFS, err := fs.Sub(lingle.WebFS, "web")
	if err != nil {
		return err
	}

	srv := server.New(server.Config{
		Title:        title,
		DefaultLang:  defaultLang,
		Location:     loc,
		TestMode:     testMode,
		StaticFS:     staticFS,
		Packs:        packs,
		Store:        db,
		Logger:       logger,
		StaticMaxAge: time.Hour,
	})

	httpSrv := &http.Server{
		Addr:              addr,
		Handler:           srv,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       20 * time.Second,
		WriteTimeout:      20 * time.Second,
		IdleTimeout:       90 * time.Second,
	}

	for _, code := range packs.Codes() {
		p, _ := packs.Get(code)
		logger.Info("language pack loaded",
			"lang", code, "answers", p.AnswerCount(),
			"definitions", p.DefinitionCount(),
			"puzzle", p.Day(time.Now(), loc)+1)
	}
	logger.Info("lingle starting",
		"version", version, "addr", addr, "db", dbPath,
		"timezone", loc.String(), "default", defaultLang)

	errCh := make(chan error, 1)
	go func() {
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	select {
	case err := <-errCh:
		return err
	case <-stop:
		logger.Info("shutting down")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpSrv.Shutdown(ctx)
}

func env(key, fallback string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return fallback
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

func truthy(s string) bool {
	b, err := strconv.ParseBool(strings.TrimSpace(s))
	return err == nil && b
}
