package model_test

import (
	"errors"
	"strings"
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

// TestValidate_Title_RejectsMissingAndBlank pins the one invariant the line
// parser cannot express: a title is required and whitespace only is empty, so
// both `title: ""` and `title: "   "` are rejected.
func TestValidate_Title_RejectsMissingAndBlank(t *testing.T) {
	for _, title := range []string{"", " ", "\t", "  \n "} {
		err := model.Ticket{Title: title}.Validate()
		if !errors.Is(err, model.ErrTitleMissing) {
			t.Fatalf("Validate(title %q) = %v, want ErrTitleMissing", title, err)
		}
		if err != nil && !strings.Contains(err.Error(), "non-empty") {
			t.Fatalf("error does not name the invariant: %v", err)
		}
	}
}

// TestValidate_NonEmptyTitle_Accepts verifies a title with surrounding spaces is
// accepted unchanged: the check trims, the stored value does not.
func TestValidate_NonEmptyTitle_Accepts(t *testing.T) {
	ticket := model.Ticket{Title: " spaced title "}
	if err := ticket.Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if ticket.Title != " spaced title " {
		t.Fatalf("title was mangled: %q", ticket.Title)
	}
}
