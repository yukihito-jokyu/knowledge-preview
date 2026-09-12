import copy
import unittest
from check_plan import validate, validate_trace


def action(event, work, agent):
    return dict(event=event, work=work, agents=[agent])


def final(number, outcomes):
    return dict(event='final', agents=[f'audit{number}', f'debt{number}'], outcomes={str(k): v for k, v in outcomes.items()})


class ProtocolTests(unittest.TestCase):
    def setUp(self):
        self.base = [dict(event='approve'), action('implement', 4, 'i1'), action('review_pass', 4, 'r1')]

    def test_parallel_work_shared_final_agents(self):
        trace = self.base + [action('implement', 5, 'i2'), action('review_pass', 5, 'r2'), final(1, {4: 'pass', 5: 'pass'}), action('e2e', 12, 'e1'), action('review_pass', 12, 'er1')]
        self.assertEqual(validate_trace(trace)[12], 'done')

    def test_final_return_threshold(self):
        trace = self.base + [final(1, {4: 'return'}), action('implement', 4, 'i2'), action('review_pass', 4, 'r2'), final(2, {4: 'return'}), dict(event='human', work=4)]
        self.assertEqual(validate_trace(trace)[4], 'human')
        with self.assertRaisesRegex(ValueError, 'second return'):
            validate_trace(trace + [action('implement', 4, 'i3')])

    def test_review_return_threshold(self):
        trace = self.base[:2] + [action('review_return', 4, 'r1'), action('implement', 4, 'i2'), action('review_return', 4, 'r2')]
        self.assertEqual(validate_trace(trace)[4], 'human')

    def test_e2e_review_return_threshold(self):
        trace = self.base + [final(1, {4: 'pass'}), action('e2e', 12, 'e1'), action('review_return', 12, 'er1'), action('e2e', 12, 'e2'), action('review_return', 12, 'er2')]
        self.assertEqual(validate_trace(trace)[12], 'human')
        with self.assertRaisesRegex(ValueError, 'second return'):
            validate_trace(trace + [action('e2e', 12, 'e3')])

    def test_product_failure_rechecks_before_e2e(self):
        trace = self.base + [final(1, {4: 'pass'}), action('e2e', 12, 'e1'), dict(event='product_failure', works=[4])]
        with self.assertRaisesRegex(ValueError, 'final checks'):
            validate_trace(trace + [action('e2e', 12, 'e2')])
        trace += [action('implement', 4, 'i2'), action('review_pass', 4, 'r2'), final(2, {4: 'pass'}), action('e2e', 12, 'e2'), action('review_pass', 12, 'er2')]
        self.assertEqual(validate_trace(trace)[12], 'done')

    def test_multiple_product_failures_from_one_e2e(self):
        trace = self.base + [action('implement', 5, 'i2'), action('review_pass', 5, 'r2'), final(1, {4: 'pass', 5: 'pass'}), action('e2e', 12, 'e1'), dict(event='product_failure', works=[4, 5])]
        states = validate_trace(trace)
        self.assertEqual(states, {4: 'fix', 5: 'fix', 12: 'e2e_retry'})
        trace += [action('implement', 4, 'i3'), action('review_pass', 4, 'r3'), action('implement', 5, 'i4'), action('review_pass', 5, 'r4'), final(2, {4: 'pass', 5: 'pass'}), action('e2e', 12, 'e2'), action('review_pass', 12, 'er2')]
        self.assertEqual(validate_trace(trace)[12], 'done')

    def test_approval_and_agent_reuse(self):
        with self.assertRaisesRegex(ValueError, 'approval'):
            validate_trace(self.base[1:])
        with self.assertRaisesRegex(ValueError, 'reuse'):
            validate_trace(self.base[:2] + [action('review_pass', 4, 'i1')])

    def test_final_cannot_skip_work(self):
        with self.assertRaisesRegex(ValueError, 'cover all'):
            validate_trace(self.base + [action('implement', 5, 'i2'), action('review_pass', 5, 'r2'), final(1, {4: 'pass'})])

    def test_plan_required_structure(self):
        data = dict(issue=1, requirements=[dict(id=1)], tasks=[dict(id=1, status='未開始', phase='調査', skills='スキルなし', depends=[])], scenarios=[dict(id=1, requirements=[1], initial='A', input='B', action='C', expected='D', method='E')])
        validate(data)
        for key, value in [('skills', 'backend-review'), ('depends', [1]), ('id', 2)]:
            bad = copy.deepcopy(data)
            bad['tasks'][0][key] = value
            with self.assertRaises(ValueError):
                validate(bad)


if __name__ == '__main__':
    unittest.main()
