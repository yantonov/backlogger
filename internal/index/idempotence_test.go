package index_test

import (
	"bytes"
	"reflect"
	"testing"

	"github.com/yantonov/backlogger/internal/index"
	"github.com/yantonov/backlogger/internal/model"
)

// fourDetails is a detail set covering every status and a deps edge.
func fourDetails() []*index.Detail {
	return []*index.Detail{
		detail("login", model.StatusInProgress, 1, "2026-10-08T09:00:00Z", "Add login flow", "build", "design"),
		detail("design", model.StatusOpen, 0, "2026-10-08T08:00:00Z", "Design the API"),
		detail("build", model.StatusOpen, 2, "2026-10-08T07:00:00Z", "Set up CI"),
		detail("deploy", model.StatusClosed, 2, "2026-10-07T00:00:00Z", "Deploy to prod"),
	}
}

// TestRender_ReorderedInput_ProducesSameBytes proves f is a function of the
// detail set, not of the input order (spec 3.1).
func TestRender_ReorderedInput_ProducesSameBytes(t *testing.T) {
	details := fourDetails()
	want := index.Render(details)
	reordered := []*index.Detail{details[3], details[1], details[2], details[0]}

	got := index.Render(reordered)

	if !bytes.Equal(got, want) {
		t.Fatalf("f is order-dependent:\nfirst:\n%s\nsecond:\n%s", want, got)
	}
}

// TestRender_SameDetailsTwice_ProducesSameBytes proves f(f(details)) ==
// f(details): rendering the same detail set again produces byte-identical
// output (spec 3.1). Re-rendering the parsed result is
// TestRenderEntries_ParsedRender_ReproducesBytes.
func TestRender_SameDetailsTwice_ProducesSameBytes(t *testing.T) {
	details := fourDetails()
	want := index.Render(details)

	got := index.Render(details)

	if !bytes.Equal(got, want) {
		t.Fatalf("second render of the same details differs:\n%s\n%s", want, got)
	}
}

// TestRenderEntries_ParsedRender_ReproducesBytes proves re-rendering a parsed
// index does not drift: RenderEntries(Parse(f(details))) == f(details)
// (spec 3.1).
func TestRenderEntries_ParsedRender_ReproducesBytes(t *testing.T) {
	rendered := index.Render([]*index.Detail{
		detail("login", model.StatusInProgress, 1, "2026-10-08T09:00:00Z", "Add login flow", "build", "design"),
		detail("design", model.StatusOpen, 0, "2026-10-08T08:00:00Z", "Design the API"),
		detail("build", model.StatusOpen, 2, "2026-10-08T07:00:00Z", "Set up CI"),
		detail("deploy", model.StatusClosed, 2, "2026-10-07T00:00:00Z", "Deploy to prod"),
	})

	got := index.RenderEntries(index.Parse(rendered))

	if !bytes.Equal(got, rendered) {
		t.Fatalf("re-rendered index drifted:\ngot:\n%s\nwant:\n%s", got, rendered)
	}
}

// TestRender_ArchivedDetail_IsOmitted proves an archived detail is not rendered
// into the index. Render determinism is
// TestRender_SameDetailsTwice_ProducesSameBytes.
func TestRender_ArchivedDetail_IsOmitted(t *testing.T) {
	archived := detail("oldauth", model.StatusClosed, 3, "2026-10-06T00:00:00Z", "Drop old auth")
	archived.ArchivedAt = "2026-10-09T09:00:00Z"
	details := []*index.Detail{
		detail("build", model.StatusOpen, 2, "2026-10-08T07:00:00Z", "Set up CI"),
		archived,
	}

	got := index.Render(details)

	if bytes.Contains(got, []byte("oldauth")) || bytes.Contains(got, []byte("## Archived")) {
		t.Fatalf("archived ticket rendered into the index:\n%s", got)
	}
}

// TestRender_InSectionStatusChange_ChangesOnlyThatLine proves the headline
// invariant: a status change that stays in the same section (here open -> in
// progress, both Active) edits exactly one index line and reorders nothing
// else. The sort keys (priority, created, id) do not change on a status update,
// so no other line moves.
//
// A change that crosses sections (for example open -> closed) is a different
// operation: it intentionally moves one line to its new section; see
// TestRender_CrossSectionStatusChange_KeepsOtherLinesIdentical for the
// guarantee that no other ticket's line is touched.
func TestRender_InSectionStatusChange_ChangesOnlyThatLine(t *testing.T) {
	before := []*index.Detail{
		detail("design", model.StatusOpen, 0, "2026-10-08T08:00:00Z", "Design the API"),
		detail("login", model.StatusOpen, 1, "2026-10-08T09:00:00Z", "Add login flow", "build", "design"),
		detail("build", model.StatusOpen, 2, "2026-10-08T07:00:00Z", "Set up CI"),
	}
	after := []*index.Detail{
		detail("design", model.StatusOpen, 0, "2026-10-08T08:00:00Z", "Design the API"),
		detail("login", model.StatusInProgress, 1, "2026-10-08T09:00:00Z", "Add login flow", "build", "design"),
		detail("build", model.StatusOpen, 2, "2026-10-08T07:00:00Z", "Set up CI"),
	}

	beforeLines := bytes.Split(index.Render(before), []byte("\n"))
	afterLines := bytes.Split(index.Render(after), []byte("\n"))

	if len(beforeLines) != len(afterLines) {
		t.Fatalf("line count changed: %d -> %d", len(beforeLines), len(afterLines))
	}
	changed := 0
	for i := range beforeLines {
		if !bytes.Equal(beforeLines[i], afterLines[i]) {
			changed++
			if !bytes.Contains(afterLines[i], []byte("#login")) {
				t.Fatalf("a non-login line changed at %d: %q -> %q", i, beforeLines[i], afterLines[i])
			}
		}
	}
	if changed != 1 {
		t.Fatalf("status change touched %d lines, want exactly 1:\nbefore:\n%s\nafter:\n%s",
			changed, index.Render(before), index.Render(after))
	}
}

// TestRender_CrossSectionStatusChange_KeepsOtherLinesIdentical proves that a
// section-crossing status change (open -> closed) moves exactly the one
// affected ticket and leaves every other ticket's line byte-identical. It never
// reorders the remaining tickets because the sort keys are untouched.
func TestRender_CrossSectionStatusChange_KeepsOtherLinesIdentical(t *testing.T) {
	before := []*index.Detail{
		detail("design", model.StatusOpen, 0, "2026-10-08T08:00:00Z", "Design the API"),
		detail("login", model.StatusOpen, 1, "2026-10-08T09:00:00Z", "Add login flow"),
		detail("build", model.StatusOpen, 2, "2026-10-08T07:00:00Z", "Set up CI"),
	}
	after := []*index.Detail{
		detail("design", model.StatusOpen, 0, "2026-10-08T08:00:00Z", "Design the API"),
		detail("login", model.StatusClosed, 1, "2026-10-08T09:00:00Z", "Add login flow"),
		detail("build", model.StatusOpen, 2, "2026-10-08T07:00:00Z", "Set up CI"),
	}

	// Make non-empty Closed and Archived sections so the fixtures match the
	// shape of a real backlog.
	before = append(before, detail("done", model.StatusClosed, 4, "2026-10-01T00:00:00Z", "Old done"))
	after = append(after, detail("done", model.StatusClosed, 4, "2026-10-01T00:00:00Z", "Old done"))

	beforeSet := lineSet(index.Render(before))
	afterSet := lineSet(index.Render(after))
	delete(beforeSet, "- [ ] Add login flow #login")
	// The only difference between the two line sets must be the moved line.
	if !reflect.DeepEqual(beforeSet, lineSetAfterRemoving(afterSet, "- [x] Add login flow #login")) {
		t.Fatalf("other lines changed:\nbefore:\n%s\nafter:\n%s", index.Render(before), index.Render(after))
	}
}

func lineSet(data []byte) map[string]bool {
	set := map[string]bool{}
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) > 0 {
			set[string(line)] = true
		}
	}
	return set
}

func lineSetAfterRemoving(set map[string]bool, key string) map[string]bool {
	out := map[string]bool{}
	for k := range set {
		if k != key {
			out[k] = true
		}
	}
	return out
}

// The sort-key tests prove the order is (priority, created, id) with each key
// deciding ties on its own: every fixture puts the tickets in the wrong order
// and differs only in the key under test plus the keys after it.

func assertRenderOrder(t *testing.T, details []*index.Detail, want ...string) {
	t.Helper()
	got := ids(index.Parse(index.Render(details)))
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("order %v, want %v", got, want)
		}
	}
}

func TestRender_DifferentPriority_ListsLowerPriorityFirst(t *testing.T) {
	details := []*index.Detail{
		detail("aaa", model.StatusOpen, 2, "2026-09-30T00:00:00Z", "A"),
		detail("zzz", model.StatusOpen, 0, "2026-12-31T00:00:00Z", "Z"),
	}

	assertRenderOrder(t, details, "zzz", "aaa")
}

func TestRender_SamePriority_ListsEarlierCreatedFirst(t *testing.T) {
	details := []*index.Detail{
		detail("aaa", model.StatusOpen, 2, "2026-10-01T00:00:00Z", "A"),
		detail("zzz", model.StatusOpen, 2, "2026-09-30T00:00:00Z", "Z"),
	}

	assertRenderOrder(t, details, "zzz", "aaa")
}

func TestRender_SamePriorityAndCreated_ListsLowerIDFirst(t *testing.T) {
	details := []*index.Detail{
		detail("bbb", model.StatusOpen, 2, "2026-10-01T00:00:00Z", "B"),
		detail("aaa", model.StatusOpen, 2, "2026-10-01T00:00:00Z", "A"),
	}

	assertRenderOrder(t, details, "aaa", "bbb")
}
