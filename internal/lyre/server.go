package lyre

import (
    "fmt"
    "net"
    "net/http"
    "net/http/fcgi"
    "os"
    "strings"

    "venti/internal/config"
)

// Lyre - небесная лира (FastCGI сервер)
type Lyre struct {
    config   *config.Config
    handler  http.Handler
    logger   Logger
    listener net.Listener
}

type Logger interface {
    Info(msg string, args ...interface{})
    Error(msg string, args ...interface{})
    Debug(msg string, args ...interface{})
    Warn(msg string, args ...interface{})
}

// Stage - сцена, где проходят выступления
type Stage interface {
    Perform(w http.ResponseWriter, r *http.Request)
}

// NewLyre - создать новую небесную лиру
func NewLyre(cfg *config.Config, stage Stage, logger Logger) *Lyre {
    httpHandler := http.HandlerFunc(stage.Perform)

    return &Lyre{
        config:  cfg,
        handler: httpHandler,
        logger:  logger,
    }
}

// Play - начать играть на лире (запустить сервер)
func (l *Lyre) Play() error {
    var err error

    switch l.config.Listener.Type {
    case "unix":
        // Удаляем старый сокет, если существует
        if err := os.Remove(l.config.Listener.Address); err != nil && !os.IsNotExist(err) {
            l.logger.Warn("Failed to remove old socket", "error", err)
        }

        // Создаем директорию для сокета
        socketDir := l.config.Listener.Address[:strings.LastIndex(l.config.Listener.Address, "/")]
        if err := os.MkdirAll(socketDir, 0755); err != nil {
            return fmt.Errorf("failed to create socket directory: %w", err)
        }

        l.listener, err = net.Listen("unix", l.config.Listener.Address)
        if err != nil {
            return fmt.Errorf("failed to listen on unix socket: %w", err)
        }

        if err := os.Chmod(l.config.Listener.Address, 0666); err != nil {
            l.logger.Error("Failed to chmod socket", "error", err)
        }

        l.logger.Info("🎵 The Skyward Lyre plays on the winds",
            "path", l.config.Listener.Address)

    case "tcp":
        l.listener, err = net.Listen("tcp", l.config.Listener.Address)
        if err != nil {
            return fmt.Errorf("failed to listen on TCP: %w", err)
        }

        l.logger.Info("🎵 The Skyward Lyre echoes through the city",
            "address", l.config.Listener.Address)

    default:
        return fmt.Errorf("unknown listener type: %s", l.config.Listener.Type)
    }

    l.logger.Info("🌬️ Venti takes the stage! Waiting for travelers from afar...")
    l.logger.Info("✨ May the wind bring many songs to perform ✨")

    // Используем встроенный FastCGI сервер
    return fcgi.Serve(l.listener, l.handler)
}

// Silence - лира замолкает (остановка сервера)
func (l *Lyre) Silence() error {
    l.logger.Info("🎵 The Skyward Lyre falls silent... Until we meet again! 🎵")
    if l.listener != nil {
        return l.listener.Close()
    }
    return nil
}