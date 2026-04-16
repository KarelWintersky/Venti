package anemo

import (
    "context"
    "fmt"
    "sync"
    "sync/atomic"
    "time"
)

type Bard interface {
    Sing(ctx context.Context, songPath string, lyrics []string, melody []byte) ([]byte, error)
    Rest() error
    IsHealthy() bool
    GetSongCount() int
    GetBirthTime() time.Time
    GetLastSongTime() time.Time
}

type AnemoPower struct {
    config      *PowerConfig
    bards       chan Bard
    activeCount int32
    mu          sync.RWMutex
    logger      Logger
    factory     BardFactory
    ctx         context.Context
    cancel      context.CancelFunc
    wg          sync.WaitGroup
}

type PowerConfig struct {
    MinBards         int
    MaxBards         int
    IdleTimeout      time.Duration
    MaxLifetime      time.Duration
    MaxSongsPerBard  int
}

type BardFactory func() (Bard, error)

type Logger interface {
    Debug(msg string, args ...interface{})
    Info(msg string, args ...interface{})
    Warn(msg string, args ...interface{})
    Error(msg string, args ...interface{})
}

func NewAnemoPower(config *PowerConfig, factory BardFactory, logger Logger) (*AnemoPower, error) {
    if config.MinBards > config.MaxBards {
        return nil, fmt.Errorf("min_bards cannot exceed max_bards")
    }

    ctx, cancel := context.WithCancel(context.Background())
    power := &AnemoPower{
        config:  config,
        bards:   make(chan Bard, config.MaxBards),
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
        power.bards <- bard
        atomic.AddInt32(&power.activeCount, 1)
    }

    // Запускаем циркуляцию энергии анемо
    power.wg.Add(1)
    go power.flow()

    logger.Info("Anemo power awakened", 
        "bards_summoned", config.MinBards,
        "max_power", config.MaxBards)

    return power, nil
}

func (p *AnemoPower) CallBard(ctx context.Context) (Bard, error) {
    select {
    case bard := <-p.bards:
        return bard, nil
    case <-ctx.Done():
        return nil, ctx.Err()
    }
}

func (p *AnemoPower) ReleaseBard(bard Bard) {
    // Проверяем, не пора ли барду на покой
    if !bard.IsHealthy() ||
        (p.config.MaxSongsPerBard > 0 && bard.GetSongCount() >= p.config.MaxSongsPerBard) ||
        (p.config.MaxLifetime > 0 && time.Since(bard.GetBirthTime()) > p.config.MaxLifetime) {

        p.logger.Warn("Bard retires", 
            "songs_performed", bard.GetSongCount(),
            "lifetime", time.Since(bard.GetBirthTime()),
            "healthy", bard.IsHealthy())

        bard.Rest()
        atomic.AddInt32(&p.activeCount, -1)
        
        // Призываем нового барда
        newBard, err := p.factory()
        if err != nil {
            p.logger.Error("Failed to summon new bard", "error", err)
            return
        }
        
        atomic.AddInt32(&p.activeCount, 1)
        p.bards <- newBard
        return
    }

    // Возвращаем барда в таверну
    select {
    case p.bards <- bard:
    default:
        // Таверна переполнена - отправляем барда отдыхать
        bard.Rest()
        atomic.AddInt32(&p.activeCount, -1)
        p.logger.Debug("Tavern is full, bard goes to rest", 
            "active_bards", atomic.LoadInt32(&p.activeCount))
    }
}

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

func (p *AnemoPower) balance() {
    currentActive := atomic.LoadInt32(&p.activeCount)
    queueLen := len(p.bards)
    
    // Если бардов достаточно, но они все в таверне - не нужно больше
    if queueLen > p.config.MinBards && currentActive > int32(p.config.MinBards) {
        return
    }

    // Нужна подмога
    if queueLen == 0 && currentActive < int32(p.config.MaxBards) {
        p.logger.Debug("Summoning additional bard", 
            "current_power", currentActive, 
            "max_power", p.config.MaxBards)
        
        bard, err := p.factory()
        if err != nil {
            p.logger.Error("Failed to summon bard", "error", err)
            return
        }
        
        atomic.AddInt32(&p.activeCount, 1)
        p.bards <- bard
    }
}

func (p *AnemoPower) cleanupIdle() {
    var bards []Bard
    
    // Собираем всех бардов из таверны
    for {
        select {
        case bard := <-p.bards:
            bards = append(bards, bard)
        default:
            goto DONE
        }
    }
    
DONE:
    // Отправляем лишних бездельничающих бардов отдыхать
    kept := 0
    for _, bard := range bards {
        idleTime := time.Since(bard.GetLastSongTime())
        
        if idleTime > p.config.IdleTimeout && len(bards)-kept > p.config.MinBards {
            bard.Rest()
            atomic.AddInt32(&p.activeCount, -1)
            p.logger.Debug("Idle bard goes to rest", 
                "idle_time", idleTime, 
                "remaining_bards", atomic.LoadInt32(&p.activeCount))
        } else {
            p.bards <- bard
            kept++
        }
    }
}

func (p *AnemoPower) Close() error {
    p.cancel()
    p.wg.Wait()
    
    close(p.bards)
    for bard := range p.bards {
        bard.Rest()
    }
    
    return nil
}

func (p *AnemoPower) GetStats() map[string]interface{} {
    return map[string]interface{}{
        "active_bards": atomic.LoadInt32(&p.activeCount),
        "resting_bards": len(p.bards),
        "max_bards": p.config.MaxBards,
        "min_bards": p.config.MinBards,
    }
}
