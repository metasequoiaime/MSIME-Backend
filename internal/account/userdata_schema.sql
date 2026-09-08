-- 用户同步数据与鉴权用户共用生命周期，删除账号时级联清理。
CREATE TABLE IF NOT EXISTS user_preferences (
 user_id text PRIMARY KEY REFERENCES auth_users(id) ON DELETE CASCADE,
 revision bigint NOT NULL DEFAULT 0,
 settings jsonb NOT NULL DEFAULT '{}'
);
CREATE TABLE IF NOT EXISTS user_clipboard_settings (
 user_id text PRIMARY KEY REFERENCES auth_users(id) ON DELETE CASCADE,
 enabled boolean NOT NULL DEFAULT false
);
CREATE TABLE IF NOT EXISTS user_clipboard (
 id text PRIMARY KEY,
 user_id text NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
 text text NOT NULL,
 text_hash text NOT NULL,
 sequence bigint NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(user_id,text_hash)
);
CREATE INDEX IF NOT EXISTS user_clipboard_order ON user_clipboard(user_id,sequence DESC);
CREATE TABLE IF NOT EXISTS user_dictionary_state (
 user_id text PRIMARY KEY REFERENCES auth_users(id) ON DELETE CASCADE,
 revision bigint NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS user_dictionary_entries (
 id text PRIMARY KEY,
 user_id text NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
 kind text NOT NULL CHECK(kind IN ('pinyin','wubi','english','quick')),
 code text NOT NULL,
 word text NOT NULL,
 weight bigint NOT NULL CHECK(weight>=0),
 revision bigint NOT NULL,
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(user_id,kind,code,word)
);
CREATE INDEX IF NOT EXISTS user_dictionary_entries_order ON user_dictionary_entries(user_id,kind,code,word);
CREATE TABLE IF NOT EXISTS user_dictionary_changes (
 user_id text NOT NULL REFERENCES auth_users(id) ON DELETE CASCADE,
 revision bigint NOT NULL,
 change jsonb NOT NULL,
 PRIMARY KEY(user_id,revision)
);
