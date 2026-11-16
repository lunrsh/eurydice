package rockbox

import (
	"fmt"
	"io"
	"os"
)

func copyFile(sourceStream io.Reader, targetPath string) error {
	destFile, err := os.OpenFile(targetPath, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0644)

	if err != nil {
		return fmt.Errorf("failed to create destination file: %w", err)
	}

	defer destFile.Close()

	_, err = io.Copy(destFile, sourceStream)

	if err != nil {
		return fmt.Errorf("failed to copy song: %w", err)
	}

	return nil
}
