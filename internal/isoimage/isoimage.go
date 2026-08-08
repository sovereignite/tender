package isoimage

import (
	"fmt"
	"os"

	backendfile "github.com/diskfs/go-diskfs/backend/file"
	"github.com/diskfs/go-diskfs/filesystem/iso9660"
)

const blockSize = int64(2048)

func Build(path, workspace, label string) error {
	output, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("create ISO output: %w", err)
	}
	closed := false
	defer func() {
		if !closed {
			_ = output.Close()
		}
	}()

	filesystem, err := iso9660.Create(backendfile.New(output, false), 0, 0, blockSize, workspace)
	if err != nil {
		return fmt.Errorf("create ISO filesystem: %w", err)
	}
	if err := filesystem.Finalize(iso9660.FinalizeOptions{
		RockRidge:        true,
		VolumeIdentifier: label,
	}); err != nil {
		return fmt.Errorf("finalize ISO filesystem: %w", err)
	}
	if err := output.Close(); err != nil {
		return fmt.Errorf("close ISO output: %w", err)
	}
	closed = true
	return nil
}
