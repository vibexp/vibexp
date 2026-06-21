CREATE TABLE prompt_shares (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    prompt_id UUID NOT NULL REFERENCES prompts(id) ON DELETE CASCADE,
    share_token VARCHAR(64) UNIQUE NOT NULL,
    share_type VARCHAR(20) NOT NULL CHECK (share_type IN ('public', 'restricted')),
    created_by UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    expires_at TIMESTAMP WITH TIME ZONE,
    is_active BOOLEAN DEFAULT true,
    access_count INTEGER DEFAULT 0,
    UNIQUE(prompt_id)
);

CREATE INDEX idx_prompt_shares_token ON prompt_shares(share_token);
CREATE INDEX idx_prompt_shares_prompt_id ON prompt_shares(prompt_id);
CREATE INDEX idx_prompt_shares_created_by ON prompt_shares(created_by);
