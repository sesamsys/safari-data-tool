package main

import (
	"bufio"
	"os"
	"strings"
	"testing"
)

// TestModuleStaysStandardLibraryOnly parses go.mod and asserts that the module
// declares no third-party dependencies: there must be no non-empty require
// directives, whether written as a single `require path version` line or inside
// a `require ( ... )` block. This keeps the build standard-library only.
//
// Validates: Requirements 1.4
func TestModuleStaysStandardLibraryOnly(t *testing.T) {
	// `go test` runs with the package directory as the working directory, so
	// for the repo-root main package go.mod sits beside this test file.
	data, err := os.ReadFile("go.mod")
	if err != nil {
		t.Fatalf("read go.mod: %v", err)
	}

	var requires []string
	inBlock := false

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Drop line comments and skip blank/comment-only lines.
		if idx := strings.Index(line, "//"); idx >= 0 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}

		if inBlock {
			if line == ")" {
				inBlock = false
				continue
			}
			requires = append(requires, line)
			continue
		}

		switch {
		case line == "require (":
			inBlock = true
		case strings.HasPrefix(line, "require ("):
			// `require (` possibly followed by content on the same line.
			rest := strings.TrimSpace(strings.TrimPrefix(line, "require ("))
			inBlock = true
			if rest != "" && rest != ")" {
				requires = append(requires, strings.TrimSuffix(rest, ")"))
			}
			if strings.HasSuffix(line, ")") {
				inBlock = false
			}
		case strings.HasPrefix(line, "require "):
			// Single-line require directive: `require path version`.
			requires = append(requires, strings.TrimSpace(strings.TrimPrefix(line, "require ")))
		}
	}
	if err := scanner.Err(); err != nil {
		t.Fatalf("scan go.mod: %v", err)
	}

	if len(requires) != 0 {
		t.Fatalf("go.mod declares third-party dependencies, want none (stdlib-only); found require directives: %v", requires)
	}
}
