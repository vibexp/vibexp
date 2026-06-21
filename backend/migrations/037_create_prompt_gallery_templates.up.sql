CREATE TABLE IF NOT EXISTS prompt_gallery_templates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    title VARCHAR(255) NOT NULL,
    description TEXT,
    content TEXT NOT NULL,
    category VARCHAR(100) NOT NULL,
    tags JSONB DEFAULT '[]',
    metadata JSONB DEFAULT '{}',
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    updated_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_prompt_gallery_templates_category ON prompt_gallery_templates(category);
CREATE INDEX idx_prompt_gallery_templates_tags ON prompt_gallery_templates USING GIN (tags);

-- Insert 5 dummy prompts for testing
INSERT INTO prompt_gallery_templates (title, description, content, category, tags, metadata) VALUES
(
    'Code Review Request',
    'Request a thorough code review with focus on best practices, security, and performance',
    'Please review the following code for:
- Code quality and best practices
- Security vulnerabilities
- Performance optimizations
- Error handling
- Code documentation

Code:
{{code}}

Context:
{{context}}',
    'Engineering',
    '["code-review", "quality", "security"]',
    '{"difficulty": "beginner", "use_case": "development"}'
),
(
    'Product Requirements Document',
    'Generate a comprehensive product requirements document for a new feature',
    'Create a Product Requirements Document (PRD) for the following feature:

Feature Name: {{feature_name}}
Problem Statement: {{problem}}
Target Users: {{users}}

Please include:
1. Executive Summary
2. Problem Statement
3. Goals and Objectives
4. User Stories
5. Functional Requirements
6. Non-Functional Requirements
7. Success Metrics
8. Timeline and Milestones',
    'Product Management',
    '["prd", "requirements", "planning"]',
    '{"difficulty": "intermediate", "use_case": "planning"}'
),
(
    'Social Media Campaign',
    'Create engaging social media content for marketing campaigns',
    'Create a social media campaign for:

Product/Service: {{product}}
Target Audience: {{audience}}
Campaign Goal: {{goal}}
Platform: {{platform}}

Generate:
- 5 engaging post variations
- Relevant hashtags
- Call-to-action suggestions
- Optimal posting times
- Engagement strategies',
    'Marketing',
    '["social-media", "content", "campaign"]',
    '{"difficulty": "beginner", "use_case": "marketing"}'
),
(
    'SQL Query Optimization',
    'Analyze and optimize SQL queries for better performance',
    'Analyze the following SQL query and provide optimization suggestions:

Query:
{{query}}

Database: {{database_type}}
Table Schema: {{schema}}

Please provide:
1. Performance analysis
2. Index recommendations
3. Query rewrite suggestions
4. Explain plan interpretation
5. Best practices recommendations',
    'Data Analysis',
    '["sql", "optimization", "performance"]',
    '{"difficulty": "advanced", "use_case": "data-engineering"}'
),
(
    'Customer Support Response',
    'Generate professional and empathetic customer support responses',
    'Customer Issue:
{{customer_issue}}

Customer Tone: {{customer_tone}}
Priority: {{priority}}

Generate a professional response that:
- Acknowledges the issue
- Shows empathy
- Provides a solution or next steps
- Maintains brand voice
- Includes escalation path if needed',
    'Customer Support',
    '["support", "customer-service", "communication"]',
    '{"difficulty": "beginner", "use_case": "customer-support"}'
);
