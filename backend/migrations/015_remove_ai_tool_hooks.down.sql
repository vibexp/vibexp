-- Reverse 015_remove_ai_tool_hooks (epic #610, issue #614).
--
-- IMPORTANT — THIS RESTORES STRUCTURE ONLY, NOT DATA.
-- The up migration deletes rows irrecoverably. Rolling back recreates the two payload
-- tables (empty), re-inserts the `ai_tools` catalog row, and restores the original
-- chk_usage_type. It CANNOT restore:
--   * hook payload rows in claude_code_hooks_payload / cursor_ide_hooks_payload;
--   * api_key_integration_permissions rows granting `ai_tools`;
--   * activities rows of type claude_code_session / claude_code_tool / claude_code_prompt;
--   * which api_keys previously had usage_type = 'ai_tools' (they were re-pointed to
--     'everything' and are indistinguishable from keys that were always 'everything').
-- Restore those from a backup if you need them.

-- 1. Restore the original chk_usage_type, which also permitted 'ai_tools'.
ALTER TABLE api_keys DROP CONSTRAINT IF EXISTS chk_usage_type;
ALTER TABLE api_keys
    ADD CONSTRAINT chk_usage_type CHECK (
        (usage_type)::text = ANY ((ARRAY[
            'ai_tools'::character varying,
            'cli'::character varying,
            'mcp'::character varying,
            'everything'::character varying
        ])::text[])
    );

-- 2. Re-insert the catalog row, with the id the baseline seeded, so any restored grant
--    rows referencing it line up again.
INSERT INTO public.api_key_integrations_catalog
    (id, integration_code, integration_name, description, is_active, created_at, updated_at)
VALUES (
    'a79a0d56-146d-486d-8a35-8375327bd6a4',
    'ai_tools',
    'AI Tools Integration',
    'Access for Claude Code, Cursor IDE, and other AI-powered development tools',
    true,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
)
ON CONFLICT (integration_code) DO NOTHING;

-- 3. Recreate claude_code_hooks_payload with its sequence, indexes, trigger and FK.
CREATE TABLE IF NOT EXISTS public.claude_code_hooks_payload (
    id integer NOT NULL,
    session_id character varying(255) NOT NULL,
    transcript_path text,
    cwd text,
    hook_event_name character varying(100) NOT NULL,
    tool_name character varying(100),
    tool_input jsonb,
    tool_response jsonb,
    prompt text,
    message text,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    user_id character varying(255),
    team_id uuid NOT NULL
);

CREATE SEQUENCE IF NOT EXISTS public.claude_code_hooks_payload_id_seq
    AS integer START WITH 1 INCREMENT BY 1 NO MINVALUE NO MAXVALUE CACHE 1;
ALTER SEQUENCE public.claude_code_hooks_payload_id_seq
    OWNED BY public.claude_code_hooks_payload.id;
ALTER TABLE ONLY public.claude_code_hooks_payload
    ALTER COLUMN id SET DEFAULT nextval('public.claude_code_hooks_payload_id_seq'::regclass);

ALTER TABLE ONLY public.claude_code_hooks_payload
    ADD CONSTRAINT claude_code_hooks_payload_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.claude_code_hooks_payload
    ADD CONSTRAINT fk_claude_code_hooks_payload_team FOREIGN KEY (team_id)
    REFERENCES public.teams(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_claude_code_hooks_created_at
    ON public.claude_code_hooks_payload USING btree (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_claude_code_hooks_event_name
    ON public.claude_code_hooks_payload USING btree (hook_event_name);
CREATE INDEX IF NOT EXISTS idx_claude_code_hooks_payload_team_id
    ON public.claude_code_hooks_payload USING btree (team_id);
CREATE INDEX IF NOT EXISTS idx_claude_code_hooks_session_id
    ON public.claude_code_hooks_payload USING btree (session_id);
CREATE INDEX IF NOT EXISTS idx_claude_code_hooks_tool_name
    ON public.claude_code_hooks_payload USING btree (tool_name);
CREATE INDEX IF NOT EXISTS idx_claude_code_hooks_user_created_at
    ON public.claude_code_hooks_payload USING btree (user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_claude_code_hooks_user_id
    ON public.claude_code_hooks_payload USING btree (user_id);
CREATE INDEX IF NOT EXISTS idx_claude_code_hooks_user_session
    ON public.claude_code_hooks_payload USING btree (user_id, session_id);

CREATE TRIGGER update_claude_code_hooks_payload_updated_at
    BEFORE UPDATE ON public.claude_code_hooks_payload
    FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();

-- 4. Recreate cursor_ide_hooks_payload with its sequence, indexes, trigger and FK.
CREATE TABLE IF NOT EXISTS public.cursor_ide_hooks_payload (
    id integer NOT NULL,
    user_id character varying(255),
    session_id character varying(255) NOT NULL,
    conversation_id character varying(255),
    generation_id character varying(255),
    hook_event_name character varying(100) NOT NULL,
    tool_name character varying(100),
    workspace_roots text[],
    configuration jsonb,
    reference jsonb,
    context jsonb,
    input jsonb,
    output jsonb,
    induced_failure jsonb,
    payload jsonb NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    team_id uuid NOT NULL
);

CREATE SEQUENCE IF NOT EXISTS public.cursor_ide_hooks_payload_id_seq
    AS integer START WITH 1 INCREMENT BY 1 NO MINVALUE NO MAXVALUE CACHE 1;
ALTER SEQUENCE public.cursor_ide_hooks_payload_id_seq
    OWNED BY public.cursor_ide_hooks_payload.id;
ALTER TABLE ONLY public.cursor_ide_hooks_payload
    ALTER COLUMN id SET DEFAULT nextval('public.cursor_ide_hooks_payload_id_seq'::regclass);

ALTER TABLE ONLY public.cursor_ide_hooks_payload
    ADD CONSTRAINT cursor_ide_hooks_payload_pkey PRIMARY KEY (id);
ALTER TABLE ONLY public.cursor_ide_hooks_payload
    ADD CONSTRAINT fk_cursor_ide_hooks_payload_team FOREIGN KEY (team_id)
    REFERENCES public.teams(id) ON DELETE CASCADE;

CREATE INDEX IF NOT EXISTS idx_cursor_ide_hooks_conversation_id
    ON public.cursor_ide_hooks_payload USING btree (conversation_id);
CREATE INDEX IF NOT EXISTS idx_cursor_ide_hooks_created_at
    ON public.cursor_ide_hooks_payload USING btree (created_at DESC);
CREATE INDEX IF NOT EXISTS idx_cursor_ide_hooks_event_name
    ON public.cursor_ide_hooks_payload USING btree (hook_event_name);
CREATE INDEX IF NOT EXISTS idx_cursor_ide_hooks_generation_id
    ON public.cursor_ide_hooks_payload USING btree (generation_id);
CREATE INDEX IF NOT EXISTS idx_cursor_ide_hooks_payload_team_id
    ON public.cursor_ide_hooks_payload USING btree (team_id);
CREATE INDEX IF NOT EXISTS idx_cursor_ide_hooks_session_id
    ON public.cursor_ide_hooks_payload USING btree (session_id);
CREATE INDEX IF NOT EXISTS idx_cursor_ide_hooks_tool_name
    ON public.cursor_ide_hooks_payload USING btree (tool_name);
CREATE INDEX IF NOT EXISTS idx_cursor_ide_hooks_user_id
    ON public.cursor_ide_hooks_payload USING btree (user_id);

CREATE TRIGGER update_cursor_ide_hooks_payload_updated_at
    BEFORE UPDATE ON public.cursor_ide_hooks_payload
    FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();
