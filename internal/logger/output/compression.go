package output

import (
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"sync"

	"github.com/hyp3rd/ewrap/pkg/ewrap"
)

const bufferSize = 32 * 1024 // 32KB buffer

// compressFile compresses the given file using gzip compression.
// The original file is removed after successful compression.
// This method is designed to run in the background to avoid blocking logging operations.
func (w *FileWriter) compressFile(path string) {
	// We'll use a WaitGroup to ensure proper cleanup in case of panic
	var wg sync.WaitGroup

	wg.Add(1)

	go func() {
		defer wg.Done()
		defer func() {
			if r := recover(); r != nil {
				// If panic occurs, ensure we don't leave partial files
				cleanupCompression(path)
			}
		}()

		if err := w.performCompression(path); err != nil {
			// Log the error but don't fail - this is a background operation
			// In a real application, you might want to send this to an error channel
			// or use your error reporting system
			_, _ = os.Stderr.WriteString("Error compressing log file: " + err.Error() + "\n")
		}
	}()

	wg.Wait()
}

// performCompression handles the actual compression work.
func (w *FileWriter) performCompression(path string) error {
	// Open the source file
	source, err := os.Open(path)
	if err != nil {
		return ewrap.Wrapf(err, "opening source file").
			WithMetadata("path", path)
	}
	defer source.Close()

	// Create the compressed file
	compressedPath := path + ".gz"
	//nolint:mnd
	compressed, err := os.OpenFile(compressedPath, os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return ewrap.Wrapf(err, "creating compressed file").
			WithMetadata("path", compressedPath)
	}

	defer compressed.Close()

	// Create gzip writer with best compression
	gzipWriter, err := gzip.NewWriterLevel(compressed, gzip.BestCompression)
	if err != nil {
		return ewrap.Wrapf(err, "creating gzip writer")
	}
	defer gzipWriter.Close()

	// Set the original file name in the gzip header
	gzipWriter.Name = filepath.Base(path)

	// Create a buffer for copying

	buffer := make([]byte, bufferSize)

	// Copy the file content in chunks
	if err := copyWithBuffer(gzipWriter, source, buffer); err != nil {
		// If compression fails, clean up the partial compressed file
		os.Remove(compressedPath)

		return ewrap.Wrapf(err, "copying file content")
	}

	// Ensure all data is written
	if err := gzipWriter.Close(); err != nil {
		os.Remove(compressedPath)

		return ewrap.Wrapf(err, "closing gzip writer")
	}

	if err := compressed.Sync(); err != nil {
		os.Remove(compressedPath)

		return ewrap.Wrapf(err, "syncing compressed file")
	}

	if err := compressed.Close(); err != nil {
		os.Remove(compressedPath)

		return ewrap.Wrapf(err, "closing compressed file")
	}

	// Verify the compressed file exists and has content
	if err := verifyCompressedFile(compressedPath); err != nil {
		os.Remove(compressedPath)

		return err
	}

	// Remove the original file only after successful compression
	if err := os.Remove(path); err != nil {
		// If we can't remove the original, remove the compressed file to avoid duplicates
		os.Remove(compressedPath)

		return ewrap.Wrapf(err, "removing original file").
			WithMetadata("path", path)
	}

	return nil
}

// copyWithBuffer copies from src to dst using the provided buffer.
func copyWithBuffer(dst io.Writer, src io.Reader, buf []byte) error {
	for {
		n, err := src.Read(buf)
		if n > 0 {
			if _, writerErr := dst.Write(buf[:n]); writerErr != nil {
				return ewrap.Wrapf(writerErr, "writing to destination")
			}
		}

		if err == io.EOF {
			break
		}

		if err != nil {
			return ewrap.Wrapf(err, "reading from source")
		}
	}

	return nil
}

// verifyCompressedFile checks if the compressed file exists and has content.
func verifyCompressedFile(path string) error {
	info, err := os.Stat(path)
	if err != nil {
		return ewrap.Wrapf(err, "verifying compressed file").
			WithMetadata("path", path)
	}

	if info.Size() == 0 {
		return ewrap.New("compressed file is empty").
			WithMetadata("path", path)
	}

	// Optional: Verify the file is a valid gzip file
	f, err := os.Open(path)
	if err != nil {
		return ewrap.Wrapf(err, "opening compressed file for verification")
	}
	defer f.Close()

	gr, err := gzip.NewReader(f)
	if err != nil {
		return ewrap.Wrapf(err, "verifying gzip format")
	}

	gr.Close()

	return nil
}

// cleanupCompression removes both the original and compressed files
// in case of a critical error or panic during compression.
func cleanupCompression(path string) {
	// Don't remove the original file in cleanup
	// Better to keep uncompressed logs than lose them
	compressedPath := path + ".gz"
	os.Remove(compressedPath)
}
