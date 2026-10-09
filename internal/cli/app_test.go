package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/yantonov/backlogger/internal/model"
)

// TestAdd_InjectedClock_StampsCreatedAndUpdatedWithIt proves timestamps come
// from the injected now() and that created == updated on creation.
func TestAdd_InjectedClock_StampsCreatedAndUpdatedWithIt(t *testing.T) {
	dir := t.TempDir()
	app, stdout, stderr := newTestApp(t, dir)

	if code := app.Run([]string{"add", "Timed", "--id", "timed"}); code != 0 {
		t.Fatalf("add exit %d: %s", code, stderr.String())
	}
	if !strings.Contains(stdout.String(), "added timed") {
		t.Fatalf("add output = %q", stdout.String())
	}

	data, err := os.ReadFile(filepath.Join(dir, ".tickets", "timed.md"))
	if err != nil {
		t.Fatal(err)
	}
	want := model.FormatTimestamp(fixedNow)
	if strings.Count(string(data), want) != 2 {
		t.Fatalf("expected created and updated both %s:\n%s", want, data)
	}
}

// TestStart_AdvancedClock_BumpsUpdatedAndKeepsCreated proves a transition
// changes updated but not created, and that the injected clock drives the new
// value.
func TestStart_AdvancedClock_BumpsUpdatedAndKeepsCreated(t *testing.T) {
	dir := t.TempDir()
	app, _, stderr := newTestApp(t, dir)
	if code := app.Run([]string{"add", "Timed", "--id", "timed"}); code != 0 {
		t.Fatalf("add exit %d: %s", code, stderr.String())
	}

	later := fixedNow.Add(90 * time.Minute)
	app.Now = func() time.Time { return later }
	if code := app.Run([]string{"start", "timed"}); code != 0 {
		t.Fatalf("take exit %d: %s", code, stderr.String())
	}

	data, err := os.ReadFile(filepath.Join(dir, ".tickets", "timed.md"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), "created: "+model.FormatTimestamp(fixedNow)) {
		t.Fatalf("created changed:\n%s", data)
	}
	if !strings.Contains(string(data), "updated: "+model.FormatTimestamp(later)) {
		t.Fatalf("updated not bumped by injected clock:\n%s", data)
	}
}

// The in-process dispatch tests mirror the process-level ones.

func TestAppRun_NoArgs_PrintsUsage(t *testing.T) {
	app, stdout, _ := newTestApp(t, t.TempDir())

	code := app.Run(nil)

	if code != 0 || !strings.Contains(stdout.String(), "usage: bl") {
		t.Fatalf("no-args: code %d stdout %q", code, stdout.String())
	}
}

func TestAppRun_UnknownCommand_ExitsUsageWithMessage(t *testing.T) {
	app, _, stderr := newTestApp(t, t.TempDir())

	code := app.Run([]string{"bogus"})

	if code != exitUsage {
		t.Fatalf("unknown command code = %d, want %d", code, exitUsage)
	}
	if !strings.Contains(stderr.String(), "unknown command") {
		t.Fatalf("unknown command stderr = %q", stderr.String())
	}
}
