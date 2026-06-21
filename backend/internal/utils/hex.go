// Package utils provides shared utility functions used across the vibexp-api service.
package utils

// IsHexChar reports whether c is a valid hexadecimal character (0-9, a-f, A-F).
func IsHexChar(c rune) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}
