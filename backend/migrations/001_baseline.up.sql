-- Consolidated baseline schema for VibeXP.
--
-- This single migration replaces the original incremental migrations 001..107.
-- It is a `pg_dump` of a database that had all of those migrations applied:
-- the full schema plus the seed/reference data they insert (the API-key
-- integration catalog, prompt gallery templates, and system resource types).
-- golang-migrate's own schema_migrations table is intentionally excluded; the
-- migrator manages it.
--
-- Migration numbering restarts here: this baseline is version 1; the next new
-- migration is 002. A fresh database runs this file to build everything in one
-- step. An existing database must instead be re-stamped to version 1 (set
-- schema_migrations.version = 1) so the migrator skips the baseline and applies
-- only later (002+) migrations -- never re-run this file against a populated DB.
--
-- PostgreSQL database dump
--




--
-- Name: uuid-ossp; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS "uuid-ossp" WITH SCHEMA public;


--
-- Name: EXTENSION "uuid-ossp"; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION "uuid-ossp" IS 'generate universally unique identifiers (UUIDs)';


--
-- Name: vector; Type: EXTENSION; Schema: -; Owner: -
--

CREATE EXTENSION IF NOT EXISTS vector WITH SCHEMA public;


--
-- Name: EXTENSION vector; Type: COMMENT; Schema: -; Owner: -
--

COMMENT ON EXTENSION vector IS 'vector data type and ivfflat and hnsw access methods';


--
-- Name: update_agents_updated_at(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.update_agents_updated_at() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$;


--
-- Name: update_updated_at_column(); Type: FUNCTION; Schema: public; Owner: -
--

CREATE FUNCTION public.update_updated_at_column() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
BEGIN
    NEW.updated_at = CURRENT_TIMESTAMP;
    RETURN NEW;
END;
$$;




--
-- Name: activities; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.activities (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    activity_type character varying(50) NOT NULL,
    entity_type character varying(50) NOT NULL,
    entity_id character varying(255),
    session_id character varying(255),
    description text NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb,
    source_ip inet,
    user_agent text,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: TABLE activities; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.activities IS 'Comprehensive activity tracking for all application actions including authentication, API usage, resource management, and user interactions';


--
-- Name: COLUMN activities.activity_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.activities.activity_type IS 'Type of activity: auth_login, auth_logout, api_key_created, prompt_created, context_created, claude_code_session, etc.';


--
-- Name: COLUMN activities.entity_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.activities.entity_type IS 'Type of entity involved: user, api_key, prompt, context, session, work_report, etc.';


--
-- Name: COLUMN activities.entity_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.activities.entity_id IS 'ID of the specific entity involved in the activity';


--
-- Name: COLUMN activities.session_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.activities.session_id IS 'Session identifier for grouping related activities';


--
-- Name: COLUMN activities.description; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.activities.description IS 'Human-readable description of the activity';


--
-- Name: COLUMN activities.metadata; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.activities.metadata IS 'Additional activity-specific data in JSON format';


--
-- Name: agent_execution_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agent_execution_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    execution_id uuid NOT NULL,
    event_type character varying(50) NOT NULL,
    event_data jsonb DEFAULT '{}'::jsonb NOT NULL,
    sequence_number integer NOT NULL,
    received_at timestamp with time zone DEFAULT now() NOT NULL,
    CONSTRAINT agent_execution_events_event_type_check CHECK (((event_type)::text = ANY ((ARRAY['task'::character varying, 'status-update'::character varying, 'artifact-update'::character varying])::text[])))
);


--
-- Name: agent_executions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agent_executions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    agent_id uuid NOT NULL,
    user_id uuid NOT NULL,
    status character varying(20) DEFAULT 'running'::character varying NOT NULL,
    input jsonb DEFAULT '{}'::jsonb NOT NULL,
    output jsonb DEFAULT '{}'::jsonb,
    error text,
    started_at timestamp with time zone DEFAULT now() NOT NULL,
    ended_at timestamp with time zone,
    duration integer,
    task_id character varying(255),
    context_id character varying(255),
    current_state character varying(50),
    artifacts jsonb DEFAULT '[]'::jsonb,
    conversation_id character varying(255),
    version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT agent_executions_status_check CHECK (((status)::text = ANY ((ARRAY['running'::character varying, 'success'::character varying, 'error'::character varying, 'pending'::character varying, 'submitted'::character varying, 'working'::character varying, 'completed'::character varying, 'failed'::character varying, 'cancelled'::character varying])::text[]))),
    CONSTRAINT check_execution_time_order CHECK (((ended_at IS NULL) OR (ended_at >= started_at)))
);


--
-- Name: COLUMN agent_executions.conversation_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.agent_executions.conversation_id IS 'Groups related executions into conversations. First execution generates this ID, subsequent messages in same conversation use the same ID.';


--
-- Name: agents; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.agents (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    name character varying(100),
    description character varying(500),
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    last_run timestamp with time zone,
    total_runs integer DEFAULT 0 NOT NULL,
    success_rate numeric(5,2) DEFAULT 0.0 NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL,
    card_url text,
    agent_card jsonb,
    last_synced_at timestamp with time zone,
    credentials jsonb,
    version bigint DEFAULT 1 NOT NULL,
    team_id uuid NOT NULL,
    CONSTRAINT agents_status_check CHECK (((status)::text = ANY ((ARRAY['active'::character varying, 'paused'::character varying, 'error'::character varying])::text[]))),
    CONSTRAINT check_agent_name_or_card CHECK ((((name IS NOT NULL) AND (description IS NOT NULL)) OR (card_url IS NOT NULL)))
);


--
-- Name: COLUMN agents.credentials; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.agents.credentials IS 'Encrypted authentication credentials stored as JSONB with structure: {"security_scheme_name": {"type": "apiKey"|"http", "value": "encrypted_credential", "metadata": {}}}';


--
-- Name: api_key_integration_permissions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.api_key_integration_permissions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    api_key_id uuid NOT NULL,
    integration_code character varying(50) NOT NULL,
    granted_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: api_key_integrations_catalog; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.api_key_integrations_catalog (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    integration_code character varying(50) NOT NULL,
    integration_name character varying(100) NOT NULL,
    description text,
    is_active boolean DEFAULT true,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: api_keys; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.api_keys (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    user_id uuid NOT NULL,
    name character varying(255) NOT NULL,
    key_hash character varying(255) NOT NULL,
    key_prefix character varying(20) NOT NULL,
    last_used_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    version bigint DEFAULT 1 NOT NULL,
    usage_type character varying(20) DEFAULT 'everything'::character varying NOT NULL,
    is_legacy boolean DEFAULT false,
    migration_notes text,
    expires_at timestamp with time zone,
    CONSTRAINT chk_usage_type CHECK (((usage_type)::text = ANY ((ARRAY['ai_tools'::character varying, 'cli'::character varying, 'mcp'::character varying, 'everything'::character varying])::text[])))
);


--
-- Name: artifacts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.artifacts (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    slug character varying(255) NOT NULL,
    user_id uuid NOT NULL,
    content text NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    title character varying(255) NOT NULL,
    description text DEFAULT ''::text,
    type character varying(50) DEFAULT 'general'::character varying NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb,
    version bigint DEFAULT 1 NOT NULL,
    team_id uuid NOT NULL,
    project_id uuid NOT NULL
);


--
-- Name: attachments; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.attachments (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    team_id uuid NOT NULL,
    user_id uuid,
    owner_type text NOT NULL,
    owner_id uuid NOT NULL,
    file_name text NOT NULL,
    content_type text NOT NULL,
    size_bytes bigint NOT NULL,
    gcs_object_key text NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE attachments; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.attachments IS 'Generic file attachments for resources, keyed polymorphically by (owner_type, owner_id); binary stored in GCS';


--
-- Name: COLUMN attachments.team_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.attachments.team_id IS 'Team that owns the attachment; cascade-deletes with the team';


--
-- Name: COLUMN attachments.user_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.attachments.user_id IS 'User who uploaded the attachment; NULL when the user is later deleted';


--
-- Name: COLUMN attachments.owner_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.attachments.owner_type IS 'Polymorphic owner type: artifact (future: memory, blueprint, prompt)';


--
-- Name: COLUMN attachments.owner_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.attachments.owner_id IS 'Polymorphic owner ID; intentionally no FK (cf. embeddings) — cleanup is app-level';


--
-- Name: COLUMN attachments.gcs_object_key; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.attachments.gcs_object_key IS 'Object key in the GCS attachments bucket: {team_id}/{owner_type}/{owner_id}/{uuid}-{filename}';


--
-- Name: blueprints; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.blueprints (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    slug character varying(255) NOT NULL,
    user_id uuid NOT NULL,
    content text NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    status character varying(20) DEFAULT 'active'::character varying NOT NULL,
    title character varying(255) NOT NULL,
    description text DEFAULT ''::text,
    type character varying(50) DEFAULT 'general'::character varying NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb,
    subtype character varying(50),
    version bigint DEFAULT 1 NOT NULL,
    team_id uuid NOT NULL,
    project_id uuid NOT NULL,
    CONSTRAINT check_blueprints_subtype CHECK (((subtype IS NULL) OR ((subtype)::text = ANY ((ARRAY['sub-agents'::character varying, 'skills'::character varying, 'slash-commands'::character varying, 'others'::character varying, 'claude-md'::character varying, 'agents'::character varying, 'commands'::character varying, 'rules'::character varying, 'cursor-md'::character varying, 'agents-md'::character varying])::text[])))),
    CONSTRAINT check_blueprints_type CHECK (((type)::text = ANY ((ARRAY['general'::character varying, 'claude-code'::character varying, 'claude'::character varying, 'cursor'::character varying, 'codex'::character varying])::text[])))
);


--
-- Name: claude_code_hooks_payload; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.claude_code_hooks_payload (
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


--
-- Name: claude_code_hooks_payload_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.claude_code_hooks_payload_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: claude_code_hooks_payload_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.claude_code_hooks_payload_id_seq OWNED BY public.claude_code_hooks_payload.id;


--
-- Name: content_versions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.content_versions (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    team_id uuid NOT NULL,
    resource_type text NOT NULL,
    resource_id uuid NOT NULL,
    version_number integer NOT NULL,
    content text NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    change_summary text,
    actor_type text DEFAULT 'human'::text NOT NULL,
    CONSTRAINT content_versions_actor_type_check CHECK ((actor_type = ANY (ARRAY['human'::text, 'system'::text])))
);


--
-- Name: TABLE content_versions; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.content_versions IS 'Polymorphic content-version snapshots for team resources (e.g. artifacts), keyed by (resource_type, resource_id)';


--
-- Name: COLUMN content_versions.team_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.content_versions.team_id IS 'Team that owns the versioned resource';


--
-- Name: COLUMN content_versions.resource_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.content_versions.resource_type IS 'Type of the versioned resource: artifact, etc.';


--
-- Name: COLUMN content_versions.resource_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.content_versions.resource_id IS 'ID of the specific resource the snapshot belongs to';


--
-- Name: COLUMN content_versions.version_number; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.content_versions.version_number IS 'Monotonic per-resource version number, computed at insert time';


--
-- Name: COLUMN content_versions.content; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.content_versions.content IS 'Snapshot of the resource content at this version';


--
-- Name: COLUMN content_versions.created_by; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.content_versions.created_by IS 'User who triggered the snapshot; NULL when the user is later deleted';


--
-- Name: COLUMN content_versions.change_summary; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.content_versions.change_summary IS 'Optional human-readable summary of the change captured at this version (e.g. "Tightened the wording"); NULL when none was supplied';


--
-- Name: COLUMN content_versions.actor_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.content_versions.actor_type IS 'Who authored this version: human (a user edit) or system (e.g. a restore or future auto-save)';


--
-- Name: cursor_ide_hooks_payload; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.cursor_ide_hooks_payload (
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


--
-- Name: cursor_ide_hooks_payload_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.cursor_ide_hooks_payload_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: cursor_ide_hooks_payload_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.cursor_ide_hooks_payload_id_seq OWNED BY public.cursor_ide_hooks_payload.id;


--
-- Name: device_tokens; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.device_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token text NOT NULL,
    platform character varying(16) NOT NULL,
    user_agent text,
    last_used_at timestamp with time zone DEFAULT now(),
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: embedding_providers; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.embedding_providers (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    user_id uuid NOT NULL,
    name character varying(255) NOT NULL,
    provider_type character varying(100) NOT NULL,
    is_default boolean DEFAULT false,
    base_url character varying(500),
    api_key_encrypted text,
    configuration jsonb DEFAULT '{}'::jsonb,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    version bigint DEFAULT 1 NOT NULL
);


--
-- Name: embeddings; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.embeddings (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    entity_type character varying(50) NOT NULL,
    entity_id uuid NOT NULL,
    vector_embeddings public.vector(1024) NOT NULL,
    model_id character varying(255) NOT NULL,
    user_id uuid NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    content text DEFAULT ''::text NOT NULL,
    team_id uuid
);


--
-- Name: feed_item_replies; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.feed_item_replies (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    team_id uuid NOT NULL,
    feed_item_id uuid NOT NULL,
    content text NOT NULL,
    posted_by_user_id uuid NOT NULL,
    ai_assistant_name character varying(30),
    posted_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: feed_items; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.feed_items (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    team_id uuid NOT NULL,
    feed_id uuid NOT NULL,
    project_id uuid,
    title character varying(255) NOT NULL,
    content text NOT NULL,
    excerpt character varying(320) NOT NULL,
    ai_assistant_name character varying(30) NOT NULL,
    posted_by_user_id uuid NOT NULL,
    archived_at timestamp with time zone,
    posted_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: feeds; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.feeds (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    team_id uuid NOT NULL,
    name character varying(255) NOT NULL,
    description text,
    created_by_user_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: financial_kpi_snapshots; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.financial_kpi_snapshots (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    snapshot_date date NOT NULL,
    currency character varying(3) DEFAULT 'eur'::character varying NOT NULL,
    mrr_cents bigint DEFAULT 0 NOT NULL,
    arr_cents bigint DEFAULT 0 NOT NULL,
    active_subscriptions integer DEFAULT 0 NOT NULL,
    trialing_subscriptions integer DEFAULT 0 NOT NULL,
    paying_seats integer DEFAULT 0 NOT NULL,
    new_subscriptions integer DEFAULT 0 NOT NULL,
    churned_subscriptions integer DEFAULT 0 NOT NULL,
    unpriced_active_subscriptions integer DEFAULT 0 NOT NULL,
    unpriced_seats integer DEFAULT 0 NOT NULL,
    mrr_by_tier jsonb DEFAULT '{}'::jsonb NOT NULL,
    subscriptions_by_tier jsonb DEFAULT '{}'::jsonb NOT NULL,
    subscriptions_by_status jsonb DEFAULT '{}'::jsonb NOT NULL,
    computed_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP NOT NULL
);


--
-- Name: TABLE financial_kpi_snapshots; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.financial_kpi_snapshots IS 'Durable daily financial KPI snapshots computed from team_subscriptions';


--
-- Name: COLUMN financial_kpi_snapshots.snapshot_date; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.financial_kpi_snapshots.snapshot_date IS 'Calendar day the snapshot covers; unique, upserted on recompute';


--
-- Name: COLUMN financial_kpi_snapshots.mrr_cents; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.financial_kpi_snapshots.mrr_cents IS 'Monthly recurring revenue in integer cents (annual prices normalized /12)';


--
-- Name: COLUMN financial_kpi_snapshots.arr_cents; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.financial_kpi_snapshots.arr_cents IS 'Annual recurring revenue in integer cents (mrr_cents * 12)';


--
-- Name: COLUMN financial_kpi_snapshots.active_subscriptions; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.financial_kpi_snapshots.active_subscriptions IS 'Count of subscriptions with status active or past_due';


--
-- Name: COLUMN financial_kpi_snapshots.trialing_subscriptions; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.financial_kpi_snapshots.trialing_subscriptions IS 'Count of trialing subscriptions (not counted toward MRR)';


--
-- Name: COLUMN financial_kpi_snapshots.new_subscriptions; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.financial_kpi_snapshots.new_subscriptions IS 'Gross subscriptions created within the trailing 30 days, bounded by snapshot_date (not net of churn)';


--
-- Name: COLUMN financial_kpi_snapshots.churned_subscriptions; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.financial_kpi_snapshots.churned_subscriptions IS 'Subscriptions reaching a terminal cancellation (canceled/unpaid) within the trailing 30 days, bounded by snapshot_date';


--
-- Name: COLUMN financial_kpi_snapshots.unpriced_active_subscriptions; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.financial_kpi_snapshots.unpriced_active_subscriptions IS 'Active subscriptions with no resolvable Stripe list price (e.g. enterprise); counted in active_subscriptions but contributing 0 to MRR';


--
-- Name: COLUMN financial_kpi_snapshots.unpriced_seats; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.financial_kpi_snapshots.unpriced_seats IS 'Seats belonging to unpriced active subscriptions; counted in paying_seats but contributing 0 to MRR';


--
-- Name: github_installation_repositories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.github_installation_repositories (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    installation_id uuid NOT NULL,
    repository_id bigint NOT NULL,
    name character varying(255) NOT NULL,
    full_name character varying(500) NOT NULL,
    private boolean DEFAULT false,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: github_installations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.github_installations (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    team_id uuid NOT NULL,
    installation_id bigint NOT NULL,
    account_login character varying(255) NOT NULL,
    account_type character varying(50) NOT NULL,
    target_type character varying(50) NOT NULL,
    encrypted_access_token text NOT NULL,
    token_expires_at timestamp with time zone NOT NULL,
    permissions jsonb DEFAULT '{}'::jsonb,
    events text[] DEFAULT '{}'::text[],
    suspended_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now()
);


--
-- Name: memories; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.memories (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    text text NOT NULL,
    metadata jsonb DEFAULT '{}'::jsonb,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    version bigint DEFAULT 1 NOT NULL,
    team_id uuid NOT NULL,
    project_id uuid NOT NULL
);


--
-- Name: notification_deliveries; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notification_deliveries (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    notification_id uuid NOT NULL,
    channel character varying(32) NOT NULL,
    status character varying(32) NOT NULL,
    reason text,
    attempts integer DEFAULT 0 NOT NULL,
    delivered_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: notification_digest_queue; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notification_digest_queue (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    notification_id uuid NOT NULL,
    scheduled_for timestamp with time zone NOT NULL,
    sent_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: notifications; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.notifications (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    recipient_user_id uuid NOT NULL,
    team_id uuid,
    type character varying(64) NOT NULL,
    category character varying(16) NOT NULL,
    title text NOT NULL,
    body text,
    action_url text,
    entity_ref jsonb,
    dedupe_key character varying(128),
    read_at timestamp with time zone,
    dismissed_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: projects; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.projects (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    user_id uuid NOT NULL,
    name character varying(255) NOT NULL,
    slug character varying(100) NOT NULL,
    description text DEFAULT ''::text,
    git_url character varying(500) DEFAULT ''::character varying,
    homepage character varying(500) DEFAULT ''::character varying,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    version bigint DEFAULT 1,
    team_id uuid NOT NULL
);


--
-- Name: prompt_gallery_templates; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.prompt_gallery_templates (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    title character varying(255) NOT NULL,
    description text,
    content text NOT NULL,
    category character varying(100) NOT NULL,
    tags jsonb DEFAULT '[]'::jsonb,
    metadata jsonb DEFAULT '{}'::jsonb,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: prompt_references; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.prompt_references (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    prompt_id uuid NOT NULL,
    referenced_prompt_id uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: TABLE prompt_references; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.prompt_references IS 'Tracks which prompts reference other prompts via @reference syntax';


--
-- Name: COLUMN prompt_references.prompt_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.prompt_references.prompt_id IS 'The prompt that contains the reference';


--
-- Name: COLUMN prompt_references.referenced_prompt_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.prompt_references.referenced_prompt_id IS 'The prompt being referenced';


--
-- Name: prompt_share_access; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.prompt_share_access (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    share_id uuid NOT NULL,
    email character varying(255) NOT NULL,
    granted_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: prompt_shares; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.prompt_shares (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    prompt_id uuid NOT NULL,
    share_token character varying(64) NOT NULL,
    share_type character varying(20) NOT NULL,
    created_by uuid NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    expires_at timestamp with time zone,
    is_active boolean DEFAULT true,
    access_count integer DEFAULT 0,
    version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT prompt_shares_share_type_check CHECK (((share_type)::text = ANY ((ARRAY['public'::character varying, 'restricted'::character varying])::text[])))
);


--
-- Name: prompts; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.prompts (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    name character varying(50) NOT NULL,
    slug character varying(255) NOT NULL,
    body text NOT NULL,
    user_id uuid NOT NULL,
    status character varying(20) DEFAULT 'draft'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    description character varying(200) DEFAULT ''::character varying,
    mcp_expose boolean DEFAULT true NOT NULL,
    labels text[] DEFAULT '{}'::text[],
    version bigint DEFAULT 1 NOT NULL,
    project_id uuid NOT NULL,
    team_id uuid NOT NULL,
    CONSTRAINT prompts_status_check CHECK (((status)::text = ANY ((ARRAY['draft'::character varying, 'published'::character varying])::text[])))
);


--
-- Name: COLUMN prompts.mcp_expose; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.prompts.mcp_expose IS 'Whether the prompt is discoverable via MCP (Model Context Protocol) tools';


--
-- Name: COLUMN prompts.labels; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.prompts.labels IS 'Array of labels for categorizing and filtering prompts. Max 10 labels, each max 50 characters.';


--
-- Name: resource_access_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.resource_access_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    team_id uuid NOT NULL,
    user_id uuid,
    resource_type text NOT NULL,
    resource_id uuid NOT NULL,
    source text NOT NULL,
    api_key_id uuid,
    user_agent text,
    source_ip inet,
    created_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE resource_access_events; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.resource_access_events IS 'Records detail-access events for team resources (e.g. prompt/agent/artifact opens) for access analytics';


--
-- Name: COLUMN resource_access_events.team_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.resource_access_events.team_id IS 'Team that owns the accessed resource';


--
-- Name: COLUMN resource_access_events.user_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.resource_access_events.user_id IS 'User who accessed the resource; NULL when the user is later deleted';


--
-- Name: COLUMN resource_access_events.resource_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.resource_access_events.resource_type IS 'Type of resource accessed: prompt, agent, artifact, etc.';


--
-- Name: COLUMN resource_access_events.resource_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.resource_access_events.resource_id IS 'ID of the specific resource accessed';


--
-- Name: COLUMN resource_access_events.source; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.resource_access_events.source IS 'Origin of the access: web, cli, mcp, etc.';


--
-- Name: COLUMN resource_access_events.api_key_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.resource_access_events.api_key_id IS 'API key used for the access, when the access was authenticated via API key';


--
-- Name: COLUMN resource_access_events.user_agent; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.resource_access_events.user_agent IS 'User agent string of the client that performed the access';


--
-- Name: COLUMN resource_access_events.source_ip; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.resource_access_events.source_ip IS 'Source IP address of the client that performed the access';


--
-- Name: team_invitations; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.team_invitations (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    team_id uuid NOT NULL,
    inviter_id uuid NOT NULL,
    invitee_email character varying(255) NOT NULL,
    role character varying(20) DEFAULT 'member'::character varying NOT NULL,
    token character varying(64) NOT NULL,
    status character varying(20) DEFAULT 'pending'::character varying NOT NULL,
    expires_at timestamp with time zone NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT team_invitations_status_check CHECK (((status)::text = ANY ((ARRAY['pending'::character varying, 'accepted'::character varying, 'rejected'::character varying, 'revoked'::character varying])::text[])))
);


--
-- Name: team_members; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.team_members (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    team_id uuid NOT NULL,
    user_id uuid NOT NULL,
    role character varying(20) DEFAULT 'member'::character varying NOT NULL,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT team_members_role_check CHECK (((role)::text = ANY ((ARRAY['owner'::character varying, 'admin'::character varying, 'member'::character varying])::text[])))
);


--
-- Name: team_subscriptions; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.team_subscriptions (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    team_id uuid NOT NULL,
    stripe_subscription_id character varying(255) NOT NULL,
    stripe_customer_id character varying(255) NOT NULL,
    tier character varying(50) NOT NULL,
    seat_count integer NOT NULL,
    status character varying(50) NOT NULL,
    billing_interval character varying(20) NOT NULL,
    current_period_start timestamp with time zone NOT NULL,
    current_period_end timestamp with time zone NOT NULL,
    trial_end timestamp with time zone,
    canceled_at timestamp with time zone,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    CONSTRAINT team_subscriptions_billing_interval_valid CHECK (((billing_interval)::text = ANY ((ARRAY['month'::character varying, 'year'::character varying])::text[]))),
    CONSTRAINT team_subscriptions_seat_count_positive CHECK ((seat_count > 0)),
    CONSTRAINT team_subscriptions_status_valid CHECK (((status)::text = ANY ((ARRAY['incomplete'::character varying, 'incomplete_expired'::character varying, 'trialing'::character varying, 'active'::character varying, 'past_due'::character varying, 'canceled'::character varying, 'unpaid'::character varying])::text[]))),
    CONSTRAINT team_subscriptions_tier_valid CHECK (((tier)::text = ANY ((ARRAY['starter'::character varying, 'professional'::character varying, 'enterprise'::character varying])::text[])))
);


--
-- Name: TABLE team_subscriptions; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.team_subscriptions IS 'Stores team subscription data from Stripe for per-seat pricing';


--
-- Name: COLUMN team_subscriptions.tier; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.team_subscriptions.tier IS 'Pricing tier: starter, professional, enterprise';


--
-- Name: COLUMN team_subscriptions.seat_count; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.team_subscriptions.seat_count IS 'Number of paid seats (licensed members)';


--
-- Name: COLUMN team_subscriptions.status; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.team_subscriptions.status IS 'Stripe subscription status: trialing, active, past_due, canceled, unpaid';


--
-- Name: CONSTRAINT team_subscriptions_status_valid ON team_subscriptions; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON CONSTRAINT team_subscriptions_status_valid ON public.team_subscriptions IS 'Valid Stripe subscription statuses: incomplete, incomplete_expired, trialing, active, past_due, canceled, unpaid';


--
-- Name: teams; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.teams (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    owner_id uuid NOT NULL,
    name character varying(100) NOT NULL,
    slug character varying(50) NOT NULL,
    description text DEFAULT ''::text,
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    is_personal boolean DEFAULT false NOT NULL
);


--
-- Name: COLUMN teams.is_personal; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.teams.is_personal IS 'True for default personal workspace (cannot invite members), false for team workspaces';


--
-- Name: test_table; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.test_table (
    id integer NOT NULL,
    name character varying(255) NOT NULL,
    created_at timestamp without time zone DEFAULT CURRENT_TIMESTAMP
);


--
-- Name: test_table_id_seq; Type: SEQUENCE; Schema: public; Owner: -
--

CREATE SEQUENCE public.test_table_id_seq
    AS integer
    START WITH 1
    INCREMENT BY 1
    NO MINVALUE
    NO MAXVALUE
    CACHE 1;


--
-- Name: test_table_id_seq; Type: SEQUENCE OWNED BY; Schema: public; Owner: -
--

ALTER SEQUENCE public.test_table_id_seq OWNED BY public.test_table.id;


--
-- Name: types; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.types (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    team_id uuid,
    resource_type text NOT NULL,
    slug character varying(255) NOT NULL,
    name character varying(255) NOT NULL,
    is_system boolean DEFAULT false NOT NULL,
    created_by uuid,
    created_at timestamp with time zone DEFAULT now() NOT NULL,
    updated_at timestamp with time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE types; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.types IS 'Resource-type-agnostic, team-customizable category taxonomy keyed by (resource_type, slug); global system rows have team_id NULL';


--
-- Name: COLUMN types.team_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.types.team_id IS 'Owning team; NULL for global system defaults visible to all teams';


--
-- Name: COLUMN types.resource_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.types.resource_type IS 'Polymorphic resource type the type applies to: artifacts (future: prompt, memory, blueprint)';


--
-- Name: COLUMN types.is_system; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.types.is_system IS 'TRUE for built-in defaults that cannot be edited or deleted by users';


--
-- Name: COLUMN types.created_by; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.types.created_by IS 'User who created the custom type; NULL for system defaults and after the creator is deleted';


--
-- Name: user_preferences; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.user_preferences (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    preferences jsonb DEFAULT '{}'::jsonb NOT NULL,
    created_at timestamp with time zone DEFAULT now(),
    updated_at timestamp with time zone DEFAULT now(),
    version integer DEFAULT 1
);


--
-- Name: TABLE user_preferences; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.user_preferences IS 'Stores user preferences including email notification settings';


--
-- Name: COLUMN user_preferences.preferences; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.user_preferences.preferences IS 'JSONB containing preferences like email_notification settings';


--
-- Name: users; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.users (
    id uuid DEFAULT public.uuid_generate_v4() NOT NULL,
    google_id character varying(255),
    email character varying(255) NOT NULL,
    name character varying(255) NOT NULL,
    avatar_url character varying(2048),
    created_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    updated_at timestamp with time zone DEFAULT CURRENT_TIMESTAMP,
    stripe_customer_id character varying(255),
    subscription_status character varying(50) DEFAULT 'basic'::character varying,
    trial_ends_at timestamp with time zone,
    subscription_plan character varying(50) DEFAULT 'basic'::character varying,
    version bigint DEFAULT 1 NOT NULL,
    default_team_id uuid,
    onboarding_completed boolean DEFAULT false,
    onboarding_completed_at timestamp with time zone,
    subscription_canceled_at timestamp with time zone,
    idp_provider character varying(50),
    idp_subject character varying(255)
);


--
-- Name: COLUMN users.subscription_canceled_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.users.subscription_canceled_at IS 'Timestamp when subscription cancellation was scheduled (Stripe cancel_at_period_end). NULL means subscription will auto-renew.';


--
-- Name: webhook_events; Type: TABLE; Schema: public; Owner: -
--

CREATE TABLE public.webhook_events (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    event_id character varying(255) NOT NULL,
    event_type character varying(100) NOT NULL,
    processed_at timestamp without time zone DEFAULT now() NOT NULL,
    team_id character varying(255),
    created_at timestamp without time zone DEFAULT now() NOT NULL
);


--
-- Name: TABLE webhook_events; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON TABLE public.webhook_events IS 'Tracks processed Stripe webhook events to prevent duplicate processing (idempotency)';


--
-- Name: COLUMN webhook_events.event_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.webhook_events.event_id IS 'Stripe event ID (evt_xxx) - unique identifier from Stripe';


--
-- Name: COLUMN webhook_events.event_type; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.webhook_events.event_type IS 'Stripe event type (e.g. checkout.session.completed)';


--
-- Name: COLUMN webhook_events.processed_at; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.webhook_events.processed_at IS 'When the webhook event was successfully processed';


--
-- Name: COLUMN webhook_events.team_id; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON COLUMN public.webhook_events.team_id IS 'Associated team ID if applicable (nullable)';


--
-- Name: claude_code_hooks_payload id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.claude_code_hooks_payload ALTER COLUMN id SET DEFAULT nextval('public.claude_code_hooks_payload_id_seq'::regclass);


--
-- Name: cursor_ide_hooks_payload id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cursor_ide_hooks_payload ALTER COLUMN id SET DEFAULT nextval('public.cursor_ide_hooks_payload_id_seq'::regclass);


--
-- Name: test_table id; Type: DEFAULT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.test_table ALTER COLUMN id SET DEFAULT nextval('public.test_table_id_seq'::regclass);


--
-- Data for Name: activities; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: agent_execution_events; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: agent_executions; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: agents; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: api_key_integration_permissions; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: api_key_integrations_catalog; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.api_key_integrations_catalog (id, integration_code, integration_name, description, is_active, created_at, updated_at) VALUES ('a79a0d56-146d-486d-8a35-8375327bd6a4', 'ai_tools', 'AI Tools Integration', 'Access for Claude Code, Cursor IDE, and other AI-powered development tools', true, '2026-06-24 17:01:40.118793+00', '2026-06-24 17:01:40.118793+00');
INSERT INTO public.api_key_integrations_catalog (id, integration_code, integration_name, description, is_active, created_at, updated_at) VALUES ('920db34f-d2a8-4c68-bcff-215490228fb8', 'cli', 'VibeXP CLI', 'Access for VibeXP command-line interface', true, '2026-06-24 17:01:40.118793+00', '2026-06-24 17:01:40.118793+00');
INSERT INTO public.api_key_integrations_catalog (id, integration_code, integration_name, description, is_active, created_at, updated_at) VALUES ('b781ab05-10fe-461e-8925-1ce16e8a247f', 'mcp_server', 'MCP Server', 'Access for Model Context Protocol server endpoints', true, '2026-06-24 17:01:40.118793+00', '2026-06-24 17:01:40.118793+00');
INSERT INTO public.api_key_integrations_catalog (id, integration_code, integration_name, description, is_active, created_at, updated_at) VALUES ('32cc54ba-abda-4ebe-82fe-932fdf5b613b', 'marketplace', 'Claude Plugin Marketplace', 'Access for Claude Plugin Marketplace APIs', true, '2026-06-24 17:01:40.118793+00', '2026-06-24 17:01:40.118793+00');


--
-- Data for Name: api_keys; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: artifacts; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: attachments; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: blueprints; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: claude_code_hooks_payload; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: content_versions; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: cursor_ide_hooks_payload; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: device_tokens; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: embedding_providers; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: embeddings; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: feed_item_replies; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: feed_items; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: feeds; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: financial_kpi_snapshots; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: github_installation_repositories; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: github_installations; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: memories; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: notification_deliveries; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: notification_digest_queue; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: notifications; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: projects; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: prompt_gallery_templates; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.prompt_gallery_templates (id, title, description, content, category, tags, metadata, created_at, updated_at) VALUES ('120d5d14-00f2-4fea-8038-5ffdbbea256a', 'Code Review Request', 'Request a thorough code review with focus on best practices, security, and performance', 'Please review the following code for:
- Code quality and best practices
- Security vulnerabilities
- Performance optimizations
- Error handling
- Code documentation

Code:
{{code}}

Context:
{{context}}', 'Engineering', '["code-review", "quality", "security"]', '{"use_case": "development", "difficulty": "beginner"}', '2026-06-24 17:01:39.935454', '2026-06-24 17:01:39.935454');
INSERT INTO public.prompt_gallery_templates (id, title, description, content, category, tags, metadata, created_at, updated_at) VALUES ('90fe23bf-0260-4e87-9b33-5ba55602ad65', 'Product Requirements Document', 'Generate a comprehensive product requirements document for a new feature', 'Create a Product Requirements Document (PRD) for the following feature:

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
8. Timeline and Milestones', 'Product Management', '["prd", "requirements", "planning"]', '{"use_case": "planning", "difficulty": "intermediate"}', '2026-06-24 17:01:39.935454', '2026-06-24 17:01:39.935454');
INSERT INTO public.prompt_gallery_templates (id, title, description, content, category, tags, metadata, created_at, updated_at) VALUES ('4347d51d-c367-4107-8eba-8410e4240247', 'Social Media Campaign', 'Create engaging social media content for marketing campaigns', 'Create a social media campaign for:

Product/Service: {{product}}
Target Audience: {{audience}}
Campaign Goal: {{goal}}
Platform: {{platform}}

Generate:
- 5 engaging post variations
- Relevant hashtags
- Call-to-action suggestions
- Optimal posting times
- Engagement strategies', 'Marketing', '["social-media", "content", "campaign"]', '{"use_case": "marketing", "difficulty": "beginner"}', '2026-06-24 17:01:39.935454', '2026-06-24 17:01:39.935454');
INSERT INTO public.prompt_gallery_templates (id, title, description, content, category, tags, metadata, created_at, updated_at) VALUES ('7cd2aef1-78c9-4992-9923-973f1e15592d', 'SQL Query Optimization', 'Analyze and optimize SQL queries for better performance', 'Analyze the following SQL query and provide optimization suggestions:

Query:
{{query}}

Database: {{database_type}}
Table Schema: {{schema}}

Please provide:
1. Performance analysis
2. Index recommendations
3. Query rewrite suggestions
4. Explain plan interpretation
5. Best practices recommendations', 'Data Analysis', '["sql", "optimization", "performance"]', '{"use_case": "data-engineering", "difficulty": "advanced"}', '2026-06-24 17:01:39.935454', '2026-06-24 17:01:39.935454');
INSERT INTO public.prompt_gallery_templates (id, title, description, content, category, tags, metadata, created_at, updated_at) VALUES ('2212816e-a8a1-4e28-ab67-720f1627d723', 'Customer Support Response', 'Generate professional and empathetic customer support responses', 'Customer Issue:
{{customer_issue}}

Customer Tone: {{customer_tone}}
Priority: {{priority}}

Generate a professional response that:
- Acknowledges the issue
- Shows empathy
- Provides a solution or next steps
- Maintains brand voice
- Includes escalation path if needed', 'Customer Support', '["support", "customer-service", "communication"]', '{"use_case": "customer-support", "difficulty": "beginner"}', '2026-06-24 17:01:39.935454', '2026-06-24 17:01:39.935454');


--
-- Data for Name: prompt_references; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: prompt_share_access; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: prompt_shares; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: prompts; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: resource_access_events; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: team_invitations; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: team_members; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: team_subscriptions; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: teams; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: test_table; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.test_table (id, name, created_at) VALUES (1, 'Test record from migration', '2026-06-24 17:01:39.447509');


--
-- Data for Name: types; Type: TABLE DATA; Schema: public; Owner: -
--

INSERT INTO public.types (id, team_id, resource_type, slug, name, is_system, created_by, created_at, updated_at) VALUES ('41446c74-ec75-4b95-b027-a8860c3a4d67', NULL, 'artifacts', 'general', 'General', true, NULL, '2026-06-24 17:01:40.742154+00', '2026-06-24 17:01:40.742154+00');
INSERT INTO public.types (id, team_id, resource_type, slug, name, is_system, created_by, created_at, updated_at) VALUES ('e54885d4-c666-48fc-b6fc-e64f757821f9', NULL, 'artifacts', 'work-reports', 'Work reports', true, NULL, '2026-06-24 17:01:40.742154+00', '2026-06-24 17:01:40.742154+00');
INSERT INTO public.types (id, team_id, resource_type, slug, name, is_system, created_by, created_at, updated_at) VALUES ('09a5f3e0-b3bd-4354-8948-c3b6b99ac94c', NULL, 'artifacts', 'static-contexts', 'Static contexts', true, NULL, '2026-06-24 17:01:40.742154+00', '2026-06-24 17:01:40.742154+00');


--
-- Data for Name: user_preferences; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: users; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Data for Name: webhook_events; Type: TABLE DATA; Schema: public; Owner: -
--



--
-- Name: claude_code_hooks_payload_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.claude_code_hooks_payload_id_seq', 1, false);


--
-- Name: cursor_ide_hooks_payload_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.cursor_ide_hooks_payload_id_seq', 1, false);


--
-- Name: test_table_id_seq; Type: SEQUENCE SET; Schema: public; Owner: -
--

SELECT pg_catalog.setval('public.test_table_id_seq', 1, true);


--
-- Name: activities activities_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activities
    ADD CONSTRAINT activities_pkey PRIMARY KEY (id);


--
-- Name: agent_execution_events agent_execution_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_execution_events
    ADD CONSTRAINT agent_execution_events_pkey PRIMARY KEY (id);


--
-- Name: agent_executions agent_executions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_executions
    ADD CONSTRAINT agent_executions_pkey PRIMARY KEY (id);


--
-- Name: agents agents_name_team_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_name_team_id_key UNIQUE (name, team_id);


--
-- Name: agents agents_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_pkey PRIMARY KEY (id);


--
-- Name: api_key_integration_permissions api_key_integration_permissions_api_key_id_integration_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_key_integration_permissions
    ADD CONSTRAINT api_key_integration_permissions_api_key_id_integration_code_key UNIQUE (api_key_id, integration_code);


--
-- Name: api_key_integration_permissions api_key_integration_permissions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_key_integration_permissions
    ADD CONSTRAINT api_key_integration_permissions_pkey PRIMARY KEY (id);


--
-- Name: api_key_integrations_catalog api_key_integrations_catalog_integration_code_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_key_integrations_catalog
    ADD CONSTRAINT api_key_integrations_catalog_integration_code_key UNIQUE (integration_code);


--
-- Name: api_key_integrations_catalog api_key_integrations_catalog_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_key_integrations_catalog
    ADD CONSTRAINT api_key_integrations_catalog_pkey PRIMARY KEY (id);


--
-- Name: api_keys api_keys_key_hash_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_key_hash_key UNIQUE (key_hash);


--
-- Name: api_keys api_keys_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_pkey PRIMARY KEY (id);


--
-- Name: artifacts artifacts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifacts
    ADD CONSTRAINT artifacts_pkey PRIMARY KEY (id);


--
-- Name: artifacts artifacts_project_id_slug_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifacts
    ADD CONSTRAINT artifacts_project_id_slug_unique UNIQUE (project_id, slug);


--
-- Name: artifacts artifacts_slug_team_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifacts
    ADD CONSTRAINT artifacts_slug_team_id_key UNIQUE (slug, team_id);


--
-- Name: attachments attachments_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attachments
    ADD CONSTRAINT attachments_pkey PRIMARY KEY (id);


--
-- Name: blueprints blueprints_project_id_slug_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blueprints
    ADD CONSTRAINT blueprints_project_id_slug_unique UNIQUE (project_id, slug);


--
-- Name: blueprints blueprints_slug_team_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blueprints
    ADD CONSTRAINT blueprints_slug_team_id_key UNIQUE (slug, team_id);


--
-- Name: claude_code_hooks_payload claude_code_hooks_payload_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.claude_code_hooks_payload
    ADD CONSTRAINT claude_code_hooks_payload_pkey PRIMARY KEY (id);


--
-- Name: content_versions content_versions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_versions
    ADD CONSTRAINT content_versions_pkey PRIMARY KEY (id);


--
-- Name: content_versions content_versions_resource_type_resource_id_version_number_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_versions
    ADD CONSTRAINT content_versions_resource_type_resource_id_version_number_key UNIQUE (resource_type, resource_id, version_number);


--
-- Name: cursor_ide_hooks_payload cursor_ide_hooks_payload_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cursor_ide_hooks_payload
    ADD CONSTRAINT cursor_ide_hooks_payload_pkey PRIMARY KEY (id);


--
-- Name: device_tokens device_tokens_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_tokens
    ADD CONSTRAINT device_tokens_pkey PRIMARY KEY (id);


--
-- Name: embedding_providers embedding_providers_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.embedding_providers
    ADD CONSTRAINT embedding_providers_pkey PRIMARY KEY (id);


--
-- Name: embeddings embeddings_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.embeddings
    ADD CONSTRAINT embeddings_pkey PRIMARY KEY (id);


--
-- Name: feed_item_replies feed_item_replies_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feed_item_replies
    ADD CONSTRAINT feed_item_replies_pkey PRIMARY KEY (id);


--
-- Name: feed_items feed_items_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feed_items
    ADD CONSTRAINT feed_items_pkey PRIMARY KEY (id);


--
-- Name: feeds feeds_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feeds
    ADD CONSTRAINT feeds_pkey PRIMARY KEY (id);


--
-- Name: financial_kpi_snapshots financial_kpi_snapshots_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_kpi_snapshots
    ADD CONSTRAINT financial_kpi_snapshots_pkey PRIMARY KEY (id);


--
-- Name: financial_kpi_snapshots financial_kpi_snapshots_snapshot_date_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.financial_kpi_snapshots
    ADD CONSTRAINT financial_kpi_snapshots_snapshot_date_key UNIQUE (snapshot_date);


--
-- Name: github_installation_repositories github_installation_repositories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.github_installation_repositories
    ADD CONSTRAINT github_installation_repositories_pkey PRIMARY KEY (id);


--
-- Name: github_installations github_installations_installation_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.github_installations
    ADD CONSTRAINT github_installations_installation_id_key UNIQUE (installation_id);


--
-- Name: github_installations github_installations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.github_installations
    ADD CONSTRAINT github_installations_pkey PRIMARY KEY (id);


--
-- Name: memories memories_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memories
    ADD CONSTRAINT memories_pkey PRIMARY KEY (id);


--
-- Name: notification_deliveries notification_deliveries_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notification_deliveries
    ADD CONSTRAINT notification_deliveries_pkey PRIMARY KEY (id);


--
-- Name: notification_digest_queue notification_digest_queue_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notification_digest_queue
    ADD CONSTRAINT notification_digest_queue_pkey PRIMARY KEY (id);


--
-- Name: notifications notifications_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_pkey PRIMARY KEY (id);


--
-- Name: projects projects_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_pkey PRIMARY KEY (id);


--
-- Name: projects projects_slug_team_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_slug_team_id_key UNIQUE (slug, team_id);


--
-- Name: prompt_gallery_templates prompt_gallery_templates_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_gallery_templates
    ADD CONSTRAINT prompt_gallery_templates_pkey PRIMARY KEY (id);


--
-- Name: prompt_references prompt_references_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_references
    ADD CONSTRAINT prompt_references_pkey PRIMARY KEY (id);


--
-- Name: prompt_references prompt_references_prompt_id_referenced_prompt_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_references
    ADD CONSTRAINT prompt_references_prompt_id_referenced_prompt_id_key UNIQUE (prompt_id, referenced_prompt_id);


--
-- Name: prompt_share_access prompt_share_access_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_share_access
    ADD CONSTRAINT prompt_share_access_pkey PRIMARY KEY (id);


--
-- Name: prompt_share_access prompt_share_access_share_id_email_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_share_access
    ADD CONSTRAINT prompt_share_access_share_id_email_key UNIQUE (share_id, email);


--
-- Name: prompt_shares prompt_shares_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_shares
    ADD CONSTRAINT prompt_shares_pkey PRIMARY KEY (id);


--
-- Name: prompt_shares prompt_shares_prompt_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_shares
    ADD CONSTRAINT prompt_shares_prompt_id_key UNIQUE (prompt_id);


--
-- Name: prompt_shares prompt_shares_share_token_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_shares
    ADD CONSTRAINT prompt_shares_share_token_key UNIQUE (share_token);


--
-- Name: prompts prompts_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompts
    ADD CONSTRAINT prompts_pkey PRIMARY KEY (id);


--
-- Name: prompts prompts_slug_team_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompts
    ADD CONSTRAINT prompts_slug_team_id_key UNIQUE (slug, team_id);


--
-- Name: resource_access_events resource_access_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.resource_access_events
    ADD CONSTRAINT resource_access_events_pkey PRIMARY KEY (id);


--
-- Name: blueprints spec_library_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blueprints
    ADD CONSTRAINT spec_library_pkey PRIMARY KEY (id);


--
-- Name: team_invitations team_invitations_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_invitations
    ADD CONSTRAINT team_invitations_pkey PRIMARY KEY (id);


--
-- Name: team_invitations team_invitations_token_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_invitations
    ADD CONSTRAINT team_invitations_token_key UNIQUE (token);


--
-- Name: team_members team_members_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_members
    ADD CONSTRAINT team_members_pkey PRIMARY KEY (id);


--
-- Name: team_members team_members_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_members
    ADD CONSTRAINT team_members_unique UNIQUE (team_id, user_id);


--
-- Name: team_subscriptions team_subscriptions_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_subscriptions
    ADD CONSTRAINT team_subscriptions_pkey PRIMARY KEY (id);


--
-- Name: team_subscriptions team_subscriptions_stripe_subscription_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_subscriptions
    ADD CONSTRAINT team_subscriptions_stripe_subscription_id_key UNIQUE (stripe_subscription_id);


--
-- Name: teams teams_owner_slug_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT teams_owner_slug_unique UNIQUE (owner_id, slug);


--
-- Name: teams teams_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT teams_pkey PRIMARY KEY (id);


--
-- Name: test_table test_table_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.test_table
    ADD CONSTRAINT test_table_pkey PRIMARY KEY (id);


--
-- Name: types types_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.types
    ADD CONSTRAINT types_pkey PRIMARY KEY (id);


--
-- Name: feeds uniq_feeds_team_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feeds
    ADD CONSTRAINT uniq_feeds_team_name UNIQUE (team_id, name);


--
-- Name: agent_execution_events unique_execution_sequence; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_execution_events
    ADD CONSTRAINT unique_execution_sequence UNIQUE (execution_id, sequence_number);


--
-- Name: github_installation_repositories unique_installation_repository; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.github_installation_repositories
    ADD CONSTRAINT unique_installation_repository UNIQUE (installation_id, repository_id);


--
-- Name: github_installations unique_team_installation; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.github_installations
    ADD CONSTRAINT unique_team_installation UNIQUE (team_id, installation_id);


--
-- Name: embedding_providers unique_user_provider_name; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.embedding_providers
    ADD CONSTRAINT unique_user_provider_name UNIQUE (user_id, name);


--
-- Name: user_preferences user_preferences_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_preferences
    ADD CONSTRAINT user_preferences_pkey PRIMARY KEY (id);


--
-- Name: user_preferences user_preferences_user_id_unique; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_preferences
    ADD CONSTRAINT user_preferences_user_id_unique UNIQUE (user_id);


--
-- Name: users users_email_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_email_key UNIQUE (email);


--
-- Name: users users_google_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_google_id_key UNIQUE (google_id);


--
-- Name: users users_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_pkey PRIMARY KEY (id);


--
-- Name: webhook_events webhook_events_event_id_key; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_events
    ADD CONSTRAINT webhook_events_event_id_key UNIQUE (event_id);


--
-- Name: webhook_events webhook_events_pkey; Type: CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.webhook_events
    ADD CONSTRAINT webhook_events_pkey PRIMARY KEY (id);


--
-- Name: device_tokens_token_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX device_tokens_token_idx ON public.device_tokens USING btree (token);


--
-- Name: device_tokens_user_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX device_tokens_user_id_idx ON public.device_tokens USING btree (user_id);


--
-- Name: idx_activities_composite; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activities_composite ON public.activities USING btree (user_id, activity_type, created_at DESC);


--
-- Name: idx_activities_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activities_created_at ON public.activities USING btree (created_at DESC);


--
-- Name: idx_activities_entity_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activities_entity_id ON public.activities USING btree (entity_id);


--
-- Name: idx_activities_entity_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activities_entity_type ON public.activities USING btree (entity_type);


--
-- Name: idx_activities_session_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activities_session_id ON public.activities USING btree (session_id);


--
-- Name: idx_activities_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activities_type ON public.activities USING btree (activity_type);


--
-- Name: idx_activities_type_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activities_type_created ON public.activities USING btree (activity_type, created_at DESC);


--
-- Name: idx_activities_user_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activities_user_created ON public.activities USING btree (user_id, created_at DESC);


--
-- Name: idx_activities_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_activities_user_id ON public.activities USING btree (user_id);


--
-- Name: idx_agent_execution_events_event_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_execution_events_event_type ON public.agent_execution_events USING btree (event_type);


--
-- Name: idx_agent_execution_events_execution_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_execution_events_execution_id ON public.agent_execution_events USING btree (execution_id);


--
-- Name: idx_agent_execution_events_sequence; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_execution_events_sequence ON public.agent_execution_events USING btree (execution_id, sequence_number);


--
-- Name: idx_agent_executions_agent_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_executions_agent_id ON public.agent_executions USING btree (agent_id);


--
-- Name: idx_agent_executions_context_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_executions_context_id ON public.agent_executions USING btree (context_id) WHERE (context_id IS NOT NULL);


--
-- Name: idx_agent_executions_conversation; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_executions_conversation ON public.agent_executions USING btree (user_id, agent_id, conversation_id) WHERE (conversation_id IS NOT NULL);


--
-- Name: idx_agent_executions_current_state; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_executions_current_state ON public.agent_executions USING btree (current_state) WHERE (current_state IS NOT NULL);


--
-- Name: idx_agent_executions_started_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_executions_started_at ON public.agent_executions USING btree (started_at);


--
-- Name: idx_agent_executions_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_executions_status ON public.agent_executions USING btree (status);


--
-- Name: idx_agent_executions_task_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_executions_task_id ON public.agent_executions USING btree (task_id) WHERE (task_id IS NOT NULL);


--
-- Name: idx_agent_executions_user_agent; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_executions_user_agent ON public.agent_executions USING btree (user_id, agent_id);


--
-- Name: idx_agent_executions_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_executions_user_id ON public.agent_executions USING btree (user_id);


--
-- Name: idx_agent_executions_user_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agent_executions_user_status ON public.agent_executions USING btree (user_id, status);


--
-- Name: idx_agents_card_url; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agents_card_url ON public.agents USING btree (card_url);


--
-- Name: idx_agents_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agents_created_at ON public.agents USING btree (created_at);


--
-- Name: idx_agents_credentials; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agents_credentials ON public.agents USING gin (credentials) WHERE (credentials IS NOT NULL);


--
-- Name: idx_agents_last_run; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agents_last_run ON public.agents USING btree (last_run);


--
-- Name: idx_agents_last_synced_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agents_last_synced_at ON public.agents USING btree (last_synced_at);


--
-- Name: idx_agents_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agents_status ON public.agents USING btree (status);


--
-- Name: idx_agents_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agents_team_id ON public.agents USING btree (team_id);


--
-- Name: idx_agents_team_id_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agents_team_id_user_id ON public.agents USING btree (team_id, user_id) WHERE (team_id IS NOT NULL);


--
-- Name: idx_agents_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agents_user_id ON public.agents USING btree (user_id);


--
-- Name: idx_agents_user_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_agents_user_status ON public.agents USING btree (user_id, status);


--
-- Name: idx_api_key_permissions_integration; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_key_permissions_integration ON public.api_key_integration_permissions USING btree (integration_code);


--
-- Name: idx_api_key_permissions_key_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_key_permissions_key_id ON public.api_key_integration_permissions USING btree (api_key_id);


--
-- Name: idx_api_keys_key_hash; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_keys_key_hash ON public.api_keys USING btree (key_hash);


--
-- Name: idx_api_keys_key_prefix; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_keys_key_prefix ON public.api_keys USING btree (key_prefix);


--
-- Name: idx_api_keys_usage_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_keys_usage_type ON public.api_keys USING btree (usage_type);


--
-- Name: idx_api_keys_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_api_keys_user_id ON public.api_keys USING btree (user_id);


--
-- Name: idx_artifacts_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_artifacts_created_at ON public.artifacts USING btree (created_at DESC);


--
-- Name: idx_artifacts_metadata; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_artifacts_metadata ON public.artifacts USING gin (metadata);


--
-- Name: idx_artifacts_project_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_artifacts_project_id ON public.artifacts USING btree (project_id);


--
-- Name: idx_artifacts_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_artifacts_slug ON public.artifacts USING btree (slug);


--
-- Name: idx_artifacts_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_artifacts_status ON public.artifacts USING btree (status);


--
-- Name: idx_artifacts_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_artifacts_team_id ON public.artifacts USING btree (team_id);


--
-- Name: idx_artifacts_team_id_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_artifacts_team_id_user_id ON public.artifacts USING btree (team_id, user_id) WHERE (team_id IS NOT NULL);


--
-- Name: idx_artifacts_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_artifacts_type ON public.artifacts USING btree (type);


--
-- Name: idx_artifacts_updated_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_artifacts_updated_at ON public.artifacts USING btree (updated_at DESC);


--
-- Name: idx_artifacts_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_artifacts_user_id ON public.artifacts USING btree (user_id);


--
-- Name: idx_attachments_owner; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_attachments_owner ON public.attachments USING btree (team_id, owner_type, owner_id);


--
-- Name: idx_blueprints_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_blueprints_created_at ON public.blueprints USING btree (created_at DESC);


--
-- Name: idx_blueprints_metadata; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_blueprints_metadata ON public.blueprints USING gin (metadata);


--
-- Name: idx_blueprints_project_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_blueprints_project_id ON public.blueprints USING btree (project_id);


--
-- Name: idx_blueprints_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_blueprints_slug ON public.blueprints USING btree (slug);


--
-- Name: idx_blueprints_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_blueprints_status ON public.blueprints USING btree (status);


--
-- Name: idx_blueprints_subtype; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_blueprints_subtype ON public.blueprints USING btree (subtype);


--
-- Name: idx_blueprints_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_blueprints_team_id ON public.blueprints USING btree (team_id);


--
-- Name: idx_blueprints_team_id_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_blueprints_team_id_user_id ON public.blueprints USING btree (team_id, user_id) WHERE (team_id IS NOT NULL);


--
-- Name: idx_blueprints_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_blueprints_type ON public.blueprints USING btree (type);


--
-- Name: idx_blueprints_updated_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_blueprints_updated_at ON public.blueprints USING btree (updated_at DESC);


--
-- Name: idx_blueprints_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_blueprints_user_id ON public.blueprints USING btree (user_id);


--
-- Name: idx_blueprints_user_type_subtype; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_blueprints_user_type_subtype ON public.blueprints USING btree (user_id, type, subtype);


--
-- Name: idx_claude_code_hooks_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_claude_code_hooks_created_at ON public.claude_code_hooks_payload USING btree (created_at DESC);


--
-- Name: idx_claude_code_hooks_event_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_claude_code_hooks_event_name ON public.claude_code_hooks_payload USING btree (hook_event_name);


--
-- Name: idx_claude_code_hooks_payload_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_claude_code_hooks_payload_team_id ON public.claude_code_hooks_payload USING btree (team_id);


--
-- Name: idx_claude_code_hooks_session_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_claude_code_hooks_session_id ON public.claude_code_hooks_payload USING btree (session_id);


--
-- Name: idx_claude_code_hooks_tool_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_claude_code_hooks_tool_name ON public.claude_code_hooks_payload USING btree (tool_name);


--
-- Name: idx_claude_code_hooks_user_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_claude_code_hooks_user_created_at ON public.claude_code_hooks_payload USING btree (user_id, created_at DESC);


--
-- Name: idx_claude_code_hooks_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_claude_code_hooks_user_id ON public.claude_code_hooks_payload USING btree (user_id);


--
-- Name: idx_claude_code_hooks_user_session; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_claude_code_hooks_user_session ON public.claude_code_hooks_payload USING btree (user_id, session_id);


--
-- Name: idx_content_versions_resource; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_content_versions_resource ON public.content_versions USING btree (resource_type, resource_id, version_number DESC);


--
-- Name: idx_cursor_ide_hooks_conversation_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cursor_ide_hooks_conversation_id ON public.cursor_ide_hooks_payload USING btree (conversation_id);


--
-- Name: idx_cursor_ide_hooks_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cursor_ide_hooks_created_at ON public.cursor_ide_hooks_payload USING btree (created_at DESC);


--
-- Name: idx_cursor_ide_hooks_event_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cursor_ide_hooks_event_name ON public.cursor_ide_hooks_payload USING btree (hook_event_name);


--
-- Name: idx_cursor_ide_hooks_generation_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cursor_ide_hooks_generation_id ON public.cursor_ide_hooks_payload USING btree (generation_id);


--
-- Name: idx_cursor_ide_hooks_payload_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cursor_ide_hooks_payload_team_id ON public.cursor_ide_hooks_payload USING btree (team_id);


--
-- Name: idx_cursor_ide_hooks_session_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cursor_ide_hooks_session_id ON public.cursor_ide_hooks_payload USING btree (session_id);


--
-- Name: idx_cursor_ide_hooks_tool_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cursor_ide_hooks_tool_name ON public.cursor_ide_hooks_payload USING btree (tool_name);


--
-- Name: idx_cursor_ide_hooks_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_cursor_ide_hooks_user_id ON public.cursor_ide_hooks_payload USING btree (user_id);


--
-- Name: idx_embedding_providers_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_embedding_providers_type ON public.embedding_providers USING btree (provider_type);


--
-- Name: idx_embedding_providers_user_default; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_embedding_providers_user_default ON public.embedding_providers USING btree (user_id) WHERE (is_default = true);


--
-- Name: idx_embedding_providers_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_embedding_providers_user_id ON public.embedding_providers USING btree (user_id);


--
-- Name: idx_embeddings_content; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_embeddings_content ON public.embeddings USING gin (to_tsvector('english'::regconfig, content));


--
-- Name: idx_embeddings_entity_type_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_embeddings_entity_type_id ON public.embeddings USING btree (entity_type, entity_id);


--
-- Name: idx_embeddings_team_id_entity; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_embeddings_team_id_entity ON public.embeddings USING btree (team_id, entity_type);


--
-- Name: idx_embeddings_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_embeddings_user_id ON public.embeddings USING btree (user_id);


--
-- Name: idx_embeddings_vector_cosine; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_embeddings_vector_cosine ON public.embeddings USING hnsw (vector_embeddings public.vector_cosine_ops);


--
-- Name: idx_feed_item_replies_posted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_feed_item_replies_posted_at ON public.feed_item_replies USING btree (feed_item_id, posted_at DESC);


--
-- Name: idx_feed_items_feed_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_feed_items_feed_id ON public.feed_items USING btree (feed_id);


--
-- Name: idx_feed_items_project_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_feed_items_project_id ON public.feed_items USING btree (project_id);


--
-- Name: idx_feed_items_team_archived_posted; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_feed_items_team_archived_posted ON public.feed_items USING btree (team_id, archived_at, posted_at DESC);


--
-- Name: idx_feed_items_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_feed_items_team_id ON public.feed_items USING btree (team_id);


--
-- Name: idx_feed_items_team_posted_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_feed_items_team_posted_at ON public.feed_items USING btree (team_id, posted_at DESC);


--
-- Name: idx_feeds_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_feeds_team_id ON public.feeds USING btree (team_id);


--
-- Name: idx_financial_kpi_snapshots_snapshot_date; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_financial_kpi_snapshots_snapshot_date ON public.financial_kpi_snapshots USING btree (snapshot_date DESC);


--
-- Name: idx_github_installation_repositories_installation_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_github_installation_repositories_installation_id ON public.github_installation_repositories USING btree (installation_id);


--
-- Name: idx_github_installations_installation_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_github_installations_installation_id ON public.github_installations USING btree (installation_id);


--
-- Name: idx_github_installations_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_github_installations_team_id ON public.github_installations USING btree (team_id);


--
-- Name: idx_integrations_catalog_code; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_integrations_catalog_code ON public.api_key_integrations_catalog USING btree (integration_code);


--
-- Name: idx_memories_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_memories_created_at ON public.memories USING btree (created_at DESC);


--
-- Name: idx_memories_metadata_gin; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_memories_metadata_gin ON public.memories USING gin (metadata);


--
-- Name: idx_memories_project_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_memories_project_id ON public.memories USING btree (project_id);


--
-- Name: idx_memories_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_memories_team_id ON public.memories USING btree (team_id);


--
-- Name: idx_memories_team_id_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_memories_team_id_user_id ON public.memories USING btree (team_id, user_id) WHERE (team_id IS NOT NULL);


--
-- Name: idx_memories_user_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_memories_user_created ON public.memories USING btree (user_id, created_at DESC);


--
-- Name: idx_memories_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_memories_user_id ON public.memories USING btree (user_id);


--
-- Name: idx_notifications_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_notifications_created_at ON public.notifications USING btree (created_at);


--
-- Name: idx_notifications_dedupe; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_notifications_dedupe ON public.notifications USING btree (recipient_user_id, dedupe_key) WHERE (dedupe_key IS NOT NULL);


--
-- Name: idx_notifications_user_unread; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_notifications_user_unread ON public.notifications USING btree (recipient_user_id, created_at DESC) WHERE ((read_at IS NULL) AND (dismissed_at IS NULL));


--
-- Name: idx_projects_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_projects_created_at ON public.projects USING btree (created_at DESC);


--
-- Name: idx_projects_name; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_projects_name ON public.projects USING btree (name);


--
-- Name: idx_projects_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_projects_slug ON public.projects USING btree (slug);


--
-- Name: idx_projects_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_projects_team_id ON public.projects USING btree (team_id);


--
-- Name: idx_projects_team_id_git_url_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_projects_team_id_git_url_unique ON public.projects USING btree (team_id, git_url) WHERE ((git_url IS NOT NULL) AND ((git_url)::text <> ''::text));


--
-- Name: INDEX idx_projects_team_id_git_url_unique; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON INDEX public.idx_projects_team_id_git_url_unique IS 'Ensures a GitHub repository can only be imported once per team, preventing duplicate projects. Also serves as index for GetProjectByGitURL queries.';


--
-- Name: idx_projects_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_projects_user_id ON public.projects USING btree (user_id);


--
-- Name: idx_prompt_gallery_templates_category; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prompt_gallery_templates_category ON public.prompt_gallery_templates USING btree (category);


--
-- Name: idx_prompt_gallery_templates_tags; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prompt_gallery_templates_tags ON public.prompt_gallery_templates USING gin (tags);


--
-- Name: idx_prompt_references_prompt_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prompt_references_prompt_id ON public.prompt_references USING btree (prompt_id);


--
-- Name: idx_prompt_references_referenced_prompt_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prompt_references_referenced_prompt_id ON public.prompt_references USING btree (referenced_prompt_id);


--
-- Name: idx_prompt_share_access_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prompt_share_access_email ON public.prompt_share_access USING btree (email);


--
-- Name: idx_prompt_share_access_share_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prompt_share_access_share_id ON public.prompt_share_access USING btree (share_id);


--
-- Name: idx_prompt_shares_created_by; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prompt_shares_created_by ON public.prompt_shares USING btree (created_by);


--
-- Name: idx_prompt_shares_prompt_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prompt_shares_prompt_id ON public.prompt_shares USING btree (prompt_id);


--
-- Name: idx_prompt_shares_token; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prompt_shares_token ON public.prompt_shares USING btree (share_token);


--
-- Name: idx_prompts_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prompts_created_at ON public.prompts USING btree (created_at DESC);


--
-- Name: idx_prompts_labels; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prompts_labels ON public.prompts USING gin (labels);


--
-- Name: idx_prompts_mcp_expose; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prompts_mcp_expose ON public.prompts USING btree (mcp_expose);


--
-- Name: idx_prompts_project_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prompts_project_id ON public.prompts USING btree (project_id);


--
-- Name: idx_prompts_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prompts_slug ON public.prompts USING btree (slug);


--
-- Name: idx_prompts_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prompts_status ON public.prompts USING btree (status);


--
-- Name: idx_prompts_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prompts_team_id ON public.prompts USING btree (team_id);


--
-- Name: idx_prompts_team_id_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prompts_team_id_user_id ON public.prompts USING btree (team_id, user_id) WHERE (team_id IS NOT NULL);


--
-- Name: idx_prompts_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_prompts_user_id ON public.prompts USING btree (user_id);


--
-- Name: idx_rae_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_rae_created_at ON public.resource_access_events USING btree (created_at);


--
-- Name: idx_rae_resource_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_rae_resource_created ON public.resource_access_events USING btree (team_id, resource_type, resource_id, created_at DESC);


--
-- Name: idx_rae_team_type_created; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_rae_team_type_created ON public.resource_access_events USING btree (team_id, resource_type, created_at DESC);


--
-- Name: idx_team_invitations_invitee_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_team_invitations_invitee_email ON public.team_invitations USING btree (invitee_email);


--
-- Name: idx_team_invitations_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_team_invitations_status ON public.team_invitations USING btree (status);


--
-- Name: idx_team_invitations_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_team_invitations_team_id ON public.team_invitations USING btree (team_id);


--
-- Name: idx_team_invitations_token; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_team_invitations_token ON public.team_invitations USING btree (token);


--
-- Name: idx_team_members_role; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_team_members_role ON public.team_members USING btree (role);


--
-- Name: idx_team_members_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_team_members_team_id ON public.team_members USING btree (team_id);


--
-- Name: idx_team_members_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_team_members_user_id ON public.team_members USING btree (user_id);


--
-- Name: idx_team_subscriptions_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_team_subscriptions_status ON public.team_subscriptions USING btree (status);


--
-- Name: idx_team_subscriptions_stripe_customer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_team_subscriptions_stripe_customer_id ON public.team_subscriptions USING btree (stripe_customer_id);


--
-- Name: idx_team_subscriptions_stripe_subscription_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_team_subscriptions_stripe_subscription_id ON public.team_subscriptions USING btree (stripe_subscription_id);


--
-- Name: idx_team_subscriptions_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_team_subscriptions_team_id ON public.team_subscriptions USING btree (team_id);


--
-- Name: idx_team_subscriptions_tier; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_team_subscriptions_tier ON public.team_subscriptions USING btree (tier);


--
-- Name: idx_teams_created_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_teams_created_at ON public.teams USING btree (created_at DESC);


--
-- Name: idx_teams_is_personal; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_teams_is_personal ON public.teams USING btree (is_personal);


--
-- Name: idx_teams_owner_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_teams_owner_id ON public.teams USING btree (owner_id);


--
-- Name: idx_teams_slug; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_teams_slug ON public.teams USING btree (slug);


--
-- Name: idx_types_global_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_types_global_unique ON public.types USING btree (resource_type, slug) WHERE (team_id IS NULL);


--
-- Name: idx_types_team_resource; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_types_team_resource ON public.types USING btree (team_id, resource_type);


--
-- Name: idx_types_team_unique; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_types_team_unique ON public.types USING btree (team_id, resource_type, slug) WHERE (team_id IS NOT NULL);


--
-- Name: idx_user_preferences_user_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_user_preferences_user_id ON public.user_preferences USING btree (user_id);


--
-- Name: idx_users_default_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_default_team_id ON public.users USING btree (default_team_id);


--
-- Name: idx_users_email; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_email ON public.users USING btree (email);


--
-- Name: idx_users_google_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_google_id ON public.users USING btree (google_id);


--
-- Name: idx_users_idp_provider_subject; Type: INDEX; Schema: public; Owner: -
--

CREATE UNIQUE INDEX idx_users_idp_provider_subject ON public.users USING btree (idp_provider, idp_subject) WHERE ((idp_provider IS NOT NULL) AND (idp_subject IS NOT NULL));


--
-- Name: idx_users_onboarding_completed; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_onboarding_completed ON public.users USING btree (onboarding_completed);


--
-- Name: idx_users_stripe_customer_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_stripe_customer_id ON public.users USING btree (stripe_customer_id);


--
-- Name: idx_users_subscription_canceled_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_subscription_canceled_at ON public.users USING btree (subscription_canceled_at);


--
-- Name: idx_users_subscription_status; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_users_subscription_status ON public.users USING btree (subscription_status);


--
-- Name: idx_webhook_events_event_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_events_event_id ON public.webhook_events USING btree (event_id);


--
-- Name: idx_webhook_events_event_type; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_events_event_type ON public.webhook_events USING btree (event_type);


--
-- Name: idx_webhook_events_processed_at; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_events_processed_at ON public.webhook_events USING btree (processed_at);


--
-- Name: idx_webhook_events_team_id; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX idx_webhook_events_team_id ON public.webhook_events USING btree (team_id);


--
-- Name: notification_deliveries_notification_id_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX notification_deliveries_notification_id_idx ON public.notification_deliveries USING btree (notification_id);


--
-- Name: notification_digest_queue_scheduled_for_idx; Type: INDEX; Schema: public; Owner: -
--

CREATE INDEX notification_digest_queue_scheduled_for_idx ON public.notification_digest_queue USING btree (scheduled_for) WHERE (sent_at IS NULL);


--
-- Name: agents agents_updated_at_trigger; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER agents_updated_at_trigger BEFORE UPDATE ON public.agents FOR EACH ROW EXECUTE FUNCTION public.update_agents_updated_at();


--
-- Name: claude_code_hooks_payload update_claude_code_hooks_payload_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER update_claude_code_hooks_payload_updated_at BEFORE UPDATE ON public.claude_code_hooks_payload FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: cursor_ide_hooks_payload update_cursor_ide_hooks_payload_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER update_cursor_ide_hooks_payload_updated_at BEFORE UPDATE ON public.cursor_ide_hooks_payload FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: embeddings update_embeddings_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER update_embeddings_updated_at BEFORE UPDATE ON public.embeddings FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: memories update_memories_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER update_memories_updated_at BEFORE UPDATE ON public.memories FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: team_subscriptions update_team_subscriptions_updated_at; Type: TRIGGER; Schema: public; Owner: -
--

CREATE TRIGGER update_team_subscriptions_updated_at BEFORE UPDATE ON public.team_subscriptions FOR EACH ROW EXECUTE FUNCTION public.update_updated_at_column();


--
-- Name: activities activities_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.activities
    ADD CONSTRAINT activities_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: agent_execution_events agent_execution_events_execution_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_execution_events
    ADD CONSTRAINT agent_execution_events_execution_id_fkey FOREIGN KEY (execution_id) REFERENCES public.agent_executions(id) ON DELETE CASCADE;


--
-- Name: agent_executions agent_executions_agent_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_executions
    ADD CONSTRAINT agent_executions_agent_id_fkey FOREIGN KEY (agent_id) REFERENCES public.agents(id) ON DELETE CASCADE;


--
-- Name: agent_executions agent_executions_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agent_executions
    ADD CONSTRAINT agent_executions_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: agents agents_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT agents_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: api_key_integration_permissions api_key_integration_permissions_api_key_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_key_integration_permissions
    ADD CONSTRAINT api_key_integration_permissions_api_key_id_fkey FOREIGN KEY (api_key_id) REFERENCES public.api_keys(id) ON DELETE CASCADE;


--
-- Name: api_key_integration_permissions api_key_integration_permissions_integration_code_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_key_integration_permissions
    ADD CONSTRAINT api_key_integration_permissions_integration_code_fkey FOREIGN KEY (integration_code) REFERENCES public.api_key_integrations_catalog(integration_code) ON DELETE CASCADE;


--
-- Name: api_keys api_keys_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.api_keys
    ADD CONSTRAINT api_keys_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: artifacts artifacts_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifacts
    ADD CONSTRAINT artifacts_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: attachments attachments_team_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attachments
    ADD CONSTRAINT attachments_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: attachments attachments_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.attachments
    ADD CONSTRAINT attachments_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: content_versions content_versions_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_versions
    ADD CONSTRAINT content_versions_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: content_versions content_versions_team_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.content_versions
    ADD CONSTRAINT content_versions_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: device_tokens device_tokens_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.device_tokens
    ADD CONSTRAINT device_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: embedding_providers embedding_providers_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.embedding_providers
    ADD CONSTRAINT embedding_providers_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: embeddings embeddings_team_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.embeddings
    ADD CONSTRAINT embeddings_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: embeddings embeddings_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.embeddings
    ADD CONSTRAINT embeddings_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: agents fk_agents_team; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.agents
    ADD CONSTRAINT fk_agents_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: artifacts fk_artifacts_project_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifacts
    ADD CONSTRAINT fk_artifacts_project_id FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: artifacts fk_artifacts_team; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.artifacts
    ADD CONSTRAINT fk_artifacts_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: blueprints fk_blueprints_project_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blueprints
    ADD CONSTRAINT fk_blueprints_project_id FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: blueprints fk_blueprints_team; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blueprints
    ADD CONSTRAINT fk_blueprints_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: claude_code_hooks_payload fk_claude_code_hooks_payload_team; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.claude_code_hooks_payload
    ADD CONSTRAINT fk_claude_code_hooks_payload_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: cursor_ide_hooks_payload fk_cursor_ide_hooks_payload_team; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.cursor_ide_hooks_payload
    ADD CONSTRAINT fk_cursor_ide_hooks_payload_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: feed_items fk_feed_items_feed; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feed_items
    ADD CONSTRAINT fk_feed_items_feed FOREIGN KEY (feed_id) REFERENCES public.feeds(id) ON DELETE CASCADE;


--
-- Name: feed_items fk_feed_items_project; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feed_items
    ADD CONSTRAINT fk_feed_items_project FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE SET NULL;


--
-- Name: feed_items fk_feed_items_team; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feed_items
    ADD CONSTRAINT fk_feed_items_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: feed_items fk_feed_items_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feed_items
    ADD CONSTRAINT fk_feed_items_user FOREIGN KEY (posted_by_user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: feeds fk_feeds_team; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feeds
    ADD CONSTRAINT fk_feeds_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: feeds fk_feeds_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feeds
    ADD CONSTRAINT fk_feeds_user FOREIGN KEY (created_by_user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: memories fk_memories_project_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memories
    ADD CONSTRAINT fk_memories_project_id FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: memories fk_memories_team; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memories
    ADD CONSTRAINT fk_memories_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: projects fk_projects_team; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.projects
    ADD CONSTRAINT fk_projects_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: prompts fk_prompts_project_id; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompts
    ADD CONSTRAINT fk_prompts_project_id FOREIGN KEY (project_id) REFERENCES public.projects(id) ON DELETE CASCADE;


--
-- Name: prompts fk_prompts_team; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompts
    ADD CONSTRAINT fk_prompts_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: feed_item_replies fk_replies_feed_item; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feed_item_replies
    ADD CONSTRAINT fk_replies_feed_item FOREIGN KEY (feed_item_id) REFERENCES public.feed_items(id) ON DELETE CASCADE;


--
-- Name: feed_item_replies fk_replies_team; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feed_item_replies
    ADD CONSTRAINT fk_replies_team FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: feed_item_replies fk_replies_user; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.feed_item_replies
    ADD CONSTRAINT fk_replies_user FOREIGN KEY (posted_by_user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: github_installation_repositories github_installation_repositories_installation_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.github_installation_repositories
    ADD CONSTRAINT github_installation_repositories_installation_id_fkey FOREIGN KEY (installation_id) REFERENCES public.github_installations(id) ON DELETE CASCADE;


--
-- Name: github_installations github_installations_team_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.github_installations
    ADD CONSTRAINT github_installations_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: memories memories_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.memories
    ADD CONSTRAINT memories_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: notification_deliveries notification_deliveries_notification_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notification_deliveries
    ADD CONSTRAINT notification_deliveries_notification_id_fkey FOREIGN KEY (notification_id) REFERENCES public.notifications(id) ON DELETE CASCADE;


--
-- Name: notification_digest_queue notification_digest_queue_notification_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notification_digest_queue
    ADD CONSTRAINT notification_digest_queue_notification_id_fkey FOREIGN KEY (notification_id) REFERENCES public.notifications(id) ON DELETE CASCADE;


--
-- Name: notifications notifications_recipient_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_recipient_user_id_fkey FOREIGN KEY (recipient_user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: notifications notifications_team_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.notifications
    ADD CONSTRAINT notifications_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE SET NULL;


--
-- Name: projects projects_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.projects
    ADD CONSTRAINT projects_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: prompt_references prompt_references_prompt_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_references
    ADD CONSTRAINT prompt_references_prompt_id_fkey FOREIGN KEY (prompt_id) REFERENCES public.prompts(id) ON DELETE CASCADE;


--
-- Name: prompt_references prompt_references_referenced_prompt_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_references
    ADD CONSTRAINT prompt_references_referenced_prompt_id_fkey FOREIGN KEY (referenced_prompt_id) REFERENCES public.prompts(id) ON DELETE CASCADE;


--
-- Name: prompt_share_access prompt_share_access_share_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_share_access
    ADD CONSTRAINT prompt_share_access_share_id_fkey FOREIGN KEY (share_id) REFERENCES public.prompt_shares(id) ON DELETE CASCADE;


--
-- Name: prompt_shares prompt_shares_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_shares
    ADD CONSTRAINT prompt_shares_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: prompt_shares prompt_shares_prompt_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompt_shares
    ADD CONSTRAINT prompt_shares_prompt_id_fkey FOREIGN KEY (prompt_id) REFERENCES public.prompts(id) ON DELETE CASCADE;


--
-- Name: prompts prompts_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.prompts
    ADD CONSTRAINT prompts_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: resource_access_events resource_access_events_team_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.resource_access_events
    ADD CONSTRAINT resource_access_events_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: resource_access_events resource_access_events_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.resource_access_events
    ADD CONSTRAINT resource_access_events_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: blueprints spec_library_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.blueprints
    ADD CONSTRAINT spec_library_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: team_invitations team_invitations_inviter_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_invitations
    ADD CONSTRAINT team_invitations_inviter_id_fkey FOREIGN KEY (inviter_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: team_invitations team_invitations_team_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_invitations
    ADD CONSTRAINT team_invitations_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: team_members team_members_team_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_members
    ADD CONSTRAINT team_members_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: team_members team_members_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_members
    ADD CONSTRAINT team_members_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: team_subscriptions team_subscriptions_team_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.team_subscriptions
    ADD CONSTRAINT team_subscriptions_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE RESTRICT;


--
-- Name: CONSTRAINT team_subscriptions_team_id_fkey ON team_subscriptions; Type: COMMENT; Schema: public; Owner: -
--

COMMENT ON CONSTRAINT team_subscriptions_team_id_fkey ON public.team_subscriptions IS 'Prevents team deletion when subscriptions exist, forcing proper subscription cleanup first';


--
-- Name: teams teams_owner_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.teams
    ADD CONSTRAINT teams_owner_id_fkey FOREIGN KEY (owner_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: types types_created_by_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.types
    ADD CONSTRAINT types_created_by_fkey FOREIGN KEY (created_by) REFERENCES public.users(id) ON DELETE SET NULL;


--
-- Name: types types_team_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.types
    ADD CONSTRAINT types_team_id_fkey FOREIGN KEY (team_id) REFERENCES public.teams(id) ON DELETE CASCADE;


--
-- Name: user_preferences user_preferences_user_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.user_preferences
    ADD CONSTRAINT user_preferences_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;


--
-- Name: users users_default_team_id_fkey; Type: FK CONSTRAINT; Schema: public; Owner: -
--

ALTER TABLE ONLY public.users
    ADD CONSTRAINT users_default_team_id_fkey FOREIGN KEY (default_team_id) REFERENCES public.teams(id) ON DELETE SET NULL;


--
-- PostgreSQL database dump complete
--
