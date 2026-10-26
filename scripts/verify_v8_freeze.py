"""Verify v8 source/corpus snapshots without inference; audit v7 read-only."""
import argparse
import hashlib
import json
import zipfile
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
ART = ROOT/'artifacts/plan'
PROTO = ROOT/'testdata/semantic/plan_heldout_v8_protocol.json'
CORPUS = ROOT/'testdata/semantic/plan_heldout_v8.jsonl'
SNAP = ART/'v8-frozen-source.zip'


def sha(raw):
    return hashlib.sha256(raw).hexdigest()


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument('stage', choices=['snapshot', 'before', 'after'])
    args = parser.parse_args()
    p = json.loads(PROTO.read_text(encoding='utf8'))
    assert sha(CORPUS.read_bytes()) == p['input_sha256']
    assert all(sha((ROOT/path).read_bytes()) == value for path, value in p['files'].items())
    if args.stage == 'snapshot':
        with zipfile.ZipFile(SNAP, 'x', compression=zipfile.ZIP_DEFLATED) as z:
            for path in [*p['files'], str(PROTO.relative_to(ROOT)), str(CORPUS.relative_to(ROOT))]:
                z.write(ROOT/path, path.replace('\\', '/'))
    with zipfile.ZipFile(SNAP) as z:
        assert all(sha(z.read(path)) == value for path, value in p['files'].items())
        assert sha(z.read(str(CORPUS.relative_to(ROOT)).replace('\\', '/'))) == p['input_sha256']
    v7 = json.loads((ROOT/'testdata/semantic/plan_heldout_v7_protocol.json').read_text(encoding='utf8'))
    assert sha((ROOT/'testdata/semantic/plan_heldout_v7.jsonl').read_bytes()) == v7['input_sha256']
    with zipfile.ZipFile(ART/'v7-frozen-source.zip') as z:
        assert all(sha(z.read(path)) == value for path, value in v7['files'].items())
        assert sha(z.read('testdata/semantic/plan_heldout_v7.jsonl')) == v7['input_sha256']
    entry = dict(source_files=len(p['files']), current_source_match=True, snapshot_match=True,
                 input_sha256=p['input_sha256'], prompt_sha256=p['prompt_sha256'], schema_sha256=p['schema_sha256'],
                 v7_corpus_and_archive_intact=True, v7_inference_performed=False, model=p['model'])
    if args.stage == 'after':
        result = json.loads((ART/'heldout-v8-first.json').read_text(encoding='utf8'))
        assert result['frozen'] == p
        assert result['Conversations'] >= 180 and result['Turns'] >= 360
        entry['complete_result_matches_protocol'] = True
    out = ART/'v8-freeze-verification.json'
    history = json.loads(out.read_text(encoding='utf8')) if out.exists() else {}
    history[args.stage] = entry
    out.write_text(json.dumps(history, ensure_ascii=False, indent=2), encoding='utf8')
    print(json.dumps(entry))


if __name__ == '__main__':
    main()
