package runtime

import (
	"encoding/json"
	"reflect"
	"unicode/utf8"

	"github.com/yiiilin/harness-core/pkg/harness/action"
)

func inlineActionResultWithRaw(full action.Result, limit int) action.Result {
	rawPayload := actionResultRawPayload(full)
	inline := trimActionResultToBudget(full, limit)
	inlinePayload := action.ResultPayload{
		Data:  cloneAnyMap(inline.Data),
		Meta:  cloneAnyMap(inline.Meta),
		Error: cloneActionError(inline.Error),
	}
	truncated := !actionResultPayloadEqual(rawPayload, inlinePayload)
	if truncated || full.Raw != nil {
		inline.Raw = &rawPayload
	}
	inline.Window = &action.ResultWindow{
		Truncated:     truncated,
		OriginalBytes: actionResultPayloadSizeBytes(rawPayload),
		ReturnedBytes: actionResultPayloadSizeBytes(inlinePayload),
		OriginalChars: actionResultPayloadSizeChars(rawPayload),
		ReturnedChars: actionResultPayloadSizeChars(inlinePayload),
		HasMore:       truncated,
		NextOffset:    int64(actionResultPayloadSizeBytes(inlinePayload)),
	}
	if full.RawHandle != nil {
		inline.RawHandle = cloneRawResultHandle(full.RawHandle)
	}
	return inline
}

func rawPreferredActionResult(result action.Result) action.Result {
	if result.Raw == nil {
		return result
	}
	preferred := result
	preferred.Data = cloneAnyMap(result.Raw.Data)
	preferred.Meta = cloneAnyMap(result.Raw.Meta)
	preferred.Error = cloneActionError(result.Raw.Error)
	return preferred
}

func actionResultPayloadForArtifact(result action.Result) map[string]any {
	preferred := rawPreferredActionResult(result)
	return map[string]any{
		"data":  cloneAnyMap(preferred.Data),
		"meta":  cloneAnyMap(preferred.Meta),
		"error": cloneActionError(preferred.Error),
	}
}

func actionResultRawPayload(result action.Result) action.ResultPayload {
	if result.Raw != nil {
		return action.ResultPayload{
			Data:  cloneAnyMap(result.Raw.Data),
			Meta:  cloneAnyMap(result.Raw.Meta),
			Error: cloneActionError(result.Raw.Error),
		}
	}
	return action.ResultPayload{
		Data:  cloneAnyMap(result.Data),
		Meta:  cloneAnyMap(result.Meta),
		Error: cloneActionError(result.Error),
	}
}

func actionResultPayloadEqual(left, right action.ResultPayload) bool {
	return reflect.DeepEqual(left.Data, right.Data) &&
		reflect.DeepEqual(left.Meta, right.Meta) &&
		reflect.DeepEqual(left.Error, right.Error)
}

func actionResultPayloadSizeBytes(payload action.ResultPayload) int {
	data, err := json.Marshal(actionResultArtifactEnvelope(payload))
	if err != nil {
		return 0
	}
	return len(data)
}

func actionResultPayloadSizeChars(payload action.ResultPayload) int {
	data, err := json.Marshal(actionResultArtifactEnvelope(payload))
	if err != nil {
		return 0
	}
	return utf8.RuneCount(data)
}

func actionResultArtifactEnvelope(payload action.ResultPayload) map[string]any {
	return map[string]any{
		"data":  cloneAnyMap(payload.Data),
		"meta":  cloneAnyMap(payload.Meta),
		"error": cloneActionError(payload.Error),
	}
}

func cloneActionError(err *action.Error) *action.Error {
	if err == nil {
		return nil
	}
	return &action.Error{
		Code:    err.Code,
		Message: err.Message,
	}
}

func cloneRawResultHandle(handle *action.RawResultHandle) *action.RawResultHandle {
	if handle == nil {
		return nil
	}
	return &action.RawResultHandle{
		Kind:   handle.Kind,
		Ref:    handle.Ref,
		Reread: handle.Reread,
	}
}
