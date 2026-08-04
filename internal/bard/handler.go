package bard

import (
    "bufio"
    "bytes"
    "context"
    _ "embed"
    "fmt"
    "io"
    "net"
    "net/http"
    "net/http/fcgi"
    "os"
    "os/exec"
    "path/filepath"
    "strings"
    "sync"
    "sync/atomic"
    "time"

    "venti/internal/config"
)

//go:embed worker.pl
var workerCode string

type Performer struct {
    id          int64
    name        string
    songTimeout time.Duration
    cmd         *exec.Cmd
    stdin       *os.File
    stdout      *os.File
    dead        atomic.Bool
    songsCount  int32
    birthTime   time.Time
    lastSong    time.Time
    mu          sync.RWMutex
}

type Troupe struct {
    PerlPath    string
    SongTimeout time.Duration
    nextID      int64
}

// Интерфейс для силы анемо (пула)
type AnemoPower interface {
    CallBard(ctx context.Context) (Bard, error)
    ReleaseBard(bard Bard)
}

type Bard interface {
    Sing(ctx context.Context, songPath string, lyrics []string, melody []byte) ([]byte, error)
    Rest() error
    IsHealthy() bool
    GetSongsCount() int
    GetBirthTime() time.Time
    GetLastSongTime() time.Time
}

func (t *Troupe) Recruit() (*Performer, error) {
    id := atomic.AddInt64(&t.nextID, 1)
    names := []string{"Venti", "Barbatos", "Tone-Deaf Bard", "Windborne Minstrel", "Skyward Bard"}
    name := names[id%int64(len(names))]

    inR, inW, err := os.Pipe()
    if err != nil {
        return nil, fmt.Errorf("failed to create stdin pipe: %w", err)
    }
    outR, outW, err := os.Pipe()
    if err != nil {
        inR.Close()
        inW.Close()
        return nil, fmt.Errorf("failed to create stdout pipe: %w", err)
    }
    errR, errW, err := os.Pipe()
    if err != nil {
        inR.Close()
        inW.Close()
        outR.Close()
        outW.Close()
        return nil, fmt.Errorf("failed to create stderr pipe: %w", err)
    }

    cmd := exec.Command(t.PerlPath, "-e", workerCode)
    cmd.Stdin = inR
    cmd.Stdout = outW
    cmd.Stderr = errW

    if err := cmd.Start(); err != nil {
        inR.Close()
        inW.Close()
        outR.Close()
        outW.Close()
        errR.Close()
        errW.Close()
        return nil, fmt.Errorf("failed to start perl worker: %w", err)
    }

    inR.Close()
    outW.Close()
    errW.Close()

    go func() {
        io.Copy(io.Discard, errR)
        errR.Close()
    }()

    return &Performer{
        id:          id,
        name:        name,
        songTimeout: t.SongTimeout,
        cmd:         cmd,
        stdin:       inW,
        stdout:      outR,
        birthTime:   time.Now(),
        lastSong:    time.Now(),
    }, nil
}

func (p *Performer) Sing(ctx context.Context, songPath string, lyrics []string, melody []byte) ([]byte, error) {
    p.mu.Lock()
    defer p.mu.Unlock()

    if p.dead.Load() {
        return nil, fmt.Errorf("bard is no longer on stage")
    }

    if !filepath.IsAbs(songPath) {
        return nil, fmt.Errorf("song path must be absolute: %s", songPath)
    }
    if _, err := os.Stat(songPath); os.IsNotExist(err) {
        return nil, fmt.Errorf("song not found: %s", songPath)
    }

    timeout := p.songTimeout
    if timeout == 0 {
        timeout = 30 * time.Second
    }
    deadline := time.Now().Add(timeout)
    if dl, ok := ctx.Deadline(); ok && dl.Before(deadline) {
        deadline = dl
    }

    p.lastSong = time.Now()

    envBlock := strings.Join(lyrics, "\n")
    header := fmt.Sprintf("%d %d\n", len(envBlock), len(melody))

    p.stdin.SetWriteDeadline(deadline)
    if _, err := io.WriteString(p.stdin, header); err != nil {
        p.kill()
        return nil, fmt.Errorf("failed to deliver the song request: %w", err)
    }
    if len(envBlock) > 0 {
        if _, err := io.WriteString(p.stdin, envBlock); err != nil {
            p.kill()
            return nil, fmt.Errorf("failed to deliver the lyrics: %w", err)
        }
    }
    if len(melody) > 0 {
        if _, err := p.stdin.Write(melody); err != nil {
            p.kill()
            return nil, fmt.Errorf("failed to deliver the melody: %w", err)
        }
    }

    p.stdout.SetReadDeadline(deadline)
    reader := bufio.NewReader(p.stdout)

    respLine, err := reader.ReadString('\n')
    if err != nil {
        p.kill()
        return nil, fmt.Errorf("bard lost his voice: %w", err)
    }

    var outLen, errLen, status int
    if _, err := fmt.Sscanf(respLine, "%d %d %d", &outLen, &errLen, &status); err != nil {
        p.kill()
        return nil, fmt.Errorf("bard sang an unexpected tune: %w", err)
    }

    outBuf := make([]byte, outLen)
    if _, err := io.ReadFull(reader, outBuf); err != nil {
        p.kill()
        return nil, fmt.Errorf("response cut short: %w", err)
    }

    errBuf := make([]byte, errLen)
    if _, err := io.ReadFull(reader, errBuf); err != nil {
        p.kill()
        return nil, fmt.Errorf("stderr cut short: %w", err)
    }

    if status != 0 {
        return nil, fmt.Errorf("performance failed with status %d: %s", status, strings.TrimSpace(string(errBuf)))
    }

    atomic.AddInt32(&p.songsCount, 1)

    return outBuf, nil
}

func (p *Performer) kill() error {
    if p.dead.Swap(true) {
        return nil
    }
    if p.cmd != nil && p.cmd.Process != nil {
        p.cmd.Process.Kill()
        p.cmd.Wait()
    }
    if p.stdin != nil {
        p.stdin.Close()
    }
    if p.stdout != nil {
        p.stdout.Close()
    }
    return nil
}

func (p *Performer) Rest() error {
    p.mu.Lock()
    defer p.mu.Unlock()
    return p.kill()
}

func (p *Performer) IsHealthy() bool {
    return !p.dead.Load()
}

func (p *Performer) GetSongsCount() int {
    return int(atomic.LoadInt32(&p.songsCount))
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

// Stage - сцена, где барды выступают
type Stage struct {
    config     *config.Config
    anemoPower AnemoPower
}

func NewStage(cfg *config.Config, anemoPower AnemoPower) *Stage {
    return &Stage{config: cfg, anemoPower: anemoPower}
}

// Perform - начать представление
func (s *Stage) Perform(w http.ResponseWriter, r *http.Request) {
    // Получаем путь к песне (скрипту) от nginx
    params := fcgi.ProcessEnv(r)
    songPath := params["SCRIPT_FILENAME"]
    if songPath == "" {
        songPath = r.Header.Get("SCRIPT_FILENAME")
    }
    if songPath == "" {
        http.Error(w, "No song specified by the traveler (missing SCRIPT_FILENAME)", http.StatusBadRequest)
        return
    }

    // Не выпускаем барда за пределы document root
    if root := s.config.Limits.DocumentRoot; root != "" {
        if !withinRoot(root, songPath) {
            http.Error(w, "The song lies beyond the document root", http.StatusForbidden)
            return
        }
    }

    // Готовим лирику (переменные окружения)
    lyrics := prepareLyrics(r)

    // Слушаем мелодию (тело запроса), ограничивая ее размер
    limit := s.config.Limits.MaxVerseSize
    melody, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
    if err != nil {
        http.Error(w, "Failed to hear the melody", http.StatusBadRequest)
        return
    }
    if int64(len(melody)) > limit {
        http.Error(w, "The melody is too long", http.StatusRequestEntityTooLarge)
        return
    }

    // Призываем барда из силы анемо
    ctx, cancel := context.WithTimeout(r.Context(), s.config.GetTuneUpTimeout())
    defer cancel()

    bard, err := s.anemoPower.CallBard(ctx)
    if err != nil {
        http.Error(w, "No bards available at the tavern, please try again later", http.StatusServiceUnavailable)
        return
    }
    defer s.anemoPower.ReleaseBard(bard)

    // Бард исполняет песню
    performance, err := bard.Sing(r.Context(), songPath, lyrics, melody)
    if err != nil {
        http.Error(w, fmt.Sprintf("The performance failed: %v", err), http.StatusInternalServerError)
        return
    }

    // Публикуем выступление (отправляем ответ)
    publishPerformance(w, performance)
}

// withinRoot - проверяет, что путь лежит внутри корня (без обхода через ..)
func withinRoot(root, path string) bool {
    root = filepath.Clean(root)
    clean := filepath.Clean(path)
    rel, err := filepath.Rel(root, clean)
    if err != nil {
        return false
    }
    if rel == "." {
        return true
    }
    return rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

// prepareLyrics - подготавливает лирику (CGI переменные окружения)
func prepareLyrics(r *http.Request) []string {
    params := fcgi.ProcessEnv(r)

    remoteHost, remotePort := "", ""
    if host, port, err := net.SplitHostPort(r.RemoteAddr); err == nil {
        remoteHost = host
        remotePort = port
    }

    // Базовые строфы (CGI переменные)
    cgiStanzas := map[string]string{
        "GATEWAY_INTERFACE": "CGI/1.1",
        "SERVER_SOFTWARE":   "Venti/1.0 (Anemo Archon)",
        "SERVER_PROTOCOL":   r.Proto,
        "SERVER_NAME":       r.Host,
        "REQUEST_METHOD":    r.Method,
        "REQUEST_URI":       r.RequestURI,
        "QUERY_STRING":      r.URL.RawQuery,
        "CONTENT_TYPE":      r.Header.Get("Content-Type"),
        "CONTENT_LENGTH":    fmt.Sprintf("%d", r.ContentLength),
        "REMOTE_ADDR":       remoteHost,
        "REMOTE_PORT":       remotePort,
    }

    if r.TLS != nil {
        cgiStanzas["HTTPS"] = "on"
    }

    // Все HTTP заголовки становятся строфами с префиксом HTTP_
    for key, values := range r.Header {
        stanzaKey := "HTTP_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
        cgiStanzas[stanzaKey] = strings.Join(values, ", ")
    }

    // Параметры FastCGI от nginx (SCRIPT_FILENAME, DOCUMENT_ROOT и т.д.)
    for key, value := range params {
        cgiStanzas[key] = value
    }

    // Собираем все строфы вместе.
    // Начинаем с безопасного минимума: окружение сервера в CGI не утекает,
    // только базовый PATH (перекрывается fastcgi_param PATH от nginx).
    lyrics := []string{}
    if _, ok := cgiStanzas["PATH"]; !ok {
        lyrics = append(lyrics, "PATH=/usr/local/bin:/usr/bin:/bin")
    }
    for key, value := range cgiStanzas {
        if value != "" {
            lyrics = append(lyrics, fmt.Sprintf("%s=%s", key, value))
        }
    }

    return lyrics
}

// publishPerformance - публикует выступление (отправляет HTTP ответ)
func publishPerformance(w http.ResponseWriter, performance []byte) {
    // Ищем разделитель между заголовками и телом (CRLF или LF)
    sep := []byte("\r\n\r\n")
    idx := bytes.Index(performance, sep)
    if idx == -1 {
        sep = []byte("\n\n")
        idx = bytes.Index(performance, sep)
    }

    if idx == -1 {
        // Нет заголовков, просто поем
        w.Write(performance)
        return
    }

    // Парсим заголовки выступления
    headers := bytes.Split(performance[:idx], []byte("\r\n"))
    if len(headers) == 1 {
        headers = bytes.Split(performance[:idx], []byte("\n"))
    }
    for _, header := range headers {
        headerParts := bytes.SplitN(header, []byte(":"), 2)
        if len(headerParts) == 2 {
            key := strings.TrimSpace(string(headerParts[0]))
            value := strings.TrimSpace(string(headerParts[1]))

            // Особый случай - статус песни
            if strings.EqualFold(key, "Status") {
                statusCode := strings.Split(value, " ")[0]
                w.WriteHeader(parseStatusCode(statusCode))
            } else {
                w.Header().Set(key, value)
            }
        }
    }

    // Поем основную партию
    w.Write(performance[idx+len(sep):])
}

func parseStatusCode(status string) int {
    var code int
    fmt.Sscanf(status, "%d", &code)
    if code == 0 {
        return http.StatusOK
    }
    return code
}
