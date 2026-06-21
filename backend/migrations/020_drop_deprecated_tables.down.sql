-- Rollback migration: recreate the deprecated tables
-- Note: This rollback will recreate the table structure but data will be lost

-- Recreate static_contexts table
CREATE TABLE static_contexts (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_name VARCHAR(80) NOT NULL DEFAULT 'shared',
    slug VARCHAR(255) NOT NULL,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content TEXT NOT NULL,
    title VARCHAR(255) NOT NULL DEFAULT '',
    description TEXT NOT NULL DEFAULT '',
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    expired_after TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP + INTERVAL '60 days',
    UNIQUE(project_name, slug, user_id)
);

CREATE INDEX idx_static_contexts_user_id ON static_contexts(user_id);
CREATE INDEX idx_static_contexts_project_name ON static_contexts(project_name);
CREATE INDEX idx_static_contexts_slug ON static_contexts(slug);
CREATE INDEX idx_static_contexts_created_at ON static_contexts(created_at DESC);
CREATE INDEX idx_static_contexts_expired_after ON static_contexts(expired_after);
CREATE INDEX idx_static_contexts_project_slug ON static_contexts(project_name, slug);

-- Recreate work_reports table
CREATE TABLE work_reports (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project VARCHAR(80) NOT NULL DEFAULT 'shared',
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    what TEXT NOT NULL,
    how TEXT NOT NULL,
    feedback TEXT,
    feedback_type VARCHAR(20) CHECK (feedback_type IN ('reward', 'punishment')),
    created_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP WITH TIME ZONE DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_work_reports_user_id ON work_reports(user_id);
CREATE INDEX idx_work_reports_project ON work_reports(project);
CREATE INDEX idx_work_reports_created_at ON work_reports(created_at DESC);
CREATE INDEX idx_work_reports_feedback_type ON work_reports(feedback_type);
CREATE INDEX idx_work_reports_project_user ON work_reports(project, user_id);
