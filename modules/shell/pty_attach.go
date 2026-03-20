package shellmodule

import (
	"context"
	"errors"
	"io"
	"time"

	"github.com/google/uuid"
)

type PTYAttachOptions struct {
	Input        io.Reader
	Output       io.Writer
	Offset       int64
	MaxBytes     int
	PollInterval time.Duration
}

type PTYAttachment struct {
	AttachmentID string `json:"attachment_id"`
	HandleID     string `json:"handle_id"`
	AttachedAt   int64  `json:"attached_at"`
}

type ptyAttachmentState struct {
	meta   PTYAttachment
	cancel context.CancelFunc
	input  io.Reader
}

func (m *PTYManager) Attach(ctx context.Context, handleID string, opts PTYAttachOptions) (PTYAttachment, error) {
	if _, err := m.session(handleID); err != nil {
		return PTYAttachment{}, err
	}
	if opts.Input == nil && opts.Output == nil {
		return PTYAttachment{}, errors.New("attach requires input and/or output")
	}

	attachCtx, cancel := context.WithCancel(ctx)
	meta := PTYAttachment{
		AttachmentID: "att_" + uuid.NewString(),
		HandleID:     handleID,
		AttachedAt:   time.Now().UnixMilli(),
	}
	state := &ptyAttachmentState{
		meta:   meta,
		cancel: cancel,
		input:  opts.Input,
	}

	m.mu.Lock()
	if m.attachments == nil {
		m.attachments = map[string]*ptyAttachmentState{}
	}
	m.attachments[meta.AttachmentID] = state
	m.mu.Unlock()

	if opts.Output != nil {
		go m.streamAttachedOutput(attachCtx, meta.AttachmentID, handleID, opts)
	}
	if opts.Input != nil {
		go m.streamAttachedInput(attachCtx, meta.AttachmentID, handleID, opts.Input)
	}

	return meta, nil
}

func (m *PTYManager) Detach(attachmentID string) error {
	m.mu.Lock()
	state, ok := m.attachments[attachmentID]
	if ok {
		delete(m.attachments, attachmentID)
	}
	m.mu.Unlock()
	if !ok {
		return ErrPTYSessionNotFound
	}
	state.cancel()
	if closer, ok := state.input.(io.Closer); ok {
		_ = closer.Close()
	}
	return nil
}

func (m *PTYManager) streamAttachedOutput(ctx context.Context, attachmentID, handleID string, opts PTYAttachOptions) {
	offset := opts.Offset
	maxBytes := opts.MaxBytes
	if maxBytes <= 0 {
		maxBytes = m.opts.DefaultReadBytes
	}
	pollInterval := opts.PollInterval
	if pollInterval <= 0 {
		pollInterval = 25 * time.Millisecond
	}

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		read, err := m.Read(ctx, handleID, PTYReadRequest{
			Offset:   offset,
			MaxBytes: maxBytes,
		})
		if err != nil {
			return
		}
		if read.NextOffset > offset {
			if _, err := opts.Output.Write([]byte(read.Data)); err != nil {
				return
			}
			offset = read.NextOffset
		}
		if read.Closed {
			return
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(pollInterval):
		}
	}
}

func (m *PTYManager) streamAttachedInput(ctx context.Context, attachmentID, handleID string, input io.Reader) {
	buffer := make([]byte, 1024)
	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		n, err := input.Read(buffer)
		if n > 0 {
			if _, writeErr := m.Write(ctx, handleID, string(buffer[:n])); writeErr != nil {
				return
			}
		}
		if err != nil {
			if errors.Is(err, io.EOF) {
				return
			}
			return
		}
	}
}
