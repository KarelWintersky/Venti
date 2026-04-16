package main

import (
    "flag"
    "fmt"
    "log"
    "os"
    "os/signal"
    "syscall"
    "time"

    "venti/internal/anemo"
    "venti/internal/bard"
    "venti/internal/config"
    "venti/internal/lyre"
)

// Version информация о версии (заполняется при сборке)
var Version = "1.0.0"
var BuildTime = "unknown"

type SimpleLogger struct {
    logger *log.Logger
    level  string
}

func (l *SimpleLogger) Info(msg string, args ...interface{}) {
    l.logger.Printf("[INFO] "+msg, args...)
}

func (l *SimpleLogger) Error(msg string, args ...interface{}) {
    l.logger.Printf("[ERROR] "+msg, args...)
}

func (l *SimpleLogger) Debug(msg string, args ...interface{}) {
    if l.level == "debug" {
        l.logger.Printf("[DEBUG] "+msg, args...)
    }
}

func (l *SimpleLogger) Warn(msg string, args ...interface{}) {
    l.logger.Printf("[WARN] "+msg, args...)
}

func setupLogging(cfg *config.Config) *SimpleLogger {
    logOutput := os.Stdout
    if cfg.Logging.File != "" {
        file, err := os.OpenFile(cfg.Logging.File, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
        if err != nil {
            log.Printf("Warning: failed to open log file %s: %v, using stdout", cfg.Logging.File, err)
        } else {
            logOutput = file
        }
    }

    return &SimpleLogger{
        logger: log.New(logOutput, "", log.LstdFlags),
        level:  cfg.Logging.Level,
    }
}

func printBanner() {
    banner := `
    ╔═══════════════════════════════════════════╗
    ║               V E N T I                   ║
    ║     Anemo Archon FastCGI Pool             ║
    ║     "May the wind guide your scripts"     ║
    ╚═══════════════════════════════════════════╝
    `
    fmt.Print(banner)
}

func printUsage() {
    fmt.Printf(`
Usage: venti [options]

Options:
  --config <path>     Path to configuration file (default: /etc/venti/venti.yaml)
  --version           Show version information
  --help              Show this help message

Examples:
  venti                                    # Uses /etc/venti/venti.yaml
  venti --config ./custom-config.yaml      # Uses custom config file
  venti --version                          # Show version

`)
}

func main() {
    // Определяем флаги командной строки
    var (
        configPath string
        showVersion bool
        showHelp    bool
    )

    flag.StringVar(&configPath, "config", "", "Path to configuration file")
    flag.BoolVar(&showVersion, "version", false, "Show version information")
    flag.BoolVar(&showHelp, "help", false, "Show help message")
    flag.Parse()

    // Показываем версию
    if showVersion {
        fmt.Printf("Venti version %s (built at %s)\n", Version, BuildTime)
        fmt.Println("Anemo Archon FastCGI Pool for Perl scripts")
        os.Exit(0)
    }

    // Показываем помощь
    if showHelp {
        printUsage()
        os.Exit(0)
    }

    // Определяем путь к конфигу
    if configPath == "" {
        configPath = "/etc/venti/venti.yaml"
        // Проверяем, существует ли файл
        if _, err := os.Stat(configPath); os.IsNotExist(err) {
            // Пробуем альтернативные пути
            alternativePaths := []string{
                "./venti.yaml",
                "./config/venti.yaml",
                "/usr/local/etc/venti/venti.yaml",
                "/opt/venti/config.yaml",
            }

            for _, altPath := range alternativePaths {
                if _, err := os.Stat(altPath); err == nil {
                    configPath = altPath
                    break
                }
            }
        }
    }

    // Проверяем существование конфига
    if _, err := os.Stat(configPath); os.IsNotExist(err) {
        log.Fatalf("Configuration file not found: %s\n\nPlease create config file or use --config option", configPath)
    }

    // Печатаем баннер
    printBanner()

    // Загружаем конфигурацию
    cfg, err := config.LoadConfig(configPath)
    if err != nil {
        log.Fatalf("Failed to load config from %s: %v", configPath, err)
    }

    // Настраиваем логирование
    logger := setupLogging(cfg)

    logger.Info("🎵 Venti awakens... The wind rises over Teyvat! 🎵")
    logger.Info("Starting Venti",
        "version", Version,
        "config", configPath,
        "build_time", BuildTime)

    // Проверяем права доступа к директориям
    if err := checkDirectories(cfg, logger); err != nil {
        log.Fatalf("Directory check failed: %v", err)
    }

    // Создаем труппу бардов
    troupe := &bard.Troupe{
        PerlPath:   cfg.PerlPath,
        ScriptRoot: cfg.ScriptRoot,
    }

    // Пробуждаем силу анемо
    anemoConfig := &anemo.PowerConfig{
        MinBards:         cfg.AnemoPower.MinBards,
        MaxBards:         cfg.AnemoPower.MaxBards,
        IdleTimeout:      cfg.GetIdleTimeout(),
        MaxLifetime:      cfg.GetMaxLifetime(),
        MaxSongsPerBard:  cfg.Limits.MaxSongsPerBard,
    }

    bardFactory := func() (bard.Bard, error) {
        return troupe.Recruit()
    }

    anemoPower, err := anemo.NewAnemoPower(anemoConfig, bardFactory, logger)
    if err != nil {
        log.Fatalf("Failed to awaken Anemo power: %v", err)
    }
    defer anemoPower.Close()

    // Периодически выводим статистику (если уровень логов debug)
    if cfg.Logging.Level == "debug" {
        go func() {
            ticker := time.NewTicker(30 * time.Second)
            defer ticker.Stop()
            for range ticker.C {
                stats := anemoPower.GetStats()
                logger.Debug("Anemo power stats",
                    "active_bards", stats["active_bards"],
                    "resting_bards", stats["resting_bards"],
                    "max_bards", stats["max_bards"])
            }
        }()
    }

    // Создаем сцену
    stage := bard.NewStage(anemoPower)

    // Настраиваем небесную лиру (FastCGI)
    lyreServer := lyre.NewLyre(cfg, stage, logger)

    // Обработка сигналов
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

    go func() {
        for sig := range sigChan {
            switch sig {
            case syscall.SIGHUP:
                // Перезагрузка конфигурации
                logger.Info("Received SIGHUP, reloading configuration...")
                newCfg, err := config.LoadConfig(configPath)
                if err != nil {
                    logger.Error("Failed to reload config", "error", err)
                    continue
                }
                // Обновляем конфигурацию
                cfg = newCfg
                logger.Info("Configuration reloaded successfully")

            case syscall.SIGINT, syscall.SIGTERM:
                logger.Info("🎵 Venti returns to Mondstadt... Farewell, travelers! 🎵")
                lyreServer.Silence()
                time.Sleep(2 * time.Second)
                os.Exit(0)
            }
        }
    }()

    logger.Info("Venti is ready",
        "listen_type", cfg.Listener.Type,
        "listen_address", cfg.Listener.Address,
        "script_root", cfg.ScriptRoot,
        "min_bards", cfg.AnemoPower.MinBards,
        "max_bards", cfg.AnemoPower.MaxBards)

    if err := lyreServer.Play(); err != nil {
        log.Fatalf("Failed to play the lyre: %v", err)
    }
}

// checkDirectories проверяет существование и права доступа к директориям
func checkDirectories(cfg *config.Config, logger *SimpleLogger) error {
    // Проверяем директорию со скриптами
    if info, err := os.Stat(cfg.ScriptRoot); err != nil {
        if os.IsNotExist(err) {
            logger.Warn("Script root directory does not exist, creating", "path", cfg.ScriptRoot)
            if err := os.MkdirAll(cfg.ScriptRoot, 0755); err != nil {
                return fmt.Errorf("failed to create script root: %w", err)
            }
        } else {
            return fmt.Errorf("failed to stat script root: %w", err)
        }
    } else if !info.IsDir() {
        return fmt.Errorf("script root is not a directory: %s", cfg.ScriptRoot)
    }

    // Проверяем директорию для логов
    if cfg.Logging.File != "" {
        logDir := cfg.Logging.File[:len(cfg.Logging.File)-len("/access.log")]
        if err := os.MkdirAll(logDir, 0755); err != nil {
            logger.Warn("Failed to create log directory", "path", logDir, "error", err)
        }
    }

    // Проверяем директорию для сокета (для Unix sockets)
    if cfg.Listener.Type == "unix" {
        socketDir := cfg.Listener.Address[:len(cfg.Listener.Address)-len("/venti.sock")]
        if err := os.MkdirAll(socketDir, 0755); err != nil {
            return fmt.Errorf("failed to create socket directory: %w", err)
        }
    }

    return nil
}
