package main

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxFileSize = 1024 * 1024

var forbiddenExtensions = map[string]bool{
	".a": true, ".class": true, ".crt": true, ".der": true,
	".dll": true, ".dylib": true, ".exe": true, ".jar": true,
	".key": true, ".o": true, ".p12": true, ".pem": true,
	".pfx": true, ".so": true, ".tar": true, ".zip": true,
}

func main() {
	entries, err := indexedFiles()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}

	failed := false
	for _, entry := range entries {
		content, err := gitBlob(entry.hash)
		if err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", entry.path, err)
			failed = true
			continue
		}
		for _, violation := range checkFile(entry, content) {
			fmt.Fprintf(os.Stderr, "%s: %s\n", entry.path, violation)
			failed = true
		}
	}
	if failed {
		os.Exit(1)
	}
}

type indexEntry struct {
	mode string
	hash string
	path string
}

func indexedFiles() ([]indexEntry, error) {
	cmd := exec.Command("git", "ls-files", "--stage", "-z")
	output, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("list indexed files: %w", err)
	}

	var entries []indexEntry
	for _, record := range bytes.Split(output, []byte{0}) {
		if len(record) == 0 {
			continue
		}
		metadata, path, ok := bytes.Cut(record, []byte{'\t'})
		if !ok {
			return nil, fmt.Errorf("invalid git index record %q", record)
		}
		fields := bytes.Fields(metadata)
		if len(fields) != 3 || string(fields[2]) != "0" {
			return nil, fmt.Errorf("unmerged git index entry %q", path)
		}
		entries = append(entries, indexEntry{
			mode: string(fields[0]),
			hash: string(fields[1]),
			path: string(path),
		})
	}
	return entries, nil
}

func gitBlob(hash string) ([]byte, error) {
	return exec.Command("git", "cat-file", "blob", hash).Output()
}

func checkFile(entry indexEntry, content []byte) []string {
	var violations []string
	base := filepath.Base(entry.path)
	ext := strings.ToLower(filepath.Ext(base))

	if entry.mode == "160000" {
		violations = append(violations, "git submodules are forbidden")
		return violations
	}
	if base == ".env.local" || strings.HasPrefix(base, "id_rsa") || forbiddenExtensions[ext] {
		violations = append(violations, "forbidden secret or binary filename")
	}
	if len(content) > maxFileSize {
		violations = append(violations, fmt.Sprintf("file is %d bytes; maximum is %d", len(content), maxFileSize))
	}
	if bytes.IndexByte(content, 0) >= 0 {
		violations = append(violations, "binary content is forbidden")
	} else if len(content) > 0 {
		mimeType := strings.TrimSpace(strings.SplitN(http.DetectContentType(content), ";", 2)[0])
		if !strings.HasPrefix(mimeType, "text/") && mimeType != "application/json" && mimeType != "application/xml" {
			violations = append(violations, "unsupported file type "+mimeType)
		}
	}
	if entry.mode == "100755" && !bytes.HasPrefix(content, []byte("#!")) {
		violations = append(violations, "executable file has no shebang")
	}
	return violations
}
