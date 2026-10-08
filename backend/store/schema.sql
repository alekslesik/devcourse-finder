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
