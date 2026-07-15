package main

import (
	"bufio"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	maximumProductionFileLines  = 700
	maximumFormerLargeFileLines = 500
	maximumFunctionLines        = 700
	maximumFunctionComplexity   = 160
	maximumMutableGlobals       = 16
)

// TODO(macro): The strict-file list omits window_loop.go / health_service.go /
// document_controller.go cohesion problems that still concentrate orchestration in package main.
// After splitting those god files, add them to formerLargeFiles so they cannot regress.
var formerLargeFiles = map[string]struct{}{
	"main.go":                 {},
	"media/audio.go":          {},
	"media/backend.go":        {},
	"media/manager.go":        {},
	"media/player.go":         {},
	"playback/engine.go":      {},
	"project/archive.go":      {},
	"show/manager.go":         {},
	"show/warnings.go":        {},
	"ui/cue_edit_pages.go":    {},
	"ui/main_page.go":         {},
	"ui/operator_panel.go":    {},
	"ui/settings_page.go":     {},
	"ui/tb_context.go":        {},
	"ui/timecode_timeline.go": {},
}

func TestProductionFilesStayWithinMaintainabilityLimits(t *testing.T) {
	t.Parallel()

	err := filepath.WalkDir(".", func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() {
			if path != "." && strings.HasPrefix(entry.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}

		normalized := filepath.ToSlash(path)
		normalized = strings.TrimPrefix(normalized, "./")
		limit := maximumProductionFileLines
		if _, wasOversized := formerLargeFiles[normalized]; wasOversized {
			limit = maximumFormerLargeFileLines
		}
		lines, err := countProductionLines(path)
		if err != nil {
			return err
		}
		if lines > limit {
			t.Errorf("%s has %d lines; limit is %d", normalized, lines, limit)
		}
		checkProductionStructure(t, normalized, path)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func checkProductionStructure(t *testing.T, normalized, path string) {
	t.Helper()
	set := token.NewFileSet()
	file, err := parser.ParseFile(set, path, nil, 0)
	if err != nil {
		t.Errorf("parse %s: %v", normalized, err)
		return
	}

	mutableGlobals := 0
	for _, declaration := range file.Decls {
		switch declaration := declaration.(type) {
		case *ast.FuncDecl:
			if declaration.Body == nil {
				continue
			}
			lines := set.Position(declaration.End()).Line - set.Position(declaration.Pos()).Line + 1
			if lines > maximumFunctionLines {
				t.Errorf("%s: %s has %d lines; function limit is %d", normalized, declaration.Name.Name, lines, maximumFunctionLines)
			}
			complexity := functionComplexity(declaration.Body)
			if complexity > maximumFunctionComplexity {
				t.Errorf("%s: %s has complexity %d; limit is %d", normalized, declaration.Name.Name, complexity, maximumFunctionComplexity)
			}
		case *ast.GenDecl:
			if declaration.Tok == token.VAR {
				for _, spec := range declaration.Specs {
					mutableGlobals += len(spec.(*ast.ValueSpec).Names)
				}
			}
		}
	}
	if mutableGlobals > maximumMutableGlobals {
		t.Errorf("%s has %d mutable package globals; limit is %d", normalized, mutableGlobals, maximumMutableGlobals)
	}
	checkDependencyDirection(t, normalized, file)
}

func functionComplexity(body *ast.BlockStmt) int {
	complexity := 1
	ast.Inspect(body, func(node ast.Node) bool {
		switch node := node.(type) {
		case *ast.IfStmt, *ast.ForStmt, *ast.RangeStmt, *ast.CaseClause, *ast.CommClause:
			complexity++
		case *ast.BinaryExpr:
			if node.Op == token.LAND || node.Op == token.LOR {
				complexity++
			}
		}
		return true
	})
	return complexity
}

func checkDependencyDirection(t *testing.T, normalized string, file *ast.File) {
	t.Helper()
	if file.Name.Name == "main" || strings.HasPrefix(normalized, "ui/") {
		return
	}
	for _, imported := range file.Imports {
		path := strings.Trim(imported.Path.Value, `"`)
		if path == "github.com/syspoe/cusus/ui" || strings.HasPrefix(path, "github.com/syspoe/cusus/ui/") {
			t.Errorf("%s imports presentation package %s; domain packages must not depend on UI", normalized, path)
		}
	}
}

func countProductionLines(path string) (int, error) {
	file, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer file.Close()

	lines := 0
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "//") {
			continue
		}
		lines++
	}
	if err := scanner.Err(); err != nil {
		return 0, fmt.Errorf("scan %s: %w", path, err)
	}
	return lines, nil
}
