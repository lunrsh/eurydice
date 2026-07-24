package sync

import (
	"fmt"
	"io"
	"os"
)

// Copies a song from the source path to the target path
func copySong(sourcePath, targetPath string) error {
	sourceFile, err := os.Open(sourcePath)

	if err != nil {
		return fmt.Errorf("failed to open source file: %w", err)
	}

	defer sourceFile.Close()

	destFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)

	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}

	defer destFile.Close()

	_, err = io.Copy(destFile, sourceFile)

	if err != nil {
		return fmt.Errorf("failed to copy song: %w", err)
	}

	return nil
}

// Transcodes a song from the source path to the target path
func transcodeSong(sourcePath, targetPath string) error {
	return fmt.Errorf("transcoding not implemented")
}
