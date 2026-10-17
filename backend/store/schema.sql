CREATE TABLE IF NOT EXISTS courses (
 id text PRIMARY KEY, slug text UNIQUE NOT NULL, title text NOT NULL, provider text NOT NULL,
 language text NOT NULL, direction text NOT NULL, summary text NOT NULL,
 audience text[] NOT NULL, goals text[] NOT NULL, topics text[] NOT NULL,
 source text NOT NULL, checked_at timestamptz NOT NULL, status text NOT NULL, demo boolean NOT NULL DEFAULT false
);
CREATE INDEX IF NOT EXISTS courses_lookup ON courses(status,language,direction);
CREATE TABLE IF NOT EXISTS offers (
 id text PRIMARY KEY, course_id text NOT NULL REFERENCES courses(id), name text NOT NULL,
 price bigint, price_kind text NOT NULL, free boolean NOT NULL,
 price_checked_at timestamptz NOT NULL, valid_until timestamptz,
 hours integer, weeks integer, review boolean NOT NULL, mentor boolean NOT NULL,
 schedule text NOT NULL, enrollment text NOT NULL, url text NOT NULL,
 CHECK(price IS NULL OR price >= 0)
);
CREATE INDEX IF NOT EXISTS offers_course ON offers(course_id);
CREATE TABLE IF NOT EXISTS settings(key text PRIMARY KEY,value jsonb NOT NULL);
CREATE TABLE IF NOT EXISTS imports(id bigserial PRIMARY KEY,created_at timestamptz DEFAULT now(),operator text NOT NULL,before_data jsonb NOT NULL,after_data jsonb NOT NULL);
CREATE TABLE IF NOT EXISTS events(id text PRIMARY KEY,created_at timestamptz NOT NULL DEFAULT now(),kind text NOT NULL,course_id text, language text,goal text,total integer);
CREATE INDEX IF NOT EXISTS events_time ON events(created_at);
CREATE TABLE IF NOT EXISTS daily_stats(day date,kind text,course_id text,language text,goal text,count bigint NOT NULL,PRIMARY KEY(day,kind,course_id,language,goal));

-- Automatic collection is independent of operator imports, but publication uses
-- the existing imports audit and revision so API caches invalidate normally.
CREATE TABLE IF NOT EXISTS updater_runs (
 id bigserial PRIMARY KEY, started_at timestamptz NOT NULL DEFAULT now(),
 finished_at timestamptz, status text NOT NULL DEFAULT 'running',
 published integer NOT NULL DEFAULT 0, failed integer NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS updater_sources (
 source_id text PRIMARY KEY, attempted_at timestamptz NOT NULL,
 verified_at timestamptz, code text NOT NULL, failures integer NOT NULL DEFAULT 0,
 evidence_sha256 text
);
CREATE TABLE IF NOT EXISTS updater_candidates (
 source_id text PRIMARY KEY, fingerprint text NOT NULL,
 observed_at timestamptz NOT NULL, confirmations integer NOT NULL
);

CREATE TABLE IF NOT EXISTS discovery_feeds (
 feed_id text PRIMARY KEY, cursor integer NOT NULL DEFAULT 0,
 attempted_at timestamptz, verified_at timestamptz, code text NOT NULL DEFAULT 'pending',
 failures integer NOT NULL DEFAULT 0, candidates integer NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS catalog_candidates (
 adapter text NOT NULL, external_id text NOT NULL, canonical_url text UNIQUE NOT NULL,
 feed_id text NOT NULL, state text NOT NULL DEFAULT 'pending', code text NOT NULL DEFAULT 'pending',
 first_seen_at timestamptz NOT NULL DEFAULT now(), last_seen_at timestamptz NOT NULL DEFAULT now(),
 attempted_at timestamptz, next_attempt_at timestamptz NOT NULL DEFAULT now(),
 failures integer NOT NULL DEFAULT 0, evidence_sha256 text,
 PRIMARY KEY(adapter,external_id)
);
CREATE INDEX IF NOT EXISTS catalog_candidates_due ON catalog_candidates(next_attempt_at,adapter,attempted_at);

ALTER TABLE offers ADD COLUMN IF NOT EXISTS support_known boolean;

-- Mapping and new course rows commit together. Deferred foreign keys allow
-- identity reservation before the validated catalog upsert in the transaction.
CREATE TABLE IF NOT EXISTS catalog_identities (
 adapter text NOT NULL, external_id text NOT NULL, canonical_url text UNIQUE NOT NULL,
 course_id text NOT NULL REFERENCES courses(id) DEFERRABLE INITIALLY DEFERRED,
 offer_id text NOT NULL REFERENCES offers(id) DEFERRABLE INITIALLY DEFERRED,
 PRIMARY KEY(adapter,external_id)
);

ALTER TABLE catalog_identities ADD COLUMN IF NOT EXISTS product_id text;
ALTER TABLE catalog_identities ADD COLUMN IF NOT EXISTS profession_id text;

ALTER TABLE catalog_identities ADD COLUMN IF NOT EXISTS netology_family_id bigint;
ALTER TABLE catalog_identities ADD COLUMN IF NOT EXISTS netology_program_id bigint;

-- Normalized observations are durable across collector/publisher restarts.
-- Acknowledgements commit in the same transaction as catalog publication.
CREATE TABLE IF NOT EXISTS catalog_observations (
 id bigserial PRIMARY KEY,
 canonical_url text NOT NULL,
 observed_at timestamptz NOT NULL,
 kind text NOT NULL CHECK (kind IN ('curated','discovered','failure')),
 payload jsonb NOT NULL CHECK (octet_length(payload::text) <= 65536),
 created_at timestamptz NOT NULL DEFAULT now(),
 processed_at timestamptz,
 code text NOT NULL DEFAULT 'pending',
 UNIQUE(canonical_url, observed_at, kind)
);
CREATE INDEX IF NOT EXISTS catalog_observations_pending ON catalog_observations(id) WHERE processed_at IS NULL;
CREATE INDEX IF NOT EXISTS catalog_observations_retention ON catalog_observations(processed_at) WHERE processed_at IS NOT NULL;
CREATE TABLE IF NOT EXISTS catalog_observation_watermarks (
 canonical_url text PRIMARY KEY,
 observed_at timestamptz NOT NULL,
 event_id bigint NOT NULL
);

ALTER TABLE updater_runs ADD COLUMN IF NOT EXISTS worker text NOT NULL DEFAULT 'legacy';
ALTER TABLE updater_runs ADD COLUMN IF NOT EXISTS queued integer NOT NULL DEFAULT 0;
CREATE INDEX IF NOT EXISTS updater_runs_worker_time ON updater_runs(worker,started_at DESC);

ALTER TABLE catalog_identities ADD COLUMN IF NOT EXISTS purple_binding jsonb;

ALTER TABLE catalog_identities ADD COLUMN IF NOT EXISTS verified_binding jsonb;

ALTER TABLE catalog_identities ADD COLUMN IF NOT EXISTS skypro_product_id bigint;

ALTER TABLE offers ADD COLUMN IF NOT EXISTS billing jsonb;

ALTER TABLE catalog_identities ADD COLUMN IF NOT EXISTS practicum_tariffs jsonb;

ALTER TABLE offers ADD COLUMN IF NOT EXISTS duration_months integer CHECK (duration_months BETWEEN 1 AND 120);

-- One-time ownership recovery for pre-ownership collectors. A duration-changing
-- automatic import proves collection; any later explicit operator import wins.
-- Unchanged automatic writes cannot turn retained curated values into owned ones.
DO $$
BEGIN
 IF NOT EXISTS (SELECT 1 FROM information_schema.columns WHERE table_schema=current_schema() AND table_name='offers' AND column_name='duration_collected') THEN
  ALTER TABLE offers ADD COLUMN duration_collected boolean NOT NULL DEFAULT false;
  WITH bound AS (
   SELECT course_id,offer_id FROM catalog_identities WHERE adapter='yandex'
   UNION
   SELECT i.course_id,t.value->>'offer_id' FROM catalog_identities i
    CROSS JOIN LATERAL jsonb_each(COALESCE(i.practicum_tariffs,'{}'::jsonb)) t
    WHERE i.adapter='yandex'
  ), ownership AS (
   SELECT o.id,(
    SELECT h.operator FROM imports h
     CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(h.after_data->'courses')='array' THEN h.after_data->'courses' ELSE '[]'::jsonb END) c
     CROSS JOIN LATERAL jsonb_array_elements(COALESCE(c->'offers','[]'::jsonb)) a
    WHERE c->>'id'=o.course_id AND a->>'id'=o.id
     AND (h.operator NOT IN ('automatic-catalog-discovery','automatic-catalog-publisher','automatic-catalog-updater')
      OR (SELECT b->'duration_months'
       FROM jsonb_array_elements(CASE WHEN jsonb_typeof(h.before_data)='array' THEN h.before_data ELSE '[]'::jsonb END) bc
       CROSS JOIN LATERAL jsonb_array_elements(COALESCE(bc->'offers','[]'::jsonb)) b
       WHERE bc->>'id'=o.course_id AND b->>'id'=o.id LIMIT 1) IS DISTINCT FROM a->'duration_months')
    ORDER BY h.id DESC LIMIT 1
   ) AS last_writer FROM offers o JOIN bound b ON b.offer_id=o.id AND b.course_id=o.course_id
   WHERE o.duration_months IS NOT NULL
  )
  UPDATE offers o SET duration_collected=true FROM ownership x WHERE o.id=x.id AND x.last_writer IN ('automatic-catalog-discovery','automatic-catalog-publisher','automatic-catalog-updater');
 END IF;
END $$;

ALTER TABLE offers ADD COLUMN IF NOT EXISTS learning_conditions jsonb;
ALTER TABLE offers ADD COLUMN IF NOT EXISTS learning_ownership integer NOT NULL DEFAULT 0 CHECK (learning_ownership BETWEEN 0 AND 31);

ALTER TABLE courses ADD COLUMN IF NOT EXISTS profile jsonb;
ALTER TABLE courses ADD COLUMN IF NOT EXISTS profile_ownership integer NOT NULL DEFAULT 0;

-- Optional AI metadata enrichment is independent of financial observations.
CREATE TABLE IF NOT EXISTS catalog_ai_cache (
 course_id text NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
 bundle_digest text NOT NULL, model text NOT NULL, policy text NOT NULL,
 extraction jsonb NOT NULL CHECK(octet_length(extraction::text)<=32768),
 verification jsonb NOT NULL CHECK(octet_length(verification::text)<=8192),
 used_at timestamptz NOT NULL DEFAULT now(),
 PRIMARY KEY(course_id,bundle_digest,model,policy)
);
CREATE TABLE IF NOT EXISTS catalog_ai_daily_budget (
 day date PRIMARY KEY, requests integer NOT NULL CHECK(requests>=0),
 total_tokens bigint NOT NULL DEFAULT 0 CHECK(total_tokens>=0)
);
CREATE TABLE IF NOT EXISTS catalog_ai_sources (
 course_id text PRIMARY KEY REFERENCES courses(id) ON DELETE CASCADE,
 attempted_at timestamptz NOT NULL, verified_at timestamptz,
 next_attempt_at timestamptz NOT NULL DEFAULT now(),
 code text NOT NULL DEFAULT 'pending',failures integer NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS catalog_ai_observations (
 id bigserial PRIMARY KEY,course_id text NOT NULL REFERENCES courses(id) ON DELETE CASCADE,
 observed_at timestamptz NOT NULL,payload jsonb NOT NULL CHECK(octet_length(payload::text)<=262144),
 processed_at timestamptz,code text NOT NULL DEFAULT 'pending',
 UNIQUE(course_id,observed_at)
);
CREATE INDEX IF NOT EXISTS catalog_ai_observations_pending ON catalog_ai_observations(id) WHERE processed_at IS NULL;
CREATE TABLE IF NOT EXISTS catalog_ai_watermarks (
 course_id text PRIMARY KEY REFERENCES courses(id) ON DELETE CASCADE,
 observed_at timestamptz NOT NULL,event_id bigint NOT NULL
);

-- Low bits retain native collector ownership; bits 8..12/13 mark AI priority.
-- Every AI-owned fact must also have its ordinary collector-ownership bit.
ALTER TABLE offers DROP CONSTRAINT IF EXISTS offers_learning_ownership_check;
ALTER TABLE offers ADD CONSTRAINT offers_learning_ownership_check CHECK (
 learning_ownership>=0 AND (learning_ownership & ~7967)=0
 AND ((learning_ownership>>8) & ~learning_ownership)=0
);
ALTER TABLE courses DROP CONSTRAINT IF EXISTS courses_profile_ownership_check;
ALTER TABLE courses ADD CONSTRAINT courses_profile_ownership_check CHECK (
 profile_ownership>=0 AND (profile_ownership & ~16191)=0
 AND ((profile_ownership>>8) & ~profile_ownership)=0
);
CREATE TABLE IF NOT EXISTS catalog_ai_runs (
 id bigserial PRIMARY KEY,started_at timestamptz NOT NULL DEFAULT now(),finished_at timestamptz,
 status text NOT NULL DEFAULT 'running',attempted integer NOT NULL DEFAULT 0,
 queued integer NOT NULL DEFAULT 0,cached integer NOT NULL DEFAULT 0,
 failed integer NOT NULL DEFAULT 0,tokens bigint NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS catalog_ai_provider_health (
 provider text PRIMARY KEY CHECK(provider='stepik'), available boolean NOT NULL,
 code text NOT NULL CHECK(code IN ('available','source_unavailable')),
 checked_at timestamptz NOT NULL, next_probe_at timestamptz NOT NULL
);

ALTER TABLE catalog_ai_sources ADD COLUMN IF NOT EXISTS diagnostic_stage text NOT NULL DEFAULT '';
ALTER TABLE catalog_ai_sources ADD COLUMN IF NOT EXISTS diagnostic_reason text NOT NULL DEFAULT '';
ALTER TABLE catalog_ai_runs ADD COLUMN IF NOT EXISTS useful_queued integer NOT NULL DEFAULT 0;
ALTER TABLE catalog_ai_runs ADD COLUMN IF NOT EXISTS empty_queued integer NOT NULL DEFAULT 0;
ALTER TABLE catalog_ai_runs ADD COLUMN IF NOT EXISTS verified_claims integer NOT NULL DEFAULT 0;
ALTER TABLE catalog_ai_runs ADD COLUMN IF NOT EXISTS discarded_claims integer NOT NULL DEFAULT 0;
