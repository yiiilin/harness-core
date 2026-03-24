package shellmodule

import (
	"context"
	"errors"
	"fmt"

	"github.com/yiiilin/harness-core/pkg/harness/execution"
	hruntime "github.com/yiiilin/harness-core/pkg/harness/runtime"
)

var ErrInteractiveControllerNotConfigured = errors.New("interactive controller is not configured")

type PTYInteractiveController struct {
	Manager *PTYManager
}

func NewInteractiveController(manager *PTYManager) PTYInteractiveController {
	return PTYInteractiveController{Manager: manager}
}

func (c PTYInteractiveController) StartInteractive(ctx context.Context, request hruntime.InteractiveStartRequest) (hruntime.InteractiveStartResult, error) {
	if c.Manager == nil {
		return hruntime.InteractiveStartResult{}, ErrInteractiveControllerNotConfigured
	}
	if request.Kind != "" && request.Kind != "pty" {
		return hruntime.InteractiveStartResult{}, fmt.Errorf("interactive kind %q is unsupported", request.Kind)
	}
	command, _ := request.Spec["command"].(string)
	if command == "" {
		return hruntime.InteractiveStartResult{}, errors.New("interactive start requires command")
	}
	started, err := c.Manager.Start(ctx, command, PTYStartOptions{
		CWD:      stringValue(request.Spec["cwd"]),
		Env:      stringifyEnv(asAnyMap(request.Spec["env"])),
		Metadata: mergeMaps(cloneAnyMap(request.Metadata), asAnyMap(request.Spec["metadata"])),
	})
	if err != nil {
		return hruntime.InteractiveStartResult{}, err
	}
	return hruntime.InteractiveStartResult{
		Kind:  "pty",
		Value: started.RuntimeHandle.HandleID,
		Capabilities: execution.InteractiveCapabilities{
			Reopen: true,
			View:   true,
			Write:  true,
			Close:  true,
		},
		Observation: execution.InteractiveObservation{
			NextOffset:   started.Stream.NextOffset,
			Status:       started.Stream.Status,
			StatusReason: firstNonEmptyString(started.Stream.StatusReason, "pty session active"),
		},
		Metadata: cloneAnyMap(started.RuntimeHandle.Metadata),
	}, nil
}

func (c PTYInteractiveController) ReopenInteractive(ctx context.Context, handle execution.RuntimeHandle, _ hruntime.InteractiveReopenRequest) (hruntime.InteractiveReopenResult, error) {
	if c.Manager == nil {
		return hruntime.InteractiveReopenResult{}, ErrInteractiveControllerNotConfigured
	}
	inspect, err := c.Manager.Inspect(ctx, ptyHandleValue(handle))
	if err != nil {
		return hruntime.InteractiveReopenResult{}, err
	}
	observation := interactiveObservationFromInspect(inspect)
	return hruntime.InteractiveReopenResult{
		Observation: &observation,
	}, nil
}

func (c PTYInteractiveController) ViewInteractive(ctx context.Context, handle execution.RuntimeHandle, request hruntime.InteractiveViewRequest) (hruntime.InteractiveViewResult, error) {
	if c.Manager == nil {
		return hruntime.InteractiveViewResult{}, ErrInteractiveControllerNotConfigured
	}
	read, err := c.Manager.Read(ctx, ptyHandleValue(handle), PTYReadRequest{
		Offset:   request.Offset,
		MaxBytes: request.MaxBytes,
	})
	if err != nil {
		return hruntime.InteractiveViewResult{}, err
	}
	observation := execution.InteractiveObservation{
		NextOffset:   read.NextOffset,
		Closed:       read.Closed,
		Status:       read.Status,
		StatusReason: read.StatusReason,
	}
	if read.Closed {
		exitCode := read.ExitCode
		observation.ExitCode = &exitCode
	}
	return hruntime.InteractiveViewResult{
		Data:      read.Data,
		Truncated: read.Truncated,
		Runtime: execution.InteractiveRuntime{
			Handle:       handle,
			Observation:  observation,
			Capabilities: execution.InteractiveCapabilities{Reopen: true, View: true, Write: true, Close: true},
		},
	}, nil
}

func (c PTYInteractiveController) WriteInteractive(ctx context.Context, handle execution.RuntimeHandle, request hruntime.InteractiveWriteRequest) (hruntime.InteractiveWriteResult, error) {
	if c.Manager == nil {
		return hruntime.InteractiveWriteResult{}, ErrInteractiveControllerNotConfigured
	}
	n, err := c.Manager.Write(ctx, ptyHandleValue(handle), request.Input)
	if err != nil {
		return hruntime.InteractiveWriteResult{}, err
	}
	inspect, err := c.Manager.Inspect(ctx, ptyHandleValue(handle))
	if err != nil {
		return hruntime.InteractiveWriteResult{}, err
	}
	return hruntime.InteractiveWriteResult{
		Bytes: int64(n),
		Runtime: execution.InteractiveRuntime{
			Handle:       handle,
			Observation:  interactiveObservationFromInspect(inspect),
			Capabilities: execution.InteractiveCapabilities{Reopen: true, View: true, Write: true, Close: true},
		},
	}, nil
}

func (c PTYInteractiveController) CloseInteractive(ctx context.Context, handle execution.RuntimeHandle, request hruntime.InteractiveCloseRequest) (hruntime.InteractiveCloseResult, error) {
	if c.Manager == nil {
		return hruntime.InteractiveCloseResult{}, ErrInteractiveControllerNotConfigured
	}
	if err := c.Manager.Close(ctx, ptyHandleValue(handle), request.Reason); err != nil {
		return hruntime.InteractiveCloseResult{}, err
	}
	observation := execution.InteractiveObservation{
		Closed:       true,
		Status:       "closed",
		StatusReason: firstNonEmptyString(request.Reason, "pty session closed"),
	}
	return hruntime.InteractiveCloseResult{
		Runtime: execution.InteractiveRuntime{
			Handle:       handle,
			Observation:  observation,
			Capabilities: execution.InteractiveCapabilities{Reopen: true, View: true, Write: true, Close: true},
		},
	}, nil
}

func interactiveObservationFromInspect(inspect PTYInspectResult) execution.InteractiveObservation {
	observation := execution.InteractiveObservation{
		Closed:       inspect.Closed,
		Status:       inspect.Status,
		StatusReason: inspect.StatusReason,
	}
	if inspect.Closed {
		exitCode := inspect.ExitCode
		observation.ExitCode = &exitCode
	}
	return observation
}

func ptyHandleValue(handle execution.RuntimeHandle) string {
	return firstNonEmptyString(handle.Value, handle.HandleID)
}

func stringValue(raw any) string {
	text, _ := raw.(string)
	return text
}

func firstNonEmptyString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func mergeMaps(base map[string]any, extra map[string]any) map[string]any {
	if len(base) == 0 && len(extra) == 0 {
		return nil
	}
	out := map[string]any{}
	for key, value := range base {
		out[key] = value
	}
	for key, value := range extra {
		out[key] = value
	}
	return out
}
