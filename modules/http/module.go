package httpmodule

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/permission"
	"github.com/yiiilin/harness-core/pkg/harness/session"
	"github.com/yiiilin/harness-core/pkg/harness/tool"
	"github.com/yiiilin/harness-core/pkg/harness/verify"
)

type fetchHandler struct{}
type postJSONHandler struct{}
type httpStatusChecker struct{}
type bodyContainsChecker struct{}
type jsonFieldEqualsChecker struct{}

func Register(tools *tool.Registry, verifiers *verify.Registry) {
	if tools != nil {
		tools.Register(tool.Definition{ToolName: "http.fetch", Version: "v1", CapabilityType: "http", RiskLevel: tool.RiskLow, Enabled: true, Metadata: map[string]any{"module": "http"}}, fetchHandler{})
		tools.Register(tool.Definition{ToolName: "http.post_json", Version: "v1", CapabilityType: "http", RiskLevel: tool.RiskMedium, Enabled: true, Metadata: map[string]any{"module": "http"}}, postJSONHandler{})
	}
	if verifiers != nil {
		verifiers.Register(verify.Definition{Kind: "http_status_code", Description: "Verify that the HTTP response status code is allowed."}, httpStatusChecker{})
		verifiers.Register(verify.Definition{Kind: "body_contains", Description: "Verify that the HTTP response body contains the expected text."}, bodyContainsChecker{})
		verifiers.Register(verify.Definition{Kind: "json_field_equals", Description: "Verify that a top-level JSON field equals the expected value."}, jsonFieldEqualsChecker{})
	}
}

func DefaultPolicyRules() []permission.Rule {
	return []permission.Rule{
		{Permission: "http.fetch", Pattern: "*", Action: permission.Allow},
		{Permission: "http.post_json", Pattern: "*", Action: permission.Ask},
	}
}

func (fetchHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	url, err := requiredURL(args)
	if err != nil {
		return fail("MISSING_URL", err.Error()), nil
	}
	timeoutMS, _ := asInt(args["timeout_ms"])
	if timeoutMS <= 0 {
		timeoutMS = 15000
	}
	client := http.Client{Timeout: time.Duration(timeoutMS) * time.Millisecond}
	resp, err := client.Get(url)
	if err != nil {
		return fail("HTTP_FETCH_FAILED", err.Error()), nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fail("HTTP_READ_FAILED", err.Error()), nil
	}
	return action.Result{OK: resp.StatusCode >= 200 && resp.StatusCode < 300, Data: map[string]any{"url": url, "status_code": resp.StatusCode, "body": string(body)}}, nil
}

func (postJSONHandler) Invoke(_ context.Context, args map[string]any) (action.Result, error) {
	url, err := requiredURL(args)
	if err != nil {
		return fail("MISSING_URL", err.Error()), nil
	}
	payload, _ := args["json"].(map[string]any)
	encoded, err := json.Marshal(payload)
	if err != nil {
		return fail("JSON_ENCODE_FAILED", err.Error()), nil
	}
	timeoutMS, _ := asInt(args["timeout_ms"])
	if timeoutMS <= 0 {
		timeoutMS = 15000
	}
	client := http.Client{Timeout: time.Duration(timeoutMS) * time.Millisecond}
	resp, err := client.Post(url, "application/json", bytes.NewReader(encoded))
	if err != nil {
		return fail("HTTP_POST_FAILED", err.Error()), nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fail("HTTP_READ_FAILED", err.Error()), nil
	}
	return action.Result{OK: resp.StatusCode >= 200 && resp.StatusCode < 300, Data: map[string]any{"url": url, "status_code": resp.StatusCode, "body": string(body)}}, nil
}

func (httpStatusChecker) Verify(_ context.Context, args map[string]any, result action.Result, _ session.State) (verify.Result, error) {
	allowedRaw, ok := args["allowed"]
	if !ok {
		return verify.Result{Success: false, Reason: "missing allowed status codes"}, nil
	}
	allowedList, ok := allowedRaw.([]any)
	if !ok {
		return verify.Result{Success: false, Reason: "allowed must be an array"}, nil
	}
	statusCode, ok := asInt(result.Data["status_code"])
	if !ok {
		return verify.Result{Success: false, Reason: "result missing status_code"}, nil
	}
	for _, item := range allowedList {
		candidate, ok := asInt(item)
		if ok && candidate == statusCode {
			return verify.Result{Success: true, Details: map[string]any{"status_code": statusCode}}, nil
		}
	}
	return verify.Result{Success: false, Reason: "status code not allowed", Details: map[string]any{"status_code": statusCode}}, nil
}

func (bodyContainsChecker) Verify(_ context.Context, args map[string]any, result action.Result, _ session.State) (verify.Result, error) {
	needle, _ := args["text"].(string)
	body, _ := result.Data["body"].(string)
	if needle == "" {
		return verify.Result{Success: false, Reason: "missing text"}, nil
	}
	if strings.Contains(body, needle) {
		return verify.Result{Success: true, Details: map[string]any{"text": needle}}, nil
	}
	return verify.Result{Success: false, Reason: "text not found in body", Details: map[string]any{"text": needle}}, nil
}

func (jsonFieldEqualsChecker) Verify(_ context.Context, args map[string]any, result action.Result, _ session.State) (verify.Result, error) {
	field, _ := args["field"].(string)
	expected := args["expected"]
	body, _ := result.Data["body"].(string)
	if field == "" {
		return verify.Result{Success: false, Reason: "missing field"}, nil
	}
	var parsed map[string]any
	if err := json.Unmarshal([]byte(body), &parsed); err != nil {
		return verify.Result{Success: false, Reason: err.Error()}, nil
	}
	actual, ok := parsed[field]
	if !ok {
		return verify.Result{Success: false, Reason: "field not found", Details: map[string]any{"field": field}}, nil
	}
	if valuesEqual(actual, expected) {
		return verify.Result{Success: true, Details: map[string]any{"field": field, "actual": actual}}, nil
	}
	return verify.Result{Success: false, Reason: "field value mismatch", Details: map[string]any{"field": field, "actual": actual, "expected": expected}}, nil
}

func requiredURL(args map[string]any) (string, error) {
	url, _ := args["url"].(string)
	if url == "" {
		return "", errors.New("url is required")
	}
	return url, nil
}

func asInt(v any) (int, bool) {
	switch x := v.(type) {
	case int:
		return x, true
	case int32:
		return int(x), true
	case int64:
		return int(x), true
	case float64:
		return int(x), true
	case float32:
		return int(x), true
	default:
		return 0, false
	}
}

func valuesEqual(a, b any) bool {
	aj, _ := json.Marshal(a)
	bj, _ := json.Marshal(b)
	return string(aj) == string(bj)
}

func fail(code, message string) action.Result {
	return action.Result{OK: false, Error: &action.Error{Code: code, Message: message}}
}
