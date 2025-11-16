package util

import (
	"archive/zip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

func ParseMap(envvars []string, keyName string) (map[string]string, error) {
	result := make(map[string]string)
	for _, envvar := range envvars {
		s := strings.SplitN(strings.TrimSpace(envvar), "=", 2)
		if len(s) != 2 {
			return nil, fmt.Errorf("label format is not correct, needs key=value")
		}
		envvarName := s[0]
		envvarValue := s[1]

		if !(len(envvarName) > 0) {
			return nil, fmt.Errorf("empty %s name: [%s]", keyName, envvar)
		}
		if !(len(envvarValue) > 0) {
			return nil, fmt.Errorf("empty %s value: [%s]", keyName, envvar)
		}

		result[envvarName] = envvarValue
	}
	return result, nil
}

// util.MergeMap merges two maps, with the overlay taking precedence.
// The return value allocates a new map.
func MergeMap(base map[string]string, overlay map[string]string) map[string]string {
	merged := make(map[string]string)

	for k, v := range base {
		merged[k] = v
	}
	for k, v := range overlay {
		merged[k] = v
	}

	return merged
}

func MergeSlice(values []string, overlay []string) []string {
	results := []string{}
	added := make(map[string]bool)
	for _, value := range overlay {
		results = append(results, value)
		added[value] = true
	}

	for _, value := range values {
		if exists := added[value]; !exists {
			results = append(results, value)
		}
	}

	return results
}

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
