-- Table: financial_kpi_snapshots
-- Durable, DB-backed daily snapshots of financial KPIs (MRR, ARR, active
-- subscriptions, churn) computed from team_subscriptions (the source of truth).
-- One row per calendar day; the snapshot job upserts on snapshot_date so a day
-- can be recomputed idempotently. All monetary amounts are stored in integer
-- cents to match the team plans API convention (no floating-point money).
CREATE TABLE financial_kpi_snapshots (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    snapshot_date DATE NOT NULL UNIQUE,
    currency VARCHAR(3) NOT NULL DEFAULT 'eur',
    mrr_cents BIGINT NOT NULL DEFAULT 0,
    arr_cents BIGINT NOT NULL DEFAULT 0,
    active_subscriptions INT NOT NULL DEFAULT 0,
    trialing_subscriptions INT NOT NULL DEFAULT 0,
    paying_seats INT NOT NULL DEFAULT 0,
    new_subscriptions INT NOT NULL DEFAULT 0,
    churned_subscriptions INT NOT NULL DEFAULT 0,
    unpriced_active_subscriptions INT NOT NULL DEFAULT 0,
    unpriced_seats INT NOT NULL DEFAULT 0,
    mrr_by_tier JSONB NOT NULL DEFAULT '{}',
    subscriptions_by_tier JSONB NOT NULL DEFAULT '{}',
    subscriptions_by_status JSONB NOT NULL DEFAULT '{}',
    computed_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_financial_kpi_snapshots_snapshot_date
    ON financial_kpi_snapshots (snapshot_date DESC);

COMMENT ON TABLE financial_kpi_snapshots IS 'Durable daily financial KPI snapshots computed from team_subscriptions';
COMMENT ON COLUMN financial_kpi_snapshots.snapshot_date IS 'Calendar day the snapshot covers; unique, upserted on recompute';
COMMENT ON COLUMN financial_kpi_snapshots.mrr_cents IS 'Monthly recurring revenue in integer cents (annual prices normalized /12)';
COMMENT ON COLUMN financial_kpi_snapshots.arr_cents IS 'Annual recurring revenue in integer cents (mrr_cents * 12)';
COMMENT ON COLUMN financial_kpi_snapshots.active_subscriptions IS 'Count of subscriptions with status active or past_due';
COMMENT ON COLUMN financial_kpi_snapshots.trialing_subscriptions IS 'Count of trialing subscriptions (not counted toward MRR)';
COMMENT ON COLUMN financial_kpi_snapshots.new_subscriptions IS 'Gross subscriptions created within the trailing 30 days, bounded by snapshot_date (not net of churn)';
COMMENT ON COLUMN financial_kpi_snapshots.churned_subscriptions IS 'Subscriptions reaching a terminal cancellation (canceled/unpaid) within the trailing 30 days, bounded by snapshot_date';
COMMENT ON COLUMN financial_kpi_snapshots.unpriced_active_subscriptions IS 'Active subscriptions with no resolvable Stripe list price (e.g. enterprise); counted in active_subscriptions but contributing 0 to MRR';
COMMENT ON COLUMN financial_kpi_snapshots.unpriced_seats IS 'Seats belonging to unpriced active subscriptions; counted in paying_seats but contributing 0 to MRR';
