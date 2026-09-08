#!/usr/bin/env python3
"""Render idempotent starter content SQL; operator reviews and executes it separately."""
import json
from pathlib import Path
from uuid import uuid5, NAMESPACE_URL

root = Path(__file__).resolve().parents[1]
publisher = str(uuid5(NAMESPACE_URL, 'https://msime.app/community/publisher/starter'))
def quoted(value):
    return "'" + value.replace("'", "''") + "'"

def render():
    items = json.loads((root / 'assets/community-starter-resources.json').read_text())
    lines = ["BEGIN;", "SET LOCAL ROLE msime_backend;", "SET LOCAL lock_timeout='5s';", "SET LOCAL statement_timeout='30s';",
             f"SELECT pg_advisory_xact_lock(hashtextextended({quoted(publisher)},0));",
             f"INSERT INTO auth_users(id,display_name) VALUES({quoted(publisher)},'水杉精选') ON CONFLICT DO NOTHING;",
             f"DO $$ BEGIN IF EXISTS(SELECT 1 FROM auth_identities WHERE user_id={quoted(publisher)}) OR EXISTS(SELECT 1 FROM auth_sessions WHERE user_id={quoted(publisher)}) THEN RAISE EXCEPTION 'starter publisher is interactive'; END IF; END $$;"]
    for item in items:
        ident = str(uuid5(NAMESPACE_URL, 'https://msime.app/community/starter-resource/' + item['slug'] + '/v1'))
        values = [ident, publisher, item['kind'], item['name'], item['description'], json.dumps(item['content'], ensure_ascii=False)]
        lines.append('INSERT INTO community_resources(id,owner_id,kind,name,description,content) VALUES(' + ','.join(map(quoted, values)) + ') ON CONFLICT DO NOTHING;')
        lines.append(f"DO $$ BEGIN IF NOT EXISTS(SELECT 1 FROM community_resources WHERE id={quoted(ident)} AND owner_id={quoted(publisher)} AND kind={quoted(item['kind'])} AND name={quoted(item['name'])} AND description={quoted(item['description'])} AND content={quoted(values[-1])}::jsonb) THEN RAISE EXCEPTION 'starter content mismatch'; END IF; END $$;")
    return '\n'.join(lines + ['COMMIT;']) + '\n'
if __name__ == '__main__':
    print(render(), end='')
