CREATE TABLE IF NOT EXISTS admin_events (
 id text PRIMARY KEY,
 kind text NOT NULL CHECK(kind IN ('download','crash')),
 platform text NOT NULL,
 version text NOT NULL,
 message text NOT NULL DEFAULT '',
 stack text NOT NULL DEFAULT '',
 resolved boolean NOT NULL DEFAULT false,
 created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS admin_events_kind_time ON admin_events(kind,created_at DESC,id);
CREATE TABLE IF NOT EXISTS admin_audit (
 id bigserial PRIMARY KEY,
 action text NOT NULL,
 target text NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS auth_users_created ON auth_users(created_at);
