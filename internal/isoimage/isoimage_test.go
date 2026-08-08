package isoimage

import (
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	backendfile "github.com/diskfs/go-diskfs/backend/file"
	"github.com/diskfs/go-diskfs/filesystem/iso9660"
)

func TestBuild(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "hello.txt"), []byte("hello"), 0644); err != nil {
		t.Fatal(err)
	}
	imagePath := filepath.Join(t.TempDir(), "test.iso")
	if err := Build(imagePath, workspace, "TEST_VOLUME"); err != nil {
		t.Fatal(err)
	}

	image, err := os.Open(imagePath)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = image.Close() }()
	filesystem, err := iso9660.Read(backendfile.New(image, true), 0, 0, blockSize)
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimRight(filesystem.Label(), "\x00 ") != "TEST_VOLUME" {
		t.Fatalf("label is %q", filesystem.Label())
	}
	file, err := filesystem.OpenFile("/hello.txt", os.O_RDONLY)
	if err != nil {
		t.Fatal(err)
	}
	contents, err := io.ReadAll(file)
	if err != nil {
		t.Fatal(err)
	}
	if string(contents) != "hello" {
		t.Fatalf("contents are %q", contents)
	}
}
