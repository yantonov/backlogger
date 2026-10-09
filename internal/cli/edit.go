package cli

import (
	"fmt"
	"strings"

	"github.com/yantonov/backlogger/internal/index"
	"github.com/yantonov/backlogger/internal/model"
	"github.com/yantonov/backlogger/internal/store"
)

// cmdEdit implements `bl edit <id>`: it edits structured fields and/or the
// Markdown body, last-writer-wins, preserving unknown frontmatter keys (the
// frontmatter package already writes them back verbatim). Section flags replace
// their section; --note appends to Notes.
func (a *App) cmdEdit(args []string) int {
	fs := newFlagSet()
	priorityFlag := fs.Int(model.DefaultPriority, "-p", "--priority")
	descFlag := fs.String("", "--description", "-d")
	notesFlag := fs.String("", "--notes")
	noteFlag := fs.String("", "--note")
	bodyFlag := fs.String("", "--body")
	titleFlag := fs.String("", "--title")
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	positionals := fs.positionals()
	if len(positionals) != 1 {
		return a.usageError("edit needs exactly one ticket id: bl edit <id> [fields...]")
	}

	edit := editRequest{
		setPriority: fs.Set("-p"),
		priority:    *priorityFlag,
		setTitle:    fs.Set("--title"),
		title:       *titleFlag,
		setBody:     fs.Set("--body"),
		body:        *bodyFlag,
		setDesc:     fs.Set("--description"),
		desc:        *descFlag,
		setNotes:    fs.Set("--notes"),
		notes:       *notesFlag,
		setNote:     fs.Set("--note"),
		note:        *noteFlag,
	}
	if !edit.any() {
		return a.usageError("edit needs at least one field flag")
	}
	if edit.setPriority && (edit.priority < model.MinPriority || edit.priority > model.MaxPriority) {
		return a.usageError(fmt.Sprintf("priority %d is out of range %d-%d",
			edit.priority, model.MinPriority, model.MaxPriority))
	}

	paths, err := a.discover()
	if err != nil {
		return a.fail(err)
	}
	if err := a.requireTickets(paths); err != nil {
		return a.fail(err)
	}
	id, err := resolveID(paths, positionals[0])
	if err != nil {
		return a.fail(err)
	}

	tx, err := store.Begin(paths, store.TxOptions{Reindex: index.ReindexFunc()})
	if err != nil {
		return a.fail(err)
	}
	defer func() { _ = tx.Rollback() }()

	now := a.now()
	if _, err := tx.Update(id, nil, edit.mutate, now); err != nil {
		return a.fail(err)
	}
	if err := tx.Commit(); err != nil {
		return a.fail(err)
	}

	fmt.Fprintf(a.Stdout, "edited %s\n", id)
	return exitOK
}

// editRequest holds which fields the user asked to change and their values.
type editRequest struct {
	setPriority bool
	priority    int
	setTitle    bool
	title       string
	setBody     bool
	body        string
	setDesc     bool
	desc        string
	setNotes    bool
	notes       string
	setNote     bool
	note        string
}

func (e editRequest) any() bool {
	return e.setPriority || e.setTitle || e.setBody ||
		e.setDesc || e.setNotes || e.setNote
}

// mutate applies the request to a ticket. It runs inside the lock, after the
// authoritative read.
func (e editRequest) mutate(meta *store.TicketMeta) error {
	if e.setPriority {
		meta.Ticket.Priority = e.priority
	}
	if e.setBody {
		meta.Body = []byte(e.body)
	}
	if e.setTitle {
		meta.Body = setBodyTitle(meta.Body, e.title)
	}
	if e.setDesc {
		meta.Body = setSection(meta.Body, sectionDescription, e.desc, false)
	}
	if e.setNotes {
		meta.Body = setSection(meta.Body, sectionNotes, e.notes, false)
	}
	if e.setNote {
		meta.Body = appendNote(meta.Body, e.note)
	}
	meta.Body = ensureHeading(meta.Body, meta.ID)
	return nil
}

// appendNote appends one note entry under the Notes section. Notes are
// timestamp-free in the stored body (the serializer never invents timestamps);
// entries are separated by a blank line.
func appendNote(body []byte, note string) []byte {
	return setSection(body, sectionNotes, strings.TrimRight(note, "\n"), true)
}
