// Command apisurface counts the library's exported surface and holds it
// against the ledger.
//
// The requirement is that CI reports the exported-identifier count delta on
// every change, and the failure it exists to catch is an exported identifier
// arriving without a row in docs/api-surface.md. Exported symbols are
// permanent; the ledger is where the argument for each one is written down,
// and a count that nothing checks is a count that drifts.
//
// # What is counted
//
// Types, funcs, methods, consts and vars declared at package level and
// exported, plus the exported fields of exported structs, in the two packages
// the module exports. Interface methods are not counted separately: the ledger
// gives an interface one row and describes its method in that row, so counting
// the method again would report a delta the ledger cannot express.
//
// # Why the two packages are checked differently
//
// live is complete, so its measured counts must equal the ledger's exactly and
// any difference in either direction fails.
//
// live/livetest is deliberately partial — the ledger describes the v0.1 target
// surface and most of its symbols wait on the benchmark harness that fixes
// their shape. Requiring equality there would mean either a red build for a
// year or a ledger that lies about the target. So growth is what fails:
// measured may be below the ledger and may never exceed it, because exceeding
// it is exactly the "identifier without a row" case.
//
// # Why the ledger's table is read whole
//
// This tool refuses a counts table containing a cell or a row it does not read,
// rather than skipping it. The first version matched two label patterns
// anywhere in the file and consumed the first two numeric cells of each, so the
// table's shape was never its business: a third column could hold any number
// and the tool still reported that the surface matched the ledger. That is the
// hand-maintained-number failure this program exists to end, reproduced inside
// the program. Reading the table whole means the only way to add a cell is to
// decide what reads it.
//
// Usage:
//
//	go run ./apisurface            # report the counts and check them
//	go run ./apisurface -report    # report only, exit 0
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
)

// counts is one package's measured or ledgered surface.
type counts struct {
	Identifiers int
	Fields      int
}

func (c counts) total() int { return c.Identifiers + c.Fields }

func main() {
	reportOnly := flag.Bool("report", false, "print the counts and exit 0")
	root := flag.String("root", "..", "path to the gotth-live module root")
	flag.Parse()

	ledgerPath := filepath.Join(*root, "docs", "api-surface.md")
	ledger, err := readLedger(ledgerPath)
	if err != nil {
		fail("reading the ledger: %v", err)
	}

	measured := map[string]counts{}
	for _, pkg := range []string{"live", "live/livetest"} {
		c, err := measure(filepath.Join(*root, filepath.FromSlash(pkg)))
		if err != nil {
			fail("measuring %s: %v", pkg, err)
		}
		measured[pkg] = c
	}

	fmt.Println("exported surface (FR-65)")
	fmt.Printf("  %-16s %11s %11s %11s\n", "package", "identifiers", "fields", "total")
	var ok = true
	for _, pkg := range []string{"live", "live/livetest"} {
		got, want := measured[pkg], ledger[pkg]
		fmt.Printf("  %-16s %5d/%-5d %5d/%-5d %5d/%-5d   (measured/ledger)\n",
			pkg, got.Identifiers, want.Identifiers, got.Fields, want.Fields, got.total(), want.total())

		switch pkg {
		case "live":
			if got != want {
				ok = false
				fmt.Fprintf(os.Stderr,
					"\n%s: the exported surface does not match docs/api-surface.md.\n"+
						"  measured %d identifiers and %d struct fields; the ledger says %d and %d.\n"+
						"  Every exported identifier needs a row and a requirement it satisfies.\n"+
						"  Update the ledger in the same commit, or drop the export.\n",
					pkg, got.Identifiers, got.Fields, want.Identifiers, want.Fields)
			}
		default:
			if got.Identifiers > want.Identifiers || got.Fields > want.Fields {
				ok = false
				fmt.Fprintf(os.Stderr,
					"\n%s: the exported surface has grown past the ledger's target.\n"+
						"  measured %d identifiers and %d struct fields; the ledger's target is %d and %d.\n",
					pkg, got.Identifiers, got.Fields, want.Identifiers, want.Fields)
			}
		}
	}

	total := counts{}
	for _, c := range measured {
		total.Identifiers += c.Identifiers
		total.Fields += c.Fields
	}
	fmt.Printf("  %-16s %5d       %5d       %5d\n", "total", total.Identifiers, total.Fields, total.total())

	if *reportOnly {
		return
	}
	if !ok {
		os.Exit(1)
	}
	fmt.Println("  the surface matches the ledger")
}

func fail(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "apisurface: "+format+"\n", args...)
	os.Exit(2)
}

// measure counts one package's exported surface from its source.
//
// It parses rather than builds, so it needs no toolchain beyond the parser and
// works on a package that does not compile — which matters, because the most
// likely moment to run this is while a change is in progress.
func measure(dir string) (counts, error) {
	fset := token.NewFileSet()
	pkgs, err := parser.ParseDir(fset, dir, func(fi os.FileInfo) bool {
		return !strings.HasSuffix(fi.Name(), "_test.go")
	}, 0)
	if err != nil {
		return counts{}, err
	}

	var c counts
	for name, pkg := range pkgs {
		if strings.HasSuffix(name, "_test") {
			continue
		}
		for _, file := range pkg.Files {
			for _, decl := range file.Decls {
				switch d := decl.(type) {
				case *ast.FuncDecl:
					c.Identifiers += countFunc(d)
				case *ast.GenDecl:
					ids, fields := countGen(d)
					c.Identifiers += ids
					c.Fields += fields
				}
			}
		}
	}
	return c, nil
}

func countFunc(d *ast.FuncDecl) int {
	if !d.Name.IsExported() {
		return 0
	}
	if d.Recv == nil {
		return 1
	}
	// A method on an unexported type is not reachable by a consumer, so it is
	// not surface.
	if !exportedReceiver(d.Recv) {
		return 0
	}
	return 1
}

func exportedReceiver(recv *ast.FieldList) bool {
	if len(recv.List) == 0 {
		return false
	}
	expr := recv.List[0].Type
	if star, isPointer := expr.(*ast.StarExpr); isPointer {
		expr = star.X
	}
	// A generic receiver is written App[S], so the name is under the index.
	if index, generic := expr.(*ast.IndexExpr); generic {
		expr = index.X
	}
	if index, generic := expr.(*ast.IndexListExpr); generic {
		expr = index.X
	}
	ident, named := expr.(*ast.Ident)
	return named && ident.IsExported()
}

func countGen(d *ast.GenDecl) (identifiers, fields int) {
	for _, spec := range d.Specs {
		switch s := spec.(type) {
		case *ast.TypeSpec:
			if !s.Name.IsExported() {
				continue
			}
			identifiers++
			if st, isStruct := s.Type.(*ast.StructType); isStruct {
				fields += countFields(st)
			}
		case *ast.ValueSpec:
			for _, name := range s.Names {
				if name.IsExported() {
					identifiers++
				}
			}
		}
	}
	return identifiers, fields
}

func countFields(st *ast.StructType) int {
	n := 0
	for _, field := range st.Fields.List {
		// An embedded field has no names; the module has none in its exported
		// structs, and counting one would need a rule the ledger does not have.
		for _, name := range field.Names {
			if name.IsExported() {
				n++
			}
		}
	}
	return n
}

// countsHeader anchors the counts table in the ledger's §0.
//
// The table is located by its header rather than by a row pattern, and that is
// deliberate. The previous reader matched two label patterns anywhere in the
// file and took the first two numeric cells of each, which meant it consumed a
// prefix of a table whose shape it never checked: a third column could hold any
// number at all and the tool reported that the surface matched the ledger.
// L9-1 demonstrated it by writing 9001 into that column and watching CI pass —
// the precise failure this tool was written to end, reproduced inside it.
//
// Anchoring on the header makes the whole table this reader's business. It then
// refuses anything it does not read, so a cell nobody checks cannot exist.
// The anchor is deliberately loose — it finds the table, and the checks below
// decide whether its shape is one this tool reads. A strict anchor would report
// an added column as "no counts table found", which sends the reader looking
// for the wrong mistake.
var countsHeader = regexp.MustCompile("`live` +\\(exact\\).*`live/livetest` +\\(ceiling\\)")

// countsColumns are the header's cells, after the empty label cell.
var countsColumns = []string{"`live` (exact)", "`live/livetest` (ceiling)"}

// countsLabels are the two data rows, in the order the table states them.
// The label carries the package-independent meaning of the row; the two columns
// carry the packages.
var countsLabels = []string{"Exported identifiers", "Exported struct fields"}

// tableSeparator is the markdown alignment row under a header.
var tableSeparator = regexp.MustCompile(`^\|[\s:|-]+\|$`)

func readLedger(path string) (map[string]counts, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	lines := strings.Split(string(raw), "\n")

	// Exactly one counts table. A second one — a worked example, a proposal in
	// a changelog entry — would leave the reader silently picking one of them.
	var headers []int
	for i, line := range lines {
		if countsHeader.MatchString(strings.TrimRight(line, " \t\r")) {
			headers = append(headers, i)
		}
	}
	switch len(headers) {
	case 1:
	case 0:
		return nil, fmt.Errorf(
			"%s: no counts table found.\n"+
				"  This tool anchors on the §0 header row:\n"+
				"      | | `live` (exact) | `live/livetest` (ceiling) |\n"+
				"  That table is the FR-65 baseline, so a change to its shape is a change here too.",
			path)
	default:
		return nil, fmt.Errorf(
			"%s: %d counts tables found, at lines %v. There must be exactly one: "+
				"a second copy of the baseline is a second answer to the question CI asks",
			path, len(headers), plusOne(headers))
	}

	if err := checkHeader(lines[headers[0]]); err != nil {
		return nil, fmt.Errorf("%s: the counts table header at line %d: %w", path, headers[0]+1, err)
	}

	body, err := tableBody(lines, headers[0])
	if err != nil {
		return nil, fmt.Errorf("%s: %w", path, err)
	}
	if len(body) != len(countsLabels) {
		return nil, fmt.Errorf(
			"%s: the counts table has %d data rows and this tool reads %d.\n"+
				"  Rows read: %s.\n"+
				"  A row nobody reads can hold any number at all, which is the failure this check exists to prevent.",
			path, len(body), len(countsLabels), strings.Join(countsLabels, ", "))
	}

	out := map[string]counts{}
	for i, row := range body {
		cells, err := splitRow(row)
		if err != nil {
			return nil, fmt.Errorf("%s: counts row %d: %w", path, i+1, err)
		}
		if len(cells) != 3 {
			return nil, fmt.Errorf(
				"%s: counts row %d has %d cells and this tool reads 3 "+
					"(label, `live`, `live/livetest`).\n"+
					"  Row: %s\n"+
					"  A derived column is a stored copy of a computed number; the tool prints the totals.",
				path, i+1, len(cells), strings.TrimSpace(row))
		}

		label := strings.TrimSpace(strings.Trim(cells[0], "* "))
		if !strings.HasPrefix(label, countsLabels[i]) {
			return nil, fmt.Errorf(
				"%s: counts row %d is labelled %q; this tool reads the rows in the order %s",
				path, i+1, label, strings.Join(countsLabels, ", then "))
		}

		live, err := cell(cells[1])
		if err != nil {
			return nil, fmt.Errorf("%s: counts row %d, the live column: %w", path, i+1, err)
		}
		livetest, err := cell(cells[2])
		if err != nil {
			return nil, fmt.Errorf("%s: counts row %d, the live/livetest column: %w", path, i+1, err)
		}

		if i == 0 {
			out["live"] = counts{Identifiers: live}
			out["live/livetest"] = counts{Identifiers: livetest}
			continue
		}
		out["live"] = counts{Identifiers: out["live"].Identifiers, Fields: live}
		out["live/livetest"] = counts{Identifiers: out["live/livetest"].Identifiers, Fields: livetest}
	}
	return out, nil
}

// checkHeader holds the table to the two columns this tool reads.
//
// A third column is the specific mistake worth naming, because it is the one
// that was made: a derived total sat there, unread, for months. The message
// therefore says what to do about it rather than only that something is wrong.
func checkHeader(line string) error {
	cells, err := splitRow(line)
	if err != nil {
		return err
	}
	if len(cells) != len(countsColumns)+1 {
		return fmt.Errorf(
			"it has %d cells and this tool reads %d (an empty label cell, then %s).\n"+
				"  Header: %s\n"+
				"  A column this tool does not read can hold any number at all. If the column is derived, "+
				"delete it — the tool prints every total, including the module's true measured surface.",
			len(cells), len(countsColumns)+1, strings.Join(countsColumns, " and "), strings.TrimSpace(line))
	}
	for i, want := range countsColumns {
		if got := strings.TrimSpace(cells[i+1]); got != want {
			return fmt.Errorf("column %d is %q, and this tool reads %q", i+1, got, want)
		}
	}
	return nil
}

// tableBody returns the data rows of the table whose header is at headerLine.
func tableBody(lines []string, headerLine int) ([]string, error) {
	if headerLine+1 >= len(lines) || !tableSeparator.MatchString(strings.TrimSpace(lines[headerLine+1])) {
		return nil, fmt.Errorf("the counts header at line %d is not followed by a markdown separator row", headerLine+1)
	}
	var body []string
	for i := headerLine + 2; i < len(lines); i++ {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "|") {
			break
		}
		body = append(body, line)
	}
	return body, nil
}

// splitRow returns a markdown row's cells, without the delimiting pipes.
func splitRow(row string) ([]string, error) {
	row = strings.TrimSpace(row)
	if !strings.HasPrefix(row, "|") || !strings.HasSuffix(row, "|") {
		return nil, fmt.Errorf("%q is not a delimited markdown row", row)
	}
	return strings.Split(strings.Trim(row, "|"), "|"), nil
}

// cell reads one count, tolerating the bold markers the table uses for emphasis
// and nothing else. A cell that is not a number is an error rather than a zero:
// Atoi's zero on failure is how a typo used to become a silently wrong baseline.
func cell(s string) (int, error) {
	text := strings.TrimSpace(strings.Trim(strings.TrimSpace(s), "*"))
	n, err := strconv.Atoi(strings.TrimSpace(text))
	if err != nil {
		return 0, fmt.Errorf("%q is not a count", strings.TrimSpace(s))
	}
	return n, nil
}

func plusOne(lines []int) []int {
	out := make([]int, len(lines))
	for i, n := range lines {
		out[i] = n + 1
	}
	return out
}
