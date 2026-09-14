-- 共享译文缓存。内容寻址，不带 user_id —— 缓存的是「这串文本译成什么」，跟谁请求过无关，所以这张表里没有任何能指回某个用户的列。写入前由服务端把关长度和字符集（见 internal/server/translation_cache.go），整段文本不进这里。
CREATE TABLE IF NOT EXISTS translation_cache (
    source_lang TEXT NOT NULL,
    target_lang TEXT NOT NULL,
    source_text TEXT NOT NULL,
    target_text TEXT NOT NULL,
    hit_count BIGINT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    used_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (source_lang, target_lang, source_text)
);

-- hit_count 存在的目的不是调度缓存，是量出「到底哪些词是真的反复查不到」。将来要不要把高频条目收进出货词库，靠这个索引出的分布来判断，而不是凭猜。
CREATE INDEX IF NOT EXISTS translation_cache_hits ON translation_cache(hit_count DESC, used_at DESC);
