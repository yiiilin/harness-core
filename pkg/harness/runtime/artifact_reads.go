package runtime

import (
	"context"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

type ArtifactReadRequest struct {
	Path       string `json:"path,omitempty"`
	Offset     int64  `json:"offset,omitempty"`
	MaxBytes   int    `json:"max_bytes,omitempty"`
	LineOffset int    `json:"line_offset,omitempty"`
	MaxLines   int    `json:"max_lines,omitempty"`
}

type ArtifactReadResult struct {
	ArtifactID     string `json:"artifact_id,omitempty"`
	Path           string `json:"path,omitempty"`
	Data           string `json:"data,omitempty"`
	Truncated      bool   `json:"truncated,omitempty"`
	OriginalBytes  int    `json:"original_bytes,omitempty"`
	ReturnedBytes  int    `json:"returned_bytes,omitempty"`
	OriginalChars  int    `json:"original_chars,omitempty"`
	ReturnedChars  int    `json:"returned_chars,omitempty"`
	OriginalLines  int    `json:"original_lines,omitempty"`
	ReturnedLines  int    `json:"returned_lines,omitempty"`
	HasMore        bool   `json:"has_more,omitempty"`
	NextOffset     int64  `json:"next_offset,omitempty"`
	NextLineOffset int    `json:"next_line_offset,omitempty"`
	RawRef         string `json:"raw_ref,omitempty"`
}

func (s *Service) ReadArtifact(artifactID string, request ArtifactReadRequest) (ArtifactReadResult, error) {
	artifact, err := s.getArtifactRecord(context.Background(), artifactID)
	if err != nil {
		return ArtifactReadResult{}, err
	}
	return readArtifactWindow(artifact, request)
}

func readArtifactWindow(artifact execution.Artifact, request ArtifactReadRequest) (ArtifactReadResult, error) {
	value, ok := resolveArtifactReadValue(artifact.Payload, request.Path)
	if !ok {
		return ArtifactReadResult{}, fmt.Errorf("%w: %q", ErrArtifactReadPathNotFound, request.Path)
	}
	if request.MaxLines > 0 || request.LineOffset > 0 {
		return readArtifactLineWindow(artifact, request, value)
	}
	return readArtifactByteWindow(artifact, request, value)
}

func resolveArtifactReadValue(payload map[string]any, path string) (any, bool) {
	trimmed := strings.TrimSpace(path)
	if trimmed == "" {
		return payload, true
	}
	return descendProgramValue(payload, strings.Split(trimmed, "."))
}

func readArtifactByteWindow(artifact execution.Artifact, request ArtifactReadRequest, value any) (ArtifactReadResult, error) {
	raw, ok := bytesFromProgramValue(value)
	if !ok {
		return ArtifactReadResult{}, fmt.Errorf("%w: %q", ErrArtifactReadUnsupported, request.Path)
	}
	offset := request.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > int64(len(raw)) {
		offset = int64(len(raw))
	}
	end := len(raw)
	if request.MaxBytes > 0 && int(offset)+request.MaxBytes < end {
		end = int(offset) + request.MaxBytes
	}
	window := raw[offset:int64(end)]
	hasMore := end < len(raw)
	return ArtifactReadResult{
		ArtifactID:    artifact.ArtifactID,
		Path:          request.Path,
		Data:          string(window),
		Truncated:     hasMore,
		OriginalBytes: len(raw),
		ReturnedBytes: len(window),
		OriginalChars: utf8.RuneCount(raw),
		ReturnedChars: utf8.RuneCount(window),
		HasMore:       hasMore,
		NextOffset:    int64(end),
		RawRef:        artifact.ArtifactID,
	}, nil
}

func readArtifactLineWindow(artifact execution.Artifact, request ArtifactReadRequest, value any) (ArtifactReadResult, error) {
	text, ok := stringFromProgramValue(value)
	if !ok {
		raw, bytesOK := bytesFromProgramValue(value)
		if !bytesOK {
			return ArtifactReadResult{}, fmt.Errorf("%w: %q", ErrArtifactReadUnsupported, request.Path)
		}
		text = string(raw)
	}
	lines := splitLinesPreservingDelimiters(text)
	offset := request.LineOffset
	if offset < 0 {
		offset = 0
	}
	if offset > len(lines) {
		offset = len(lines)
	}
	end := len(lines)
	if request.MaxLines > 0 && offset+request.MaxLines < end {
		end = offset + request.MaxLines
	}
	data := strings.Join(lines[offset:end], "")
	nextOffset := int64(0)
	for _, line := range lines[:end] {
		nextOffset += int64(len(line))
	}
	hasMore := end < len(lines)
	return ArtifactReadResult{
		ArtifactID:     artifact.ArtifactID,
		Path:           request.Path,
		Data:           data,
		Truncated:      hasMore,
		OriginalBytes:  len(text),
		ReturnedBytes:  len(data),
		OriginalChars:  utf8.RuneCountInString(text),
		ReturnedChars:  utf8.RuneCountInString(data),
		OriginalLines:  len(lines),
		ReturnedLines:  end - offset,
		HasMore:        hasMore,
		NextOffset:     nextOffset,
		NextLineOffset: end,
		RawRef:         artifact.ArtifactID,
	}, nil
}

func splitLinesPreservingDelimiters(text string) []string {
	if text == "" {
		return nil
	}
	lines := strings.SplitAfter(text, "\n")
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}
