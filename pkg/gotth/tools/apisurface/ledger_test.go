package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// These are table-driven standard-library tests rather than Ginkgo specs, and
// the reason is the module rather than the style. tools/ exists so that a
// minifier's dependencies cannot reach a consumer's go.mod, and readLedger is a
// pure function from markdown text to two pairs of integers — there is no
// behaviour here that reads better as prose than as a table of inputs and
// expected outcomes. Adding Ginkgo and Gomega to this module to describe
// "given a table with three columns, it errors" would buy nothing and would
// widen the graph the module's own doc comment exists to keep narrow.

// ledger renders a §0 counts table with the given rows, wrapped in enough
// surrounding document to be realistic.
func ledger(rows ...string) string {
	return strings.Join(append([]string{
		"# a ledger",
		"",
		"## 0. The rule this document enforces",
		"",
		"**Counts.** This table is the FR-65 baseline.",
		"",
		"| | `live` (exact) | `live/livetest` (ceiling) |",
		"|---|---:|---:|",
	}, rows...), "\n") + "\n\n## 1. Something else\n"
}

const (
	identifierRow = "| Exported identifiers (types, funcs, methods, consts, vars) | **45** | 8 |"
	fieldRow      = "| Exported struct fields | **49** | 6 |"
)

func writeLedger(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "api-surface.md")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("writing the scratch ledger: %v", err)
	}
	return path
}

func TestReadLedgerAcceptsTheDocumentedShape(t *testing.T) {
	got, err := readLedger(writeLedger(t, ledger(identifierRow, fieldRow)))
	if err != nil {
		t.Fatalf("the documented shape was refused: %v", err)
	}

	want := map[string]counts{
		"live":          {Identifiers: 45, Fields: 49},
		"live/livetest": {Identifiers: 8, Fields: 6},
	}
	for pkg, w := range want {
		if got[pkg] != w {
			t.Errorf("%s: read %+v, want %+v", pkg, got[pkg], w)
		}
	}
}

// The regression this reader was rewritten for.
//
// Every case below was accepted silently by the previous reader, which matched
// two label patterns anywhere in the file and took the first two numeric cells
// of each. A third column, a third row, a second table and a non-numeric cell
// were all invisible to it — so a cell nobody read could hold any number, which
// L9-1 proved by writing 9001 into one and watching CI report that the surface
// matched the ledger.
func TestReadLedgerRefusesWhatItDoesNotRead(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
		wants   string
	}{
		{
			name: "a derived total column",
			content: strings.Join([]string{
				"| | `live` (exact) | `live/livetest` (ceiling) | total |",
				"|---|---:|---:|---:|",
				"| Exported identifiers (types, funcs, methods, consts, vars) | **45** | 8 | **9001** |",
				"| Exported struct fields | **49** | 6 | **55** |",
			}, "\n"),
			wants: "it has 4 cells and this tool reads 3",
		},
		{
			name:    "a derived total column on the rows alone",
			content: ledger(identifierRow+" **9001** |", fieldRow+" 55 |"),
			wants:   "has 4 cells and this tool reads 3",
		},
		{
			name:    "a derived total row",
			content: ledger(identifierRow, fieldRow, "| **Total incl. fields** | **94** | 14 |"),
			wants:   "has 3 data rows and this tool reads 2",
		},
		{
			name:    "a missing row",
			content: ledger(identifierRow),
			wants:   "has 1 data rows and this tool reads 2",
		},
		{
			name:    "the rows in the wrong order",
			content: ledger(fieldRow, identifierRow),
			wants:   "is labelled",
		},
		{
			name:    "a cell that is not a number",
			content: ledger("| Exported identifiers | forty-five | 8 |", fieldRow),
			wants:   "is not a count",
		},
		{
			name:    "no table at all",
			content: "# a ledger\n\nnothing here counts anything.\n",
			wants:   "no counts table found",
		},
		{
			name:    "two counts tables",
			content: ledger(identifierRow, fieldRow) + ledger(identifierRow, fieldRow),
			wants:   "2 counts tables found",
		},
		{
			name: "a header with no separator under it",
			content: strings.Join([]string{
				"| | `live` (exact) | `live/livetest` (ceiling) |",
				identifierRow,
				fieldRow,
			}, "\n"),
			wants: "is not followed by a markdown separator row",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := readLedger(writeLedger(t, tc.content))
			if err == nil {
				t.Fatalf("accepted a ledger it does not read: a cell or row nobody checks can hold any number")
			}
			if !strings.Contains(err.Error(), tc.wants) {
				t.Errorf("error does not say what is wrong\n got: %v\nwant it to contain: %q", err, tc.wants)
			}
		})
	}
}

// The ledger CI actually reads, parsed by the code that reads it. A shape
// change to §0 that this tool cannot follow is a change that must fail here
// rather than in a CI log a week later.
func TestReadLedgerReadsTheCommittedLedger(t *testing.T) {
	got, err := readLedger(filepath.Join("..", "..", "docs", "api-surface.md"))
	if err != nil {
		t.Fatalf("the committed ledger is unreadable: %v", err)
	}
	if got["live"].Identifiers == 0 || got["live"].Fields == 0 {
		t.Errorf("the committed ledger parsed to zero counts for live: %+v", got["live"])
	}
}
