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
    songsCount  int32      // количество исполненных песен
    birthTime   time.Time  // время рождения барда
    lastSong    time.Time  // время последней песни
    mu          sync.RWMutex
}

type Troupe struct {
    PerlPath   string
    nextID     int64
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

    return &Performer{
        id:        id,
        name:      name,
        perlPath:  t.PerlPath,
        birthTime: time.Now(),
        lastSong:  time.Now(),
    }, nil
}

func (p *Performer) Sing(ctx context.Context, songPath string, lyrics []string, melody []byte) ([]byte, error) {
    p.mu.Lock()
    p.lastSong = time.Now()
    atomic.AddInt32(&p.songsCount, 1)
    p.mu.Unlock()

    // Проверяем, что путь к песне абсолютный и безопасный
    if !filepath.IsAbs(songPath) {
        return nil, fmt.Errorf("song path must be absolute: %s", songPath)
    }

    // Проверяем, что песня существует
    if _, err := os.Stat(songPath); os.IsNotExist(err) {
        return nil, fmt.Errorf("song not found: %s", songPath)
    }

    // Исполняем песню (запускаем Perl скрипт)
    execCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
    defer cancel()

    cmd := exec.CommandContext(execCtx, p.perlPath, songPath)
    cmd.Env = lyrics  // лирика = переменные окружения

    if melody != nil {
        cmd.Stdin = bytes.NewReader(melody)  // мелодия = тело запроса
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
    // Бард уходит на покой
    return nil
}

func (p *Performer) IsHealthy() bool {
    return true
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
    anemoPower AnemoPower
}

func NewStage(anemoPower AnemoPower) *Stage {
    return &Stage{anemoPower: anemoPower}
}

// Perform - начать представление
func (s *Stage) Perform(w http.ResponseWriter, r *http.Request) {
    // Получаем путь к песне (скрипту) от nginx
    songPath := r.Header.Get("SCRIPT_FILENAME")
    if songPath == "" {
        songPath = r.Header.Get("SCRIPT_NAME")
        if songPath == "" {
            http.Error(w, "No song specified by the traveler (missing SCRIPT_FILENAME)", http.StatusBadRequest)
            return
        }
    }

    // Готовим лирику (переменные окружения)
    lyrics := prepareLyrics(r)

    // Слушаем мелодию (тело запроса)
    melody, err := io.ReadAll(r.Body)
    if err != nil {
        http.Error(w, "Failed to hear the melody", http.StatusBadRequest)
        return
    }

    // Призываем барда из силы анемо
    ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
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

// prepareLyrics - подготавливает лирику (CGI переменные окружения)
func prepareLyrics(r *http.Request) []string {
    lyrics := os.Environ()

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
        "REMOTE_ADDR":       r.RemoteAddr,
    }

    // Все HTTP заголовки становятся строфами с префиксом HTTP_
    for key, values := range r.Header {
        stanzaKey := "HTTP_" + strings.ToUpper(strings.ReplaceAll(key, "-", "_"))
        cgiStanzas[stanzaKey] = strings.Join(values, ", ")
    }

    // Добавляем песни от путешественника (nginx)
    for _, stanza := range []string{
        "SCRIPT_FILENAME", "SCRIPT_NAME", "PATH_INFO",
        "PATH_TRANSLATED", "DOCUMENT_ROOT", "DOCUMENT_URI",
    } {
        if val := r.Header.Get(stanza); val != "" {
            cgiStanzas[stanza] = val
        }
    }

    // Собираем все строфы вместе
    for key, value := range cgiStanzas {
        if value != "" {
            lyrics = append(lyrics, fmt.Sprintf("%s=%s", key, value))
        }
    }

    return lyrics
}

// publishPerformance - публикует выступление (отправляет HTTP ответ)
func publishPerformance(w http.ResponseWriter, performance []byte) {
    // Ищем разделитель между заголовками и телом
    parts := bytes.SplitN(performance, []byte("\r\n\r\n"), 2)

    if len(parts) < 2 {
        // Нет заголовков, просто поем
        w.Write(performance)
        return
    }

    // Парсим заголовки выступления
    headers := bytes.Split(parts[0], []byte("\r\n"))
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
    w.Write(parts[1])
}

func parseStatusCode(status string) int {
    var code int
    fmt.Sscanf(status, "%d", &code)
    if code == 0 {
        return http.StatusOK
    }
    return code
}