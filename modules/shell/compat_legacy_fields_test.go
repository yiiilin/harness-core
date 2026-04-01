package shellmodule

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestPTYReadResultKeepsLegacyPreviewFields(t *testing.T) {
	ctx := context.Background()
	manager := NewPTYManager(PTYManagerOptions{})
	t.Cleanup(func() {
		_ = manager.CloseAll(ctx, "test cleanup")
	})

	started, err := manager.Start(ctx, "printf 'hello world'; sleep 2", PTYStartOptions{})
	if err != nil {
		t.Fatalf("start pty session: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		read, err := manager.Read(ctx, started.RuntimeHandle.HandleID, PTYReadRequest{MaxBytes: 5})
		if err != nil {
			t.Fatalf("read pty session: %v", err)
		}
		if !strings.Contains(read.Data, "hello") {
			time.Sleep(25 * time.Millisecond)
			continue
		}
		if read.Window == nil || read.RawHandle == nil {
			t.Fatalf("expected unified preview metadata, got %#v", read)
		}
		if read.NextOffset != read.Window.NextOffset || read.OriginalBytes != read.Window.OriginalBytes || read.ReturnedBytes != read.Window.ReturnedBytes {
			t.Fatalf("expected legacy preview fields to mirror window metadata, got %#v", read)
		}
		if read.Truncated != read.Window.Truncated || read.HasMore != read.Window.HasMore {
			t.Fatalf("expected legacy truncation flags to mirror window metadata, got %#v", read)
		}
		if read.RawRef != read.RawHandle.Ref {
			t.Fatalf("expected legacy raw_ref to mirror raw handle, got %#v", read)
		}
		return
	}

	t.Fatalf("timed out waiting for PTY preview data")
}

type legacyInteractiveViewShape struct {
	Truncated     bool
	OriginalBytes int
	ReturnedBytes int
	HasMore       bool
	NextOffset    int64
	RawRef        string
}

func TestSetInteractiveViewLegacyPreviewFieldsBackfillsLegacyShape(t *testing.T) {
	target := &legacyInteractiveViewShape{}
	read := PTYReadResult{
		Window: &ResultWindow{
			Truncated:     true,
			OriginalBytes: 11,
			ReturnedBytes: 5,
			HasMore:       true,
			NextOffset:    5,
		},
		RawHandle: &RawResultHandle{
			Ref:    "hdl_legacy",
			Reread: true,
		},
	}

	setInteractiveViewLegacyPreviewFields(target, read)

	if !target.Truncated || target.OriginalBytes != 11 || target.ReturnedBytes != 5 || !target.HasMore || target.NextOffset != 5 || target.RawRef != "hdl_legacy" {
		t.Fatalf("expected legacy interactive view fields to be backfilled, got %#v", target)
	}
}
