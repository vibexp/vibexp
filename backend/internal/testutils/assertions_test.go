package testutils

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/vibexp/vibexp/internal/models"
)

func TestAssertUserEqual(t *testing.T) {
	user1 := CreateTestUser()
	user2 := CreateTestUser()

	// Same users should pass
	AssertUserEqual(t, user1, user2)

	// Different users should fail
	mockT := &mockTesting{}
	user3 := CreateTestUserWithID("different-id")
	AssertUserEqual(mockT, user1, user3)

	if !mockT.errorFCalled {
		t.Error("Expected assertion to fail for different users")
	}

	// Nil cases
	AssertUserEqual(t, nil, nil) // Should pass

	mockT2 := &mockTesting{}
	AssertUserEqual(mockT2, user1, nil)
	if !mockT2.errorFCalled {
		t.Error("Expected assertion to fail for nil comparison")
	}
}

func TestAssertAPIKeyEqual(t *testing.T) {
	userID := "test-user-123"
	apiKey1, _, err := CreateTestAPIKey(userID)
	if err != nil {
		t.Fatal(err)
	}

	apiKey2, _, err := CreateTestAPIKey(userID)
	if err != nil {
		t.Fatal(err)
	}

	// Set same ID for comparison
	apiKey2.ID = apiKey1.ID
	apiKey2.KeyPrefix = apiKey1.KeyPrefix

	// Same API keys should pass
	AssertAPIKeyEqual(t, apiKey1, apiKey2)

	// Different API keys should fail
	mockT := &mockTesting{}
	apiKey3, _, err := CreateTestAPIKeyWithName("different-user", "Different Key")
	require.NoError(t, err)
	AssertAPIKeyEqual(mockT, apiKey1, apiKey3)

	if !mockT.errorFCalled {
		t.Error("Expected assertion to fail for different API keys")
	}
}

func TestAssertPromptEqual(t *testing.T) {
	now := time.Now()
	prompt1 := &models.Prompt{
		ID:          "prompt-123",
		Name:        "Test Prompt",
		Slug:        "test-prompt",
		Description: "A test prompt",
		Body:        "Test body",
		UserID:      "user-123",
		Status:      "published",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	prompt2 := &models.Prompt{
		ID:          "prompt-123",
		Name:        "Test Prompt",
		Slug:        "test-prompt",
		Description: "A test prompt",
		Body:        "Test body",
		UserID:      "user-123",
		Status:      "published",
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	// Same prompts should pass
	AssertPromptEqual(t, prompt1, prompt2)

	// Different prompts should fail
	mockT := &mockTesting{}
	prompt3 := &models.Prompt{
		ID:          "prompt-456",
		Name:        "Different Prompt",
		Slug:        "different-prompt",
		Description: "A different prompt",
		Body:        "Different body",
		UserID:      "user-456",
		Status:      "draft",
		CreatedAt:   now,
		UpdatedAt:   now,
	}
	AssertPromptEqual(mockT, prompt1, prompt3)

	if !mockT.errorFCalled {
		t.Error("Expected assertion to fail for different prompts")
	}
}

func TestAssertSubscriptionEqual(t *testing.T) {
	now := time.Now()
	planName := models.PlanPro

	sub1 := &models.Subscription{
		ID:                   "sub-123",
		UserID:               "user-123",
		StripeSubscriptionID: nil,
		StripeCustomerID:     nil,
		Status:               models.SubscriptionStatusActive,
		PlanName:             &planName,
		CurrentPeriodStart:   &now,
		CurrentPeriodEnd:     nil,
		TrialEnd:             nil,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	sub2 := &models.Subscription{
		ID:                   "sub-123",
		UserID:               "user-123",
		StripeSubscriptionID: nil,
		StripeCustomerID:     nil,
		Status:               models.SubscriptionStatusActive,
		PlanName:             &planName,
		CurrentPeriodStart:   &now,
		CurrentPeriodEnd:     nil,
		TrialEnd:             nil,
		CreatedAt:            now,
		UpdatedAt:            now,
	}

	// Same subscriptions should pass
	AssertSubscriptionEqual(t, sub1, sub2)

	// Different subscriptions should fail
	mockT := &mockTesting{}
	sub3 := &models.Subscription{
		ID:                   "sub-456",
		UserID:               "user-456",
		StripeSubscriptionID: nil,
		StripeCustomerID:     nil,
		Status:               models.SubscriptionStatusBasic,
		PlanName:             nil,
		CurrentPeriodStart:   nil,
		CurrentPeriodEnd:     nil,
		TrialEnd:             nil,
		CreatedAt:            now,
		UpdatedAt:            now,
	}
	AssertSubscriptionEqual(mockT, sub1, sub3)

	if !mockT.errorFCalled {
		t.Error("Expected assertion to fail for different subscriptions")
	}
}

func TestAssertTimeAlmostEqual(t *testing.T) {
	baseTime := time.Now()
	closeTime := baseTime.Add(500 * time.Millisecond)
	farTime := baseTime.Add(2 * time.Second)

	// Close times should pass
	AssertTimeAlmostEqual(t, baseTime, closeTime, time.Second)

	// Far times should fail
	mockT := &mockTesting{}
	AssertTimeAlmostEqual(mockT, baseTime, farTime, time.Second)

	if !mockT.errorFCalled {
		t.Error("Expected assertion to fail for times too far apart")
	}
}

func TestAssertSliceEqual(t *testing.T) {
	slice1 := []string{"a", "b", "c"}
	slice2 := []string{"a", "b", "c"}
	slice3 := []string{"a", "b", "d"}

	// Same slices should pass
	AssertSliceEqual(t, slice1, slice2)

	// Different slices should fail
	mockT := &mockTesting{}
	AssertSliceEqual(mockT, slice1, slice3)

	if !mockT.errorFCalled {
		t.Error("Expected assertion to fail for different slices")
	}
}

func TestAssertSliceContains(t *testing.T) {
	slice := []string{"a", "b", "c"}

	// Element in slice should pass
	AssertSliceContains(t, slice, "b")

	// Element not in slice should fail
	mockT := &mockTesting{}
	AssertSliceContains(mockT, slice, "d")

	if !mockT.errorFCalled {
		t.Error("Expected assertion to fail for element not in slice")
	}

	// Non-slice should fail
	mockT2 := &mockTesting{}
	AssertSliceContains(mockT2, "not a slice", "element")

	if !mockT2.errorFCalled {
		t.Error("Expected assertion to fail for non-slice argument")
	}
}

func TestAssertNotNil(t *testing.T) {
	var nilPtr *string
	nonNilPtr := &[]string{"test"}[0]
	var nilSlice []string
	nonNilSlice := []string{"test"}

	// Non-nil values should pass
	AssertNotNil(t, nonNilPtr, "pointer should not be nil")
	AssertNotNil(t, nonNilSlice, "slice should not be nil")
	AssertNotNil(t, "string", "string should not be nil")

	// Nil values should fail
	mockT := &mockTesting{}
	AssertNotNil(mockT, nilPtr, "pointer should not be nil")
	if !mockT.errorFCalled {
		t.Error("Expected assertion to fail for nil pointer")
	}

	mockT2 := &mockTesting{}
	AssertNotNil(mockT2, nilSlice, "slice should not be nil")
	if !mockT2.errorFCalled {
		t.Error("Expected assertion to fail for nil slice")
	}

	mockT3 := &mockTesting{}
	AssertNotNil(mockT3, nil, "value should not be nil")
	if !mockT3.errorFCalled {
		t.Error("Expected assertion to fail for nil value")
	}
}

func TestAssertNil(t *testing.T) {
	var nilPtr *string
	nonNilPtr := &[]string{"test"}[0]
	var nilSlice []string
	nonNilSlice := []string{"test"}

	// Nil values should pass
	AssertNil(t, nilPtr, "pointer should be nil")
	AssertNil(t, nilSlice, "slice should be nil")
	AssertNil(t, nil, "value should be nil")

	// Non-nil values should fail
	mockT := &mockTesting{}
	AssertNil(mockT, nonNilPtr, "pointer should be nil")
	if !mockT.errorFCalled {
		t.Error("Expected assertion to fail for non-nil pointer")
	}

	mockT2 := &mockTesting{}
	AssertNil(mockT2, nonNilSlice, "slice should be nil")
	if !mockT2.errorFCalled {
		t.Error("Expected assertion to fail for non-nil slice")
	}

	mockT3 := &mockTesting{}
	AssertNil(mockT3, "not nil", "value should be nil")
	if !mockT3.errorFCalled {
		t.Error("Expected assertion to fail for non-nil value")
	}
}

func TestEqualStringPointers(t *testing.T) {
	str1 := "test"
	str2 := "test"
	str3 := "different"

	// Both nil should be equal
	if !equalStringPointers(nil, nil) {
		t.Error("Both nil pointers should be equal")
	}

	// One nil, one not nil should not be equal
	if equalStringPointers(nil, &str1) {
		t.Error("nil and non-nil pointers should not be equal")
	}

	if equalStringPointers(&str1, nil) {
		t.Error("non-nil and nil pointers should not be equal")
	}

	// Same values should be equal
	if !equalStringPointers(&str1, &str2) {
		t.Error("Pointers to same string values should be equal")
	}

	// Different values should not be equal
	if equalStringPointers(&str1, &str3) {
		t.Error("Pointers to different string values should not be equal")
	}
}
