package verify

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"reflect"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/yiiilin/harness-core/pkg/harness/action"
	"github.com/yiiilin/harness-core/pkg/harness/session"
)

func RegisterBuiltins(reg *Registry) {
	if reg == nil {
		return
	}
	reg.Register(Definition{Kind: "exit_code", Description: "Verify that an execution result exit code is in the allowed set."}, ExitCodeChecker{})
	reg.Register(Definition{Kind: "output_contains", Description: "Verify that stdout or stderr contains a target substring."}, OutputContainsChecker{})
	reg.Register(Definition{Kind: "value_exists", Description: "Verify that a value exists at a result path such as result.data.foo."}, ValueExistsChecker{})
	reg.Register(Definition{Kind: "value_equals", Description: "Verify that a value at a result path equals the expected value."}, ValueEqualsChecker{})
	reg.Register(Definition{Kind: "value_in", Description: "Verify that a value at a result path is one of the allowed values."}, ValueInChecker{})
	reg.Register(Definition{Kind: "string_contains_at", Description: "Verify that a string value at a result path contains the expected substring."}, StringContainsAtChecker{})
	reg.Register(Definition{Kind: "string_matches_at", Description: "Verify that a string value at a result path matches the expected regular expression."}, StringMatchesAtChecker{})
	reg.Register(Definition{Kind: "number_compare", Description: "Verify that a numeric value at a result path satisfies a comparison operator."}, NumberCompareChecker{})
	reg.Register(Definition{Kind: "collection_contains", Description: "Verify that a list value at a result path contains the expected item."}, CollectionContainsChecker{})
	reg.Register(Definition{Kind: "tcp_port_open", Description: "Verify that a TCP endpoint accepts connections within the timeout."}, TCPPortOpenChecker{})
	reg.Register(Definition{Kind: "file_exists_eventually", Description: "Verify that a file eventually appears within the timeout."}, FileExistsEventuallyChecker{})
	reg.Register(Definition{Kind: "file_content_contains_eventually", Description: "Verify that a file eventually contains the expected text within the timeout."}, FileContentContainsEventuallyChecker{})
	reg.Register(Definition{Kind: "http_eventually_status_code", Description: "Verify that a URL eventually returns an allowed HTTP status code."}, HTTPEventuallyStatusCodeChecker{})
	reg.Register(Definition{Kind: "http_eventually_json_field_equals", Description: "Verify that a URL eventually returns a JSON field equal to the expected value."}, HTTPEventuallyJSONFieldEqualsChecker{})
}

type ExitCodeChecker struct{}

func (ExitCodeChecker) Verify(_ context.Context, args map[string]any, result action.Result, _ session.State) (Result, error) {
	allowedRaw, ok := args["allowed"]
	if !ok {
		return Result{Success: false, Reason: "missing allowed exit codes"}, nil
	}
	allowedList, ok := allowedRaw.([]any)
	if !ok {
		return Result{Success: false, Reason: "allowed must be an array"}, nil
	}
	exitCodeRaw, ok := result.Data["exit_code"]
	if !ok {
		return Result{Success: false, Reason: "result missing exit_code"}, nil
	}
	exitCode, ok := asInt(exitCodeRaw)
	if !ok {
		return Result{Success: false, Reason: "exit_code is not numeric"}, nil
	}
	for _, item := range allowedList {
		candidate, ok := asInt(item)
		if ok && candidate == exitCode {
			return Result{Success: true, Details: map[string]any{"exit_code": exitCode}}, nil
		}
	}
	return Result{Success: false, Reason: fmt.Sprintf("exit_code %d not allowed", exitCode), Details: map[string]any{"exit_code": exitCode}}, nil
}

type OutputContainsChecker struct{}

func (OutputContainsChecker) Verify(_ context.Context, args map[string]any, result action.Result, _ session.State) (Result, error) {
	needle, _ := args["text"].(string)
	if needle == "" {
		return Result{Success: false, Reason: "missing text"}, nil
	}
	stdout, _ := result.Data["stdout"].(string)
	stderr, _ := result.Data["stderr"].(string)
	combined := stdout + "\n" + stderr
	if contains(combined, needle) {
		return Result{Success: true, Details: map[string]any{"text": needle}}, nil
	}
	return Result{Success: false, Reason: "text not found in output", Details: map[string]any{"text": needle}}, nil
}

func contains(s, needle string) bool {
	return len(needle) > 0 && len(s) >= len(needle) && (func() bool { return stringIndex(s, needle) >= 0 })()
}

func stringIndex(s, sep string) int {
	for i := 0; i+len(sep) <= len(s); i++ {
		if s[i:i+len(sep)] == sep {
			return i
		}
	}
	return -1
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

func asInt64(v any) (int64, bool) {
	switch x := v.(type) {
	case int:
		return int64(x), true
	case int32:
		return int64(x), true
	case int64:
		return x, true
	case float64:
		return int64(x), true
	case float32:
		return int64(x), true
	default:
		return 0, false
	}
}

func asFloat64(v any) (float64, bool) {
	switch x := v.(type) {
	case int:
		return float64(x), true
	case int32:
		return float64(x), true
	case int64:
		return float64(x), true
	case float64:
		return x, true
	case float32:
		return float64(x), true
	default:
		return 0, false
	}
}

type ValueExistsChecker struct{}

func (ValueExistsChecker) Verify(_ context.Context, args map[string]any, result action.Result, _ session.State) (Result, error) {
	path, ok := requiredResultPath(args)
	if !ok {
		return Result{Success: false, Reason: "missing path"}, nil
	}
	value, found := resolveResultPath(result, path)
	if !found {
		return Result{Success: false, Reason: "path not found", Details: map[string]any{"path": path}}, nil
	}
	return Result{Success: true, Details: map[string]any{"path": path, "value": value}}, nil
}

type ValueEqualsChecker struct{}

func (ValueEqualsChecker) Verify(_ context.Context, args map[string]any, result action.Result, _ session.State) (Result, error) {
	path, ok := requiredResultPath(args)
	if !ok {
		return Result{Success: false, Reason: "missing path"}, nil
	}
	expected, ok := args["expected"]
	if !ok {
		return Result{Success: false, Reason: "missing expected"}, nil
	}
	actual, found := resolveResultPath(result, path)
	if !found {
		return Result{Success: false, Reason: "path not found", Details: map[string]any{"path": path}}, nil
	}
	if valuesEqual(actual, expected) {
		return Result{Success: true, Details: map[string]any{"path": path, "actual": actual}}, nil
	}
	return Result{Success: false, Reason: "value mismatch", Details: map[string]any{"path": path, "actual": actual, "expected": expected}}, nil
}

type ValueInChecker struct{}

func (ValueInChecker) Verify(_ context.Context, args map[string]any, result action.Result, _ session.State) (Result, error) {
	path, ok := requiredResultPath(args)
	if !ok {
		return Result{Success: false, Reason: "missing path"}, nil
	}
	allowed, ok := args["allowed"].([]any)
	if !ok {
		return Result{Success: false, Reason: "allowed must be an array"}, nil
	}
	actual, found := resolveResultPath(result, path)
	if !found {
		return Result{Success: false, Reason: "path not found", Details: map[string]any{"path": path}}, nil
	}
	for _, item := range allowed {
		if valuesEqual(actual, item) {
			return Result{Success: true, Details: map[string]any{"path": path, "actual": actual}}, nil
		}
	}
	return Result{Success: false, Reason: "value not allowed", Details: map[string]any{"path": path, "actual": actual, "allowed": allowed}}, nil
}

type StringContainsAtChecker struct{}

func (StringContainsAtChecker) Verify(_ context.Context, args map[string]any, result action.Result, _ session.State) (Result, error) {
	path, ok := requiredResultPath(args)
	if !ok {
		return Result{Success: false, Reason: "missing path"}, nil
	}
	text, _ := args["text"].(string)
	if text == "" {
		return Result{Success: false, Reason: "missing text"}, nil
	}
	actual, found := resolveResultPath(result, path)
	if !found {
		return Result{Success: false, Reason: "path not found", Details: map[string]any{"path": path}}, nil
	}
	actualString, ok := actual.(string)
	if !ok {
		return Result{Success: false, Reason: "value is not a string", Details: map[string]any{"path": path}}, nil
	}
	if strings.Contains(actualString, text) {
		return Result{Success: true, Details: map[string]any{"path": path, "text": text}}, nil
	}
	return Result{Success: false, Reason: "text not found", Details: map[string]any{"path": path, "text": text}}, nil
}

type StringMatchesAtChecker struct{}

func (StringMatchesAtChecker) Verify(_ context.Context, args map[string]any, result action.Result, _ session.State) (Result, error) {
	path, ok := requiredResultPath(args)
	if !ok {
		return Result{Success: false, Reason: "missing path"}, nil
	}
	pattern, _ := args["pattern"].(string)
	if pattern == "" {
		return Result{Success: false, Reason: "missing pattern"}, nil
	}
	actual, found := resolveResultPath(result, path)
	if !found {
		return Result{Success: false, Reason: "path not found", Details: map[string]any{"path": path}}, nil
	}
	actualString, ok := actual.(string)
	if !ok {
		return Result{Success: false, Reason: "value is not a string", Details: map[string]any{"path": path}}, nil
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return Result{Success: false, Reason: err.Error(), Details: map[string]any{"pattern": pattern}}, nil
	}
	if re.MatchString(actualString) {
		return Result{Success: true, Details: map[string]any{"path": path, "pattern": pattern}}, nil
	}
	return Result{Success: false, Reason: "pattern did not match", Details: map[string]any{"path": path, "pattern": pattern}}, nil
}

type NumberCompareChecker struct{}

func (NumberCompareChecker) Verify(_ context.Context, args map[string]any, result action.Result, _ session.State) (Result, error) {
	path, ok := requiredResultPath(args)
	if !ok {
		return Result{Success: false, Reason: "missing path"}, nil
	}
	op, _ := args["op"].(string)
	if op == "" {
		return Result{Success: false, Reason: "missing op"}, nil
	}
	expectedRaw, ok := args["expected"]
	if !ok {
		return Result{Success: false, Reason: "missing expected"}, nil
	}
	actualRaw, found := resolveResultPath(result, path)
	if !found {
		return Result{Success: false, Reason: "path not found", Details: map[string]any{"path": path}}, nil
	}
	actual, ok := asFloat64(actualRaw)
	if !ok {
		return Result{Success: false, Reason: "value is not numeric", Details: map[string]any{"path": path}}, nil
	}
	expected, ok := asFloat64(expectedRaw)
	if !ok {
		return Result{Success: false, Reason: "expected is not numeric"}, nil
	}
	if compareNumbers(actual, expected, op) {
		return Result{Success: true, Details: map[string]any{"path": path, "actual": actual, "expected": expected, "op": op}}, nil
	}
	return Result{Success: false, Reason: "numeric comparison failed", Details: map[string]any{"path": path, "actual": actual, "expected": expected, "op": op}}, nil
}

type CollectionContainsChecker struct{}

func (CollectionContainsChecker) Verify(_ context.Context, args map[string]any, result action.Result, _ session.State) (Result, error) {
	path, ok := requiredResultPath(args)
	if !ok {
		return Result{Success: false, Reason: "missing path"}, nil
	}
	expected, ok := args["expected"]
	if !ok {
		return Result{Success: false, Reason: "missing expected"}, nil
	}
	actual, found := resolveResultPath(result, path)
	if !found {
		return Result{Success: false, Reason: "path not found", Details: map[string]any{"path": path}}, nil
	}
	value := reflect.ValueOf(actual)
	if !value.IsValid() || (value.Kind() != reflect.Slice && value.Kind() != reflect.Array) {
		return Result{Success: false, Reason: "value is not a collection", Details: map[string]any{"path": path}}, nil
	}
	for i := 0; i < value.Len(); i++ {
		if valuesEqual(value.Index(i).Interface(), expected) {
			return Result{Success: true, Details: map[string]any{"path": path, "expected": expected}}, nil
		}
	}
	return Result{Success: false, Reason: "expected item not found", Details: map[string]any{"path": path, "expected": expected}}, nil
}

type TCPPortOpenChecker struct{}

func (TCPPortOpenChecker) Verify(ctx context.Context, args map[string]any, _ action.Result, _ session.State) (Result, error) {
	address, ok := tcpAddressFromArgs(args)
	if !ok {
		return Result{Success: false, Reason: "missing address or host/port"}, nil
	}
	err := pollUntil(ctx, args, func() (bool, Result, error) {
		conn, err := net.DialTimeout("tcp", address, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true, Result{Success: true, Details: map[string]any{"address": address}}, nil
		}
		return false, Result{Success: false, Reason: err.Error(), Details: map[string]any{"address": address}}, nil
	})
	if err == nil {
		return Result{Success: true, Details: map[string]any{"address": address}}, nil
	}
	return resultFromPollError(err, map[string]any{"address": address}), nil
}

type FileExistsEventuallyChecker struct{}

func (FileExistsEventuallyChecker) Verify(ctx context.Context, args map[string]any, _ action.Result, _ session.State) (Result, error) {
	path, _ := args["path"].(string)
	if path == "" {
		return Result{Success: false, Reason: "missing path"}, nil
	}
	err := pollUntil(ctx, args, func() (bool, Result, error) {
		if _, err := os.Stat(path); err == nil {
			return true, Result{Success: true, Details: map[string]any{"path": path}}, nil
		}
		return false, Result{Success: false, Reason: "file not found", Details: map[string]any{"path": path}}, nil
	})
	if err == nil {
		return Result{Success: true, Details: map[string]any{"path": path}}, nil
	}
	return resultFromPollError(err, map[string]any{"path": path}), nil
}

type FileContentContainsEventuallyChecker struct{}

func (FileContentContainsEventuallyChecker) Verify(ctx context.Context, args map[string]any, _ action.Result, _ session.State) (Result, error) {
	path, _ := args["path"].(string)
	text, _ := args["text"].(string)
	if path == "" {
		return Result{Success: false, Reason: "missing path"}, nil
	}
	if text == "" {
		return Result{Success: false, Reason: "missing text"}, nil
	}
	err := pollUntil(ctx, args, func() (bool, Result, error) {
		body, err := os.ReadFile(path)
		if err != nil {
			return false, Result{Success: false, Reason: err.Error(), Details: map[string]any{"path": path}}, nil
		}
		if strings.Contains(string(body), text) {
			return true, Result{Success: true, Details: map[string]any{"path": path, "text": text}}, nil
		}
		return false, Result{Success: false, Reason: "text not found in file", Details: map[string]any{"path": path, "text": text}}, nil
	})
	if err == nil {
		return Result{Success: true, Details: map[string]any{"path": path, "text": text}}, nil
	}
	return resultFromPollError(err, map[string]any{"path": path, "text": text}), nil
}

type HTTPEventuallyStatusCodeChecker struct{}

func (HTTPEventuallyStatusCodeChecker) Verify(ctx context.Context, args map[string]any, _ action.Result, _ session.State) (Result, error) {
	url, _ := args["url"].(string)
	allowed, ok := args["allowed"].([]any)
	if url == "" {
		return Result{Success: false, Reason: "missing url"}, nil
	}
	if !ok {
		return Result{Success: false, Reason: "allowed must be an array"}, nil
	}
	err := pollUntil(ctx, args, func() (bool, Result, error) {
		status, _, err := fetchHTTP(url)
		if err != nil {
			return false, Result{Success: false, Reason: err.Error(), Details: map[string]any{"url": url}}, nil
		}
		for _, item := range allowed {
			candidate, ok := asInt(item)
			if ok && candidate == status {
				return true, Result{Success: true, Details: map[string]any{"url": url, "status_code": status}}, nil
			}
		}
		return false, Result{Success: false, Reason: "status code not allowed", Details: map[string]any{"url": url, "status_code": status, "allowed": allowed}}, nil
	})
	if err == nil {
		return Result{Success: true, Details: map[string]any{"url": url}}, nil
	}
	return resultFromPollError(err, map[string]any{"url": url}), nil
}

type HTTPEventuallyJSONFieldEqualsChecker struct{}

func (HTTPEventuallyJSONFieldEqualsChecker) Verify(ctx context.Context, args map[string]any, _ action.Result, _ session.State) (Result, error) {
	url, _ := args["url"].(string)
	field, _ := args["field"].(string)
	expected, ok := args["expected"]
	if url == "" {
		return Result{Success: false, Reason: "missing url"}, nil
	}
	if field == "" {
		return Result{Success: false, Reason: "missing field"}, nil
	}
	if !ok {
		return Result{Success: false, Reason: "missing expected"}, nil
	}
	err := pollUntil(ctx, args, func() (bool, Result, error) {
		_, body, err := fetchHTTP(url)
		if err != nil {
			return false, Result{Success: false, Reason: err.Error(), Details: map[string]any{"url": url}}, nil
		}
		var parsed map[string]any
		if err := json.Unmarshal([]byte(body), &parsed); err != nil {
			return false, Result{Success: false, Reason: err.Error(), Details: map[string]any{"url": url}}, nil
		}
		actual, exists := parsed[field]
		if !exists {
			return false, Result{Success: false, Reason: "field not found", Details: map[string]any{"url": url, "field": field}}, nil
		}
		if valuesEqual(actual, expected) {
			return true, Result{Success: true, Details: map[string]any{"url": url, "field": field, "actual": actual}}, nil
		}
		return false, Result{Success: false, Reason: "field value mismatch", Details: map[string]any{"url": url, "field": field, "actual": actual, "expected": expected}}, nil
	})
	if err == nil {
		return Result{Success: true, Details: map[string]any{"url": url, "field": field}}, nil
	}
	return resultFromPollError(err, map[string]any{"url": url, "field": field}), nil
}

func requiredResultPath(args map[string]any) (string, bool) {
	path, _ := args["path"].(string)
	path = strings.TrimSpace(path)
	return path, path != ""
}

func resolveResultPath(result action.Result, path string) (any, bool) {
	parts := strings.Split(strings.TrimSpace(path), ".")
	if len(parts) == 0 || parts[0] != "result" {
		return nil, false
	}
	if len(parts) == 1 {
		return result, true
	}

	var current any
	switch parts[1] {
	case "ok":
		current = result.OK
	case "data":
		current = result.Data
	case "meta":
		current = result.Meta
	case "error":
		if result.Error == nil {
			return nil, false
		}
		current = map[string]any{
			"code":    result.Error.Code,
			"message": result.Error.Message,
		}
	default:
		return nil, false
	}
	for _, part := range parts[2:] {
		next, ok := descendPathValue(current, part)
		if !ok {
			return nil, false
		}
		current = next
	}
	return current, true
}

func descendPathValue(current any, segment string) (any, bool) {
	switch value := current.(type) {
	case map[string]any:
		next, ok := value[segment]
		return next, ok
	case []any:
		index, err := strconv.Atoi(segment)
		if err != nil || index < 0 || index >= len(value) {
			return nil, false
		}
		return value[index], true
	default:
		reflectValue := reflect.ValueOf(current)
		if !reflectValue.IsValid() {
			return nil, false
		}
		if reflectValue.Kind() == reflect.Slice || reflectValue.Kind() == reflect.Array {
			index, err := strconv.Atoi(segment)
			if err != nil || index < 0 || index >= reflectValue.Len() {
				return nil, false
			}
			return reflectValue.Index(index).Interface(), true
		}
		return nil, false
	}
}

func valuesEqual(a, b any) bool {
	aj, aerr := json.Marshal(a)
	bj, berr := json.Marshal(b)
	if aerr == nil && berr == nil {
		return string(aj) == string(bj)
	}
	return reflect.DeepEqual(a, b)
}

func compareNumbers(actual, expected float64, op string) bool {
	switch op {
	case "eq":
		return actual == expected
	case "ne":
		return actual != expected
	case "gt":
		return actual > expected
	case "gte":
		return actual >= expected
	case "lt":
		return actual < expected
	case "lte":
		return actual <= expected
	default:
		return false
	}
}

func tcpAddressFromArgs(args map[string]any) (string, bool) {
	if address, _ := args["address"].(string); strings.TrimSpace(address) != "" {
		return strings.TrimSpace(address), true
	}
	host, _ := args["host"].(string)
	port, ok := asInt(args["port"])
	if strings.TrimSpace(host) == "" || !ok || port <= 0 {
		return "", false
	}
	return net.JoinHostPort(strings.TrimSpace(host), strconv.Itoa(port)), true
}

func fetchHTTP(url string) (int, string, error) {
	client := http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(url)
	if err != nil {
		return 0, "", err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return 0, "", err
	}
	return resp.StatusCode, string(body), nil
}

func pollUntil(ctx context.Context, args map[string]any, fn func() (bool, Result, error)) error {
	timeout := durationFromArgs(args, "timeout_ms", time.Second)
	interval := durationFromArgs(args, "interval_ms", 50*time.Millisecond)
	pollCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		ok, _, err := fn()
		if err != nil {
			return err
		}
		if ok {
			return nil
		}
		select {
		case <-pollCtx.Done():
			return pollCtx.Err()
		case <-ticker.C:
		}
	}
}

func resultFromPollError(err error, details map[string]any) Result {
	if err == nil {
		return Result{Success: true, Details: details}
	}
	return Result{Success: false, Reason: err.Error(), Details: details}
}

func durationFromArgs(args map[string]any, key string, fallback time.Duration) time.Duration {
	if value, ok := asInt64(args[key]); ok && value > 0 {
		return time.Duration(value) * time.Millisecond
	}
	return fallback
}
