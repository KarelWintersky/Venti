package main

import (
    "log"
    "os"
    "os/signal"
    "syscall"

    "venti/internal/anemo"
    "venti/internal/bard"
    "venti/internal/config"
    "venti/internal/lyre"
)

func main() {
    // Загружаем конфигурацию
    cfg, err := config.LoadConfig("/etc/venti/venti.yaml")
    if err != nil {
        log.Fatalf("Failed to load config: %v", err)
    }

    // Настраиваем логирование
    logger := setupLogging(cfg)
    
    logger.Info("🎵 Venti awakens... The wind rises over Teyvat! 🎵")
    logger.Info("Anemo Archon ready to perform", "version", "1.0.0")
    
    // Создаем труппу бардов
    troupe := &bard.Troupe{
        PerlPath:   cfg.PerlPath,
        ScriptRoot: cfg.ScriptRoot,
    }
    
    // Пробуждаем силу анемо
    anemoConfig := &anemo.PowerConfig{
        MinBards:        cfg.AnemoPower.MinBards,
        MaxBards:        cfg.AnemoPower.MaxBards,
        IdleTimeout:     cfg.GetIdleTimeout(),
        MaxLifetime:     cfg.GetMaxLifetime(),
        MaxSongsPerBard: cfg.Limits.MaxSongsPerBard,
    }
    
    bardFactory := func() (anemo.Bard, error) {
        return troupe.Recruit()
    }
    
    anemoPower, err := anemo.NewAnemoPower(anemoConfig, bardFactory, logger)
    if err != nil {
        log.Fatalf("Failed to awaken Anemo power: %v", err)
    }
    defer anemoPower.Close()
    
    // Создаем сцену
    stage := bard.NewStage(anemoPower)
    
    // Настраиваем небесную лиру (FastCGI)
    lyreServer := lyre.NewLyre(cfg, stage, logger)
    
    // Обработка сигналов (время возвращаться в Мондштадт)
    sigChan := make(chan os.Signal, 1)
    signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
    
    go func() {
        <-sigChan
        logger.Info("🎵 Venti returns to Mondstadt... Farewell, travelers! 🎵")
        lyreServer.Silence()
    }()
    
    if err := lyreServer.Play(); err != nil {
        log.Fatalf("Failed to play the lyre: %v", err)
    }
}
