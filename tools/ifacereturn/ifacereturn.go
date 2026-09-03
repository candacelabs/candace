// Package ifacereturn reports every function and method declaration whose
// declared result type is an interface.
//
// It is the wide half of a two-lane enforcement of house rule CS-8, "return
// concrete implementations, only accept interfaces". The narrow lane is a
// lexical gate that blocks CI and deliberately reports only a receiverless
// function returning a bare, repository-declared, I-prefixed interface — five
// restrictions that exist so that every finding has a fix. This lane has one
// exemption and no restrictions: it is type-aware
// (go/types answers "is this an interface" exactly, where a lexer can only
// guess), it reads methods as well as functions, and it reports stdlib and
// third-party interfaces the lexer cannot see.
//
// It therefore reports code that has been ruled correct — sealed sum types,
// hook implementations whose signature a func type fixes, pass-throughs of a
// library's own contract, and the method-position returns that survived the
// 2026-09-02 data-shaped re-audit. That is the point rather than a defect:
// operator directive, 2026-09-02, "i want a ci lint check that checks to see
// if there are any interface return types and flags them". Keeping the ruled
// cases visible is what the lane is for, so it flags and never blocks, and a
// ruled case is answered in the rule's own exceptions record rather than
// silenced here. There is no marker comment and no exclusion list.
//
// The one exemption is error. Go's own contract, implemented by everything,
// matched by errors.Is and errors.As, and returned by roughly every function
// in this tree; CS-8 states it as the rule's single structural exemption and
// this lane inherits it verbatim.
//
// Type parameters are not findings either, and that is a correctness fix
// rather than a policy: types.IsInterface reports true for a type parameter,
// because a type parameter's underlying type is its constraint. `func f[S
// any]() S` returns the caller's type, not an interface the callee chose, and
// reporting it would be reporting the constraint syntax.
package ifacereturn

import (
	"go/ast"
	"go/token"
	"go/types"

	"golang.org/x/tools/go/analysis"
)

// Doc is the analyzer's one-line description, and the text `-help` prints.
const Doc = "report every function or method result whose declared type is an interface (house rule CS-8, flagging lane; error is the only exemption)"

// Finding is one result position whose declared type is an interface.
type Finding struct {
	// Pos is the offending result type expression, so a report points at the
	// type rather than at the declaration's name.
	Pos token.Pos
	// Declaration names the function or method, receiver included, in the
	// form a reader would grep for: `NewRealClock`, `(RealClock).NewTimer`.
	Declaration string
	// Position is the 1-based index of this result in the result list, and
	// Arity is how many results there are, so a multi-result signature says
	// which one is meant.
	Position int
	Arity    int
	// Interface is the result type's full name, package path included
	// (`github.com/candacelabs/candace/services/warden.IClock`, `io.Reader`,
	// `any`).
	Interface string
}

// Message is the diagnostic text for one finding, and the string the analyzer
// reports. It names the whole signature position because "returns an
// interface" is not actionable without knowing which result.
func (finding Finding) Message() string {
	place := ""
	if finding.Arity > 1 {
		place = " (result " + itoa(finding.Position) + " of " + itoa(finding.Arity) + ")"
	}
	return finding.Declaration + " returns the interface " + finding.Interface + place
}

// itoa is strconv.Itoa for small non-negative counts, kept local so this
// package's import list stays the analysis framework plus go/*.
func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	digits := make([]byte, 0, 3)
	for value > 0 {
		digits = append([]byte{byte('0' + value%10)}, digits...)
		value /= 10
	}
	return string(digits)
}

// Analyzer is the go/analysis entry point. Run collects the same findings
// Inspect does — there is one walk, so the analyzer, the command and the tests
// cannot drift apart.
var Analyzer = &analysis.Analyzer{
	Name: "ifacereturn",
	Doc:  Doc,
	Run:  run,
}

// run adapts Inspect to the analysis framework.
//
// Its own `any` result is this lane's first finding and is left standing on
// purpose: the signature belongs to analysis.Analyzer.Run, not to this
// function, which is precisely the library-pass-through class CS-8 exempts by
// ruling and this lane reports anyway.
func run(pass *analysis.Pass) (any, error) {
	for _, finding := range Inspect(pass.Files, pass.TypesInfo) {
		pass.Reportf(finding.Pos, "%s", finding.Message())
	}
	return nil, nil
}

// Inspect walks every function and method declaration in files and returns one
// Finding per interface-typed result, in source order.
//
// info must be the type information for those files; a result whose type
// cannot be resolved is skipped rather than guessed at, because a lint that
// invents a finding from an unresolved type is worse than one that misses it.
func Inspect(files []*ast.File, info *types.Info) []Finding {
	var findings []Finding
	for _, file := range files {
		for _, declaration := range file.Decls {
			function, ok := declaration.(*ast.FuncDecl)
			if !ok || function.Type == nil || function.Type.Results == nil {
				continue
			}
			findings = append(findings, resultFindings(function, info)...)
		}
	}
	return findings
}

// resultFindings reports the interface-typed results of one declaration.
func resultFindings(function *ast.FuncDecl, info *types.Info) []Finding {
	results := flattenResults(function.Type.Results)
	name := declarationName(function)
	var findings []Finding
	for index, expression := range results {
		resultType := info.TypeOf(expression)
		if resultType == nil || !isReportable(resultType) {
			continue
		}
		findings = append(findings, Finding{
			Pos:         expression.Pos(),
			Declaration: name,
			Position:    index + 1,
			Arity:       len(results),
			Interface:   types.TypeString(resultType, nil),
		})
	}
	return findings
}

// flattenResults expands a result list into one type expression per result, so
// that `(a, b Store)` counts as two and the reported positions match what a
// caller destructures.
func flattenResults(results *ast.FieldList) []ast.Expr {
	var expressions []ast.Expr
	for _, field := range results.List {
		repeat := len(field.Names)
		if repeat == 0 {
			repeat = 1
		}
		for index := 0; index < repeat; index++ {
			expressions = append(expressions, field.Type)
		}
	}
	return expressions
}

// isReportable decides whether one resolved result type is a finding: an
// interface that is neither error nor a type parameter.
func isReportable(resultType types.Type) bool {
	if !types.IsInterface(resultType) {
		return false
	}
	if _, ok := types.Unalias(resultType).(*types.TypeParam); ok {
		return false
	}
	return !isError(resultType)
}

// isError reports whether resultType is the predeclared error, identified by
// its object living in no package rather than by its spelling — a local
// `type error interface{...}` is a different type and stays reportable.
func isError(resultType types.Type) bool {
	named, ok := types.Unalias(resultType).(*types.Named)
	if !ok {
		return false
	}
	object := named.Obj()
	return object != nil && object.Pkg() == nil && object.Name() == "error"
}

// declarationName renders a declaration the way a reader greps for it: bare
// for a function, receiver-qualified for a method.
func declarationName(function *ast.FuncDecl) string {
	if function.Recv == nil || len(function.Recv.List) == 0 {
		return function.Name.Name
	}
	return "(" + types.ExprString(function.Recv.List[0].Type) + ")." + function.Name.Name
}
