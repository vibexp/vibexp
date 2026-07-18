package models

// TeamResourceQuota defines base and per-seat quota for a specific resource type
type TeamResourceQuota struct {
	Base    int // Base quota for the team
	PerSeat int // Additional quota per seat
}

// TeamQuotaConfig defines quota limits for a team tier across all resource types
type TeamQuotaConfig struct {
	Prompts     TeamResourceQuota
	Artifacts   TeamResourceQuota
	Memories    TeamResourceQuota
	SpecLibrary TeamResourceQuota
	Agents      TeamResourceQuota
	AgentConv   TeamResourceQuota
	AITools     TeamResourceQuota
	AISessions  TeamResourceQuota
	Feeds       TeamResourceQuota
	FeedItems   TeamResourceQuota
	MaxMembers  int
}

// TeamQuotaConfigs defines quota configurations for each team tier
// Values are derived from PRD: docs/prd/prd-team-pricing-implementation.md
var TeamQuotaConfigs = map[string]TeamQuotaConfig{
	TeamTierStarter: {
		Prompts:     TeamResourceQuota{Base: 1000, PerSeat: 100},
		Artifacts:   TeamResourceQuota{Base: 500, PerSeat: 50},
		Memories:    TeamResourceQuota{Base: 200, PerSeat: 20},
		SpecLibrary: TeamResourceQuota{Base: 150, PerSeat: 15},
		Agents:      TeamResourceQuota{Base: 5, PerSeat: 1},
		AgentConv:   TeamResourceQuota{Base: 500, PerSeat: 50},
		AITools:     TeamResourceQuota{Base: 3, PerSeat: 0},
		AISessions:  TeamResourceQuota{Base: 750, PerSeat: 75},
		Feeds:       TeamResourceQuota{Base: 10, PerSeat: 1},
		FeedItems:   TeamResourceQuota{Base: 2500, PerSeat: 250},
		MaxMembers:  10,
	},
	TeamTierProfessional: {
		Prompts:     TeamResourceQuota{Base: 5000, PerSeat: 500},
		Artifacts:   TeamResourceQuota{Base: 1000, PerSeat: 100},
		Memories:    TeamResourceQuota{Base: 500, PerSeat: 50},
		SpecLibrary: TeamResourceQuota{Base: 300, PerSeat: 30},
		Agents:      TeamResourceQuota{Base: 10, PerSeat: 2},
		AgentConv:   TeamResourceQuota{Base: 1000, PerSeat: 100},
		AITools:     TeamResourceQuota{Base: 5, PerSeat: 0},
		AISessions:  TeamResourceQuota{Base: 1500, PerSeat: 150},
		Feeds:       TeamResourceQuota{Base: 25, PerSeat: 5},
		FeedItems:   TeamResourceQuota{Base: 10000, PerSeat: 1000},
		MaxMembers:  50,
	},
	TeamTierEnterprise: {
		Prompts:     TeamResourceQuota{Base: -1, PerSeat: 0}, // Unlimited
		Artifacts:   TeamResourceQuota{Base: -1, PerSeat: 0}, // Unlimited
		Memories:    TeamResourceQuota{Base: -1, PerSeat: 0}, // Unlimited
		SpecLibrary: TeamResourceQuota{Base: -1, PerSeat: 0}, // Unlimited
		Agents:      TeamResourceQuota{Base: -1, PerSeat: 0}, // Unlimited
		AgentConv:   TeamResourceQuota{Base: -1, PerSeat: 0}, // Unlimited
		AITools:     TeamResourceQuota{Base: -1, PerSeat: 0}, // Unlimited
		AISessions:  TeamResourceQuota{Base: -1, PerSeat: 0}, // Unlimited
		Feeds:       TeamResourceQuota{Base: -1, PerSeat: 0}, // Unlimited
		FeedItems:   TeamResourceQuota{Base: -1, PerSeat: 0}, // Unlimited
		MaxMembers:  -1,                                      // Unlimited
	},
}

// GetTeamResourceQuota returns the quota configuration for a specific resource type and tier
func GetTeamResourceQuota(tier, resourceType string) (base, perSeat int) {
	config, exists := TeamQuotaConfigs[tier]
	if !exists {
		// Default to starter tier if tier is not recognized
		config = TeamQuotaConfigs[TeamTierStarter]
	}

	var quota TeamResourceQuota
	switch resourceType {
	case "prompt":
		quota = config.Prompts
	case "artifact":
		quota = config.Artifacts
	case "memory":
		quota = config.Memories
	case "blueprint":
		quota = config.SpecLibrary
	case "agent":
		quota = config.Agents
	case "agent_conversation":
		quota = config.AgentConv
	case "ai_tool":
		quota = config.AITools
	case "ai_session":
		quota = config.AISessions
	case "feed":
		quota = config.Feeds
	case "feed_item":
		quota = config.FeedItems
	default:
		return 0, 0
	}

	return quota.Base, quota.PerSeat
}

// CalculateTeamQuota calculates resource quota for a given tier and seat count
// Returns -1 for unlimited resources (Enterprise tier)
// This function is used for backward compatibility with tests
func CalculateTeamQuota(tier string, seatCount int, resourceType string) int {
	base, perSeat := GetTeamResourceQuota(tier, resourceType)

	if base == -1 {
		return -1 // Unlimited
	}

	return base + (perSeat * seatCount)
}
