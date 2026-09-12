"""Validate optional structured plans or recorded transitions; never starts agents."""
import argparse
import json
from pathlib import Path


def require(condition, message):
    if not condition:
        raise ValueError(message)


def validate(data):
    require(isinstance(data['issue'], int) and data['issue'] > 0, 'invalid issue')
    for key in ('requirements', 'tasks', 'scenarios'):
        items = data[key]
        require(bool(items), f'{key}: empty')
        require([x['id'] for x in items] == list(range(1, len(items) + 1)), f'{key}: IDs must be sequential')
    tasks = {x['id']: x for x in data['tasks']}
    for task in tasks.values():
        require(task['status'] in ('未開始', '実行中', '完了', '差し戻し', '人間判断待ち', '実施不要'), 'invalid status')
        if task['phase'] == '調査':
            require(task['skills'] == 'スキルなし', 'research must use no skill')
        require(all(x in tasks and x != task['id'] for x in task['depends']), 'invalid dependency')
    visited, active = set(), set()

    def visit(i):
        require(i not in active, 'dependency cycle')
        if i in visited:
            return
        active.add(i)
        for dep in tasks[i]['depends']:
            visit(dep)
        active.remove(i)
        visited.add(i)

    for i in tasks:
        visit(i)
    for scenario in data['scenarios']:
        require(scenario['requirements'] and all(1 <= x <= len(data['requirements']) for x in scenario['requirements']), 'invalid requirement reference')
        require(all(scenario.get(x) for x in ('initial', 'input', 'action', 'expected', 'method')), 'missing scenario field')


def validate_trace(events):
    """Validate a recorded protocol; IDs describe actual distinct invocations."""
    approved = False
    seen = set()
    states, review_counts, final_counts = {}, {}, {}
    e2e_works = set()

    def agents(event, count):
        ids = event.get('agents', [])
        require(len(ids) == count, 'missing agent IDs')
        for agent in ids:
            require(agent and agent not in seen, 'agent reuse')
            seen.add(agent)

    def returned(work, counts, retry_state):
        counts[work] = counts.get(work, 0) + 1
        states[work] = 'human' if counts[work] >= 2 else retry_state

    for event in events:
        kind = event['event']
        if kind == 'approve':
            approved = True
            continue
        require(approved, 'implementation before approval')
        if kind == 'final':
            outcomes = event['outcomes']
            require(outcomes and all(str(w) in outcomes for w in states if w not in e2e_works), 'final check must cover all product work')
            require(all(states.get(int(w)) in ('final', 'e2e') for w in outcomes), 'final check before review pass')
            require(all(v in ('pass', 'return') for v in outcomes.values()), 'invalid final verdict')
            agents(event, 2)
            for key, verdict in outcomes.items():
                work = int(key)
                if verdict == 'return':
                    returned(work, final_counts, 'fix')
                else:
                    states[work] = 'e2e'
            continue
        if kind == 'product_failure':
            # One E2E result can identify problems in several product work items.
            targets = event['works']
            require(targets and len(targets) == len(set(targets)), 'invalid failure targets')
            require(any(states[w] == 'e2e_review' for w in e2e_works), 'product failure without E2E execution')
            require(all(w not in e2e_works and states.get(w) == 'e2e' for w in targets), 'invalid product failure target')
            for target in targets:
                states[target] = 'fix'
            for w in e2e_works:
                if states[w] == 'e2e_review':
                    states[w] = 'e2e_retry'
            continue
        work = event['work']
        state = states.get(work, 'ready')
        if kind == 'human':
            require(state == 'human', 'human handoff without threshold')
            continue
        require(state != 'human', 'automatic work after second return')
        require(kind in ('implement', 'review_pass', 'review_return', 'e2e'), 'unknown event')
        agents(event, 1)
        if kind == 'implement':
            require(work not in e2e_works and state in ('ready', 'fix'), 'implementation out of order')
            states[work] = 'review'
        elif kind.startswith('review_'):
            require(state in ('review', 'e2e_review'), 'review out of order')
            if kind == 'review_pass':
                states[work] = 'done' if work in e2e_works else 'final'
            else:
                returned(work, review_counts, 'e2e_retry' if work in e2e_works else 'fix')
        else:
            products = [s for w, s in states.items() if w not in e2e_works]
            require(products and all(s == 'e2e' for s in products), 'E2E before all final checks pass')
            require(state in ('ready', 'e2e_retry'), 'E2E out of order')
            e2e_works.add(work)
            states[work] = 'e2e_review'
    return states


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description='Inspect structured plan input or a recorded synthetic trace; does not control agents.')
    parser.add_argument('input', type=Path)
    parser.add_argument('--trace', action='store_true', help='Check a JSON event array instead of a plan object')
    args = parser.parse_args()
    data = json.loads(args.input.read_text())
    if args.trace:
        print(json.dumps(validate_trace(data), ensure_ascii=False))
    else:
        validate(data)
        print('Plan structure OK; semantic correctness not verified')
