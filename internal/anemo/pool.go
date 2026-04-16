package anemo

import (
    "context"
    "fmt"
    "sync"
    "sync/atomic"
    "time"

    "venti/internal/bard"
)

// AnemoPower - сила анемо, управляющая бардами
type AnemoPower struct {
    config      *PowerConfig
    tavern      chan bard.Bard  // таверна, где отдыхают барды
    activeBards int32
    mu          sync.RWMutex
    logger      Logger
    factory     BardFactory
    ctx         context.Context
    cancel      context.CancelFunc
    wg          sync.WaitGroup
}

// PowerConfig - конфигурация силы анемо
type PowerConfig struct {
    MinBards           int           // минимальное количество бардов
    MaxBards           int           // максимальное количество бардов
    IdleTimeout        time.Duration // время бездействия перед уходом на покой
    MaxLifetime        time.Duration // максимальное время жизни барда
    MaxSongsPerBard    int           // максимальное количество песен на барда
}

type BardFactory func() (bard.Bard, error)

type Logger interface {
    Debug(msg string, args ...interface{})
    Info(msg string, args ...interface{})
    Warn(msg string, args ...interface{})
    Error(msg string, args ...interface{})
}

// NewAnemoPower - пробудить силу анемо
func NewAnemoPower(config *PowerConfig, factory BardFactory, logger Logger) (*AnemoPower, error) {
    if config.MinBards > config.MaxBards {
        return nil, fmt.Errorf("min_bards cannot exceed max_bards")
    }

    ctx, cancel := context.WithCancel(context.Background())
    power := &AnemoPower{
        config:  config,
        tavern:  make(chan bard.Bard, config.MaxBards),
        factory: factory,
        logger:  logger,
        ctx:     ctx,
        cancel:  cancel,
    }

    // Призываем минимальное количество бардов
    for i := 0; i < config.MinBards; i++ {
        bard, err := factory()
        if err != nil {
            return nil, fmt.Errorf("failed to summon bard: %w", err)
        }
        power.tavern <- bard
        atomic.AddInt32(&power.activeBards, 1)
    }

    // Запускаем циркуляцию энергии анемо
    power.wg.Add(1)
    go power.flow()

    logger.Info("🌪️ Anemo power awakened",
        "bards_summoned", config.MinBards,
        "max_power", config.MaxBards)

    return power, nil
}

// CallBard - призвать барда из таверны
func (p *AnemoPower) CallBard(ctx context.Context) (bard.Bard, error) {
    select {
    case b := <-p.tavern:
        return b, nil
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}

// ReleaseBard - отпустить барда в таверну
func (p *AnemoPower) ReleaseBard(b bard.Bard) {
    // Проверяем, не пора ли барду на покой
    if !b.IsHealthy() ||
        (p.config.MaxSongsPerBard > 0 && b.GetSongsCount() >= p.config.MaxSongsPerBard) ||
        (p.config.MaxLifetime > 0 && time.Since(b.GetBirthTime()) > p.config.MaxLifetime) {

        p.logger.Warn("🎭 Bard retires from the stage",
            "songs_performed", b.GetSongsCount(),
            "lifetime", time.Since(b.GetBirthTime()),
            "healthy", b.IsHealthy())

        b.Rest()
        atomic.AddInt32(&p.activeBards, -1)

        // Призываем нового барда
        newBard, err := p.factory()
        if err != nil {
            p.logger.Error("Failed to summon new bard", "error", err)
            return
        }

        atomic.AddInt32(&p.activeBards, 1)
        p.tavern <- newBard
        return
    }

    // Возвращаем барда в таверну
    select {
    case p.tavern <- b:
    default:
        // Таверна переполнена - отправляем барда отдыхать
        b.Rest()
        atomic.AddInt32(&p.activeBards, -1)
        p.logger.Debug("🍺 Tavern is full, bard goes to rest",
            "active_bards", atomic.LoadInt32(&p.activeBards))
    }
}

// flow - циркуляция энергии анемо (управление пулом)
func (p *AnemoPower) flow() {
    defer p.wg.Done()

    ticker := time.NewTicker(10 * time.Second)
    defer ticker.Stop()

    for {
        select {
        case <-p.ctx.Done():
            return
        case <-ticker.C:
            p.balance()
            p.cleanupIdle()
        }
    }
}

// balance - балансировка количества бардов
func (p *AnemoPower) balance() {
    currentActive := atomic.LoadInt32(&p.activeBards)
    queueLen := len(p.tavern)

    // Если бардов достаточно, но они все в таверне - не нужно больше
    if queueLen > p.config.MinBards && currentActive > int32(p.config.MinBards) {
        return
    }

    // Нужна подмога - призываем нового барда
    if queueLen == 0 && currentActive < int32(p.config.MaxBards) {
        p.logger.Debug("🎵 Summoning additional bard",
            "current_power", currentActive,
            "max_power", p.config.MaxBards)

        bard, err := p.factory()
        if err != nil {
            p.logger.Error("Failed to summon bard", "error", err)
            return
        }

        atomic.AddInt32(&p.activeBards, 1)
        p.tavern <- bard
    }
}

// cleanupIdle - отправляем бездельничающих бардов на покой
func (p *AnemoPower) cleanupIdle() {
    var bards []bard.Bard

    // Собираем всех бардов из таверны
    for {
        select {
        case b := <-p.tavern:
            bards = append(bards, b)
        default:
            goto DONE
        }
    }

DONE:
    // Отправляем лишних бездельников отдыхать
    kept := 0
    for _, b := range bards {
        idleTime := time.Since(b.GetLastSongTime())

        if idleTime > p.config.IdleTimeout && len(bards)-kept > p.config.MinBards {
            b.Rest()
            atomic.AddInt32(&p.activeBards, -1)
            p.logger.Debug("😴 Idle bard goes to rest",
                "idle_time", idleTime,
                "remaining_bards", atomic.LoadInt32(&p.activeBards))
        } else {
            p.tavern <- b
            kept++
        }
    }
}

// Close - закрыть таверну, отпустить всех бардов
func (p *AnemoPower) Close() error {
    p.cancel()
    p.wg.Wait()

    close(p.tavern)
    for b := range p.tavern {
        b.Rest()
    }

    return nil
}

// GetStats - получить статистику выступлений
func (p *AnemoPower) GetStats() map[string]interface{} {
    return map[string]interface{}{
        "active_bards":  atomic.LoadInt32(&p.activeBards),
        "resting_bards": len(p.tavern),
        "max_bards":     p.config.MaxBards,
        "min_bards":     p.config.MinBards,
    }
}