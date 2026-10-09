package cli

import (
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/yantonov/backlogger/internal/store"
)

// usage is the top-level help text. It lists the command groups from the
// design spec section 7.
const usage = `bl - local-first task tracker

usage: bl <command> [flags]

Core
  bl list [--status S] [--archived] [--all] [--json] [-v|-vv]
  bl add <title> [--id ID] [-d desc] [-p prio]
  bl show <id> [--attachments] [--json]
  bl edit <id> [-p prio] [--title TEXT] [--description TEXT]
                [--notes TEXT] [--note TEXT] [--body TEXT]
  bl start <id>                Atomically claim (open -> in progress)
  bl done <id> / reopen <id>

Dependencies
  bl dep add <id> <dep>    A depends on B
  bl dep rm  <id> <dep>
  bl dep tree [--full] <id> [--json]
  bl dep cycle [--json]
  bl dep order [--json] [-v|-vv]

Views
  bl started [--json] [-v|-vv]   in progress (claimed)
  bl blocked [--json] [-v|-vv]   active, some dep open
  bl available [--json] [-v|-vv] open AND deps satisfied (claimable)
  bl closed [--json] [-v|-vv]    the Closed section in committed order

Archive
  bl archive <id>... [--dry-run]        archive closed tickets
  bl archive --closed-older-than D [--dry-run]
  bl unarchive <id>             clear archived state, move file back

Attachments
  bl attach <id> <file...>      copy file into .tickets/attachments/
  bl detach <id> <file>         remove an attachment
  bl attachments <id> [--json]  list a ticket's attachments

Maintenance
  bl doctor                   Check invariants (JSON)
  bl doctor fix               Repair every safe finding, then report
  bl reindex                  Regenerate backlog.md from the detail files
  bl version                  Print the version
  bl help                     Print usage
`

// Exit codes. Any error is nonzero; usage errors use 2 so scripts can tell a
// mistyped command from a runtime failure.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

// App holds the dependencies of every command handler. It is deliberately a
// plain struct so tests can drive the CLI in-process with a chosen working
// directory, output writers, clock, and randomness source.
type App struct {
	// Stdout receives normal command output.
	Stdout io.Writer
	// Stderr receives error messages and usage.
	Stderr io.Writer
	// Dir is the directory discovery starts from. NewApp uses the process
	// working directory.
	Dir string
	// Now returns the current time. It is injectable so tests get deterministic
	// timestamps.
	Now func() time.Time
	// Rand supplies randomness for ID generation. A nil value means crypto/rand
	// (see ids.Generate).
	Rand io.Reader
}

// NewApp returns an App wired to the process environment. It never fails; a
// missing working directory falls back to ".".
func NewApp() *App {
	wd, err := os.Getwd()
	if err != nil {
		wd = "."
	}
	return &App{
		Stdout: os.Stdout,
		Stderr: os.Stderr,
		Dir:    wd,
		Now:    time.Now,
	}
}

// Run dispatches a command line and returns the process exit code. No
// arguments and the help command print usage and succeed; an unknown command
// prints usage and fails (spec 7.1).
func (a *App) Run(args []string) int {
	if len(args) == 0 {
		fmt.Fprint(a.Stdout, usage)
		return exitOK
	}
	cmd, rest := args[0], args[1:]
	switch cmd {
	case "help", "-h", "--help":
		fmt.Fprint(a.Stdout, usage)
		return exitOK
	case "add":
		return a.cmdAdd(rest)
	case "list":
		return a.cmdList(rest)
	case "show":
		return a.cmdShow(rest)
	case "edit":
		return a.cmdEdit(rest)
	case "start":
		return a.cmdTransition(rest, transitionStart)
	case "done":
		return a.cmdTransition(rest, transitionDone)
	case "reopen":
		return a.cmdTransition(rest, transitionReopen)
	case "dep":
		return a.cmdDep(rest)
	case "started":
		return a.cmdStarted(rest)
	case "blocked":
		return a.cmdBlocked(rest)
	case "available":
		return a.cmdAvailable(rest)
	case "closed":
		return a.cmdClosed(rest)
	case "archive":
		return a.cmdArchive(rest)
	case "unarchive":
		return a.cmdUnarchive(rest)
	case "attach":
		return a.cmdAttach(rest)
	case "detach":
		return a.cmdDetach(rest)
	case "attachments":
		return a.cmdAttachments(rest)
	case "doctor":
		if len(rest) > 0 && rest[0] == "fix" {
			return a.cmdDoctorFix(rest[1:])
		}
		return a.cmdDoctor(rest)
	case "reindex":
		return a.cmdReindex(rest)
	case "version":
		return a.cmdVersion(rest)
	default:
		fmt.Fprintf(a.Stderr, "bl: unknown command %q\n\n", cmd)
		fmt.Fprint(a.Stderr, usage)
		return exitUsage
	}
}

func (a *App) now() time.Time {
	if a.Now != nil {
		return a.Now()
	}
	return time.Now()
}

func (a *App) discover() (*store.Paths, error) {
	dir := a.Dir
	if dir == "" {
		dir = "."
	}
	return store.Discover(dir)
}

func (a *App) fail(err error) int {
	fmt.Fprintf(a.Stderr, "bl: %v\n", err)
	return exitError
}

func (a *App) usageError(msg string) int {
	fmt.Fprintf(a.Stderr, "bl: %s\n", msg)
	return exitUsage
}

// requireTickets enforces the read-vs-write rule: a missing .tickets/ is an
// error with guidance for every read command (spec 7.1).
func (a *App) requireTickets(paths *store.Paths) error {
	return paths.RequireTickets()
}

// idExists returns a predicate that reports whether an ID is already taken in
// either the active or the archive directory. It is called inside the lock.
func idExists(paths *store.Paths) func(string) (bool, error) {
	return func(id string) (bool, error) {
		for _, path := range []string{paths.ActivePath(id), paths.ArchivedPath(id)} {
			if _, err := os.Stat(path); err == nil {
				return true, nil
			} else if !os.IsNotExist(err) {
				return false, fmt.Errorf("stat %s: %w", path, err)
			}
		}
		return false, nil
	}
}

// ---------------------------------------------------------------------------
// Flag parsing
//
// The standard library flag package stops at the first positional argument, but
// the spec places flags after the title/ID (for example `bl add "T" -p 1`).
// This small parser accepts flags and positionals in any order and supports
// `--flag value`, `--flag=value`, and single-dash forms.
// ---------------------------------------------------------------------------

type flagKind int

const (
	flagBool flagKind = iota
	flagString
	flagInt
)

type flagDef struct {
	kind    flagKind
	set     bool
	boolVal *bool
	strVal  *string
	intVal  *int
}

type flagSet struct {
	byName map[string]*flagDef
	args   []string
}

func newFlagSet() *flagSet {
	return &flagSet{byName: make(map[string]*flagDef)}
}

func (f *flagSet) register(def *flagDef, names ...string) {
	for _, n := range names {
		f.byName[n] = def
	}
}

// Bool registers a boolean flag under every given name.
func (f *flagSet) Bool(names ...string) *bool {
	v := new(bool)
	f.register(&flagDef{kind: flagBool, boolVal: v}, names...)
	return v
}

// String registers a string flag. defaultValue is the value before parsing.
func (f *flagSet) String(defaultValue string, names ...string) *string {
	v := new(string)
	*v = defaultValue
	f.register(&flagDef{kind: flagString, strVal: v}, names...)
	return v
}

// Int registers an integer flag. defaultValue is the value before parsing.
func (f *flagSet) Int(defaultValue int, names ...string) *int {
	v := new(int)
	*v = defaultValue
	f.register(&flagDef{kind: flagInt, intVal: v}, names...)
	return v
}

// Parse scans args, filling registered flags and collecting positionals. A
// "--" terminates flag parsing. Unknown flags and missing values are errors.
func (f *flagSet) Parse(args []string) error {
	i := 0
	for i < len(args) {
		arg := args[i]
		if arg == "--" {
			f.args = append(f.args, args[i+1:]...)
			break
		}
		if len(arg) > 1 && arg[0] == '-' {
			name := arg
			value := ""
			hasValue := false
			if eq := strings.IndexByte(arg, '='); eq >= 0 {
				name = arg[:eq]
				value = arg[eq+1:]
				hasValue = true
			}
			def, ok := f.byName[name]
			if !ok {
				return fmt.Errorf("unknown flag %q", name)
			}
			switch def.kind {
			case flagBool:
				if hasValue {
					b, err := strconv.ParseBool(value)
					if err != nil {
						return fmt.Errorf("flag %s: %v", name, err)
					}
					*def.boolVal = b
				} else {
					*def.boolVal = true
				}
				def.set = true
			case flagString:
				if !hasValue {
					i++
					if i >= len(args) {
						return fmt.Errorf("flag %s needs a value", name)
					}
					value = args[i]
				}
				*def.strVal = value
				def.set = true
			case flagInt:
				if !hasValue {
					i++
					if i >= len(args) {
						return fmt.Errorf("flag %s needs a value", name)
					}
					value = args[i]
				}
				n, err := strconv.Atoi(value)
				if err != nil {
					return fmt.Errorf("flag %s: %q is not an integer", name, value)
				}
				*def.intVal = n
				def.set = true
			}
		} else {
			f.args = append(f.args, arg)
		}
		i++
	}
	return nil
}

func (f *flagSet) positionals() []string { return f.args }

// Set reports whether the named flag was present on the command line. name is
// any registered alias.
func (f *flagSet) Set(name string) bool {
	def, ok := f.byName[name]
	return ok && def.set
}
