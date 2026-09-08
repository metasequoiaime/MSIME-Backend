#!/usr/bin/env python3
"""验证真实 Engine 桥接程序；MSIME_ENGINE_TEST_BINARY 指定本机编译产物。"""
import json
import os
import subprocess
import unittest


@unittest.skipUnless(os.environ.get('MSIME_ENGINE_TEST_BINARY'), '需要编译原生 Engine')
class NativeEngineTest(unittest.TestCase):
    def call(self, value):
        result = subprocess.run([os.environ['MSIME_ENGINE_TEST_BINARY']], input=json.dumps(value),
                                text=True, capture_output=True, check=True, timeout=10)
        return json.loads(result.stdout)

    def test_unicode_and_invalid_scalar(self):
        result = self.call({'operation': 'unicode', 'text': '4E2D', 'limit': 5})
        self.assertEqual(result['candidates'][0]['word'], '中')
        result = self.call({'operation': 'unicode', 'text': 'D800'})
        self.assertEqual(result['candidates'], [])

    def test_reference_date(self):
        result = self.call({'operation': 'datetime', 'text': 'date', 'limit': 5,
                            'date': dict(year=2026, month=9, day=8, weekday=2, hour=15, minute=30, second=0)})
        words = [item['word'] for item in result['candidates']]
        self.assertIn('2026-09-08', words)
        self.assertEqual(len(words), 5)

    def test_engine_normalization_and_validation(self):
        request = dict(operation='validate_dictionary', kind='pinyin', code='ni hao', text='你好', weight=10)
        result = self.call(request)
        self.assertEqual(result['code'], "ni'hao")
        request['code'] = "not-valid"
        self.assertEqual(self.call(request)['error'], 'invalid_dictionary_entry')

    def test_missing_resources_is_not_empty_success(self):
        self.assertEqual(self.call(dict(operation='english', text='hello'))['error'], 'resources_unavailable')

    def test_bounded_requests(self):
        self.assertEqual(self.call(dict(operation='unicode', text='4E2D', limit=201))['error'], 'invalid_request')
        self.assertEqual(self.call(dict(operation='unicode', text='x' * 70000))['error'], 'invalid_request')


if __name__ == '__main__':
    unittest.main()
