package testutils

// TestingT is the interface that testing helpers expect
type TestingT interface {
	Errorf(format string, args ...interface{})
	Error(args ...interface{})
	Fatal(args ...interface{})
	Fatalf(format string, args ...interface{})
	Helper()
}
