package models

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPendingInvitationsListResponse_JSONStructure(t *testing.T) {
	// Test that the response structure marshals correctly to JSON
	response := PendingInvitationsListResponse{
		Invitations: []InvitationResponse{
			{
				ID:           "inv-1",
				Token:        "abc123",
				TeamID:       "team-1",
				TeamName:     "Engineering Team",
				InviteeEmail: "test@example.com",
				Role:         "member",
				Status:       "pending",
				ExpiresAt:    "2024-01-01T00:00:00Z",
				CreatedAt:    "2024-01-01T00:00:00Z",
				InvitedBy: &InviterInfo{
					ID:    "user-1",
					Name:  "John Doe",
					Email: "john@example.com",
				},
			},
		},
		TotalCount: 1,
		Page:       1,
		PageSize:   20,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(response)
	assert.NoError(t, err, "Should marshal to JSON without error")

	// Verify JSON structure
	var result map[string]interface{}
	err = json.Unmarshal(jsonData, &result)
	assert.NoError(t, err, "Should unmarshal JSON without error")

	// Verify fields exist and have correct types
	assert.Contains(t, result, "invitations", "Should have invitations field")
	assert.Contains(t, result, "total_count", "Should have total_count field")
	assert.Contains(t, result, "page", "Should have page field")
	assert.Contains(t, result, "page_size", "Should have page_size field")

	// Verify values
	assert.Equal(t, float64(1), result["total_count"], "total_count should be 1")
	assert.Equal(t, float64(1), result["page"], "page should be 1")
	assert.Equal(t, float64(20), result["page_size"], "page_size should be 20")

	// Verify invitations array
	invitations, ok := result["invitations"].([]interface{})
	assert.True(t, ok, "invitations should be an array")
	assert.Equal(t, 1, len(invitations), "Should have 1 invitation")
}

func TestPendingInvitationsListResponse_EmptyInvitations(t *testing.T) {
	// Test empty invitations array
	response := PendingInvitationsListResponse{
		Invitations: []InvitationResponse{},
		TotalCount:  0,
		Page:        1,
		PageSize:    20,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(response)
	assert.NoError(t, err, "Should marshal to JSON without error")

	// Verify JSON structure
	var result map[string]interface{}
	err = json.Unmarshal(jsonData, &result)
	assert.NoError(t, err, "Should unmarshal JSON without error")

	// Verify empty array structure
	invitations, ok := result["invitations"].([]interface{})
	assert.True(t, ok, "invitations should be an array")
	assert.Equal(t, 0, len(invitations), "Should have 0 invitations")
	assert.Equal(t, float64(0), result["total_count"], "total_count should be 0")
}

func TestPendingInvitationsListResponse_UnmarshalFromJSON(t *testing.T) {
	// Test unmarshaling from JSON (simulating what frontend receives)
	jsonData := `{
		"invitations": [
			{
				"id": "inv-1",
				"token": "abc123",
				"team_id": "team-1",
				"team_name": "Engineering Team",
				"invitee_email": "test@example.com",
				"role": "member",
				"status": "pending",
				"expires_at": "2024-01-01T00:00:00Z",
				"created_at": "2024-01-01T00:00:00Z",
				"invited_by": {
					"id": "user-1",
					"name": "John Doe",
					"email": "john@example.com"
				}
			}
		],
		"total_count": 1,
		"page": 1,
		"page_size": 20
	}`

	var response PendingInvitationsListResponse
	err := json.Unmarshal([]byte(jsonData), &response)
	assert.NoError(t, err, "Should unmarshal from JSON without error")

	// Verify structure
	assert.NotNil(t, response.Invitations, "Invitations should not be nil")
	assert.Equal(t, 1, len(response.Invitations), "Should have 1 invitation")
	assert.Equal(t, 1, response.TotalCount, "TotalCount should be 1")
	assert.Equal(t, 1, response.Page, "Page should be 1")
	assert.Equal(t, 20, response.PageSize, "PageSize should be 20")

	// Verify invitation data
	assert.Equal(t, "inv-1", response.Invitations[0].ID)
	assert.Equal(t, "abc123", response.Invitations[0].Token)
	assert.Equal(t, "team-1", response.Invitations[0].TeamID)
	assert.Equal(t, "Engineering Team", response.Invitations[0].TeamName)
	assert.Equal(t, "test@example.com", response.Invitations[0].InviteeEmail)

	// Verify invited_by data
	assert.NotNil(t, response.Invitations[0].InvitedBy, "InvitedBy should not be nil")
	assert.Equal(t, "user-1", response.Invitations[0].InvitedBy.ID)
	assert.Equal(t, "John Doe", response.Invitations[0].InvitedBy.Name)
	assert.Equal(t, "john@example.com", response.Invitations[0].InvitedBy.Email)
}

func TestPendingInvitationsListResponse_MatchesTeamListResponsePattern(t *testing.T) {
	// Verify that PendingInvitationsListResponse follows the same pattern as TeamListResponse

	// Both should have similar fields when marshaled
	invitationResponse := PendingInvitationsListResponse{
		Invitations: []InvitationResponse{},
		TotalCount:  0,
		Page:        1,
		PageSize:    20,
	}

	teamResponse := TeamListResponse{
		Teams:      []Team{},
		TotalCount: 0,
		Page:       1,
		PageSize:   20,
	}

	// Marshal both
	invJSON, err := json.Marshal(invitationResponse)
	assert.NoError(t, err, "Should marshal invitation response")
	teamJSON, err := json.Marshal(teamResponse)
	assert.NoError(t, err, "Should marshal team response")

	// Parse both
	var invMap, teamMap map[string]interface{}
	err = json.Unmarshal(invJSON, &invMap)
	assert.NoError(t, err, "Should unmarshal invitation JSON")
	err = json.Unmarshal(teamJSON, &teamMap)
	assert.NoError(t, err, "Should unmarshal team JSON")

	// Both should have the same metadata fields
	assert.Equal(t, invMap["total_count"], teamMap["total_count"], "Both should have total_count")
	assert.Equal(t, invMap["page"], teamMap["page"], "Both should have page")
	assert.Equal(t, invMap["page_size"], teamMap["page_size"], "Both should have page_size")

	// Both should have their respective array fields
	assert.Contains(t, invMap, "invitations", "Should have invitations array")
	assert.Contains(t, teamMap, "teams", "Should have teams array")
}

func TestInvitationResponse_WithAllFields(t *testing.T) {
	// Test that InvitationResponse includes all required fields including new ones
	response := InvitationResponse{
		ID:           "inv-1",
		Token:        "abc123xyz",
		TeamID:       "team-1",
		TeamName:     "Engineering Team",
		InviteeEmail: "test@example.com",
		Role:         "member",
		Status:       "pending",
		ExpiresAt:    "2024-01-01T00:00:00Z",
		CreatedAt:    "2024-01-01T00:00:00Z",
		InvitedBy: &InviterInfo{
			ID:    "user-1",
			Name:  "John Doe",
			Email: "john@example.com",
		},
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(response)
	assert.NoError(t, err, "Should marshal to JSON without error")

	// Verify JSON structure
	var result map[string]interface{}
	err = json.Unmarshal(jsonData, &result)
	assert.NoError(t, err, "Should unmarshal JSON without error")

	// Verify all fields exist
	assert.Contains(t, result, "id", "Should have id field")
	assert.Contains(t, result, "token", "Should have token field")
	assert.Contains(t, result, "team_id", "Should have team_id field")
	assert.Contains(t, result, "team_name", "Should have team_name field")
	assert.Contains(t, result, "invitee_email", "Should have invitee_email field")
	assert.Contains(t, result, "role", "Should have role field")
	assert.Contains(t, result, "status", "Should have status field")
	assert.Contains(t, result, "expires_at", "Should have expires_at field")
	assert.Contains(t, result, "created_at", "Should have created_at field")
	assert.Contains(t, result, "invited_by", "Should have invited_by field")

	// Verify new field values
	assert.Equal(t, "abc123xyz", result["token"], "token should match")
	assert.Equal(t, "Engineering Team", result["team_name"], "team_name should match")

	// Verify invited_by structure
	invitedBy, ok := result["invited_by"].(map[string]interface{})
	assert.True(t, ok, "invited_by should be an object")
	assert.Equal(t, "user-1", invitedBy["id"], "inviter id should match")
	assert.Equal(t, "John Doe", invitedBy["name"], "inviter name should match")
	assert.Equal(t, "john@example.com", invitedBy["email"], "inviter email should match")
}

func TestInvitationResponse_WithoutInvitedBy(t *testing.T) {
	// Test that invited_by is omitted when nil
	response := InvitationResponse{
		ID:           "inv-1",
		Token:        "abc123xyz",
		TeamID:       "team-1",
		TeamName:     "Engineering Team",
		InviteeEmail: "test@example.com",
		Role:         "member",
		Status:       "pending",
		ExpiresAt:    "2024-01-01T00:00:00Z",
		CreatedAt:    "2024-01-01T00:00:00Z",
		InvitedBy:    nil,
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(response)
	assert.NoError(t, err, "Should marshal to JSON without error")

	// Verify JSON structure
	var result map[string]interface{}
	err = json.Unmarshal(jsonData, &result)
	assert.NoError(t, err, "Should unmarshal JSON without error")

	// Verify invited_by is omitted (not present in JSON)
	_, exists := result["invited_by"]
	assert.False(t, exists, "invited_by should be omitted when nil (omitempty)")
}

func TestAcceptInvitationResponse_JSONStructure(t *testing.T) {
	// Test that AcceptInvitationResponse marshals correctly to JSON
	response := AcceptInvitationResponse{
		TeamID:   "team-1",
		TeamName: "Engineering Team",
		Message:  "Successfully joined team Engineering Team",
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(response)
	assert.NoError(t, err, "Should marshal to JSON without error")

	// Verify JSON structure
	var result map[string]interface{}
	err = json.Unmarshal(jsonData, &result)
	assert.NoError(t, err, "Should unmarshal JSON without error")

	// Verify fields exist
	assert.Contains(t, result, "team_id", "Should have team_id field")
	assert.Contains(t, result, "team_name", "Should have team_name field")
	assert.Contains(t, result, "message", "Should have message field")

	// Verify values
	assert.Equal(t, "team-1", result["team_id"], "team_id should match")
	assert.Equal(t, "Engineering Team", result["team_name"], "team_name should match")
	assert.Equal(t, "Successfully joined team Engineering Team", result["message"], "message should match")
}

func TestInviterInfo_JSONStructure(t *testing.T) {
	// Test that InviterInfo marshals correctly to JSON
	inviter := InviterInfo{
		ID:    "user-1",
		Name:  "John Doe",
		Email: "john@example.com",
	}

	// Marshal to JSON
	jsonData, err := json.Marshal(inviter)
	assert.NoError(t, err, "Should marshal to JSON without error")

	// Verify JSON structure
	var result map[string]interface{}
	err = json.Unmarshal(jsonData, &result)
	assert.NoError(t, err, "Should unmarshal JSON without error")

	// Verify fields exist
	assert.Contains(t, result, "id", "Should have id field")
	assert.Contains(t, result, "name", "Should have name field")
	assert.Contains(t, result, "email", "Should have email field")

	// Verify values
	assert.Equal(t, "user-1", result["id"], "id should match")
	assert.Equal(t, "John Doe", result["name"], "name should match")
	assert.Equal(t, "john@example.com", result["email"], "email should match")
}
