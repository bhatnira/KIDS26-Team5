package sse

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
)

// Writer writes Server-Sent Events to an http.ResponseWriter.
// NewWriter must be called before any data is written to w (i.e. before headers are flushed).
type Writer struct {
	w       http.ResponseWriter
	flusher http.Flusher
}

// NewWriter creates a Writer and sets the required SSE headers.
// Returns an error if w does not implement http.Flusher, which is required for SSE.
func NewWriter(w http.ResponseWriter) (*Writer, error) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		return nil, errors.New("sse: response writer does not support flushing")
	}

	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	h.Set("X-Accel-Buffering", "no")

	return &Writer{w: w, flusher: flusher}, nil
}

// WriteEvent sends a named SSE event with a string payload.
// Multi-line data is handled correctly per the SSE spec.
func (s *Writer) WriteEvent(eventType, data string) error {
	var sb strings.Builder
	sb.WriteString("event: ")
	sb.WriteString(eventType)
	sb.WriteByte('\n')
	writeDataLines(&sb, data)
	sb.WriteString("\n")

	if _, err := fmt.Fprint(s.w, sb.String()); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

// WriteData sends an unnamed SSE data frame.
// Multi-line data is handled correctly per the SSE spec.
func (s *Writer) WriteData(data string) error {
	var sb strings.Builder
	writeDataLines(&sb, data)
	sb.WriteString("\n")

	if _, err := fmt.Fprint(s.w, sb.String()); err != nil {
		return err
	}
	s.flusher.Flush()
	return nil
}

// WriteJSON marshals v to JSON and sends it as a named event.
func (s *Writer) WriteJSON(eventType string, v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("sse json marshal: %w", err)
	}
	return s.WriteEvent(eventType, string(b))
}

// WriteDataJSON marshals v to JSON and sends it as an unnamed data frame.
func (s *Writer) WriteDataJSON(v any) error {
	b, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("sse json marshal: %w", err)
	}
	return s.WriteData(string(b))
}

// Flush manually flushes buffered data to the client.
func (s *Writer) Flush() {
	s.flusher.Flush()
}

// writeDataLines writes each line of data as a separate "data: ...\n" field,
// conforming to the SSE specification for multi-line values.
func writeDataLines(sb *strings.Builder, data string) {
	lines := strings.SplitSeq(data, "\n")
	for line := range lines {
		sb.WriteString("data: ")
		sb.WriteString(line)
		sb.WriteByte('\n')
	}
}
