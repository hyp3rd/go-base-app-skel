package output

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/hyp3rd/ewrap/pkg/ewrap"
)

const (
	defaultMaxSizeMB = 100
	bytesPerMB       = 1024 * 1024
)

// Writer defines an interface for log output destinations.
type Writer interface {
	io.Writer
	// Sync ensures all data is written.
	Sync() error
	// Close releases any resources.
	Close() error
}

// FileWriter implements Writer for file-based logging.
type FileWriter struct {
	mu       sync.Mutex
	file     *os.File
	path     string
	maxSize  int64
	size     int64
	compress bool
}

// FileConfig holds configuration for file output.
type FileConfig struct {
	// Path is the log file path
	Path string
	// MaxSize is the maximum size in bytes before rotation
	MaxSize int64
	// Compress determines if rotated files should be compressed
	Compress bool
	// FileMode sets the permissions for new log files
	FileMode os.FileMode
}

// NewFileWriter creates a new file-based log writer.
func NewFileWriter(config FileConfig) (*FileWriter, error) {
	if config.Path == "" {
		return nil, ewrap.New("log file path is required")
	}

	if config.MaxSize == 0 {
		config.MaxSize = defaultMaxSizeMB * bytesPerMB // 100MB default
	}

	if config.FileMode == 0 {
		config.FileMode = 0o644
	}

	// Ensure directory exists
	dir := filepath.Dir(config.Path)
	//nolint:mnd
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, ewrap.Wrapf(err, "creating log directory").
			WithMetadata("path", dir)
	}

	// Open or create the log file
	file, err := os.OpenFile(config.Path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, config.FileMode)
	if err != nil {
		return nil, ewrap.Wrapf(err, "opening log file").
			WithMetadata("path", config.Path)
	}

	// Get initial file size
	info, err := file.Stat()
	if err != nil {
		file.Close()

		return nil, ewrap.Wrapf(err, "getting file stats").
			WithMetadata("path", config.Path)
	}

	return &FileWriter{
		file:     file,
		path:     config.Path,
		maxSize:  config.MaxSize,
		size:     info.Size(),
		compress: config.Compress,
	}, nil
}

// Write implements io.Writer.
func (w *FileWriter) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// Check if rotation is needed
	if w.size+int64(len(data)) > w.maxSize {
		if err := w.rotate(); err != nil {
			return 0, ewrap.Wrapf(err, "rotating log file")
		}
	}

	bytesWritten, err := w.file.Write(data)
	if err != nil {
		return bytesWritten, ewrap.Wrap(err, "failed writing to log file")
	}

	w.size += int64(bytesWritten)

	return bytesWritten, nil // Return nil error on success, don't wrap it
}

// rotate moves the current log file to a timestamped backup
// and creates a new log file.
func (w *FileWriter) rotate() error {
	// Close current file
	if err := w.file.Close(); err != nil {
		return ewrap.Wrapf(err, "closing current log file")
	}

	// Generate backup filename with timestamp
	timestamp := time.Now().Format("2006-01-02T15-04-05")
	backupPath := filepath.Join(
		filepath.Dir(w.path),
		fmt.Sprintf("%s.%s", filepath.Base(w.path), timestamp),
	)

	// Rename current file to backup
	if err := os.Rename(w.path, backupPath); err != nil {
		return ewrap.Wrapf(err, "renaming log file").
			WithMetadata("from", w.path).
			WithMetadata("to", backupPath)
	}

	// Compress backup file if enabled
	if w.compress {
		go w.compressFile(backupPath) // Run compression in background
	}

	// Create new log file
	//nolint:mnd
	file, err := os.OpenFile(w.path, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return ewrap.Wrapf(err, "creating new log file")
	}

	w.file = file
	w.size = 0

	return nil
}

func (w *FileWriter) Sync() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil // Already closed, no error
	}

	err := w.file.Sync()
	if err != nil {
		return ewrap.Wrapf(err, "syncing log file")
	}

	return nil // Clean success
}

func (w *FileWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()

	if w.file == nil {
		return nil // Already closed, no error
	}

	// First sync any remaining data
	if err := w.file.Sync(); err != nil {
		return ewrap.Wrapf(err, "final sync before close")
	}

	// Then close the file
	err := w.file.Close()
	if err != nil {
		return ewrap.Wrapf(err, "closing log file")
	}

	w.file = nil // Mark as closed

	return nil // Clean success
}

// ConsoleWriter implements Writer for console output with color support.
type ConsoleWriter struct {
	out  io.Writer
	mode ColorMode
}

// ColorMode determines how colors are handled.
type ColorMode int

const (
	// ColorModeAuto detects if the output supports colors.
	ColorModeAuto ColorMode = iota
	// ColorModeAlways forces color output.
	ColorModeAlways
	// ColorModeNever disables color output.
	ColorModeNever
)

// NewConsoleWriter creates a new ConsoleWriter that writes to the provided io.Writer with the specified ColorMode.
// If out is nil, it defaults to os.Stdout.
func NewConsoleWriter(out io.Writer, mode ColorMode) *ConsoleWriter {
	if out == nil {
		out = os.Stdout
	}

	return &ConsoleWriter{
		out:  out,
		mode: mode,
	}
}

// Write writes the provided byte slice to the underlying output writer.
// It wraps any errors that occur during the write operation.
func (w *ConsoleWriter) Write(p []byte) (int, error) {
	n, err := w.out.Write(p)
	if err != nil {
		return n, ewrap.Wrap(err, "failed writing to console output")
	}

	return n, nil
}

// Sync synchronizes the console output, skipping sync for stdout/stderr if it's not needed.
// It ignores "inappropriate ioctl for device" errors for terminal devices.
func (w *ConsoleWriter) Sync() error {
	// For stdout/stderr, sync is not needed and can be safely skipped
	if syncer, ok := w.out.(interface{ Sync() error }); ok {
		if err := syncer.Sync(); err != nil {
			// Ignore "inappropriate ioctl for device" errors for terminal devices
			if strings.Contains(err.Error(), "inappropriate ioctl for device") {
				return nil
			}

			return ewrap.Wrapf(err, "syncing console output")
		}
	}

	return nil
}

// Close closes the underlying output writer if it implements io.Closer.
// It wraps any errors that occur during the close operation.
func (w *ConsoleWriter) Close() error {
	if closer, ok := w.out.(io.Closer); ok {
		err := closer.Close()
		if err != nil {
			return ewrap.Wrapf(err, "closing console output")
		}
	}

	return nil // Return nil directly, don't wrap it
}

// MultiWriter combines multiple writers into one.
type MultiWriter struct {
	Writers []Writer
	mu      sync.RWMutex
	// Add a debug name for each writer to help with diagnostics
	writerNames map[Writer]string
}

// NewMultiWriter creates a new writer that writes to all provided writers.
// It filters out nil writers and returns an error if no valid writers are provided.
func NewMultiWriter(writers ...Writer) (*MultiWriter, error) {
	if len(writers) == 0 {
		return nil, ewrap.New("at least one writer is required")
	}

	validWriters := make([]Writer, 0, len(writers))
	writerNames := make(map[Writer]string)

	// Create descriptive names for each writer
	for i, w := range writers {
		if w != nil {
			validWriters = append(validWriters, w)
			// Store a descriptive name based on the writer type
			writerNames[w] = fmt.Sprintf("%T[%d]", w, i)
		}
	}

	if len(validWriters) == 0 {
		return nil, ewrap.New("no valid writers provided")
	}

	return &MultiWriter{
		Writers:     validWriters,
		writerNames: writerNames,
	}, nil
}

// Write sends the output to all writers with detailed diagnostics.
func (mw *MultiWriter) Write(payload []byte) (int, error) {
	mw.mu.RLock()
	defer mw.mu.RUnlock()

	return mw.writeToWriters(payload)
}

func (mw *MultiWriter) writeToWriters(payload []byte) (int, error) {
	expectedBytes := len(payload)
	results := mw.performWrites(payload, expectedBytes)

	successCount, failures := mw.processResults(results, expectedBytes)

	fmt.Fprintf(os.Stderr, "Total successes: %d/%d\n", successCount, len(results))

	if len(failures) > 0 {
		return expectedBytes, mw.createErrorReport(results, successCount, failures)
	}

	return expectedBytes, nil
}

func (mw *MultiWriter) performWrites(payload []byte, expectedBytes int) []WriteResult {
	results := make([]WriteResult, 0, len(mw.Writers))

	fmt.Fprintf(os.Stderr, "MultiWriter attempting to write %d bytes\n", expectedBytes)

	for _, writer := range mw.Writers {
		if writer == nil {
			continue
		}

		n, err := writer.Write(payload)
		result := WriteResult{
			Writer: writer,
			Name:   mw.writerNames[writer],
			Bytes:  n,
			Err:    err,
		}

		fmt.Fprintf(os.Stderr, "Writer %s: wrote %d bytes, err: %v\n",
			result.Name, result.Bytes, result.Err)

		results = append(results, result)
	}

	return results
}

func (mw *MultiWriter) processResults(results []WriteResult, expectedBytes int) (int, []string) {
	successCount := 0

	var failures []string

	for _, result := range results {
		if result.Err == nil && result.Bytes == expectedBytes {
			successCount++

			fmt.Fprintf(os.Stderr, "Writer %s succeeded\n", result.Name)
		} else {
			reason := "incomplete write"
			if result.Err != nil {
				reason = result.Err.Error()
			}

			failures = append(failures, fmt.Sprintf(
				"%s: wrote %d/%d bytes (%s)",
				result.Name,
				result.Bytes,
				expectedBytes,
				reason,
			))
		}
	}

	return successCount, failures
}

func (mw *MultiWriter) createErrorReport(results []WriteResult, successCount int, failures []string) error {
	var diagMsg strings.Builder

	diagMsg.WriteString("Write operation status:\n")

	fmt.Fprintf(&diagMsg, "  Total writers: %d\n", len(results))
	fmt.Fprintf(&diagMsg, "  Successful writes: %d\n", successCount)
	fmt.Fprintf(&diagMsg, "  Failed writes: %d\n", len(failures))
	fmt.Fprintf(&diagMsg, "  Failures:\n")

	for _, failure := range failures {
		fmt.Fprintf(&diagMsg, "    - %s\n", failure)
	}

	return ewrap.New(diagMsg.String())
}

// Sync ensures all writers are synced with comprehensive diagnostics.
func (mw *MultiWriter) Sync() error {
	mw.mu.RLock()
	defer mw.mu.RUnlock()

	fmt.Fprintf(os.Stderr, "DEBUG: Starting sync operation for %d writers\n", len(mw.Writers))

	var syncErrors []string

	successCount := 0

	for i, writer := range mw.Writers {
		if writer == nil {
			fmt.Fprintf(os.Stderr, "DEBUG: Writer %d is nil, skipping\n", i)

			continue
		}

		fmt.Fprintf(os.Stderr, "DEBUG: Syncing writer %d (%T)\n", i, writer)
		err := writer.Sync()

		if err != nil {
			msg := fmt.Sprintf("%T: %v", writer, err)
			fmt.Fprintf(os.Stderr, "DEBUG: Sync failed: %s\n", msg)
			syncErrors = append(syncErrors, msg)
		} else {
			fmt.Fprintf(os.Stderr, "DEBUG: Sync succeeded for writer %d\n", i)

			successCount++
		}
	}

	fmt.Fprintf(os.Stderr, "DEBUG: Sync complete. Successes: %d, Failures: %d\n",
		successCount, len(syncErrors))

	if len(syncErrors) > 0 {
		return ewrap.New("sync operation partially failed").
			WithMetadata("failed_syncs", syncErrors).
			WithMetadata("successful_syncs", successCount).
			WithMetadata("total_writers", len(mw.Writers))
	}

	return nil
}

// Close closes all writers with detailed cleanup tracking.
func (mw *MultiWriter) Close() error {
	mw.mu.Lock()
	defer mw.mu.Unlock()

	fmt.Fprintf(os.Stderr, "DEBUG: Starting close operation for %d writers\n", len(mw.Writers))

	var closeErrors []string

	successCount := 0

	for i, writer := range mw.Writers {
		if writer == nil {
			fmt.Fprintf(os.Stderr, "DEBUG: Writer %d is nil, skipping\n", i)

			continue
		}

		fmt.Fprintf(os.Stderr, "DEBUG: Closing writer %d (%T)\n", i, writer)
		err := writer.Close()

		if err != nil { // Simplified error check
			msg := fmt.Sprintf("%T: %v", writer, err)
			fmt.Fprintf(os.Stderr, "DEBUG: Close failed: %s\n", msg)
			closeErrors = append(closeErrors, msg)
		} else {
			fmt.Fprintf(os.Stderr, "DEBUG: Close succeeded for writer %d\n", i)

			successCount++
		}
	}

	fmt.Fprintf(os.Stderr, "DEBUG: Close complete. Successes: %d, Failures: %d\n",
		successCount, len(closeErrors))

	// Clear writers slice
	for i := range mw.Writers {
		mw.Writers[i] = nil
	}

	mw.Writers = nil

	if len(closeErrors) > 0 {
		return ewrap.New("close operation partially failed").
			WithMetadata("failed_closes", closeErrors).
			WithMetadata("successful_closes", successCount)
	}

	return nil
}

// AddWriter adds a new writer to the MultiWriter.
func (mw *MultiWriter) AddWriter(writer Writer) error {
	if writer == nil {
		return ewrap.New("cannot add nil writer")
	}

	mw.mu.Lock()
	defer mw.mu.Unlock()

	mw.Writers = append(mw.Writers, writer)

	return nil
}

// RemoveWriter removes a writer from the MultiWriter.
func (mw *MultiWriter) RemoveWriter(writer Writer) {
	if writer == nil {
		return
	}

	mw.mu.Lock()
	defer mw.mu.Unlock()

	for i, existingWriter := range mw.Writers {
		if existingWriter == writer {
			// Remove the writer by replacing it with the last element
			// and truncating the slice
			lastIdx := len(mw.Writers) - 1
			mw.Writers[i] = mw.Writers[lastIdx]
			mw.Writers[lastIdx] = nil // Clear the reference
			mw.Writers = mw.Writers[:lastIdx]

			break
		}
	}
}
