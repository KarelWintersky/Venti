package bard

import (
    "bytes"
    "context"
    "fmt"
    "io"
    "net/http"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "sync"
    "sync/atomic"
    "time"
)

type Performer struct {
    id          int64
    name        string
    perlPath    string
    scriptRoot  string
    songCount   int32
    birthTime   time.Time
    lastSong    time.Time
    mu          sync.RWMutex
}

type Troupe struct {
    PerlPath   string
    ScriptRoot string
    nextID     int64
}

func (t *Troupe) Recruit() (*Performer, error) {
    id := atomic.AddInt64(&t.nextID, 1)
    names := []string{"Venti", "Barbatos", "Tone-Deaf Bard", "Windborne Minstrel"}
    name := names[id%int64(len(names))]
    
    return &Performer{
        id:         id,
        name:       name,
        perlPath:   t.PerlPath,
        scriptRoot: t.ScriptRoot,
        birthTime:  time.Now(),
        lastSong:   time.Now(),
    }, nil
}

func (p *Performer) Sing(ctx context.Context, songPath string, lyrics []string, melody []byte) ([]byte, error) {
    p.mu.Lock()
    p.lastSong = time.Now()
    atomic.AddInt32(&p.songCount, 1)
    p.mu.Unlock()

    // Проверяем безопасность пути
    fullPath := filepath.Join(p.scriptRoot, songPath)
    if !strings.HasPrefix(fullPath, p.scriptRoot) {
        return nil, fmt.Errorf("forbidden melody: %s", songPath)
    }

    // Проверяем существование песни (скрипта)
    if _, err := os.Stat(fullPath); os.IsNotExist(err) {
        return nil, fmt.Errorf("song not found: %s", songPath)
    }

    // Исполняем песню
    execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    cmd := exec.CommandContext(execCtx, p.perlPath, fullPath)
    cmd.Env = lyrics

    if melody != nil {
        cmd.Stdin = bytes.NewReader(melody)
    }

    var stdout, stderr bytes.Buffer
    cmd.Stdout = &stdout
    cmd.Stderr = &stderr

    err := cmd.Run()
    if err != nil {
        return nil, fmt.Errorf("performance failed: %w, stderr: %s", err, stderr.String())
    }

    return stdout.Bytes(), nil
}

func (p *Performer) Rest() error {
    // Уходим на покой
    return nil
}

func (p *Performer) IsHealthy() bool {
    return true
}

func (p *Performer) GetSongCount() int {
    return int(atomic.LoadInt32(&p.songCount))
}

func (p *Performer) GetBirthTime() time.Time {
    return p.birthTime
}

func (p *Performer) GetLastSongTime() time.Time {
    p.mu.RLock()
    defer p.mu.RUnlock()
    return p.lastSong
}

func (p *Performer) GetName() string {
    return p.name
}

type Stage struct {
    anemoPower *AnemoPower
}

func NewStage(anemoPower *AnemoPower) *Stage {
    return &Stage{anemoPower: anemoPower}
}

func (s *Stage) Perform(w http.ResponseWriter, r *http.Request) {
    // Готовим партитуру (окружение)
    lyrics := prepareLyrics(r)
    
    // Читаем мелодию (тело запроса)
    melody, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "Failed to read melody", http.StatusBadRequest)
        return
    }
    
    // Призываем барда
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
    defer cancel()
    
    bard, err := s.anemoPower.CallBard(ctx)
    if err != nil {
        http.Error(w, "No bards available", http.StatusServiceUnavailable)
        return
    }
    defer s.anemoPower.ReleaseBard(bard)
    
    // Исполняем песню
    song := r.URL.Path
    output, err := bard.Sing(r.Context(), song, lyrics, melody)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }
    
    // Публикуем исполнение
    publishPerformance(w, output)
}

func prepareLyrics(r *http.Request) []string {
    env := os.Environ()
    
    cgiLyrics := map[string]string{
        "GATEWAY_INTERFACE": "CGI/1.1",
        "SERVER_SOFTWARE":   "Venti/1.0",
        "SERVER_PROTOCOL":   r.Proto,
        "REQUEST_METHOD":    r.Method,
        "REQUEST_URI":       r.RequestURI,
        "SCRIPT_NAME":       r.URL.Path,
        "PATH_INFO":         r.URL.Path,
        "QUERY_STRING":      r.URL.RawQuery,
        "CONTENT_TYPE":      r.Header.Get("Content-Type"),
        "CONTENT_LENGTH":    fmt.Sprintf("%d", r.ContentLength),
        "REMOTE_ADDR":       r.RemoteAddr,
    }
    
    for key, values := range r.Header {
        envKey := "HTTP_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
        cgiLyrics[envKey] = strings.Join(values, ", ")
    }
    
    for key, value := range cgiLyrics {
        if value != "" {
            env = append(env, fmt.Sprintf("%s=%s", key, value))
        }
    }
    
    return env
}

func publishPerformance(w http.ResponseWriter, data []byte) {
    parts := bytes.SplitN(data, []byte("\r\n\r\n"), 2)
    
    if len(parts) < 2 {
        w.Write(data)
        return
    }
    
    headers := bytes.Split(parts[0], []byte("\r\n"))
    for _, header := range headers {
        headerParts := bytes.SplitN(header, []byte(":"), 2)
        if len(headerParts) == 2 {
            key := strings.TrimSpace(string(headerParts[0]))
            value := strings.TrimSpace(string(headerParts[1]))
            w.Header().Set(key, value)
        }
    }
    
    w.Write(parts[1])
}
