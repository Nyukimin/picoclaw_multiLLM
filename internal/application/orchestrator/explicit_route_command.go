package orchestrator

import (
	"reflect"
	"strings"
)

func isExplicitRouteCommand(message string) bool {
	trimmed := strings.TrimSpace(message)
	if strings.HasPrefix(trimmed, "/") {
		return true
	}
	lower := strings.ToLower(trimmed)
	for _, prefix := range []string{
		"code:",
		"code1:",
		"code2:",
		"code3:",
		"code4:",
		"plan:",
		"ops:",
		"analyze:",
		"research:",
		"wild:",
		"chat:",
	} {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func isNilRecallTraceStore(store RecallTraceStore) bool {
	if store == nil {
		return true
	}
	v := reflect.ValueOf(store)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
