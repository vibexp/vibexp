-- Table: team_subscriptions
-- Stores Stripe subscription information for team workspaces
CREATE TABLE team_subscriptions (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    team_id UUID NOT NULL REFERENCES teams(id) ON DELETE CASCADE,
    stripe_subscription_id VARCHAR(255) UNIQUE NOT NULL,
    stripe_customer_id VARCHAR(255) NOT NULL,
    tier VARCHAR(50) NOT NULL,
    seat_count INT NOT NULL,
    status VARCHAR(50) NOT NULL,
    billing_interval VARCHAR(20) NOT NULL,
    current_period_start TIMESTAMP WITH TIME ZONE NOT NULL,
    current_period_end TIMESTAMP WITH TIME ZONE NOT NULL,
    trial_end TIMESTAMP WITH TIME ZONE,
    canceled_at TIMESTAMP WITH TIME ZONE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,

    -- Constraints
    CONSTRAINT team_subscriptions_seat_count_positive CHECK (seat_count > 0),
    CONSTRAINT team_subscriptions_tier_valid CHECK (tier IN ('starter', 'professional', 'enterprise')),
    CONSTRAINT team_subscriptions_status_valid CHECK (status IN ('trialing', 'active', 'past_due', 'canceled', 'unpaid')),
    CONSTRAINT team_subscriptions_billing_interval_valid CHECK (billing_interval IN ('month', 'year'))
);

-- Indexes for performance
CREATE UNIQUE INDEX idx_team_subscriptions_team_id ON team_subscriptions(team_id);
CREATE INDEX idx_team_subscriptions_stripe_subscription_id ON team_subscriptions(stripe_subscription_id);
CREATE INDEX idx_team_subscriptions_stripe_customer_id ON team_subscriptions(stripe_customer_id);
CREATE INDEX idx_team_subscriptions_status ON team_subscriptions(status);
CREATE INDEX idx_team_subscriptions_tier ON team_subscriptions(tier);

-- Trigger for updated_at
CREATE TRIGGER update_team_subscriptions_updated_at
    BEFORE UPDATE ON team_subscriptions
    FOR EACH ROW
    EXECUTE FUNCTION update_updated_at_column();

COMMENT ON TABLE team_subscriptions IS 'Stores team subscription data from Stripe for per-seat pricing';
COMMENT ON COLUMN team_subscriptions.tier IS 'Pricing tier: starter, professional, enterprise';
COMMENT ON COLUMN team_subscriptions.seat_count IS 'Number of paid seats (licensed members)';
COMMENT ON COLUMN team_subscriptions.status IS 'Stripe subscription status: trialing, active, past_due, canceled, unpaid';
