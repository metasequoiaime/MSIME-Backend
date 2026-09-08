import unittest
from check_api_test_results import verify


class APIResultGateTests(unittest.TestCase):
    matrix = {'shared': [], 'public': {'GET /example': ['internal/server/example_test.go::TestExample']}, 'admin_and_docs': {}}

    def event(self, action):
        return {'Package': 'github.com/metasequoiaime/MSIME-Backend/internal/server', 'Test': 'TestExample', 'Action': action}

    def test_requires_actual_success(self):
        self.assertEqual(verify(self.matrix, [self.event('pass')]), [])
        for events in ([], [self.event('skip')], [self.event('run')], [self.event('fail')]):
            self.assertTrue(verify(self.matrix, events))

    def test_unmapped_package_failure_still_fails(self):
        self.assertTrue(verify(self.matrix, [self.event('pass'), {'Package': 'other', 'Action': 'fail'}]))

    def test_matching_name_in_other_package_does_not_count(self):
        event = self.event('pass')
        event['Package'] = 'github.com/metasequoiaime/MSIME-Backend/internal/account'
        self.assertTrue(verify(self.matrix, [event]))
