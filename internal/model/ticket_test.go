package model_test

import (
	"testing"
	"time"

	"github.com/yantonov/backlogger/internal/model"
)

func TestFormatTimestamp_UTCTime_RendersCanonicalForm(t *testing.T) {
	now := time.Date(2026, 10, 8, 11, 40, 0, 0, time.UTC)

	got := model.FormatTimestamp(now)

	if got != "2026-10-08T11:40:00Z" {
		t.Fatalf("FormatTimestamp: got %q", got)
	}
}
