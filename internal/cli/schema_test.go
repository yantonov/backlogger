package cli

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// updateGolden regenerates the golden JSON files under testdata/json when set:
//
//	go test ./internal/cli -run TestJSON_EveryReadCommand_MatchesGolden -update
var updateGolden = flag.Bool("update", false, "rewrite the JSON schema golden files")

// givenSchemaFixture builds the deterministic backlog the JSON goldens are
// taken from (fixed clock, explicit IDs): four tickets exercising every status
// and the archive path, a dependency edge, and an attachment.
func givenSchemaFixture(t *testing.T) *testCLI {
	t.Helper()
	dir := t.TempDir()
	c := newCLI(t, dir)
	for _, args := range [][]string{
		{"add", "Alpha task", "--id", "alpha", "-p", "1", "-d", "First description"},
		{"add", "Beta task", "--id", "beta", "-p", "0"},
		{"dep", "add", "beta", "alpha"},
		{"start", "alpha"},
		{"add", "Gamma task", "--id", "gamma"},
		{"start", "gamma"},
		{"done", "gamma"},
		{"add", "Delta task", "--id", "delta"},
		{"start", "delta"},
		{"done", "delta"},
		{"archive", "delta"},
	} {
		c.must(t, args...)
	}
	src := filepath.Join(dir, "screenshot.png")
	if err := os.WriteFile(src, []byte("PNG"), 0o644); err != nil {
		t.Fatal(err)
	}
	c.must(t, "attach", "alpha", src)
	return c
}

// TestJSON_EveryReadCommand_MatchesGolden freezes the machine-readable shape of
// every read command: each `--json` output is compared byte-for-byte with a
// golden file. A change in field name, field order, or value therefore fails
// the suite and must be a deliberate golden update (spec 7). Each case builds
// its own fixture, so doctor fix cannot change what a later case reads.
func TestJSON_EveryReadCommand_MatchesGolden(t *testing.T) {
	cases := []struct {
		name string
		args []string
	}{
		{"list_default", []string{"list", "--json"}},
		{"list_status", []string{"list", "--status", "closed", "--json"}},
		{"list_archived", []string{"list", "--archived", "--json"}},
		{"list_all", []string{"list", "--all", "--json"}},
		{"started", []string{"started", "--json"}},
		{"blocked", []string{"blocked", "--json"}},
		{"available", []string{"available", "--json"}},
		{"closed", []string{"closed", "--json"}},
		{"dep_tree", []string{"dep", "tree", "beta", "--json"}},
		{"dep_cycle", []string{"dep", "cycle", "--json"}},
		{"dep_order", []string{"dep", "order", "--json"}},
		{"attachments", []string{"attachments", "alpha", "--json"}},
		{"doctor", []string{"doctor"}},
		{"doctor_fix", []string{"doctor", "fix"}},
		{"show", []string{"show", "alpha", "--json"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := givenSchemaFixture(t)

			got := []byte(c.must(t, tc.args...))

			if !json.Valid(got) {
				t.Fatalf("%v is not valid JSON:\n%s", tc.args, got)
			}
			golden := filepath.Join("testdata", "json", tc.name+".json")
			if *updateGolden {
				if err := os.MkdirAll(filepath.Dir(golden), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(golden, got, 0o644); err != nil {
					t.Fatal(err)
				}
				return
			}
			want, err := os.ReadFile(golden)
			if err != nil {
				t.Fatalf("read golden %s (run with -update to create): %v", golden, err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("%v JSON drifted from %s\ngot:\n%s\nwant:\n%s", tc.args, golden, got, want)
			}
		})
	}
}

// TestJSON_EveryCommand_CarriesVersionAndCommand proves every response carries
// the frozen version and the command name. It parses with generic maps
// because it asserts the envelope, not the per-command golden.
func TestJSON_EveryCommand_CarriesVersionAndCommand(t *testing.T) {
	for _, args := range [][]string{
		{"list", "--json"},
		{"show", "ver", "--json"},
		{"started", "--json"},
		{"blocked", "--json"},
		{"available", "--json"},
		{"closed", "--json"},
		{"dep", "tree", "ver", "--json"},
		{"dep", "cycle", "--json"},
		{"dep", "order", "--json"},
		{"attachments", "ver", "--json"},
		{"doctor"},
		{"doctor", "fix"},
	} {
		t.Run(strings.Join(args, "_"), func(t *testing.T) {
			c := newCLI(t, t.TempDir())
			c.must(t, "add", "V", "--id", "ver")

			out := c.must(t, args...)

			var generic map[string]any
			if err := json.Unmarshal([]byte(out), &generic); err != nil {
				t.Fatalf("not JSON: %v\n%s", err, out)
			}
			if v, ok := generic["version"].(float64); !ok || int(v) != jsonSchemaVersion {
				t.Fatalf("version field = %v, want %d", generic["version"], jsonSchemaVersion)
			}
			if _, ok := generic["command"].(string); !ok {
				t.Fatalf("missing command field: %s", out)
			}
		})
	}
}

// TestListJSON_OneTicket_EmitsArrayOfLeanItems proves list emits an array,
// never null, and each navigation item has exactly the lean fields: no
// priority, timestamps, attachments, or hash (spec 7).
func TestListJSON_OneTicket_EmitsArrayOfLeanItems(t *testing.T) {
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "V", "--id", "ver")

	out := c.must(t, "list", "--json")

	var list struct {
		Tickets []map[string]json.RawMessage `json:"tickets"`
	}
	if err := json.Unmarshal([]byte(out), &list); err != nil {
		t.Fatal(err)
	}
	if len(list.Tickets) != 1 {
		t.Fatalf("tickets = %d, want 1", len(list.Tickets))
	}
	for _, field := range []string{"id", "status", "title", "deps", "archived"} {
		if _, ok := list.Tickets[0][field]; !ok {
			t.Fatalf("ticket is missing required field %q: %s", field, out)
		}
	}
	for _, field := range []string{"priority", "created", "updated", "attachments", "hash"} {
		if _, ok := list.Tickets[0][field]; ok {
			t.Fatalf("navigation ticket must not carry detail field %q: %s", field, out)
		}
	}
}

// The hash tests prove the derived content hash in show --json is present,
// stable for an unchanged detail, and changes when the detail changes. It is
// the reserved --if-match token (spec 6.4).

// showHash returns the hash field of `show id --json`, failing when it is
// empty.
func showHash(t *testing.T, c *testCLI, id string) string {
	t.Helper()
	out := c.must(t, "show", id, "--json")
	var resp struct {
		Ticket struct {
			Hash string `json:"hash"`
		} `json:"ticket"`
	}
	if err := json.Unmarshal([]byte(out), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.Ticket.Hash == "" {
		t.Fatalf("show --json has no hash:\n%s", out)
	}
	return resp.Ticket.Hash
}

func TestShowJSON_UnchangedDetail_RepeatsHash(t *testing.T) {
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "Hash me", "--id", "hashme")
	first := showHash(t, c, "hashme")

	second := showHash(t, c, "hashme")

	if first != second {
		t.Fatalf("hash changed for an unchanged detail: %s != %s", first, second)
	}
}

func TestShowJSON_EditedDetail_ChangesHash(t *testing.T) {
	c := newCLI(t, t.TempDir())
	c.must(t, "add", "Hash me", "--id", "hashme")
	before := showHash(t, c, "hashme")
	c.must(t, "edit", "hashme", "--note", "changed")

	after := showHash(t, c, "hashme")

	if after == before {
		t.Fatalf("hash did not change after a detail change: %s", after)
	}
}

// TestShowJSON_OneTicket_CarriesFullDetailFields proves `show --json` is the
// one detail read path: it still carries priority, created, updated, and hash,
// unlike the lean navigation views (spec 7).
func TestShowJSON_OneTicket_CarriesFullDetailFields(t *testing.T) {
	dir := t.TempDir()
	app, _, stderr := newTestApp(t, dir)
	mustRunApp(t, app, stderr, []string{"add", "Full", "--id", "full", "-p", "1"})

	var stdout bytes.Buffer
	app.Stdout = &stdout
	if code := app.Run([]string{"show", "full", "--json"}); code != exitOK {
		t.Fatalf("show --json exit %d: %s", code, stderr.String())
	}
	var resp struct {
		Ticket map[string]json.RawMessage `json:"ticket"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"id", "title", "status", "priority", "created", "hash"} {
		if _, ok := resp.Ticket[field]; !ok {
			t.Fatalf("show ticket is missing full field %q: %s", field, stdout.Bytes())
		}
	}
}

func mustRunApp(t *testing.T, app *App, stderr *bytes.Buffer, args ...[]string) {
	t.Helper()
	for _, argv := range args {
		stderr.Reset()
		if code := app.Run(argv); code != exitOK {
			t.Fatalf("%v exit %d: %s", argv, code, stderr.String())
		}
	}
}
