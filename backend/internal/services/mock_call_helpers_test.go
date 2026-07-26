package services

import (
	"github.com/stretchr/testify/mock"
)

// testify's mock.Mock.Called derives the mock method name from the call
// stack, so mechanically identical mock methods cannot share one body
// directly. These helpers take the method name explicitly (MethodCalled) so
// boilerplate mock methods across this package's test files can delegate to a
// single shared implementation.
//
// They previously lived in resource_usage_test.go, which #650 deleted along with
// ResourceUsageService; several unrelated suites in this package depend on them,
// so they are re-homed here. Mirrors internal/server/mock_call_helpers_test.go.

func mockErrCall(m *mock.Mock, method string, callArgs ...any) error {
	return m.MethodCalled(method, callArgs...).Error(0)
}

func mockPtrCall[T any](m *mock.Mock, method string, callArgs ...any) (*T, error) {
	args := m.MethodCalled(method, callArgs...)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*T), args.Error(1)
}

func mockListCall[T any](m *mock.Mock, method string, callArgs ...any) ([]T, int, error) {
	args := m.MethodCalled(method, callArgs...)
	return args.Get(0).([]T), args.Int(1), args.Error(2)
}
