# Community resource API

`/v1/community/resources` exposes data-only `dictionary` and `reply` works separately from existing skin APIs. `GET` requires `kind`; `q` searches names, `offset` paginates 20 works, and `scope=saved|mine` requires a user session. Detail includes content, revision, saves, ownership and rating state. User IDs and credentials are never published.

Authenticated `POST` accepts a stable UUID, kind, name (1–32 characters), description (0–280), content and revision. New works use revision 0; updates use the current revision. Identical retries return the existing version; differing stale updates return 409. Only the owner may update or delete; an account may publish at most 50 resources. Account deletion cascades through resources, saves and ratings.

Dictionary content contains 1–128 explicit `{kind,code,word,weight}` entries. Kinds are pinyin, wubi, english and quick. The fixed Engine validates and normalizes each entry; normalized duplicates are rejected. No personal dictionary or learning history is read implicitly. Reply content contains a prompt of 1–2000 characters; dictionaries cannot carry prompts and replies cannot carry entries. Unknown fields are rejected. No code, remote assets or provider credentials are part of either format.

`PUT /{id}/save` accepts `{saved:bool}` and is idempotent. `PUT /{id}/rating` accepts 1–5 stars; saving is required and self-rating is forbidden. Reading a saved work returns its latest revision. Clients explicitly preview and apply updates; the server never overwrites a subscriber's personal words.

Deploy the additive `internal/account/community_schema.sql` migration before the new image. Production tables belong to the existing owner role; grant runtime DML on the three new `community_resource*` tables, not schema CREATE. Older images ignore the added tables and can roll back without removing them.

`python3 scripts/community_resources_seed.py` renders a transaction for five original starter works (three prompts, two word packs). Review before execution. UUIDs are deterministic, repeated execution does not replace content or fabricate saves/ratings. Native validation is covered by `TestStarterResourcesUseValidContent` with `MSIME_ENGINE_TEST_BINARY` set.
