// Copyright (c) Alex Ellis 2017. All rights reserved.
// Licensed under the MIT license. See LICENSE file in the project root for full license information.

package util

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// ZipDirectory creates a zip archive of the specified directory
func ZipDirectory(sourceDir string) ([]byte, error) {
	// Create a buffer to write our archive to.
	buf := new(strings.Builder)
	zipWriter := zip.NewWriter(&stringWriterAdapter{buf})

	// Walk through the directory
	err := filepath.Walk(sourceDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}

		// Skip directories
		if info.IsDir() {
			return nil
		}

		// Get the relative path
		relPath, err := filepath.Rel(sourceDir, path)
		if err != nil {
			return err
		}

		// Create a new file in the zip archive
		zipFile, err := zipWriter.Create(relPath)
		if err != nil {
			return err
		}

		// Open the source file
		srcFile, err := os.Open(path)
		if err != nil {
			return err
		}
		defer srcFile.Close()

		// Copy the file content to the zip
		_, err = io.Copy(zipFile, srcFile)
		return err
	})

	if err != nil {
		return nil, fmt.Errorf("error walking directory: %w", err)
	}

	// Close the zip writer
	err = zipWriter.Close()
	if err != nil {
		return nil, fmt.Errorf("error closing zip writer: %w", err)
	}

	return []byte(buf.String()), nil
}

// stringWriterAdapter adapts strings.Builder to io.Writer
type stringWriterAdapter struct {
	*strings.Builder
}

func (w *stringWriterAdapter) Write(p []byte) (n int, err error) {
	return w.Builder.Write(p)
}
