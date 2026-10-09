package cli

import (
	"fmt"

	"github.com/yantonov/backlogger/internal/doctor"
)

// cmdDoctor implements `bl doctor`: it validates every backlog invariant and
// prints the findings as JSON. It always exits nonzero when there is anything
// to fix (an error or a warning) and zero on a clean repo, so a script can gate
// on the exit code without parsing the payload (spec 6.4/7).
//
// `bl doctor fix` (see cmdDoctorFix) is the repair counterpart.
func (a *App) cmdDoctor(args []string) int {
	fs := newFlagSet()
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	if extra := fs.positionals(); len(extra) > 0 {
		return a.usageError(fmt.Sprintf("doctor takes no positional arguments: %v", extra))
	}

	paths, err := a.discover()
	if err != nil {
		return a.fail(err)
	}
	if err := a.requireTickets(paths); err != nil {
		return a.fail(err)
	}

	report, err := doctor.Run(paths)
	if err != nil {
		return a.fail(err)
	}
	a.renderDoctorJSON(report)
	if len(report.Issues) > 0 {
		return exitError
	}
	return exitOK
}

// cmdDoctorFix implements `bl doctor fix`: it repairs every finding that can be
// repaired without changing ticket identity, prints the applied repairs and the
// remaining findings as JSON, and exits nonzero when anything is still broken.
// It never requires a dry run; git history is the undo (spec 6.4/7).
func (a *App) cmdDoctorFix(args []string) int {
	fs := newFlagSet()
	if err := fs.Parse(args); err != nil {
		return a.usageError(err.Error())
	}
	if extra := fs.positionals(); len(extra) > 0 {
		return a.usageError(fmt.Sprintf("doctor fix takes no positional arguments: %v", extra))
	}

	paths, err := a.discover()
	if err != nil {
		return a.fail(err)
	}
	if err := a.requireTickets(paths); err != nil {
		return a.fail(err)
	}

	fixed, err := doctor.Fix(paths)
	if err != nil {
		return a.fail(err)
	}

	// Re-inspect so the report reflects the repaired state: the index was just
	// regenerated and the remaining findings are what a human must resolve.
	report, err := doctor.Run(paths)
	if err != nil {
		return a.fail(err)
	}
	a.renderDoctorFixJSON(fixed, report)
	if len(report.Issues) > 0 {
		return exitError
	}
	return exitOK
}
