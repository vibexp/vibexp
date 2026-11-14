// Package rolemapping evaluates an identity provider's role mapping (#1644):
// whether a user's verified claims carry one of the values that make them an
// instance admin.
package rolemapping

import "strings"

// Evaluate reads the claim at path, a dotted path such as realm_access.roles,
// from claims and reports the first entry of adminValues it carries. The claim
// must be a string or an array of strings; anything else (a missing segment, a
// number, an object, an array holding a non-string) is no match, as is an empty
// path. Values are compared exactly and case-sensitively. adminValues is
// searched in order, so the matched value is deterministic.
func Evaluate(claims map[string]any, path string, adminValues []string) (matched string, ok bool) {
	values, ok := claimValues(claims, path)
	if !ok {
		return "", false
	}
	for _, want := range adminValues {
		if _, found := values[want]; found {
			return want, true
		}
	}
	return "", false
}

// claimValues returns the set of string values at path, or false when the
// claim is missing or not a string or an array of strings.
func claimValues(claims map[string]any, path string) (map[string]struct{}, bool) {
	if path == "" {
		return nil, false
	}
	var node any = claims
	for _, segment := range strings.Split(path, ".") {
		object, isObject := node.(map[string]any)
		if !isObject {
			return nil, false
		}
		if node, isObject = object[segment]; !isObject {
			return nil, false
		}
	}
	switch v := node.(type) {
	case string:
		return map[string]struct{}{v: {}}, true
	case []any:
		values := make(map[string]struct{}, len(v))
		for _, item := range v {
			s, isString := item.(string)
			if !isString {
				return nil, false
			}
			values[s] = struct{}{}
		}
		return values, true
	}
	return nil, false
}
