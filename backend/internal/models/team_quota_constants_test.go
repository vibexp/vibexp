package models

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetTeamResourceQuotaPrompts(t *testing.T) {
	tests := []struct {
		name        string
		tier        string
		seatCount   int
		wantBase    int
		wantPerSeat int
	}{
		{"starter", TeamTierStarter, 5, 1000, 100},
		{"professional", TeamTierProfessional, 5, 5000, 500},
		{"enterprise", TeamTierEnterprise, 5, -1, 0}, // Unlimited
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, perSeat := GetTeamResourceQuota(tt.tier, "prompt")
			assert.Equal(t, tt.wantBase, base)
			assert.Equal(t, tt.wantPerSeat, perSeat)

			// Verify calculated quota
			if base == -1 {
				assert.Equal(t, -1, base, "Enterprise should be unlimited")
			} else {
				quota := base + (perSeat * tt.seatCount)
				expectedQuotas := map[string]int{
					"starter":      1500, // 1000 + (100 × 5)
					"professional": 7500, // 5000 + (500 × 5)
				}
				assert.Equal(t, expectedQuotas[tt.name], quota)
			}
		})
	}
}

func TestGetTeamResourceQuotaArtifacts(t *testing.T) {
	tests := []struct {
		name        string
		tier        string
		wantBase    int
		wantPerSeat int
	}{
		{"starter", TeamTierStarter, 500, 50},
		{"professional", TeamTierProfessional, 1000, 100},
		{"enterprise", TeamTierEnterprise, -1, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			base, perSeat := GetTeamResourceQuota(tt.tier, "artifact")
			assert.Equal(t, tt.wantBase, base)
			assert.Equal(t, tt.wantPerSeat, perSeat)
		})
	}
}

func TestGetTeamResourceQuotaEdgeCases(t *testing.T) {
	t.Run("unknown_tier_defaults_to_starter", func(t *testing.T) {
		base, perSeat := GetTeamResourceQuota("unknown_tier", "prompt")
		assert.Equal(t, 1000, base, "Unknown tier should default to Starter")
		assert.Equal(t, 100, perSeat)
	})

	t.Run("unknown_resource_type", func(t *testing.T) {
		base, perSeat := GetTeamResourceQuota(TeamTierStarter, "unknown")
		assert.Equal(t, 0, base, "Unknown resource should return 0")
		assert.Equal(t, 0, perSeat)
	})
}

func TestTeamQuotaConfigsExist(t *testing.T) {
	assert.Contains(t, TeamQuotaConfigs, TeamTierStarter, "Starter tier should be defined")
	assert.Contains(t, TeamQuotaConfigs, TeamTierProfessional, "Professional tier should be defined")
	assert.Contains(t, TeamQuotaConfigs, TeamTierEnterprise, "Enterprise tier should be defined")
}

// TestPRDSpecificationCompliance validates the exact examples from issue #642
func TestPRDSpecificationCompliance(t *testing.T) {
	t.Run("PRD_Example_Teams_Starter_5_Seats", func(t *testing.T) {
		base, perSeat := GetTeamResourceQuota(TeamTierStarter, "prompt")
		quota := base + (perSeat * 5)
		assert.Equal(t, 1500, quota, "Teams Starter with 5 seats should provide 1,500 prompts (1000 + 100×5)")
	})

	t.Run("PRD_Example_Teams_Professional_5_Seats", func(t *testing.T) {
		base, perSeat := GetTeamResourceQuota(TeamTierProfessional, "prompt")
		quota := base + (perSeat * 5)
		assert.Equal(t, 7500, quota, "Teams Professional with 5 seats should provide 7,500 prompts (5000 + 500×5)")
	})

	t.Run("PRD_Example_Teams_Enterprise_Unlimited", func(t *testing.T) {
		base, _ := GetTeamResourceQuota(TeamTierEnterprise, "prompt")
		assert.Equal(t, -1, base, "Teams Enterprise should provide unlimited prompts (-1)")
	})
}
