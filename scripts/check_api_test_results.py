#!/usr/bin/env python3
"""Require mapped API behavior tests to actually pass (skips are not coverage)."""
import json
import sys
from pathlib import Path, PurePosixPath


def verify(matrix, events):
    references = set(matrix['shared'])
    for group in ('public', 'admin_and_docs'):
        for tests in matrix[group].values():
            references.update(tests)
    outcomes = {}
    failures = []
    for event in events:
        if event.get('Action') == 'fail':
            failures.append(f"{event.get('Package')}: {event.get('Test', 'package')}")
        if event.get('Test') and event.get('Action') in ('pass', 'skip', 'fail'):
            package = event['Package'].removeprefix('github.com/metasequoiaime/MSIME-Backend/')
            outcomes[(package, event['Test'])] = event['Action']
    for reference in sorted(references):
        path, test = reference.split('::')
        result = outcomes.get((str(PurePosixPath(path).parent), test), 'not executed')
        if result != 'pass':
            failures.append(f'{reference}: {result}')
    return failures


if __name__ == '__main__':
    root = Path(__file__).resolve().parents[1]
    matrix = json.loads((root / 'internal/server/testdata/api-coverage.json').read_text())
    with open(sys.argv[1], encoding='utf-8') as source:
        failures = verify(matrix, [json.loads(line) for line in source if line.strip()])
    if failures:
        print('\n'.join(failures), file=sys.stderr)
        sys.exit(1)
    print(f"API behavior tests passed: {len(matrix['public'])} public operations, "
          f"{len(matrix['admin_and_docs'])} admin/documentation operations")
