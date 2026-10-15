"""Freeze a stratified DEV screening set without reading held-out examples."""
import collections
import hashlib
import json
import pathlib
import random

root = pathlib.Path(__file__).resolve().parents[1]
corpus = root / 'testdata/semantic/semantic_corpus.jsonl'
cases = [json.loads(line) for line in corpus.read_text(encoding='utf-8-sig').splitlines() if line.strip()]
regressions = {json.loads(line)['text'] for line in (root / 'testdata/semantic/structured_regressions.jsonl').read_text(encoding='utf-8-sig').splitlines() if line.strip()}
rng = random.Random(20261015)
selected, seen = [], set()

def pick(label, count, predicate):
    pool = [case for case in cases if case['id'] not in seen and case['text'] not in regressions and predicate(case)]
    rng.shuffle(pool)
    assert len(pool) >= count, (label, len(pool), count)
    # Cover voice/ASR in every creation class, plus conversational text.
    if label in ('task', 'meeting', 'reminder'):
        voice = [case for case in pool if 'asr' in case['tags']]
        pool = voice[:2] + [case for case in pool if case not in voice[:2]]
    for case in pool[:count]:
        selected.append(case)
        seen.add(case['id'])

for kind in ('task', 'meeting', 'reminder'):
    pick(kind, 10, lambda c: c['expected']['action'] == 'create' and c['expected']['kind'] == kind and not c['expected'].get('recurrence'))
pick('list', 6, lambda c: c['expected']['action'] == 'list')
pick('search', 10, lambda c: c['expected']['action'] == 'search')
for action, count in [('update',6), ('delete',5), ('complete',5)]:
    pick(action, count, lambda c: c['expected']['action'] == action)
pick('recurrence', 4, lambda c: bool(c['expected'].get('recurrence')))
pick('ambiguity/safety', 6, lambda c: c['expected']['action'] == 'help')
assert len(selected) == 72
out = root / 'artifacts/gigachat'
out.mkdir(parents=True, exist_ok=True)
sample = out / 'dev-screening.jsonl'
sample.write_text(''.join(json.dumps(case, ensure_ascii=False, separators=(',',':'))+'\n' for case in selected), encoding='utf-8')
files = [corpus, sample, root/'testdata/semantic/structured_heldout.jsonl', root/'testdata/semantic/structured_contract.json', root/'internal/llm/semantic.go', root/'internal/nlp/semantic.go']
manifest = {'seed':20261015, 'dev_total':len(cases), 'sample_total':len(selected), 'ids':[c['id'] for c in selected], 'sha256':{str(p.relative_to(root)):hashlib.sha256(p.read_bytes()).hexdigest() for p in files}}
(out/'freeze.json').write_text(json.dumps(manifest,indent=2),encoding='utf-8')
print(json.dumps({'screening':len(selected),'actions':dict(collections.Counter(c['expected']['action'] for c in selected)),'asr':sum('asr' in c['tags'] for c in selected)},ensure_ascii=False))
