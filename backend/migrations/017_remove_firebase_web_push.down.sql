-- Reverse of 017_remove_firebase_web_push.up.sql.
--
-- ⚠️ THIS RESTORES STRUCTURE ONLY. The device_tokens rows are DROPPED by the up
-- migration and are NOT recoverable here. Rolling back gives you the empty
-- table, not the registrations that were there before. Restore from a backup if
-- the data mattered. (Same wording and caveat as migrations 015 and 016.)
--
-- Note that rolling this migration back does NOT bring web push back: the
-- client code, the API endpoints and the delivery channel were removed from the
-- application in the same change. The table is recreated purely so a
-- down-then-up cycle lands on the original schema.
--
-- The DDL below is copied verbatim from 001_baseline.up.sql rather than
-- reconstructed.

CREATE TABLE IF NOT EXISTS public.device_tokens (
    id uuid DEFAULT gen_random_uuid() NOT NULL,
    user_id uuid NOT NULL,
    token text NOT NULL,
    platform character varying(16) NOT NULL,
    user_agent text,
    last_used_at timestamp with time zone DEFAULT now(),
    created_at timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.device_tokens
    DROP CONSTRAINT IF EXISTS device_tokens_pkey;
ALTER TABLE ONLY public.device_tokens
    ADD CONSTRAINT device_tokens_pkey PRIMARY KEY (id);

CREATE UNIQUE INDEX IF NOT EXISTS device_tokens_token_idx ON public.device_tokens USING btree (token);
CREATE INDEX IF NOT EXISTS device_tokens_user_id_idx ON public.device_tokens USING btree (user_id);

ALTER TABLE ONLY public.device_tokens
    DROP CONSTRAINT IF EXISTS device_tokens_user_id_fkey;
ALTER TABLE ONLY public.device_tokens
    ADD CONSTRAINT device_tokens_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;
