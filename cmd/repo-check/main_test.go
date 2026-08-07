package main

import (
	"strings"
	"testing"
)

func TestCheckFileRejectsBinaryContent(t *testing.T) {
	t.Parallel()
	violations := checkFile(indexEntry{mode: "100644", path: "artifact"}, []byte{'E', 'L', 'F', 0})
	if !containsViolation(violations, "binary content") {
		t.Fatalf("violations = %v", violations)
	}
}

func TestCheckFileRejectsForbiddenFilename(t *testing.T) {
	t.Parallel()
	violations := checkFile(indexEntry{mode: "100644", path: "secret.pem"}, []byte("certificate"))
	if !containsViolation(violations, "forbidden secret or binary filename") {
		t.Fatalf("violations = %v", violations)
	}
}

func TestCheckFileAllowsSourceText(t *testing.T) {
	t.Parallel()
	violations := checkFile(indexEntry{mode: "100644", path: "main.go"}, []byte("package main\n"))
	if len(violations) != 0 {
		t.Fatalf("violations = %v", violations)
	}
}

func containsViolation(violations []string, want string) bool {
	for _, violation := range violations {
		if strings.Contains(violation, want) {
			return true
		}
	}
	return false
}
