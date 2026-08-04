package bard

import (
    "net/http"
    "net/http/httptest"
    "strings"
    "testing"
)

func TestPublishPerformanceCRLF(t *testing.T) {
    rr := httptest.NewRecorder()
    publishPerformance(rr, []byte("Content-Type: text/html\r\nX-Custom: yes\r\n\r\n<body>hi</body>"))

    if rr.Code != http.StatusOK {
        t.Errorf("Code = %d, want 200", rr.Code)
    }
    if got := rr.Header().Get("Content-Type"); got != "text/html" {
        t.Errorf("Content-Type = %q, want text/html", got)
    }
    if got := rr.Header().Get("X-Custom"); got != "yes" {
        t.Errorf("X-Custom = %q, want yes", got)
    }
    if got := rr.Body.String(); got != "<body>hi</body>" {
        t.Errorf("Body = %q", got)
    }
}

func TestPublishPerformanceLF(t *testing.T) {
    rr := httptest.NewRecorder()
    publishPerformance(rr, []byte("Content-Type: text/html\nX-Custom: yes\n\n<body>hi</body>"))

    if got := rr.Header().Get("Content-Type"); got != "text/html" {
        t.Errorf("Content-Type = %q, want text/html (LF separator)", got)
    }
    if got := rr.Body.String(); got != "<body>hi</body>" {
        t.Errorf("Body = %q", got)
    }
}

func TestPublishPerformanceStatus(t *testing.T) {
    rr := httptest.NewRecorder()
    publishPerformance(rr, []byte("Status: 404 Not Found\nContent-Type: text/html\n\nnot here"))

    if rr.Code != http.StatusNotFound {
        t.Errorf("Code = %d, want 404", rr.Code)
    }
    if got := rr.Body.String(); got != "not here" {
        t.Errorf("Body = %q", got)
    }
}

func TestPublishPerformanceNoHeaders(t *testing.T) {
    rr := httptest.NewRecorder()
    publishPerformance(rr, []byte("just a body"))

    if got := rr.Body.String(); got != "just a body" {
        t.Errorf("Body = %q, want raw passthrough", got)
    }
}

func TestWithinRoot(t *testing.T) {
    cases := []struct {
        root string
        path string
        want bool
    }{
        {"/var/www", "/var/www/index.pl", true},
        {"/var/www", "/var/www/sub/dir.pl", true},
        {"/var/www", "/var/www", true},
        {"/var/www", "/etc/passwd", false},
        {"/var/www", "/var/www2/index.pl", false},
        {"/var/www", "/var/www/../etc/passwd", false},
        {"/var/www", "/var/www/../../etc/passwd", false},
    }
    for _, tc := range cases {
        if got := withinRoot(tc.root, tc.path); got != tc.want {
            t.Errorf("withinRoot(%q, %q) = %v, want %v", tc.root, tc.path, got, tc.want)
        }
    }
}

func TestPrepareLyricsNoServerEnvLeak(t *testing.T) {
    t.Setenv("SUPER_SECRET_SERVER_VAR", "must-not-leak")

    r := httptest.NewRequest("POST", "http://example.com/cgi.pl?q=1", strings.NewReader(""))
    lyrics := prepareLyrics(r)

    joined := strings.Join(lyrics, "\n")
    if strings.Contains(joined, "SUPER_SECRET_SERVER_VAR") {
        t.Errorf("server env leaked into CGI environment: %q", joined)
    }

    // Базовые CGI переменные должны быть
    for _, want := range []string{
        "GATEWAY_INTERFACE=CGI/1.1",
        "REQUEST_METHOD=POST",
        "QUERY_STRING=q=1",
        "PATH=",
    } {
        if !strings.Contains(joined, want) {
            t.Errorf("missing %q in env: %q", want, joined)
        }
    }
}

func TestPrepareLyricsHTTPSAndHeaders(t *testing.T) {
    r := httptest.NewRequest("GET", "https://example.com/", nil)
    r.Header.Set("X-Forwarded-For", "1.2.3.4")

    lyrics := prepareLyrics(r)
    joined := strings.Join(lyrics, "\n")

    if !strings.Contains(joined, "HTTPS=on") {
        t.Errorf("HTTPS=on missing in env: %q", joined)
    }
    if !strings.Contains(joined, "HTTP_X_FORWARDED_FOR=1.2.3.4") {
        t.Errorf("HTTP_X_FORWARDED_FOR missing in env: %q", joined)
    }
}
