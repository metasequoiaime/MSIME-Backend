-- Community designs belong to accounts; unique keys make counters safe across replicas.
CREATE TABLE IF NOT EXISTS community_skins (
 id text PRIMARY KEY,
 owner_id text NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
 name text NOT NULL,
 description text NOT NULL DEFAULT '',
 design jsonb NOT NULL,
 created_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(owner_id,id)
);
CREATE INDEX IF NOT EXISTS community_skins_newest ON community_skins(created_at DESC,id);
CREATE TABLE IF NOT EXISTS community_skin_downloads (
 skin_id text NOT NULL REFERENCES community_skins(id) ON DELETE CASCADE,
 user_id text NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
 PRIMARY KEY(skin_id,user_id)
);
CREATE TABLE IF NOT EXISTS community_skin_ratings (
 skin_id text NOT NULL REFERENCES community_skins(id) ON DELETE CASCADE,
 user_id text NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
 stars integer NOT NULL CHECK(stars BETWEEN 1 AND 5),
 PRIMARY KEY(skin_id,user_id)
);
