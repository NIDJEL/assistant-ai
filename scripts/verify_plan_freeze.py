"""Read-only source/corpus/archive integrity checks, no model inference."""
import argparse
import hashlib
import json
import zipfile
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
ART=ROOT/'artifacts/plan'
PROTO=ROOT/'testdata/semantic/plan_heldout_v7_protocol.json'
CORPUS=ROOT/'testdata/semantic/plan_heldout_v7.jsonl'
SNAP=ART/'v7-frozen-source.zip'


def sha(raw):return hashlib.sha256(raw).hexdigest()


def main():
    parser=argparse.ArgumentParser();parser.add_argument('stage',choices=['snapshot','before','after']);args=parser.parse_args()
    protocol=json.loads(PROTO.read_text(encoding='utf8'))
    assert sha(CORPUS.read_bytes())==protocol['input_sha256']
    checks={path:sha((ROOT/path).read_bytes())==expected for path,expected in protocol['files'].items()}
    assert all(checks.values()), [k for k,v in checks.items() if not v]
    if args.stage=='snapshot':
        with zipfile.ZipFile(SNAP,'x',compression=zipfile.ZIP_DEFLATED) as z:
            for path in [*protocol['files'],str(PROTO.relative_to(ROOT)).replace('\\','/'),str(CORPUS.relative_to(ROOT)).replace('\\','/')]:
                z.write(ROOT/path,path)
    with zipfile.ZipFile(SNAP) as z:
        archive_checks={path:sha(z.read(path))==expected for path,expected in protocol['files'].items()}
        assert all(archive_checks.values())
        assert sha(z.read(str(CORPUS.relative_to(ROOT)).replace('\\','/')))==protocol['input_sha256']
    v6=json.loads((ROOT/'testdata/semantic/plan_heldout_v6_protocol.json').read_text(encoding='utf8'))
    v6_checksum_unchanged=sha((ROOT/'testdata/semantic/plan_heldout_v6.jsonl').read_bytes())==v6['input_sha256']
    assert v6_checksum_unchanged
    out=ART/'v7-freeze-verification.json'
    history=json.loads(out.read_text(encoding='utf8')) if out.exists() else {}
    history[args.stage]=dict(source_files=len(checks),source_match=all(checks.values()),snapshot_match=all(archive_checks.values()),
                            input_sha256=protocol['input_sha256'],prompt_sha256=protocol['prompt_sha256'],schema_sha256=protocol['schema_sha256'],
                            evaluator_sha256=protocol['files']['cmd/plan-eval/main.go'],
                            frozen_v6_corpus_checksum_unchanged=v6_checksum_unchanged,v6_inference_performed=False)
    if args.stage=='after':
        r=json.loads((ART/'heldout-v7-first.json').read_text(encoding='utf8'))
        assert r['frozen']==protocol
        assert r['Conversations']>=150 and r['Turns']>=300
        history[args.stage]['full_result_matches_protocol']=True
    out.write_text(json.dumps(history,ensure_ascii=False,indent=2),encoding='utf8')
    print(json.dumps(history[args.stage]))


if __name__=='__main__':main()
