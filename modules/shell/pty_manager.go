package shellmodule

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"

	"github.com/creack/pty"
	"github.com/google/uuid"
	"github.com/yiiilin/harness-core/pkg/harness/execution"
)

var ErrPTYSessionNotFound = errors.New("pty session not found")
var ErrPTYSessionClosed = errors.New("pty session closed")

type PTYManagerOptions struct {
	ShellPath        string
	DefaultColumns   int
	DefaultRows      int
	DefaultReadBytes int
}

type PTYStartOptions struct {
	CWD      string
	Env      map[string]string
	Metadata map[string]any
}

type PTYSessionStart struct {
	RuntimeHandle execution.RuntimeHandle `json:"runtime_handle"`
	Stream        PTYStreamInfo           `json:"stream"`
	PID           int                     `json:"pid"`
}

type PTYStreamInfo struct {
	HandleID     string `json:"handle_id"`
	Kind         string `json:"kind"`
	Status       string `json:"status"`
	NextOffset   int64  `json:"next_offset"`
	StatusReason string `json:"status_reason,omitempty"`
}

type PTYReadRequest struct {
	Offset   int64 `json:"offset,omitempty"`
	MaxBytes int   `json:"max_bytes,omitempty"`
}

type PTYReadResult struct {
	HandleID     string `json:"handle_id"`
	Data         string `json:"data,omitempty"`
	NextOffset   int64  `json:"next_offset"`
	Closed       bool   `json:"closed,omitempty"`
	ExitCode     int    `json:"exit_code,omitempty"`
	Status       string `json:"status"`
	StatusReason string `json:"status_reason,omitempty"`
	Truncated    bool   `json:"truncated,omitempty"`
}

type PTYResizeRequest struct {
	Columns int `json:"columns,omitempty"`
	Rows    int `json:"rows,omitempty"`
}

type PTYManager struct {
	mu          sync.RWMutex
	sessions    map[string]*ptySession
	attachments map[string]*ptyAttachmentState
	opts        PTYManagerOptions
}

type ptySession struct {
	mu           sync.RWMutex
	handle       execution.RuntimeHandle
	file         *os.File
	cmd          *exec.Cmd
	buffer       []byte
	closed       bool
	exitCode     int
	statusReason string
}

func NewPTYManager(opts PTYManagerOptions) *PTYManager {
	if opts.ShellPath == "" {
		opts.ShellPath = "/bin/bash"
	}
	if opts.DefaultColumns <= 0 {
		opts.DefaultColumns = 80
	}
	if opts.DefaultRows <= 0 {
		opts.DefaultRows = 24
	}
	if opts.DefaultReadBytes <= 0 {
		opts.DefaultReadBytes = 4096
	}
	return &PTYManager{
		sessions:    map[string]*ptySession{},
		attachments: map[string]*ptyAttachmentState{},
		opts:        opts,
	}
}

func (m *PTYManager) Start(ctx context.Context, command string, opts PTYStartOptions) (PTYSessionStart, error) {
	if command == "" {
		return PTYSessionStart{}, errors.New("command is required")
	}
	select {
	case <-ctx.Done():
		return PTYSessionStart{}, ctx.Err()
	default:
	}

	cmd := exec.Command(m.opts.ShellPath, "-lc", command)
	if opts.CWD != "" {
		cmd.Dir = opts.CWD
	}
	cmd.Env = append(os.Environ(), envPairs(opts.Env)...)

	now := time.Now().UnixMilli()
	handleID := "hdl_" + uuid.NewString()
	handle := execution.RuntimeHandle{
		HandleID:     handleID,
		Kind:         "pty",
		Value:        handleID,
		Status:       execution.RuntimeHandleActive,
		StatusReason: "pty session active",
		Metadata: map[string]any{
			"command":                               command,
			"cwd":                                   opts.CWD,
			"mode":                                  "pty",
			execution.InteractiveMetadataKeyEnabled: true,
			execution.InteractiveMetadataKeySupportsView:  true,
			execution.InteractiveMetadataKeySupportsWrite: true,
			execution.InteractiveMetadataKeySupportsClose: true,
			execution.InteractiveMetadataKeyStatus:        "active",
			execution.InteractiveMetadataKeyStatusReason:  "pty session active",
			execution.InteractiveMetadataKeyNextOffset:    int64(0),
		},
		CreatedAt: now,
		UpdatedAt: now,
	}
	for key, value := range cloneAnyMap(opts.Metadata) {
		handle.Metadata[key] = value
	}

	win := &pty.Winsize{Cols: uint16(m.opts.DefaultColumns), Rows: uint16(m.opts.DefaultRows)}
	tty, err := pty.StartWithSize(cmd, win)
	if err != nil {
		return PTYSessionStart{}, err
	}
	handle.Metadata["pid"] = cmd.Process.Pid

	session := &ptySession{
		handle:       handle,
		file:         tty,
		cmd:          cmd,
		statusReason: "pty session active",
	}

	m.mu.Lock()
	m.sessions[handleID] = session
	m.mu.Unlock()

	go session.captureOutput()
	go session.waitForExit()

	return PTYSessionStart{
		RuntimeHandle: handle,
		Stream: PTYStreamInfo{
			HandleID:   handleID,
			Kind:       "pty",
			Status:     "active",
			NextOffset: 0,
		},
		PID: cmd.Process.Pid,
	}, nil
}

func (m *PTYManager) Read(_ context.Context, handleID string, req PTYReadRequest) (PTYReadResult, error) {
	session, err := m.session(handleID)
	if err != nil {
		return PTYReadResult{}, err
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	offset := req.Offset
	if offset < 0 {
		offset = 0
	}
	if offset > int64(len(session.buffer)) {
		offset = int64(len(session.buffer))
	}
	maxBytes := req.MaxBytes
	if maxBytes <= 0 {
		maxBytes = m.opts.DefaultReadBytes
	}

	end := len(session.buffer)
	truncated := false
	if maxBytes > 0 && int(offset)+maxBytes < end {
		end = int(offset) + maxBytes
		truncated = true
	}

	status := "active"
	if session.closed {
		status = "closed"
	}

	return PTYReadResult{
		HandleID:     handleID,
		Data:         string(session.buffer[offset:int64(end)]),
		NextOffset:   int64(end),
		Closed:       session.closed,
		ExitCode:     session.exitCode,
		Status:       status,
		StatusReason: session.statusReason,
		Truncated:    truncated,
	}, nil
}

func (m *PTYManager) Inspect(_ context.Context, handleID string) (PTYInspectResult, error) {
	session, err := m.session(handleID)
	if err != nil {
		return PTYInspectResult{}, err
	}

	session.mu.RLock()
	defer session.mu.RUnlock()

	status := "active"
	if session.closed {
		status = "closed"
	}
	return PTYInspectResult{
		HandleID:     handleID,
		Closed:       session.closed,
		ExitCode:     session.exitCode,
		Status:       status,
		StatusReason: session.statusReason,
	}, nil
}

func (m *PTYManager) Write(ctx context.Context, handleID, input string) (int, error) {
	session, err := m.session(handleID)
	if err != nil {
		return 0, err
	}
	select {
	case <-ctx.Done():
		return 0, ctx.Err()
	default:
	}

	session.mu.RLock()
	closed := session.closed
	file := session.file
	session.mu.RUnlock()
	if closed {
		return 0, ErrPTYSessionClosed
	}
	if file == nil {
		return 0, ErrPTYSessionNotFound
	}
	return file.Write([]byte(input))
}

func (m *PTYManager) Resize(_ context.Context, handleID string, req PTYResizeRequest) error {
	session, err := m.session(handleID)
	if err != nil {
		return err
	}
	session.mu.RLock()
	defer session.mu.RUnlock()
	if session.file == nil {
		return ErrPTYSessionNotFound
	}
	cols := req.Columns
	rows := req.Rows
	if cols <= 0 {
		cols = m.opts.DefaultColumns
	}
	if rows <= 0 {
		rows = m.opts.DefaultRows
	}
	return pty.Setsize(session.file, &pty.Winsize{Cols: uint16(cols), Rows: uint16(rows)})
}

func (m *PTYManager) Close(_ context.Context, handleID, reason string) error {
	session, err := m.session(handleID)
	if err != nil {
		return err
	}
	m.detachForHandle(handleID)

	session.mu.Lock()
	if reason != "" {
		session.statusReason = reason
	}
	file := session.file
	process := session.cmd.Process
	session.mu.Unlock()

	if process != nil {
		_ = process.Kill()
	}
	if file != nil {
		_ = file.Close()
	}
	return nil
}

func (m *PTYManager) CloseAll(ctx context.Context, reason string) error {
	m.detachAll()
	m.mu.RLock()
	ids := make([]string, 0, len(m.sessions))
	for id := range m.sessions {
		ids = append(ids, id)
	}
	m.mu.RUnlock()

	for _, id := range ids {
		if err := m.Close(ctx, id, reason); err != nil && !errors.Is(err, ErrPTYSessionNotFound) {
			return err
		}
	}
	return nil
}

func (m *PTYManager) detachForHandle(handleID string) {
	m.mu.Lock()
	attachments := []*ptyAttachmentState{}
	for id, state := range m.attachments {
		if state.meta.HandleID != handleID {
			continue
		}
		delete(m.attachments, id)
		attachments = append(attachments, state)
	}
	m.mu.Unlock()

	for _, state := range attachments {
		state.cancel()
		if closer, ok := state.input.(io.Closer); ok {
			_ = closer.Close()
		}
	}
}

func (m *PTYManager) detachAll() {
	m.mu.Lock()
	attachments := make([]*ptyAttachmentState, 0, len(m.attachments))
	for id, state := range m.attachments {
		delete(m.attachments, id)
		attachments = append(attachments, state)
	}
	m.mu.Unlock()

	for _, state := range attachments {
		state.cancel()
		if closer, ok := state.input.(io.Closer); ok {
			_ = closer.Close()
		}
	}
}

func (m *PTYManager) session(handleID string) (*ptySession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	session, ok := m.sessions[handleID]
	if !ok {
		return nil, ErrPTYSessionNotFound
	}
	return session, nil
}

func (s *ptySession) captureOutput() {
	buf := make([]byte, 1024)
	for {
		n, err := s.file.Read(buf)
		if n > 0 {
			s.mu.Lock()
			s.buffer = append(s.buffer, buf[:n]...)
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

func (s *ptySession) waitForExit() {
	err := s.cmd.Wait()
	exitCode := 0
	if err != nil {
		var exitErr *exec.ExitError
		if errors.As(err, &exitErr) {
			exitCode = exitErr.ExitCode()
		} else {
			exitCode = -1
		}
	}

	now := time.Now().UnixMilli()
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	s.exitCode = exitCode
	if s.statusReason == "" {
		s.statusReason = "pty session exited"
	}
	s.handle.Status = execution.RuntimeHandleClosed
	s.handle.StatusReason = s.statusReason
	if s.handle.ClosedAt == 0 {
		s.handle.ClosedAt = now
	}
	s.handle.UpdatedAt = now
}

func envPairs(env map[string]string) []string {
	out := make([]string, 0, len(env))
	for key, value := range env {
		if key == "" {
			continue
		}
		out = append(out, key+"="+value)
	}
	return out
}
