package testutils

import (
	"reflect"
	"time"

	"github.com/vibexp/vibexp/internal/models"
)

// AssertUserEqual asserts that two User models are equal
func AssertUserEqual(t TestingT, expected, actual *models.User) {
	t.Helper()

	if expected == nil && actual == nil {
		return
	}

	if expected == nil || actual == nil {
		t.Errorf("User mismatch: expected %v, actual %v", expected, actual)
		return
	}

	if expected.ID != actual.ID {
		t.Errorf("User ID mismatch: expected %s, actual %s", expected.ID, actual.ID)
	}

	if !equalStringPointers(expected.GoogleID, actual.GoogleID) {
		t.Errorf("User GoogleID mismatch: expected %v, actual %v", expected.GoogleID, actual.GoogleID)
	}

	if expected.Email != actual.Email {
		t.Errorf("User Email mismatch: expected %s, actual %s", expected.Email, actual.Email)
	}

	if expected.Name != actual.Name {
		t.Errorf("User Name mismatch: expected %s, actual %s", expected.Name, actual.Name)
	}

	if !equalStringPointers(expected.AvatarURL, actual.AvatarURL) {
		t.Errorf("User AvatarURL mismatch: expected %v, actual %v", expected.AvatarURL, actual.AvatarURL)
	}

	if expected.SubscriptionStatus != actual.SubscriptionStatus {
		t.Errorf("User SubscriptionStatus mismatch: expected %s, actual %s",
			expected.SubscriptionStatus, actual.SubscriptionStatus)
	}
}

// AssertAPIKeyEqual asserts that two APIKey models are equal (excluding sensitive fields)
func AssertAPIKeyEqual(t TestingT, expected, actual *models.APIKey) {
	t.Helper()

	if expected == nil && actual == nil {
		return
	}

	if expected == nil || actual == nil {
		t.Errorf("APIKey mismatch: expected %v, actual %v", expected, actual)
		return
	}

	if expected.ID != actual.ID {
		t.Errorf("APIKey ID mismatch: expected %s, actual %s", expected.ID, actual.ID)
	}

	if expected.UserID != actual.UserID {
		t.Errorf("APIKey UserID mismatch: expected %s, actual %s", expected.UserID, actual.UserID)
	}

	if expected.Name != actual.Name {
		t.Errorf("APIKey Name mismatch: expected %s, actual %s", expected.Name, actual.Name)
	}

	if expected.KeyPrefix != actual.KeyPrefix {
		t.Errorf("APIKey KeyPrefix mismatch: expected %s, actual %s", expected.KeyPrefix, actual.KeyPrefix)
	}
}

// AssertPromptEqual asserts that two Prompt models are equal
func AssertPromptEqual(t TestingT, expected, actual *models.Prompt) {
	t.Helper()

	if expected == nil && actual == nil {
		return
	}

	if expected == nil || actual == nil {
		t.Errorf("Prompt mismatch: expected %v, actual %v", expected, actual)
		return
	}

	if expected.ID != actual.ID {
		t.Errorf("Prompt ID mismatch: expected %s, actual %s", expected.ID, actual.ID)
	}

	if expected.Name != actual.Name {
		t.Errorf("Prompt Name mismatch: expected %s, actual %s", expected.Name, actual.Name)
	}

	if expected.Slug != actual.Slug {
		t.Errorf("Prompt Slug mismatch: expected %s, actual %s", expected.Slug, actual.Slug)
	}

	if expected.Description != actual.Description {
		t.Errorf("Prompt Description mismatch: expected %s, actual %s", expected.Description, actual.Description)
	}

	if expected.Body != actual.Body {
		t.Errorf("Prompt Body mismatch: expected %s, actual %s", expected.Body, actual.Body)
	}

	if expected.UserID != actual.UserID {
		t.Errorf("Prompt UserID mismatch: expected %s, actual %s", expected.UserID, actual.UserID)
	}

	if expected.Status != actual.Status {
		t.Errorf("Prompt Status mismatch: expected %s, actual %s", expected.Status, actual.Status)
	}
}

// AssertSubscriptionEqual asserts that two Subscription models are equal
func AssertSubscriptionEqual(t TestingT, expected, actual *models.Subscription) {
	t.Helper()

	if expected == nil && actual == nil {
		return
	}

	if expected == nil || actual == nil {
		t.Errorf("Subscription mismatch: expected %v, actual %v", expected, actual)
		return
	}

	if expected.ID != actual.ID {
		t.Errorf("Subscription ID mismatch: expected %s, actual %s", expected.ID, actual.ID)
	}

	if expected.UserID != actual.UserID {
		t.Errorf("Subscription UserID mismatch: expected %s, actual %s", expected.UserID, actual.UserID)
	}

	if expected.Status != actual.Status {
		t.Errorf("Subscription Status mismatch: expected %s, actual %s", expected.Status, actual.Status)
	}

	if !equalStringPointers(expected.PlanName, actual.PlanName) {
		t.Errorf("Subscription PlanName mismatch: expected %v, actual %v", expected.PlanName, actual.PlanName)
	}
}

// AssertTimeAlmostEqual asserts that two times are equal within a small tolerance
func AssertTimeAlmostEqual(t TestingT, expected, actual time.Time, tolerance time.Duration) {
	t.Helper()

	diff := expected.Sub(actual)
	if diff < 0 {
		diff = -diff
	}

	if diff > tolerance {
		t.Errorf("Time difference too large: expected %v, actual %v, diff %v (tolerance %v)",
			expected, actual, diff, tolerance)
	}
}

// AssertSliceEqual asserts that two slices are equal
func AssertSliceEqual(t TestingT, expected, actual interface{}) {
	t.Helper()

	if !reflect.DeepEqual(expected, actual) {
		t.Errorf("Slice mismatch: expected %v, actual %v", expected, actual)
	}
}

// AssertSliceContains asserts that a slice contains an element
func AssertSliceContains(t TestingT, slice interface{}, element interface{}) {
	t.Helper()

	v := reflect.ValueOf(slice)
	if v.Kind() != reflect.Slice {
		t.Errorf("Expected slice, got %T", slice)
		return
	}

	for i := 0; i < v.Len(); i++ {
		if reflect.DeepEqual(v.Index(i).Interface(), element) {
			return
		}
	}

	t.Errorf("Slice does not contain element: %v not in %v", element, slice)
}

// AssertNotNil asserts that a value is not nil
func AssertNotNil(t TestingT, value interface{}, message string) {
	t.Helper()

	if value == nil {
		t.Errorf("Expected non-nil value: %s", message)
		return
	}

	// Check for nil pointers, slices, maps, channels, functions
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func, reflect.Interface:
		if v.IsNil() {
			t.Errorf("Expected non-nil value: %s", message)
		}
	}
}

// AssertNil asserts that a value is nil
func AssertNil(t TestingT, value interface{}, message string) {
	t.Helper()

	if value == nil {
		return
	}

	// Check for nil pointers, slices, maps, channels, functions
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Ptr, reflect.Slice, reflect.Map, reflect.Chan, reflect.Func, reflect.Interface:
		if !v.IsNil() {
			t.Errorf("Expected nil value: %s, got %v", message, value)
		}
	default:
		t.Errorf("Expected nil value: %s, got %v", message, value)
	}
}

// Helper function to compare string pointers
func equalStringPointers(a, b *string) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
