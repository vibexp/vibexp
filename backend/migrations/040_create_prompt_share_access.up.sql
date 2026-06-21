CREATE TABLE prompt_share_access (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    share_id UUID NOT NULL REFERENCES prompt_shares(id) ON DELETE CASCADE,
    email VARCHAR(255) NOT NULL,
    granted_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(share_id, email)
);

CREATE INDEX idx_prompt_share_access_share_id ON prompt_share_access(share_id);
CREATE INDEX idx_prompt_share_access_email ON prompt_share_access(email);
