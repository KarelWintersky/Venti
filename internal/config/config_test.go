package config

import (
    "os"
    "path/filepath"
    "testing"
    "time"
)

func writeTempConfig(t *testing.T, content string) string {
    t.Helper()
    dir := t.TempDir()
    path := filepath.Join(dir, "venti.yaml")
    if err := os.WriteFile(path, []byte(content), 0644); err != nil {
        t.Fatalf("write config: %v", err)
    }
    return path
}

func TestLoadConfigDefaults(t *testing.T) {
    path := writeTempConfig(t, `
listener:
  type: unix
  address: /tmp/venti-test.sock
`)
    cfg, err := LoadConfig(path)
    if err != nil {
        t.Fatalf("LoadConfig: %v", err)
    }

    if cfg.PerlPath != "/usr/bin/perl" {
        t.Errorf("PerlPath = %q, want /usr/bin/perl", cfg.PerlPath)
    }
    if cfg.Listener.Mode != 0660 {
        t.Errorf("Listener.Mode = %04o, want 0660", cfg.Listener.Mode)
    }
    if cfg.AnemoPower.MinBards != 1 {
        t.Errorf("MinBards = %d, want 1", cfg.AnemoPower.MinBards)
    }
    if cfg.Timeouts.SongDuration != 30 {
        t.Errorf("SongDuration = %d, want 30", cfg.Timeouts.SongDuration)
    }
    if cfg.Timeouts.TuneUp != 5 {
        t.Errorf("TuneUp = %d, want 5", cfg.Timeouts.TuneUp)
    }
    if cfg.Timeouts.ShutdownGrace != 10 {
        t.Errorf("ShutdownGrace = %d, want 10", cfg.Timeouts.ShutdownGrace)
    }
    if cfg.Limits.MaxSongsPerBard != 1000 {
        t.Errorf("MaxSongsPerBard = %d, want 1000", cfg.Limits.MaxSongsPerBard)
    }
    if cfg.Limits.MaxVerseSize != 10<<20 {
        t.Errorf("MaxVerseSize = %d, want %d", cfg.Limits.MaxVerseSize, 10<<20)
    }
}

func TestLoadConfigExplicitValues(t *testing.T) {
    path := writeTempConfig(t, `
listener:
  type: tcp
  address: 127.0.0.1:9000
  mode: 0770
perl_path: /usr/bin/perl
anemo_power:
  min_bards: 2
  max_bards: 8
timeouts:
  song_duration: 15
  tune_up: 3
  shutdown_grace: 5
limits:
  max_verse_size: 100
  max_songs_per_bard: 42
  document_root: /var/www
`)
    cfg, err := LoadConfig(path)
    if err != nil {
        t.Fatalf("LoadConfig: %v", err)
    }

    if cfg.Listener.Type != "tcp" || cfg.Listener.Address != "127.0.0.1:9000" {
        t.Errorf("listener = %+v", cfg.Listener)
    }
    if cfg.Listener.Mode != 0770 {
        t.Errorf("Listener.Mode = %04o, want 0770", cfg.Listener.Mode)
    }
    if cfg.AnemoPower.MinBards != 2 || cfg.AnemoPower.MaxBards != 8 {
        t.Errorf("bards = %d/%d", cfg.AnemoPower.MinBards, cfg.AnemoPower.MaxBards)
    }
    if cfg.Limits.MaxVerseSize != 100 {
        t.Errorf("MaxVerseSize = %d, want 100", cfg.Limits.MaxVerseSize)
    }
    if cfg.Limits.DocumentRoot != "/var/www" {
        t.Errorf("DocumentRoot = %q", cfg.Limits.DocumentRoot)
    }
    if cfg.GetSongDuration() != 15*time.Second {
        t.Errorf("GetSongDuration = %v", cfg.GetSongDuration())
    }
    if cfg.GetTuneUpTimeout() != 3*time.Second {
        t.Errorf("GetTuneUpTimeout = %v", cfg.GetTuneUpTimeout())
    }
    if cfg.GetShutdownGrace() != 5*time.Second {
        t.Errorf("GetShutdownGrace = %v", cfg.GetShutdownGrace())
    }
}

func TestLoadConfigInvalid(t *testing.T) {
    cases := []struct {
        name    string
        content string
    }{
        {"bad listener type", "listener:\n  type: pipe\n  address: /tmp/x.sock\n"},
        {"empty address", "listener:\n  type: unix\n  address: \"\"\n"},
    }
    for _, tc := range cases {
        t.Run(tc.name, func(t *testing.T) {
            path := writeTempConfig(t, tc.content)
            if _, err := LoadConfig(path); err == nil {
                t.Errorf("expected error for %q", tc.name)
            }
        })
    }
}

func TestLoadConfigClampsMaxBards(t *testing.T) {
    path := writeTempConfig(t, "listener:\n  type: unix\n  address: /tmp/x.sock\nanemo_power:\n  min_bards: 5\n  max_bards: 2\n")
    cfg, err := LoadConfig(path)
    if err != nil {
        t.Fatalf("LoadConfig: %v", err)
    }
    if cfg.AnemoPower.MaxBards != 5 {
        t.Errorf("MaxBards = %d, want clamped to 5", cfg.AnemoPower.MaxBards)
    }
}
