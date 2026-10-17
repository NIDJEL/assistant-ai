"""Scan known credentials and generic token patterns; output counts/paths only."""
import argparse
import base64
import hashlib
import json
import os
import pathlib
import re
import subprocess
import sys

ROOT=pathlib.Path(__file__).resolve().parents[1]
LABELS={'Authorization Key':'GIGACHAT_AUTHORIZATION_KEY','Client ID':'GIGACHAT_CLIENT_ID','Client Secret':'GIGACHAT_CLIENT_SECRET'}
SKIP={'.git','node_modules','.venv','__pycache__','.pytest_cache','.pnpm-store'}
FIXTURE_ALLOWLIST=ROOT/'scripts/security-fixtures.json'
FIXTURES=json.loads(FIXTURE_ALLOWLIST.read_text(encoding='utf-8')) if FIXTURE_ALLOWLIST.exists() else {}
HASHES={}

def git(*args):
    return subprocess.check_output(['git',*args],cwd=ROOT,stderr=subprocess.DEVNULL)

def env_values(raw):
    result={}
    for line in raw.decode('utf-8-sig').splitlines():
        match=re.match(r'^\s*(?:export\s+)?([A-Z][A-Z0-9_]*)\s*=\s*(.*)$',line)
        if match:result[match[1]]=match[2].strip().strip('"').strip("'")
    return result

def source_values(path):
    raw=path.read_bytes()
    text=raw.decode('utf-16' if raw.startswith((b'\xff\xfe',b'\xfe\xff')) else 'utf-8-sig')
    result={}
    for label,name in LABELS.items():
        match=re.search(r'(?im)^\s*'+re.escape(label)+r'\s*(?::\s*\r?\n\s*|=\s*)["\x27]?([^"\x27\r\n]+)',text)
        if match:result[name]=match[1].strip()
    return result

def findings(raw,known,path=''):
    types=set()
    for value,kind in known.items():
        if value in raw:types.add(kind)
    if HASHES:
        for candidate in re.findall(rb'[A-Za-z0-9_+/.=-]{24,}',raw):
            variants=[candidate]
            if b'=' in candidate and re.fullmatch(rb'[A-Za-z_][A-Za-z0-9_]*',candidate.partition(b'=')[0]):variants.append(candidate.partition(b'=')[2])
            for value in variants:
                digest=hashlib.sha256(value).hexdigest()
                if digest in HASHES:types.add(HASHES[digest])
    if re.search(rb'-----BEGIN (?:RSA |EC |OPENSSH )?PRIVATE KEY-----',raw):types.add('private-key material')
    if re.search(rb'eyJ[A-Za-z0-9_-]{16,}\.[A-Za-z0-9_-]{16,}\.[A-Za-z0-9_-]{16,}',raw):types.add('JWT/access-token candidate')
    pattern=rb'(?i)(?:["\x27](?:access_token|refresh_token|client_secret|authorization_key|password|secret)["\x27][ \t]*:[ \t]*["\x27]|\b(?:GIGACHAT_AUTHORIZATION_KEY|GIGACHAT_CLIENT_SECRET|SESSION_SECRET|POSTGRES_PASSWORD)[ \t]*=[ \t]*["\x27]?)([^"\x27\s\r\n,;}{]{12,})'
    for match in re.finditer(pattern,raw):
        value=match[1];lower=value.lower()
        if lower.startswith((b'test',b'unit',b'ci-only',b'fake',b'dummy',b'example',b'change',b'your',b'<',b'$',b'os.',b'env(',b'value(',b'process.',b'secrets.',b'configuration')):continue
        if value in (b'not-a-real-secret',b'correct-password',b'wrong-password',b'context.Canceled',b'context.DeadlineExceeded'):continue
        if value.lstrip(b'`+').startswith(b'strings.Repeat('):continue
        if hashlib.sha256(value).hexdigest() in FIXTURES.get(path,[]):continue
        types.add('credential literal candidate')
    for match in re.finditer(rb'(?i)\b(?:Bearer|Basic)\s+([A-Za-z0-9_+/.=-]{24,})',raw):
        if not match[1].lower().startswith((b'test',b'unit',b'fake',b'dummy',b'example')):types.add('authorization token candidate')
    return sorted(types)

def objects(ids):
    proc=subprocess.Popen(['git','cat-file','--batch'],cwd=ROOT,stdin=subprocess.PIPE,stdout=subprocess.PIPE,stderr=subprocess.DEVNULL)
    try:
        for oid in ids:
            proc.stdin.write(oid.encode()+b'\n');proc.stdin.flush()
            head=proc.stdout.readline().split()
            if len(head)!=3:raise RuntimeError('invalid object header')
            data=proc.stdout.read(int(head[2]));proc.stdout.read(1)
            yield oid,head[1].decode(),data
    finally:
        proc.stdin.close();proc.stdout.close();proc.wait()

def main():
    parser=argparse.ArgumentParser()
    parser.add_argument('--extra-source',action='append',default=[])
    parser.add_argument('--report',default='artifacts/security/local-audit.json')
    parser.add_argument('--no-history',action='store_true')
    parser.add_argument('--hash-manifest')
    args=parser.parse_args()
    if args.hash_manifest:
        HASHES.update({entry['sha256']:entry['kind'] for entry in json.loads(pathlib.Path(args.hash_manifest).read_text(encoding='utf-8'))['entries']})
    local_env=env_values((ROOT/'.env').read_bytes())
    if any(not local_env.get(name) for name in LABELS.values()):raise RuntimeError('credentials not configured')
    known={}
    def add(values,prefix):
        for name,value in values.items():
            if len(value)<8:continue
            raw=value.encode();known[raw]=prefix+':'+name
            known[base64.b64encode(raw)]=prefix+':'+name+' (encoded)'
    private={name:value for name,value in local_env.items() if name in LABELS.values() or re.search(r'(?:SECRET|PASSWORD|PRIVATE_KEY|API_KEY|ACCESS_TOKEN|REFRESH_TOKEN)$',name)}
    add(private,'current credential')
    for name in args.extra_source:
        path=pathlib.Path(name)
        if path.exists():add(source_values(path),'old/extra GigaChat credential')
    tracked=set(filter(None,git('ls-files','-z').decode().split('\0')))
    records=[]
    def record(category,path,types,oid=None):
        if types:
            item={'category':category,'path':path,'types':types}
            if oid:item['object']=oid
            records.append(item)
    work_count=0
    for folder,dirs,files in os.walk(ROOT):
        dirs[:]=[d for d in dirs if d not in SKIP]
        for name in files:
            path=pathlib.Path(folder)/name;rel=path.relative_to(ROOT).as_posix()
            if rel=='.env' or path.is_symlink():continue
            raw=path.read_bytes();work_count+=1
            category='tracked' if rel in tracked else 'untracked'
            if rel.startswith('artifacts/'):category='artifacts'
            if path.suffix.lower() in ('.log','.jsonl') and ('log' in rel.lower() or '/tmp/' in '/'+rel):category='logs'
            record(category,rel,findings(raw,known,rel))
    stages={}
    for item in git('ls-files','--stage','-z').split(b'\0'):
        if not item:continue
        meta,path=item.split(b'\t',1);oid=meta.split()[1].decode();stages.setdefault(oid,[]).append(path.decode())
    for oid,kind,raw in objects(stages):
        for path in stages[oid]:record('index',path,findings(raw,known,path),oid)
    history_count=0
    if not args.no_history:
        history={}
        for line in git('rev-list','--objects','--all','--reflog').splitlines():
            oid,_,path=line.partition(b' ');history.setdefault(oid.decode(),path.decode('utf-8','replace'))
        for oid,kind,raw in objects(history):
            if kind not in ('blob','commit','tag'):continue
            history_count+=1
            record('history',history[oid] or ('git '+kind+' '+oid),findings(raw,known,history[oid]),oid)
    # Known credential checks on diff are exact. Generic literals are inspected
    # per file above, where the reviewed test-fixture allowlist has path scope.
    raw_diff=git('diff','--binary','HEAD')
    record('diff','git diff HEAD',sorted({kind for value,kind in known.items() if value in raw_diff}))
    ignored=subprocess.run(['git','check-ignore','-q','.env'],cwd=ROOT).returncode==0
    if '.env' in tracked or not ignored:record('placement','.env',['env must be ignored and untracked'])
    counts={category:sum(r['category']==category for r in records) for category in ['tracked','untracked','index','history','artifacts','logs','diff','placement']}
    report={'counts':counts,'work_files_scanned':work_count,'history_objects_scanned':history_count,'env_ignored':ignored,'env_tracked':'.env' in tracked,'findings':records,'excluded_dependency_directories':sorted(SKIP)}
    target=ROOT/args.report;target.parent.mkdir(parents=True,exist_ok=True);target.write_text(json.dumps(report,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
    for category,count in counts.items():print(category+' secret matches: '+str(count))
    for r in records:print(json.dumps(r,ensure_ascii=False))
    print('audit files='+str(work_count)+' history objects='+str(history_count))
    return 1 if records else 0

if __name__=='__main__':
    try:sys.exit(main())
    except Exception as error:
        print('Secret audit failed safely: '+type(error).__name__);sys.exit(2)
