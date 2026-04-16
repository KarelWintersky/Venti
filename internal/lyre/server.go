package lyre

import (
    "fmt"
    "net"
    "net/http"
    "os"

    "venti/internal/config"
    
    "github.com/yookoala/gofast"
)

type Lyre struct {
    config   *config.Config
    handler  gofast.Handler
    logger   Logger
    listener net.Listener
}

type Logger interface {
    Info(msg string, args ...interface{})
    Error(msg string, args ...interface{})
    Debug(msg string, args ...interface{})
    Warn(msg string, args ...interface{})
}

type Stage interface {
    Perform(w http.ResponseWriter, r *http.Request)
}

func NewLyre(cfg *config.Config, stage Stage, logger Logger) *Lyre {
    // Создаем HTTP обработчик
    httpHandler := http.HandlerFunc(stage.Perform)
    
    return &Lyre{
        config:  cfg,
        handler: gofast.NewHandler(httpHandler),
        logger:  logger,
    }
}

func (l *Lyre) Play() error {
    var err error
    
    switch l.config.Listener.Type {
    case "unix":
        // Удаляем старый сокет, если существует
        if err := os.Remove(l.config.Listener.Address); err != nil && !os.IsNotExist(err) {
            l.logger.Warn("Failed to remove old socket", "error", err)
        }
        
        // Создаем директорию для сокета, если её нет
        socketDir := l.config.Listener.Address[:len(l.config.Listener.Address)-len("/venti.sock")]
        if err := os.MkdirAll(socketDir, 0755); err != nil {
            return fmt.Errorf("failed to create socket directory: %w", err)
        }
        
        l.listener, err = net.Listen("unix", l.config.Listener.Address)
        if err != nil {
            return fmt.Errorf("failed to listen on unix socket: %w", err)
        }
        
        // Устанавливаем права на сокет
        if err := os.Chmod(l.config.Listener.Address, 0666); err != nil {
            l.logger.Error("Failed to chmod socket", "error", err)
        }
        
        l.logger.Info("🎵 Lyre is playing on Unix socket", 
            "path", l.config.Listener.Address)
        
    case "tcp":
        l.listener, err = net.Listen("tcp", l.config.Listener.Address)
        if err != nil {
            return fmt.Errorf("failed to listen on TCP: %w", err)
        }
        
        l.logger.Info("🎵 Lyre is playing on TCP", 
            "address", l.config.Listener.Address)
        
    default:
        return fmt.Errorf("unknown listener type: %s", l.config.Listener.Type)
    }
    
    l.logger.Info("🌬️ Venti is ready to perform! Waiting for travelers...")
    
    return l.handler.Serve(l.listener)
}

func (l *Lyre) Silence() error {
    l.logger.Info("🎵 The lyre falls silent...")
    if l.listener != nil {
        return l.listener.Close()
    }
    return nil
}
