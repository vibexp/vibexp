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
