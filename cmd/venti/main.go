package main

import (
    "flag"
    "fmt"
    "io"
    "log"
    "log/slog"
    "os"
    "os/signal"
    "strings"
    "syscall"
    "time"

    "venti/internal/anemo"
    "venti/internal/bard"
    "venti/internal/config"
    "venti/internal/lyre"
)

var Version = "1.0.0"
var BuildTime = "unknown"

func printBanner() {
    banner := `
    ╔═══════════════════════════════════════════════════════╗
    ║                                                       ║
    ║    ██╗   ██╗███████╗███╗   ██╗████████╗██╗            ║
    ║    ██║   ██║██╔════╝████╗  ██║╚══██╔══╝██║            ║
    ║    ██║   ██║█████╗  ██╔██╗ ██║   ██║   ██║            ║
    ║    ╚██╗ ██╔╝██╔══╝  ██║╚██╗██║   ██║   ██║            ║
    ║     ╚████╔╝ ███████╗██║ ╚████║   ██║   ██║            ║
    ║      ╚═══╝  ╚══════╝╚═╝  ╚═══╝   ╚═╝   ╚═╝            ║
    ║                                                       ║
    ║         Anemo Archon · FastCGI Pool                   ║
    ║     "May the wind guide your Perl scripts"            ║
    ║                                                       ║
    ╚═══════════════════════════════════════════════════════╝
    `
    fmt.Print(banner)
}

func main() {
    var (
        configPath  string
        showVersion bool
        showHelp    bool
    )

    flag.StringVar(&configPath, "config", "", "Path to configuration file")
    flag.BoolVar(&showVersion, "version", false, "Show version information")
    flag.BoolVar(&showHelp, "help", false, "Show this help message")
    flag.Parse()

    if showVersion {
        fmt.Printf("Venti %s (built at %s)\n", Version, BuildTime)
        fmt.Println("🌬️ The Windborne Bard stands ready to perform")
        os.Exit(0)
    }

    if showHelp {
        printHelp()
        os.Exit(0)
    }

    // Поиск конфига
    if configPath == "" {
        configPath = "/etc/venti/venti.yaml"
        if _, err := os.Stat(configPath); os.IsNotExist(err) {
            altPaths := []string{"./venti.yaml", "./configs/venti.yaml", "/usr/local/etc/venti/venti.yaml"}
            for _, alt := range altPaths {
                if _, err := os.Stat(alt); err == nil {
                    configPath = alt
                    break
                }
            }
        }
    }

    if _, err := os.Stat(configPath); os.IsNotExist(err) {
        log.Fatalf("💔 Configuration not found: %s\n\nHave you summoned Venti with --config ?", configPath)
    }

    printBanner()

    cfg, err := config.LoadConfig(configPath)
    if err != nil {
        log.Fatalf("💔 Failed to read the sacred texts: %v", err)
    }

    logger := setupLogging(cfg)

    logger.Info("🎶 Venti awakens from his slumber...",
        "version", Version,
        "config", configPath)

    // Создаем труппу бардов
    troupe := &bard.Troupe{
        PerlPath: cfg.PerlPath,
    }

    // Пробуждаем силу анемо
    anemoConfig := &anemo.PowerConfig{
        MinBards:        cfg.AnemoPower.MinBards,
        MaxBards:        cfg.AnemoPower.MaxBards,
        IdleTimeout:     cfg.GetIdleTimeout(),
        MaxLifetime:     cfg.GetMaxLifetime(),
        MaxSongsPerBard: cfg.Limits.MaxSongsPerBard,
    }

    bardFactory := func() (bard.Bard, error) {
        return troupe.Recruit()
    }

    anemoPower, err := anemo.NewAnemoPower(anemoConfig, bardFactory, logger)
    if err != nil {
        logger.Error("💔 Failed to awaken Anemo power", "error", err)
        os.Exit(1)
    }
    defer anemoPower.Close()

    // Статистика для отладки
    if cfg.Logging.Level == "debug" {
        go func() {
            ticker := time.NewTicker(30 * time.Second)
            defer ticker.Stop()
            for range ticker.C {
                stats := anemoPower.GetStats()
                logger.Debug("🎭 Current performance stats",
                    "bards_on_stage", stats["active_bards"],
                    "bards_resting", stats["resting_bards"])
            }
        }()
    }

    stage := bard.NewStage(anemoPower)
    lyreServer := lyre.NewLyre(cfg, stage, logger)

    // Слушаем сиглы (зов путешественников)
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

    go func() {
        for sig := range sigChan {
            switch sig {
            case syscall.SIGHUP:
                logger.Info("🔄 Traveler requests new setlist, reloading configuration...")
                newCfg, err := config.LoadConfig(configPath)
                if err != nil {
                    logger.Error("Failed to reload the sacred texts", "error", err)
                } else {
                    cfg = newCfg
                    logger.Info("📜 New setlist learned!")
                }
            case syscall.SIGINT, syscall.SIGTERM:
                logger.Info("🎵 The performance ends... Venti returns to Mondstadt...")
                logger.Info("🌬️ Farewell, dear travelers! May the wind guide your paths!")
                lyreServer.Silence()
                time.Sleep(2 * time.Second)
                os.Exit(0)
            }
        }
    }()

    logger.Info("🌪️ Venti takes a deep breath...",
        "listening_on", cfg.Listener.Address,
        "bards_ready", fmt.Sprintf("%d/%d", cfg.AnemoPower.MinBards, cfg.AnemoPower.MaxBards))

    logger.Info("🎤 The stage is set! Waiting for the audience to arrive...")

    if err := lyreServer.Play(); err != nil {
        logger.Error("💔 The Skyward Lyre broke", "error", err)
        os.Exit(1)
    }
}

func setupLogging(cfg *config.Config) *slog.Logger {
    logOutput := io.Writer(os.Stdout)
    if cfg.Logging.File != "" {
        file, err := os.OpenFile(cfg.Logging.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
        if err != nil {
            log.Printf("Warning: cannot write to %s, using stdout", cfg.Logging.File)
        } else {
            logOutput = file
        }
    }

    var level slog.Level
    switch strings.ToLower(cfg.Logging.Level) {
    case "debug":
        level = slog.LevelDebug
    case "warn", "warning":
        level = slog.LevelWarn
    case "error":
        level = slog.LevelError
    default:
        level = slog.LevelInfo
    }

    opts := &slog.HandlerOptions{Level: level}

    var handler slog.Handler
    if strings.ToLower(cfg.Logging.Format) == "json" {
        handler = slog.NewJSONHandler(logOutput, opts)
    } else {
        handler = slog.NewTextHandler(logOutput, opts)
    }

    return slog.New(handler)
}

func printHelp() {
    fmt.Println(`
🌬️ Venti - The Windborne Bard's FastCGI Pool

Usage:
  venti [options]

Options:
  --config <path>     Path to the sacred texts (config file)
                      Default: /etc/venti/venti.yaml

  --version           Hear the bard's song (show version)

  --help              Show this melody (help)

Examples:
  venti                                    # Start with default config
  venti --config ./my-melody.yaml          # Start with custom config
  venti --version                          # Feel the wind's wisdom

The wind will guide your Perl scripts to the stage! 🎵
`)
}