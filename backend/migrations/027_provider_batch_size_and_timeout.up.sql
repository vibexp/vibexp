-- Issue #1665: per-provider embedding request batch size and timeout.
--
-- Both were hardcoded (32 chunks per /embeddings request, 30 s per request),
-- which a CPU-only self-hosted backend can exceed on a full batch. The defaults
-- are those hardcoded values, so every existing provider behaves exactly as it
-- did. The batch size cap of 32 matches text-embeddings-inference's default
-- --max-client-batch-size (#756): the setting only lowers the batch.
ALTER TABLE public.embedding_providers
    ADD COLUMN batch_size integer NOT NULL DEFAULT 32
        CONSTRAINT embedding_providers_batch_size_range CHECK (batch_size BETWEEN 1 AND 32),
    ADD COLUMN request_timeout_seconds integer NOT NULL DEFAULT 30
        CONSTRAINT embedding_providers_request_timeout_seconds_range
            CHECK (request_timeout_seconds BETWEEN 5 AND 300);
