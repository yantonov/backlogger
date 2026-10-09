package cli

import (
	"fmt"
	"strings"

	"github.com/yantonov/backlogger/internal/ids"
	"github.com/yantonov/backlogger/internal/index"
	"github.com/yantonov/backlogger/internal/model"
	"github.com/yantonov/backlogger/internal/store"
)

// cmdAdd implements `bl add <title>`: it creates .tickets/ when missing (the
// only command that creates state), generates a unique ID unless one is given,
// writes the detail file, and regenerates the index in the same lock
// (spec 5.1/7.1).
func (a *App) cmdAdd(args []string) int {
	fs := newFlagSet()
	idFlag := fs.String("", "--id")
	descFlag := fs.String("", "-d", "--description")
	priorityFlag := fs.Int(model.DefaultPriority, "-p", "--priority")
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}

	positionals := fs.positionals()
	if len(positionals) == 0 {
		return a.usageError("add needs a title: bl add <title> [flags]")
	}
	title := strings.Join(positionals, " ")
	if strings.TrimSpace(title) == "" {
		return a.usageError("add needs a non-empty title")
	}
	if *priorityFlag < model.MinPriority || *priorityFlag > model.MaxPriority {
		return a.usageError(fmt.Sprintf("priority %d is out of range %d-%d",
			*priorityFlag, model.MinPriority, model.MaxPriority))
	}

	paths, err := a.discover()
	if err != nil {
		return a.fail(err)
	}

	tx, err := store.Begin(paths, store.TxOptions{
		Create:  true,
		Reindex: index.ReindexFunc(),
	})
	if err != nil {
		return a.fail(err)
	}
	defer func() { _ = tx.Rollback() }()

	exists := idExists(paths)
	var id string
	if fs.Set("--id") {
		if err := ids.CheckAvailable(*idFlag, exists); err != nil {
			return a.fail(err)
		}
		id, err = ids.Normalize(*idFlag)
		if err != nil {
			return a.fail(err)
		}
	} else {
		id, err = ids.Generate(exists, a.Rand)
		if err != nil {
			return a.fail(err)
		}
	}

	now := a.now()
	ticket := model.Ticket{
		Status:   model.StatusOpen,
		Priority: *priorityFlag,
		Created:  model.FormatTimestamp(now),
		Updated:  model.FormatTimestamp(now),
	}
	body := newBody(title, []bodySection{
		{Name: sectionDescription, Text: *descFlag},
	})

	meta := &store.TicketMeta{ID: id, Ticket: ticket, Body: body}
	if err := tx.WriteDetail(meta); err != nil {
		return a.fail(err)
	}
	if err := tx.Commit(); err != nil {
		return a.fail(err)
	}

	fmt.Fprintf(a.Stdout, "added %s\n", id)
	return exitOK
}
