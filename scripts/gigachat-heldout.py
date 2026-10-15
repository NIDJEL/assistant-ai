"""Select at most two finalists using DEV, then freeze and run held-out once."""
import hashlib
import json
import os
import pathlib
import subprocess
from datetime import datetime, timezone

root = pathlib.Path(__file__).resolve().parents[1]
out = root / 'artifacts/gigachat'
runner = root / 'tmp/semantic-eval.exe'
sample = out / 'dev-screening.jsonl'
env = dict(os.environ, GIGACHAT_INTEGRATION='1')
models = ['GigaChat-3-Ultra','GigaChat-2-Max','GigaChat-3-Pro','GigaChat-2']
held = root / 'testdata/semantic/structured_heldout.jsonl'
assert hashlib.sha256(held.read_bytes()).hexdigest() == '704b0c5324dab12bf5673afeab92df1c7507d7555f1761e619456eb8ab093b1e'
protocol_file = out / 'heldout-protocol.json'
if protocol_file.exists():
    raise SystemExit('Held-out protocol already exists. Use saved reports; do not treat a rerun as independent.')

def run(*args):
    subprocess.run([str(runner), *map(str,args)],cwd=root,env=env,check=True)

eligible = []
for model in models:
    path = out / ('dev-' + model + '.json')
    if not path.exists():
        continue
    live = json.loads(path.read_text(encoding='utf-8-sig'))
    if live['total'] != 72 or live.get('aborted_reason'):
        continue
    mapped = out / ('dev-final-' + model + '.json')
    run('-model',model,'-mode','go','-input',sample,'-output',mapped)
    final = json.loads(mapped.read_text(encoding='utf-8-sig'))
    if final['total'] != 72 or final.get('aborted_reason'):
        continue
    eligible.append((model, final['core_metrics']['domain_command_accuracy']['rate'], final['metrics']['domain_command_accuracy']['rate'], live['latency_ms']['warm']['p95']))
eligible.sort(key=lambda row:(-row[1],-row[2],row[3]))
finalists = [row[0] for row in eligible[:2]]
assert finalists, 'No model completed DEV screening'
files = ['internal/llm/semantic.go','internal/llm/gigachat.go','internal/nlp/semantic.go','internal/nlp/semantic_temporal.go','internal/nlp/dates.go','internal/nlp/relative_time.go','cmd/semantic-eval/main.go','testdata/semantic/structured_contract.json','testdata/semantic/structured_heldout.jsonl']
protocol = {'frozen_at':datetime.now(timezone.utc).isoformat(),'finalists':finalists,'selection':'CORE domain, then all domain, then warm p95 on DEV72; no held-out results used','dev_ranking':eligible,'sha256':{name:hashlib.sha256((root/name).read_bytes()).hexdigest() for name in files}}
protocol_file.write_text(json.dumps(protocol,indent=2),encoding='utf-8')
print('Frozen finalists: ' + ', '.join(finalists),flush=True)
for model in finalists:
    run('-model',model,'-input','testdata/semantic/structured_regressions.jsonl','-output',out/('mandatory-'+model+'.json'))
for model in finalists:
    run('-model',model,'-input',held,'-output',out/('heldout-'+model+'.json'))
    assert all(hashlib.sha256((root/name).read_bytes()).hexdigest()==digest for name,digest in protocol['sha256'].items()), 'Protocol changed during held-out'
