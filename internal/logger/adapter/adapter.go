package adapter

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/hyp3rd/base/internal/logger"
	"github.com/hyp3rd/base/internal/logger/output"
	"github.com/hyp3rd/ewrap/pkg/ewrap"
)

const (
	callerDepth   = 3
	bufferTimeout = 100 * time.Millisecond
)

// bufferPool maintains a pool of reusable byte buffers to minimize allocations.
//
//nolint:gochecknoglobals
var bufferPool = sync.Pool{
	New: func() interface{} {
		return new(bytes.Buffer)
	},
}

// adapter implements the Logger interface with high-performance logging.
type adapter struct {
	config logger.Config
	mu     sync.RWMutex
	fields []logger.Field
	buffer chan logEntry
	done   chan struct{}
	wg     *sync.WaitGroup // Change to pointer
}

// logEntry represents a single log entry.
type logEntry struct {
	Level     logger.Level
	Message   string
	Fields    []logger.Field
	Timestamp time.Time
	Caller    string
	Error     error
}

// NewAdapter creates a new logger adapter.
func NewAdapter(config logger.Config) (logger.Logger, error) {
	if config.Output == nil {
		return nil, ewrap.New("output writer is required")
	}

	if config.AsyncBufferSize == 0 {
		config.AsyncBufferSize = logger.DefaultAsyncBufferSize
	}

	wg := new(sync.WaitGroup) // Create WaitGroup pointer

	loggerAdapter := &adapter{
		config: config,
		buffer: make(chan logEntry, config.AsyncBufferSize),
		done:   make(chan struct{}),
		wg:     wg, // Store pointer
	}

	// Start background writer
	loggerAdapter.wg.Add(1)
	go loggerAdapter.processLogs()

	return loggerAdapter, nil
}

// processLogs handles the background processing of log entries with proper shutdown.
func (a *adapter) processLogs() {
	defer a.wg.Done()

	for {
		select {
		case entry, ok := <-a.buffer:
			if !ok {
				// Channel is closed, process any remaining entries
				return
			}

			a.writeLog(entry)
		case <-a.done:
			// Process any remaining entries in the buffer
			for {
				select {
				case entry, ok := <-a.buffer:
					if !ok {
						return
					}

					a.writeLog(entry)
				default:
					// No more entries in buffer
					return
				}
			}
		}
	}
}

// writeLog handles the actual writing of log entries with improved error reporting.
func (a *adapter) writeLog(entry logEntry) {
	if a.config.Output == nil {
		return
	}

	buf := a.getBuffer()
	defer bufferPool.Put(buf)

	a.formatEntry(buf, entry)
	a.ensureNewline(buf)

	contents := buf.Bytes()

	a.mu.Lock()
	defer a.mu.Unlock()

	switch output := a.config.Output.(type) {
	case *output.MultiWriter:
		a.handleMultiWriter(output, contents, entry)
	default:
		a.handleSingleWriter(output, contents, entry)
	}
}

func (a *adapter) getBuffer() *bytes.Buffer {
	buf, ok := bufferPool.Get().(*bytes.Buffer)
	if !ok {
		buf = new(bytes.Buffer)
	}

	buf.Reset()

	return buf
}

func (a *adapter) formatEntry(buf *bytes.Buffer, entry logEntry) {
	if a.config.EnableJSON {
		a.writeJSONLog(buf, entry)
	} else {
		a.writeTextLog(buf, entry)
	}
}

func (a *adapter) ensureNewline(buf *bytes.Buffer) {
	if buf.Len() > 0 && buf.Bytes()[buf.Len()-1] != '\n' {
		buf.WriteByte('\n')
	}
}

func (a *adapter) handleMultiWriter(output *output.MultiWriter, contents []byte, entry logEntry) {
	writeResults := a.collectWriteResults(output, contents)
	successCount, incompleteWrites, errorWrites := a.analyzeResults(writeResults, contents)

	if len(errorWrites) > 0 || len(incompleteWrites) > 0 {
		a.reportWriteIssues(entry, contents, successCount, len(writeResults), incompleteWrites, errorWrites)
	}
}

func (a *adapter) collectWriteResults(mwOutput *output.MultiWriter, contents []byte) []output.WriteResult {
	writeResults := make([]output.WriteResult, 0, len(mwOutput.Writers))

	for _, writer := range mwOutput.Writers {
		if writer == nil {
			continue
		}

		bytesWritten, err := writer.Write(contents)
		writeResults = append(writeResults, output.WriteResult{
			Writer: writer,
			Name:   fmt.Sprintf("%T", writer),
			Bytes:  bytesWritten,
			Err:    err,
		})
	}

	return writeResults
}

func (a *adapter) analyzeResults(writeResults []output.WriteResult, contents []byte) (int, []string, []string) {
	successCount := 0

	var incompleteWrites, errorWrites []string

	for _, result := range writeResults {
		switch {
		case result.Err != nil:
			errorWrites = append(errorWrites, fmt.Sprintf("%s: error: %v", result.Name, result.Err))
		case result.Bytes != len(contents):
			incompleteWrites = append(incompleteWrites, fmt.Sprintf("%s: partial write %d/%d bytes", result.Name, result.Bytes, len(contents)))
		default:
			successCount++
		}
	}

	return successCount, incompleteWrites, errorWrites
}

func (a *adapter) reportWriteIssues(entry logEntry, contents []byte, successCount, totalWrites int, incompleteWrites, errorWrites []string) {
	diagMsg := fmt.Sprintf(
		"Write issues detected:\n"+
			"  Level: %s\n"+
			"  Message: %q\n"+
			"  Buffer size: %d bytes\n"+
			"  Successful writes: %d/%d",
		entry.Level,
		entry.Message,
		len(contents),
		successCount,
		totalWrites,
	)

	if len(errorWrites) > 0 {
		diagMsg += "\n  Errors:\n    " + strings.Join(errorWrites, "\n    ")
	}

	if len(incompleteWrites) > 0 {
		diagMsg += "\n  Incomplete writes:\n    " + strings.Join(incompleteWrites, "\n    ")
	}

	fmt.Fprintln(os.Stderr, diagMsg)
}

func (a *adapter) handleSingleWriter(output io.Writer, contents []byte, entry logEntry) {
	bytesWritten, err := output.Write(contents)
	if err != nil || bytesWritten != len(contents) {
		fmt.Fprintf(os.Stderr,
			"Write issue detected:\n"+
				"  Level: %s\n"+
				"  Message: %q\n"+
				"  Writer type: %T\n"+
				"  Bytes written: %d/%d\n"+
				"  Error: %v\n",
			entry.Level,
			entry.Message,
			output,
			bytesWritten,
			len(contents),
			err,
		)
	}
}

// writeJSONLog formats and writes the log entry as JSON.
func (a *adapter) writeJSONLog(buf *bytes.Buffer, entry logEntry) {
	// Pre-allocate a map with enough capacity for all fields
	capacity := len(entry.Fields)
	if !a.config.DisableTimestamp {
		capacity++
	}

	if entry.Caller != "" {
		capacity++
	}

	if entry.Error != nil {
		capacity++
	}

	capacity += 2 // level and message are always present

	logMap := make(map[string]interface{}, capacity)

	// Add standard fields
	logMap["level"] = entry.Level.String()
	logMap["message"] = entry.Message

	if !a.config.DisableTimestamp {
		logMap["timestamp"] = entry.Timestamp.Format(a.config.TimeFormat)
	}

	if entry.Caller != "" {
		logMap["caller"] = entry.Caller
	}

	// Add all custom fields
	for _, field := range entry.Fields {
		logMap[field.Key] = field.Value
	}

	// Add any additional fields configured globally
	for _, field := range a.config.AdditionalFields {
		logMap[field.Key] = field.Value
	}

	// Marshal to JSON
	encoder := json.NewEncoder(buf)
	encoder.SetEscapeHTML(false)

	err := encoder.Encode(logMap)
	if err != nil {
		buf.WriteString(fmt.Sprintf("failed to marshal log entry to JSON: %s", err))
	}
}

// writeTextLog formats and writes the log entry as human-readable text.
//
//nolint:cyclop
func (a *adapter) writeTextLog(buf *bytes.Buffer, entry logEntry) {
	// Write timestamp if enabled
	if !a.config.DisableTimestamp {
		buf.WriteString(entry.Timestamp.Format(a.config.TimeFormat))
		buf.WriteByte(' ')
	}

	// Write log level with fixed width padding
	fmt.Fprintf(buf, "%-5s ", entry.Level.String())

	// Write caller information if available
	if entry.Caller != "" {
		buf.WriteByte('[')
		buf.WriteString(entry.Caller)
		buf.WriteString("] ")
	}

	// Write the message
	buf.WriteString(entry.Message)

	// Write fields if present
	if len(entry.Fields) > 0 || len(a.config.AdditionalFields) > 0 {
		buf.WriteString(" {")

		// Write custom fields
		for i, field := range entry.Fields {
			if i > 0 {
				buf.WriteString(", ")
			}

			writeField(buf, field)
		}

		// Write additional fields
		if len(entry.Fields) > 0 && len(a.config.AdditionalFields) > 0 {
			buf.WriteString(", ")
		}

		for i, field := range a.config.AdditionalFields {
			if i > 0 {
				buf.WriteString(", ")
			}

			writeField(buf, field)
		}

		buf.WriteByte('}')
	}
}

// writeField formats and writes a single field.
func writeField(buf *bytes.Buffer, field logger.Field) {
	buf.WriteString(field.Key)
	buf.WriteString("=")

	// Handle different value types
	switch val := field.Value.(type) {
	case string:
		buf.WriteByte('"')
		buf.WriteString(val)
		buf.WriteByte('"')
	case time.Time:
		buf.WriteByte('"')
		buf.WriteString(val.Format(time.RFC3339))
		buf.WriteByte('"')
	case error:
		buf.WriteByte('"')
		buf.WriteString(val.Error())
		buf.WriteByte('"')
	default:
		fmt.Fprintf(buf, "%v", val)
	}
}

// WithContext adds contextual information to the log entry.
func (a *adapter) WithContext(ctx context.Context) logger.Logger {
	// Extract relevant information from context
	// Example: trace IDs, request IDs, etc.
	fields := extractContextFields(ctx)

	return a.WithFields(fields...)
}

// WithFields adds additional fields to the log entry.
func (a *adapter) WithFields(fields ...logger.Field) logger.Logger {
	a.mu.Lock()
	defer a.mu.Unlock()

	newAdapter := &adapter{
		config: a.config,
		buffer: a.buffer,
		done:   a.done,
		wg:     a.wg, // Share the pointer to WaitGroup
		fields: make([]logger.Field, len(a.fields), len(a.fields)+len(fields)),
	}
	copy(newAdapter.fields, a.fields)
	newAdapter.fields = append(newAdapter.fields, fields...)

	return newAdapter
}

// WithError adds an error field to the log entry.
func (a *adapter) WithError(err error) logger.Logger {
	if err == nil {
		return a
	}

	fields := []logger.Field{
		{Key: "error", Value: err.Error()},
	}

	// If it's our custom error type, extract additional information
	if wrappedErr, ok := err.(interface{ StackTrace() string }); ok {
		fields = append(fields, logger.Field{
			Key:   "stack_trace",
			Value: wrappedErr.StackTrace(),
		})
	}

	return a.WithFields(fields...)
}

// log ensures entries are properly handled even during shutdown.
func (a *adapter) log(level logger.Level, msg string) {
	if level < a.config.Level {
		return
	}

	entry := logEntry{
		Level:     level,
		Message:   msg,
		Fields:    a.fields,
		Timestamp: time.Now(),
	}

	if a.config.EnableCaller {
		entry.Caller = getCaller()
	}

	// Try to send to buffer with a timeout
	select {
	case a.buffer <- entry:
		// Successfully queued the entry
	case <-time.After(bufferTimeout):
		// Buffer full or shutdown in progress, fall back to synchronous write
		a.writeLog(entry)
	}
}

func getCaller() string {
	_, file, line, ok := runtime.Caller(callerDepth)
	if !ok {
		return "unknown"
	}

	// Trim the file path to the last two directories
	parts := strings.Split(file, "/")
	//nolint:mnd
	if len(parts) > 2 {
		file = strings.Join(parts[len(parts)-2:], "/")
	}

	return fmt.Sprintf("%s:%d", file, line)
}

// Implement all the logging methods.
func (a *adapter) Trace(msg string)                          { a.log(logger.TraceLevel, msg) }
func (a *adapter) Debug(msg string)                          { a.log(logger.DebugLevel, msg) }
func (a *adapter) Info(msg string)                           { a.log(logger.InfoLevel, msg) }
func (a *adapter) Warn(msg string)                           { a.log(logger.WarnLevel, msg) }
func (a *adapter) Error(msg string)                          { a.log(logger.ErrorLevel, msg) }
func (a *adapter) Fatal(msg string)                          { a.log(logger.FatalLevel, msg) }
func (a *adapter) Tracef(format string, args ...interface{}) { a.Trace(fmt.Sprintf(format, args...)) }
func (a *adapter) Debugf(format string, args ...interface{}) { a.Debug(fmt.Sprintf(format, args...)) }
func (a *adapter) Infof(format string, args ...interface{})  { a.Info(fmt.Sprintf(format, args...)) }
func (a *adapter) Warnf(format string, args ...interface{})  { a.Warn(fmt.Sprintf(format, args...)) }
func (a *adapter) Errorf(format string, args ...interface{}) { a.Error(fmt.Sprintf(format, args...)) }
func (a *adapter) Fatalf(format string, args ...interface{}) { a.Fatal(fmt.Sprintf(format, args...)) }

// GetLevel returns the current logging level for the adapter.
// This allows controlling the verbosity of the logging output.
func (a *adapter) GetLevel() logger.Level {
	a.mu.RLock()
	defer a.mu.RUnlock()

	return a.config.Level
}

// SetLevel sets the logging level for the adapter. This allows controlling the
// verbosity of the logging output.
func (a *adapter) SetLevel(level logger.Level) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.config.Level = level
}

// Sync ensures all pending logs are written before shutdown.
func (a *adapter) Sync() error {
	// Signal shutdown
	close(a.done)

	// Close the buffer channel after signaling shutdown
	close(a.buffer)

	// Wait for all pending writes to complete
	a.wg.Wait()

	// Sync the underlying writer
	if syncer, ok := a.config.Output.(interface{ Sync() error }); ok {
		return syncer.Sync()
	}

	return nil
}

// Helper functions to extract context fields.
func extractContextFields(ctx context.Context) []logger.Field {
	var fields []logger.Field

	// Example: Extract trace ID
	if traceID := ctx.Value("trace_id"); traceID != nil {
		fields = append(fields, logger.Field{
			Key:   "trace_id",
			Value: traceID,
		})
	}

	return fields
}
