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
	if actionResultPayloadEqual(rawPayload, inlinePayload) && full.Raw == nil && !full.WasTrimmed {
		return inline
	}
	inline.Raw = &rawPayload
	inline.WasTrimmed = full.WasTrimmed || !actionResultPayloadEqual(rawPayload, inlinePayload)
	inline.RawSizeBytes = actionResultPayloadSizeBytes(rawPayload)
	inline.InlineSizeChars = actionResultPayloadSizeChars(inlinePayload)
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
	data, err := json.Marshal(payload)
	if err != nil {
		return 0
	}
	return len(data)
}

func actionResultPayloadSizeChars(payload action.ResultPayload) int {
	data, err := json.Marshal(payload)
	if err != nil {
		return 0
	}
	return utf8.RuneCount(data)
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
