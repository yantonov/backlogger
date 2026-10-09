package cli

import (
	"fmt"

	"github.com/yantonov/backlogger/internal/attach"
	"github.com/yantonov/backlogger/internal/frontmatter"
	"github.com/yantonov/backlogger/internal/ids"
	"github.com/yantonov/backlogger/internal/store"
)

func serializeDocument(meta *store.TicketMeta) []byte {
	return frontmatter.Serialize(&frontmatter.Document{Ticket: meta.Ticket, Body: meta.Body})
}

// cmdShow implements `bl show <id>`: it resolves a partial ID and prints the
// authoritative detail file verbatim (spec 3.1/7). --attachments appends the
// attachment block, and --json emits the basic machine-readable form from
// render_json.go.
func (a *App) cmdShow(args []string) int {
	fs := newFlagSet()
	jsonFlag := fs.Bool("--json")
	attachmentsFlag := fs.Bool("--attachments")
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	positionals := fs.positionals()
	if len(positionals) != 1 {
		return a.usageError("show needs exactly one ticket id: bl show <id>")
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
	meta, err := paths.ReadDetail(id)
	if err != nil {
		return a.fail(err)
	}

	if *jsonFlag {
		return a.renderShowJSON(meta)
	}
	if _, err := a.Stdout.Write(serializeMeta(meta)); err != nil {
		return a.fail(err)
	}
	if *attachmentsFlag {
		entries, err := attach.Entries(paths.AttachmentsDir, id, meta.Ticket.Attachments)
		if err != nil {
			return a.fail(err)
		}
		issues, err := attach.CheckLinks(paths.AttachmentsDir, id, meta.Ticket.Attachments)
		if err != nil {
			return a.fail(err)
		}
		if _, err := fmt.Fprint(a.Stdout, renderAttachmentSection(entries, issues)); err != nil {
			return a.fail(err)
		}
	}
	return exitOK
}

// resolveID maps an operand to a stored ticket ID using partial matching
// (spec 5.1). It lists the identity of every detail file (active and archived)
// without opening any of them, then delegates to ids.Resolve, so an exact match
// wins over a substring and an ambiguity lists the candidates. Resolution never
// needs the archive contents, only the archive file names.
func resolveID(paths *store.Paths, operand string) (string, error) {
	candidates, err := paths.CandidateIDs()
	if err != nil {
		return "", err
	}
	return ids.Resolve(candidates, operand)
}

// serializeMeta renders a detail file in canonical form. It is what `show`
// prints and what an edit comparison is based on.
func serializeMeta(meta *store.TicketMeta) []byte {
	return serializeDocument(meta)
}
