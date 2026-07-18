package server

import "github.com/stretchr/testify/mock"

// Shared bodies for otherwise-identical hand-written testify mock methods
// (SonarCloud go:S4144). testify's mock.Called() resolves the expectation from
// the calling function's name, so a shared helper must record the call via
// MethodCalled with the method name passed explicitly; behavior is otherwise
// identical to the original inline bodies.

// mockErr records the call under method and returns result 0 as an error.
func mockErr(m *mock.Mock, method string, callArgs ...any) error {
	return m.MethodCalled(method, callArgs...).Error(0)
}

// mockTyped records the call under method and type-asserts result 0 to T.
func mockTyped[T any](m *mock.Mock, method string, callArgs ...any) T {
	return m.MethodCalled(method, callArgs...).Get(0).(T)
}

// mockTypedOrZero is mockTyped, but returns T's zero value when result 0 is
// nil instead of panicking on the type assertion.
func mockTypedOrZero[T any](m *mock.Mock, method string, callArgs ...any) T {
	args := m.MethodCalled(method, callArgs...)
	if args.Get(0) == nil {
		var zero T
		return zero
	}
	return args.Get(0).(T)
}
