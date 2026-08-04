package bard

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func perlPathOrSkip(t *testing.T) string {
	t.Helper()
	const p = "/usr/bin/perl"
	if _, err := os.Stat(p); err != nil {
		t.Skipf("perl not found at %s", p)
	}
	return p
}

func writeScript(t *testing.T, name, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(body), 0755); err != nil {
		t.Fatalf("write script: %v", err)
	}
	return path
}

func TestPerformerSing(t *testing.T) {
	perl := perlPathOrSkip(t)
	script := writeScript(t, "hello.pl", `#!/usr/bin/perl
print "Content-Type: text/html\n\n";
print "hello from perl\n";
print "method=$ENV{REQUEST_METHOD} qs=$ENV{QUERY_STRING}\n";
`)

	performer, err := (&Troupe{PerlPath: perl, SongTimeout: 10 * time.Second}).Recruit()
	if err != nil {
		t.Fatalf("Recruit: %v", err)
	}
	t.Cleanup(func() { performer.Rest() })

	lyrics := []string{
		"SCRIPT_FILENAME=" + script,
		"REQUEST_METHOD=GET",
		"QUERY_STRING=a=1&b=2",
	}
	out, err := performer.Sing(context.Background(), script, lyrics, nil)
	if err != nil {
		t.Fatalf("Sing: %v", err)
	}

	s := string(out)
	if !strings.Contains(s, "hello from perl") {
		t.Errorf("output missing hello: %q", s)
	}
	if !strings.Contains(s, "method=GET qs=a=1&b=2") {
		t.Errorf("env not propagated: %q", s)
	}
}

func TestPerformerSingStdinBody(t *testing.T) {
	perl := perlPathOrSkip(t)
    script := writeScript(t, "echo.pl", `#!/usr/bin/perl
local $/; print <STDIN>;
`)

	performer, err := (&Troupe{PerlPath: perl, SongTimeout: 10 * time.Second}).Recruit()
	if err != nil {
		t.Fatalf("Recruit: %v", err)
	}
	t.Cleanup(func() { performer.Rest() })

	out, err := performer.Sing(context.Background(), script, []string{"SCRIPT_FILENAME=" + script}, []byte("hello world"))
	if err != nil {
		t.Fatalf("Sing: %v", err)
	}
	if string(out) != "hello world" {
		t.Errorf("body = %q, want %q", string(out), "hello world")
	}
}

func TestPerformerSingExitCode(t *testing.T) {
	perl := perlPathOrSkip(t)
	script := writeScript(t, "exit42.pl", `#!/usr/bin/perl
print "Content-Type: text/html\n\n";
print "partial output\n";
exit 42;
`)

	performer, err := (&Troupe{PerlPath: perl, SongTimeout: 10 * time.Second}).Recruit()
	if err != nil {
		t.Fatalf("Recruit: %v", err)
	}
	t.Cleanup(func() { performer.Rest() })

	_, err = performer.Sing(context.Background(), script, []string{"SCRIPT_FILENAME=" + script}, nil)
	if err == nil {
		t.Fatalf("expected error for exit 42, got nil")
	}
	if !strings.Contains(err.Error(), "42") {
		t.Errorf("error should mention status 42: %v", err)
	}
}

func TestPerformerSingMissingScript(t *testing.T) {
	perl := perlPathOrSkip(t)
	performer, err := (&Troupe{PerlPath: perl, SongTimeout: 10 * time.Second}).Recruit()
	if err != nil {
		t.Fatalf("Recruit: %v", err)
	}
	t.Cleanup(func() { performer.Rest() })

	missing := filepath.Join(t.TempDir(), "nope.pl")
	if _, err := performer.Sing(context.Background(), missing, []string{"SCRIPT_FILENAME=" + missing}, nil); err == nil {
		t.Errorf("expected error for missing script, got nil")
	}
}

func TestPerformerSurvivesMultipleSongs(t *testing.T) {
	perl := perlPathOrSkip(t)
	script := writeScript(t, "counter.pl", `#!/usr/bin/perl
$main::COUNT = ($main::COUNT || 0) + 1;
print "Content-Type: text/html\n\n";
print "song #$main::COUNT\n";
`)

	performer, err := (&Troupe{PerlPath: perl, SongTimeout: 10 * time.Second}).Recruit()
	if err != nil {
		t.Fatalf("Recruit: %v", err)
	}
	t.Cleanup(func() { performer.Rest() })

	for i := 1; i <= 3; i++ {
		out, err := performer.Sing(context.Background(), script, []string{"SCRIPT_FILENAME=" + script}, nil)
		if err != nil {
			t.Fatalf("Sing #%d: %v", i, err)
		}
		want := "song #" + string(rune('0'+i))
		if !strings.Contains(string(out), want) {
			t.Errorf("Sing #%d output = %q, want it to contain %q (state should persist)", i, string(out), want)
		}
	}

	if got := performer.GetSongsCount(); got != 3 {
		t.Errorf("GetSongsCount = %d, want 3", got)
	}
}

func TestPerformerKilled(t *testing.T) {
    perl := perlPathOrSkip(t)

    performer, err := (&Troupe{PerlPath: perl, SongTimeout: 10 * time.Second}).Recruit()
	if err != nil {
		t.Fatalf("Recruit: %v", err)
	}

	if err := performer.Rest(); err != nil {
		t.Fatalf("Rest: %v", err)
	}
	if performer.IsHealthy() {
		t.Errorf("performer should be dead after Rest")
	}

	// Повторный Rest должен быть безвреден
	if err := performer.Rest(); err != nil {
		t.Errorf("second Rest should be no-op, got %v", err)
	}
}
