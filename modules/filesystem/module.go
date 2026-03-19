package filesystemmodule

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type existsHandler struct{}

type readHandler struct{}

type writeHandler struct{}

type listHandler struct{}

type fileExistsChecker struct{}

type fileContentContainsChecker struct{}

func Register(tools *tool.Registry, verifiers *verify.Registry) {
	if tools != nil {
		tools.Register(tool.Definition{ToolName: "fs.exists", Version: "v1", CapabilityType: "filesystem", RiskLevel: tool.RiskLow, Enabled: true, Metadata: map[string]any{"module": "filesystem"}}, existsHandler{})
		tools.Register(tool.Definition{ToolName: "fs.read", Version: "v1", CapabilityType: "filesystem", RiskLevel: tool.RiskLow, Enabled: true, Metadata: map[string]any{"module": "filesystem"}}, readHandler{})
		tools.Register(tool.Definition{ToolName: "fs.write", Version: "v1", CapabilityType: "filesystem", RiskLevel: tool.RiskMedium, Enabled: true, Metadata: map[string]any{"module": "filesystem"}}, writeHandler{})
		tools.Register(tool.Definition{ToolName: "fs.list", Version: "v1", CapabilityType: "filesystem", RiskLevel: tool.RiskLow, Enabled: true, Metadata: map[string]any{"module": "filesystem"}}, listHandler{})
	}
	if verifiers != nil {
		verifiers.Register(verify.Definition{Kind: "file_exists", Description: "Verify that a file exists at the specified path."}, fileExistsChecker{})
		verifiers.Register(verify.Definition{Kind: "file_content_contains", Description: "Verify that a file contains the expected text."}, fileContentContainsChecker{})
	}
}

func DefaultPolicyRules() []permission.Rule {
	return []permission.Rule{
		{Permission: "fs.exists", Pattern: "*", Action: permission.Allow},
		{Permission: "fs.read", Pattern: "*", Action: permission.Allow},
		{Permission: "fs.list", Pattern: "*", Action: permission.Allow},
		{Permission: "fs.write", Pattern: "*", Action: permission.Ask},
	}
}

func (existsHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	path, err := requiredPath(args)
	if err != nil {
		return fail("MISSING_PATH", err.Error()), nil
	}
	_, statErr := os.Stat(path)
	exists := statErr == nil
	return action.Result{OK: true, Data: map[string]any{"path": path, "exists": exists}}, nil
}

func (readHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	path, err := requiredPath(args)
	if err != nil {
		return fail("MISSING_PATH", err.Error()), nil
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		return fail("READ_FAILED", readErr.Error()), nil
	}
	return action.Result{OK: true, Data: map[string]any{"path": path, "content": string(content), "size_bytes": len(content)}}, nil
}

func (writeHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	path, err := requiredPath(args)
	if err != nil {
		return fail("MISSING_PATH", err.Error()), nil
	}
	content, _ := args["content"].(string)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fail("MKDIR_FAILED", err.Error()), nil
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return fail("WRITE_FAILED", err.Error()), nil
	}
	return action.Result{OK: true, Data: map[string]any{"path": path, "written_bytes": len(content)}}, nil
}

func (listHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	path, _ := args["path"].(string)
	if path == "" {
		path = "."
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return fail("LIST_FAILED", err.Error()), nil
	}
	items := make([]map[string]any, 0, len(entries))
	for _, entry := range entries {
		items = append(items, map[string]any{"name": entry.Name(), "is_dir": entry.IsDir()})
	}
	sort.Slice(items, func(i, j int) bool { return items[i]["name"].(string) < items[j]["name"].(string) })
	return action.Result{OK: true, Data: map[string]any{"path": path, "items": items}}, nil
}

func (fileExistsChecker) Verify(_ context.Context, args map[string]any, result action.Result, _ session.State) (verify.Result, error) {
	path, err := requiredPath(args)
	if err != nil {
		return verify.Result{Success: false, Reason: err.Error()}, nil
	}
	_, statErr := os.Stat(path)
	if statErr == nil {
		return verify.Result{Success: true, Details: map[string]any{"path": path}}, nil
	}
	return verify.Result{Success: false, Reason: statErr.Error(), Details: map[string]any{"path": path}}, nil
}

func (fileContentContainsChecker) Verify(_ context.Context, args map[string]any, _ action.Result, _ session.State) (verify.Result, error) {
	path, err := requiredPath(args)
	if err != nil {
		return verify.Result{Success: false, Reason: err.Error()}, nil
	}
	text, _ := args["text"].(string)
	if text == "" {
		return verify.Result{Success: false, Reason: "missing text"}, nil
	}
	content, readErr := os.ReadFile(path)
	if readErr != nil {
		return verify.Result{Success: false, Reason: readErr.Error(), Details: map[string]any{"path": path}}, nil
	}
	if strings.Contains(string(content), text) {
		return verify.Result{Success: true, Details: map[string]any{"path": path, "text": text}}, nil
	}
	return verify.Result{Success: false, Reason: "text not found in file", Details: map[string]any{"path": path, "text": text}}, nil
}

func requiredPath(args map[string]any) (string, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return "", errors.New("path is required")
	}
	return path, nil
}

func fail(code, message string) action.Result {
	return action.Result{OK: false, Error: &action.Error{Code: code, Message: message}}
}
