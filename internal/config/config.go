package config

import (
    "fmt"
    "os"
    "time"

    "gopkg.in/yaml.v3"
)

type Config struct {
    Listener   ListenerConfig   `yaml:"listener"`
    ScriptRoot string           `yaml:"script_root"`
    PerlPath   string           `yaml:"perl_path"`
    AnemoPower AnemoPowerConfig `yaml:"anemo_power"`
    Timeouts   TimeoutConfig    `yaml:"timeouts"`
    Logging    LogConfig        `yaml:"logging"`
    Limits     LimitsConfig     `yaml:"limits"`
}

type ListenerConfig struct {
    Type    string `yaml:"type"`    // unix или tcp
    Address string `yaml:"address"` // путь к сокету или адрес:порт
}

type AnemoPowerConfig struct {
    MinBards       int `yaml:"min_bards"`
    MaxBards       int `yaml:"max_bards"`
    IdleTimeout    int `yaml:"idle_timeout"`     // seconds
    MaxLifetime    int `yaml:"max_lifetime"`     // seconds
}

type TimeoutConfig struct {
    SongDuration int `yaml:"song_duration"` // seconds
    TuneUp       int `yaml:"tune_up"`       // seconds
}

type LogConfig struct {
    Level  string `yaml:"level"`
    File   string `yaml:"file"`
    Format string `yaml:"format"`
}

type LimitsConfig struct {
    MaxVerseSize     int64 `yaml:"max_verse_size"`
    MaxSongsPerBard  int   `yaml:"max_songs_per_bard"`
}

func LoadConfig(path string) (*Config, error) {
    data, err := os.ReadFile(path)
    if err != nil {
        return nil, fmt.Errorf("failed to read config file: %w", err)
    }

    var config Config
    if err := yaml.Unmarshal(data, &config); err != nil {
        return nil, fmt.Errorf("failed to parse config file: %w", err)
    }

    if err := config.Validate(); err != nil {
        return nil, fmt.Errorf("invalid config: %w", err)
    }

    return &config, nil
}

func (c *Config) Validate() error {
    // Проверяем тип слушателя
    if c.Listener.Type != "unix" && c.Listener.Type != "tcp" {
        return fmt.Errorf("listener.type must be 'unix' or 'tcp'")
    }
    
    // Проверяем адрес
    if c.Listener.Address == "" {
        return fmt.Errorf("listener.address is required")
    }
    
    // Проверяем корень скриптов
    if c.ScriptRoot == "" {
        return fmt.Errorf("script_root is required")
    }
    
    // Проверяем существование директории со скриптами
    if _, err := os.Stat(c.ScriptRoot); os.IsNotExist(err) {
        return fmt.Errorf("script_root directory does not exist: %s", c.ScriptRoot)
    }
    
    // Устанавливаем значения по умолчанию для Perl пути
    if c.PerlPath == "" {
        c.PerlPath = "/usr/bin/perl"
    }
    
    // Проверяем существование Perl
    if _, err := os.Stat(c.PerlPath); os.IsNotExist(err) {
        return fmt.Errorf("perl binary not found: %s", c.PerlPath)
    }
    
    // Валидируем настройки пула
    if c.AnemoPower.MinBards < 1 {
        c.AnemoPower.MinBards = 1
    }
    if c.AnemoPower.MaxBards < c.AnemoPower.MinBards {
        c.AnemoPower.MaxBards = c.AnemoPower.MinBards
    }
    
    // Таймауты по умолчанию
    if c.Timeouts.SongDuration == 0 {
        c.Timeouts.SongDuration = 30
    }
    if c.Timeouts.TuneUp == 0 {
        c.Timeouts.TuneUp = 5
    }
    
    // Лимиты по умолчанию
    if c.Limits.MaxSongsPerBard == 0 {
        c.Limits.MaxSongsPerBard = 1000
    }
    
    return nil
}

func (c *Config) GetSongDuration() time.Duration {
    return time.Duration(c.Timeouts.SongDuration) * time.Second
}

func (c *Config) GetTuneUpTimeout() time.Duration {
    return time.Duration(c.Timeouts.TuneUp) * time.Second
}

func (c *Config) GetIdleTimeout() time.Duration {
    return time.Duration(c.AnemoPower.IdleTimeout) * time.Second
}

func (c *Config) GetMaxLifetime() time.Duration {
    return time.Duration(c.AnemoPower.MaxLifetime) * time.Second
}
