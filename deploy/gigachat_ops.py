"""Root-side release operations. Input is private JSON on stdin; stdout is metadata.

Never pass credential values as argv, print command output, or persist the input.
The audit accepts hashes only. The install action writes a key only to the
root-owned production env, after a verified database/config backup exists.
"""
import base64
import datetime
import gzip
import hashlib
import io
import json
import os
import pathlib
import re
import shutil
import ssl
import subprocess
import sys
import tarfile
import time
import urllib.request
import uuid

CURRENT=pathlib.Path('/opt/rhythm/current')
RELEASES=pathlib.Path('/opt/rhythm/releases')
ENV=pathlib.Path('/etc/rhythm/production.env')
UNIT=pathlib.Path('/etc/systemd/system/rhythm.service')
CA=pathlib.Path('/etc/rhythm/certs/gigachat-root.pem')
NGINX=pathlib.Path('/etc/nginx/conf.d/rhythm.conf')

def run(args,timeout=120,extra_env=None):
    result=subprocess.run(args,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=timeout,env=dict(os.environ,**(extra_env or {})))
    if result.returncode:raise RuntimeError('operation failed: '+pathlib.Path(args[0]).name)
    return result.stdout

def env_values():
    result={}
    for line in ENV.read_text().splitlines():
        match=re.match(r'^\s*([A-Z][A-Z0-9_]*)\s*=\s*(.*)$',line)
        if match:result[match[1]]=match[2].strip().strip('"').strip("'")
    return result

def write_env(updates):
    lines=[line for line in ENV.read_text().splitlines() if not re.match(r'^\s*(?:'+'|'.join(map(re.escape,updates))+r')\s*=',line)]
    lines += [key+'='+value for key,value in updates.items() if value is not None]
    temporary=ENV.with_name('.production.env.gigachat-new')
    fd=os.open(temporary,os.O_WRONLY|os.O_CREAT|os.O_TRUNC,0o600)
    with os.fdopen(fd,'w') as stream:stream.write('\n'.join(lines)+'\n');stream.flush();os.fsync(stream.fileno())
    os.chmod(temporary,0o600);os.replace(temporary,ENV)

def compose(release,override=True):
    args=['docker','compose','--env-file',str(ENV),'-f',str(release/'deploy/compose.production.yaml')]
    if override:args += ['-f',str(release/'deploy/compose.gigachat.example.yaml')]
    return args

def containers():
    ids=run(['docker','ps','-aq','--filter','label=com.docker.compose.project=rhythm']).decode().split()
    return json.loads(run(['docker','inspect',*ids])) if ids else []

def container_summary(items):
    return [{'service':v['Config']['Labels'].get('com.docker.compose.service'),'state':v['State']['Status'],'health':v['State'].get('Health',{}).get('Status'),'restarts':v['RestartCount'],'oom':v['State'].get('OOMKilled',False),'image':v['Config']['Image']} for v in items]

def read_json(url,body=None,headers=None,context=None):
    req=urllib.request.Request(url,data=body,headers=headers or {})
    with urllib.request.urlopen(req,context=context,timeout=35) as response:
        raw=response.read((1<<20)+1)
        if len(raw)>1<<20:raise RuntimeError('response limit')
        return json.loads(raw)

def sudo_input_logging():
    files=[pathlib.Path('/etc/sudoers')]+list(pathlib.Path('/etc/sudoers.d').glob('*'))
    return any(re.search(r'(?m)^[^#\n]*(?<![!\w])log_input\b',p.read_text(errors='replace')) for p in files if p.is_file())

def audit(payload):
    known={entry['sha256']:entry['kind'] for entry in payload.get('credential_hashes',[])}
    for key,value in env_values().items():
        if len(value)>=12 and re.search(r'(?:SECRET|PASSWORD|PRIVATE_KEY|AUTHORIZATION_KEY|API_KEY)$',key):known[hashlib.sha256(value.encode()).hexdigest()]='production secret:'+key
    matches=[]
    def scan_bytes(raw,path,category):
        found=set()
        for candidate in re.findall(rb'[A-Za-z0-9_+/.=-]{24,}',raw):
            variants=[candidate]
            if b'=' in candidate and re.fullmatch(rb'[A-Za-z_][A-Za-z0-9_]*',candidate.partition(b'=')[0]):variants.append(candidate.partition(b'=')[2])
            for value in variants:
                digest=hashlib.sha256(value).hexdigest()
                if digest in known:found.add(known[digest])
        if re.search(rb'eyJ[A-Za-z0-9_-]{16,}\.[A-Za-z0-9_-]{16,}\.[A-Za-z0-9_-]{16,}',raw):found.add('JWT/access-token candidate')
        if found:matches.append({'category':category,'path':str(path),'types':sorted(found)})
    def scan_file(path,category):
        if path.is_symlink() or not path.is_file():return
        if path.name.endswith(('.tar.gz','.tgz')):
            try:
                with tarfile.open(path,'r:gz') as archive:
                    for member in archive:
                        if member.isfile() and member.size<32*1024*1024:
                            stream=archive.extractfile(member)
                            if stream:scan_bytes(stream.read(),str(path)+'!'+member.name,category)
                return
            except (tarfile.TarError,OSError):return
        opener=gzip.open if path.suffix=='.gz' else open
        try:
            with opener(path,'rb') as stream:
                carry=b''
                while True:
                    chunk=stream.read(1<<20)
                    if not chunk:break
                    scan_bytes(carry+chunk,path,category);carry=chunk[-8192:]
        except OSError:raise RuntimeError('audit read failed')
    for folder,dirs,files in os.walk('/opt/rhythm'):
        dirs[:]=[d for d in dirs if d not in {'.git','node_modules','.venv','__pycache__'}]
        for name in files:
            path=pathlib.Path(folder)/name
            scan_file(path,'backup' if any(x in path.name.lower() for x in ['backup','.tar','.dump']) else ('compose' if path.suffix in ('.yaml','.yml') else 'repo'))
    for folder in ['/var/log/nginx','/var/log/rhythm']:
        directory=pathlib.Path(folder)
        if directory.exists():
            for path in directory.rglob('*'):scan_file(path,'logs')
    items=containers()
    for item in items:
        log=pathlib.Path(item.get('LogPath','/nonexistent'))
        for path in log.parent.glob(log.name+'*'):scan_file(path,'logs')
        # Image configuration must not contain runtime secrets.
        image=json.loads(run(['docker','image','inspect',item['Image']]))[0]
        scan_bytes(json.dumps(image.get('Config',{})).encode(),'docker image '+item['Image'],'image')
    for path in [pathlib.Path('/root/.bash_history'),pathlib.Path('/home/rhythm-admin/.bash_history')]:scan_file(path,'logs')
    scan_bytes(run(['journalctl','-u','rhythm','--no-pager','-n','2000']), 'journal:rhythm','logs')
    # Report only counts/types. Production env and container Env are approved
    # secret environments, checked for placement/state instead of displayed.
    config=env_values();key=config.get('GIGACHAT_AUTHORIZATION_KEY','')
    key_state='absent' if not key else known.get(hashlib.sha256(key.encode()).hexdigest(),'other configured credential')
    current=CURRENT.resolve();stat=ENV.stat();memory={}
    for line in pathlib.Path('/proc/meminfo').read_text().splitlines():
        k,v=line.split(':',1)
        if k in ('MemTotal','MemAvailable'):memory[k+'_kib']=int(v.split()[0])
    unique={(m['category'],m['path'],tuple(m['types'])):m for m in matches};matches=list(unique.values())
    return {'current_release':str(current),'revision':(current/'REVISION').read_text().strip() if (current/'REVISION').exists() else None,'containers':container_summary(items),'disk_free_bytes':shutil.disk_usage('/opt').free, 'memory':memory,'env_location':str(ENV),'env_mode':oct(stat.st_mode&0o777),'env_root_owned':stat.st_uid==0,'giga_env_state':key_state,'sudo_input_logging':sudo_input_logging(),'counts':{k:sum(m['category']==k for m in matches) for k in ['repo','compose','backup','logs','image']},'findings':matches,'public_url':config.get('PUBLIC_URL','')}

def backup(payload):
    previous=CURRENT.resolve();stamp=datetime.datetime.now(datetime.timezone.utc).strftime('%Y%m%dT%H%M%SZ')
    folder=pathlib.Path('/var/backups/rhythm')/('gigachat-release-'+stamp)
    folder.mkdir(mode=0o700)
    run(['sh',str(previous/'deploy/backup.sh')],timeout=180)
    dumps=sorted(pathlib.Path('/var/backups/rhythm').glob('postgres-*.dump'),key=lambda p:p.stat().st_mtime)
    if not dumps or not dumps[-1].stat().st_size:raise RuntimeError('database backup missing')
    pg=next(item for item in containers() if item['Config']['Labels'].get('com.docker.compose.service')=='postgres')
    with dumps[-1].open('rb') as stream:
        verified=subprocess.run(['docker','exec','-i',pg['Id'],'pg_restore','--list'],stdin=stream,stdout=subprocess.PIPE,stderr=subprocess.PIPE,timeout=60)
    if verified.returncode:raise RuntimeError('database backup validation failed')
    shutil.copy2(ENV,folder/'production.env');os.chmod(folder/'production.env',0o600)
    shutil.copy2(UNIT,folder/'rhythm.service')
    shutil.copy2(NGINX,folder/'nginx.conf')
    (folder/'compose.json').write_bytes(run(compose(previous)+['config','--format','json']));os.chmod(folder/'compose.json',0o600)
    (folder/'containers.json').write_text(json.dumps(container_summary(containers())))
    (folder/'env-schema.json').write_text(json.dumps(sorted(env_values())))
    state={'previous_release':str(previous),'previous_revision':(previous/'REVISION').read_text().strip(),'previous_tag':env_values().get('RELEASE_TAG','current'),'database_dump':str(dumps[-1]),'database_dump_bytes':dumps[-1].stat().st_size,'backup_dir':str(folder)}
    (folder/'state.json').write_text(json.dumps(state));os.chmod(folder/'state.json',0o600)
    return state

def state(payload):
    path=pathlib.Path(payload['backup_dir']).resolve()
    if not path.is_relative_to(pathlib.Path('/var/backups/rhythm')):raise RuntimeError('invalid backup directory')
    return json.loads((path/'state.json').read_text())

def prepare(payload):
    saved=state(payload)
    if CURRENT.resolve()!=pathlib.Path(saved['previous_release']):raise RuntimeError('current release changed')
    if sudo_input_logging():raise RuntimeError('sudo input logging must be disabled before secret transfer')
    sha=payload['revision'];tag=payload['release']
    if not re.fullmatch(r'[0-9a-f]{40}',sha) or not re.fullmatch(r'[a-z0-9-]{1,96}',tag):raise RuntimeError('invalid release identity')
    archive=pathlib.Path(payload['archive']).resolve()
    if archive.parent!=pathlib.Path('/tmp') or hashlib.sha256(archive.read_bytes()).hexdigest()!=payload['archive_sha256']:raise RuntimeError('archive verification failed')
    release=RELEASES/tag
    if release.exists():raise RuntimeError('release already exists')
    release.mkdir(mode=0o755)
    with tarfile.open(archive) as data:
        for member in data:
            path=(release/member.name).resolve()
            if not path.is_relative_to(release) or member.issym() or member.islnk():raise RuntimeError('unsafe archive member')
        data.extractall(release,filter='data')
    (release/'REVISION').write_text(sha+'\n')
    if payload.get('preserve_credentials'):
        config=env_values()
        if not config.get('GIGACHAT_AUTHORIZATION_KEY') or not pathlib.Path(config.get('GIGACHAT_CA_FILE_HOST','')).is_file():raise RuntimeError('existing GigaChat configuration missing')
        run(compose(release)+['config','--quiet'])
        return {'release':str(release),'revision':sha,'existing_credentials_preserved':True,'compose_config':'PASS'}
    key=payload['authorization_key']
    if not key or re.search(r'\s',key):raise RuntimeError('invalid authorization configuration')
    CA.parent.mkdir(parents=True,exist_ok=True,mode=0o755)
    pem=payload['ca_pem'];ssl.PEM_cert_to_DER_cert(pem)
    CA.write_text(pem);os.chmod(CA,0o644)
    write_env({'GENA_SEMANTIC_PROVIDER':'gigachat','GIGACHAT_AUTHORIZATION_KEY':key,'GIGACHAT_CLIENT_ID':None,'GIGACHAT_CLIENT_SECRET':None,'GIGACHAT_SCOPE':'GIGACHAT_API_PERS','GIGACHAT_BASE_URL':'https://api.giga.chat','GIGACHAT_MODEL':'GigaChat-3-Ultra','GIGACHAT_MAX_CONCURRENT':'1','GIGACHAT_TIMEOUT':'30s','GIGACHAT_CA_FILE_HOST':str(CA)})
    run(compose(release)+['config','--quiet'])
    return {'release':str(release),'revision':sha,'env_mode':oct(ENV.stat().st_mode&0o777),'ca_verified':True,'compose_config':'PASS'}

def build(payload):
    release=RELEASES/payload['release']
    run(compose(release)+['build','app'],timeout=1200,extra_env={'RELEASE_TAG':payload['release']})
    return {'build':'PASS','release':str(release)}

def configure_stt(payload):
    state(payload)  # A verified release backup must exist before secret transfer.
    if sudo_input_logging():raise RuntimeError('sudo input logging blocks secret transfer')
    key=payload['groq_api_key']
    if not re.fullmatch(r'[A-Za-z0-9_-]{16,256}',key):raise RuntimeError('invalid Groq key format')
    write_env({'GROQ_API_KEY':key,'GENA_STT_PROVIDER':'groq','GROQ_STT_MODEL':'whisper-large-v3-turbo'})
    return {'stt_provider':'groq','model':'whisper-large-v3-turbo','env_mode':oct(ENV.stat().st_mode&0o777)}

def activate(payload):
    saved=state(payload);release=RELEASES/payload['release']
    if not release.is_dir() or (release/'REVISION').read_text().strip()!=payload['revision']:raise RuntimeError('release revision mismatch')
    write_env({'RELEASE_TAG':payload['release'],'GENA_NLU_MODE':payload.get('nlu_mode','simple_llm')})
    temporary=CURRENT.with_name('.current-gigachat-new')
    temporary.symlink_to(release,target_is_directory=True);os.replace(temporary,CURRENT)
    shutil.copy2(release/'deploy/rhythm.service',UNIT);run(['systemctl','daemon-reload'])
    run(compose(release)+['up','-d','--no-deps','--wait','--wait-timeout','180','app'],timeout=240)
    # This upload route needs its larger body limit and disk buffering disabled.
    shutil.copy2(release/'deploy/nginx.conf',NGINX)
    run(['nginx','-t']);run(['systemctl','reload','nginx'])
    return check(payload)

def check(payload):
    config=env_values();items=containers();summary=container_summary(items)
    healthy=all(v['state']=='running' and v['health']=='healthy' and not v['oom'] and v['restarts']==0 for v in summary if v['service'] in ('app','postgres'))
    if not healthy or {v['service'] for v in summary}!={'app','postgres'}:raise RuntimeError('container health failed')
    run(['nginx','-t']);nginx=run(['systemctl','is-active','nginx']).decode().strip()=='active'
    health=read_json(config['PUBLIC_URL'].rstrip('/')+'/health')
    with urllib.request.urlopen(config['PUBLIC_URL'],timeout=20) as response:html=response.read(1<<20).decode()
    app=next(v for v in items if v['Config']['Labels'].get('com.docker.compose.service')=='app')
    runtime=dict(line.split('=',1) for line in app['Config']['Env'] if '=' in line)
    matches=runtime.get('GIGACHAT_AUTHORIZATION_KEY')==config.get('GIGACHAT_AUTHORIZATION_KEY') and bool(runtime.get('GIGACHAT_AUTHORIZATION_KEY'))
    mount=any(m['Destination']=='/etc/gigachat/ca.pem' and not m['RW'] for m in app['Mounts'])
    if not nginx or health.get('nlp_provider')!='gigachat' or not matches or not mount or 'id="root"' not in html:raise RuntimeError('public/provider health failed')
    if payload.get('nlu_mode') and runtime.get('GENA_NLU_MODE')!=payload['nlu_mode']:raise RuntimeError('runtime mode mismatch')
    return {'nlu_mode':runtime.get('GENA_NLU_MODE','semantic_plan_legacy'),'model':runtime.get('GIGACHAT_MODEL'),'concurrency':runtime.get('GIGACHAT_MAX_CONCURRENT'),'revision':(CURRENT/'REVISION').read_text().strip(),'containers':summary,'nginx':True,'https':True,'frontend_html':True,'provider':health['nlp_provider'],'runtime_key_matches_secret_env':True,'ca_mount_read_only':True,'public_url':config['PUBLIC_URL']}

def strict(value,schema):
    if value is None:
        if 'enum' in schema and None not in schema['enum']:raise ValueError('enum')
        if 'type' in schema and 'null' not in schema['type']:raise ValueError('null')
        return
    if 'enum' in schema and value not in schema['enum']:raise ValueError('enum')
    if 'properties' in schema:
        if not isinstance(value,dict) or set(value)!=set(schema['required']):raise ValueError('shape')
        for key,item in value.items():strict(item,schema['properties'][key])
    elif schema.get('type')=='boolean' and type(value)!=bool:raise ValueError('boolean')
    elif isinstance(schema.get('type'),list) and 'string' in schema['type']:
        if not isinstance(value,str) or not schema.get('minLength',0)<=len(value)<=schema.get('maxLength',100000):raise ValueError('string')

def giga(payload):
    config=env_values();context=ssl.create_default_context();context.load_verify_locations(cafile=str(CA))
    started=time.monotonic()
    auth=read_json('https://ngw.devices.sberbank.ru:9443/api/v2/oauth',b'scope=GIGACHAT_API_PERS',{'Authorization':'Basic '+config['GIGACHAT_AUTHORIZATION_KEY'],'RqUID':str(uuid.uuid4()),'Content-Type':'application/x-www-form-urlencoded'},context)
    headers={'Authorization':'Bearer '+auth['access_token'],'Content-Type':'application/json'}
    models=read_json('https://api.giga.chat/v1/models',headers=headers,context=context)
    if 'GigaChat-3-Ultra' not in [m['id'] for m in models['data']]:raise RuntimeError('Ultra is unavailable')
    contract=json.loads((CURRENT/'testdata/semantic/structured_contract.json').read_text())
    request={'model':'GigaChat-3-Ultra','messages':[{'role':'system','content':contract['system_prompt']+'\nJSON Schema:\n'+json.dumps(contract['schema'],ensure_ascii=False,separators=(',',':'))},{'role':'user','content':json.dumps({'source':'text','text':'Напомни завтра в 9 проверить почту'},ensure_ascii=False,separators=(',',':'))}],'stream':False,'temperature':0,'max_tokens':768,'response_format':{'type':'json_schema','schema':contract['schema'],'strict':True}}
    response=read_json('https://api.giga.chat/v1/chat/completions',json.dumps(request,ensure_ascii=False).encode(),headers,context)
    choice=response['choices'][0]
    if choice['finish_reason']!='stop' or choice['message']['role']!='assistant':raise RuntimeError('incomplete structured output')
    def unique(pairs):
        result={}
        for key,value in pairs:
            if key in result:raise ValueError('duplicate key')
            result[key]=value
        return result
    result=json.loads(choice['message']['content'],object_pairs_hook=unique);strict(result,contract['schema'])
    if result['status']!='ok' or result['action']!='create' or result['kind']!='reminder' or not result.get('title'):raise RuntimeError('semantic connectivity command failed')
    return {'oauth':'PASS','models':'PASS','ultra':'PASS','strict_semantic':'PASS','latency_ms':round((time.monotonic()-started)*1000,2),'usage':response.get('usage',{})}

def rollback(payload):
    saved=state(payload);previous=pathlib.Path(saved['previous_release'])
    shutil.copy2(pathlib.Path(saved['backup_dir'])/'production.env',ENV);os.chmod(ENV,0o600)
    # The retired GigaChat key must never be reinstated during rollback.
    if not payload.get('preserve_credentials'):
        write_env({'GIGACHAT_AUTHORIZATION_KEY':None,'GIGACHAT_CLIENT_ID':None,'GIGACHAT_CLIENT_SECRET':None,'GENA_SEMANTIC_PROVIDER':'rules'})
    shutil.copy2(pathlib.Path(saved['backup_dir'])/'rhythm.service',UNIT)
    nginx_backup=pathlib.Path(saved['backup_dir'])/'nginx.conf'
    if nginx_backup.exists():
        shutil.copy2(nginx_backup,NGINX);run(['nginx','-t']);run(['systemctl','reload','nginx'])
    temporary=CURRENT.with_name('.current-gigachat-rollback');temporary.symlink_to(previous,target_is_directory=True);os.replace(temporary,CURRENT)
    run(['systemctl','daemon-reload']);run(compose(previous,bool(payload.get('preserve_credentials')))+['up','-d','--no-deps','--wait','--wait-timeout','180','app'],timeout=240)
    return {'rollback':'PASS','revision':saved['previous_revision'],'containers':container_summary(containers()),'database_restored':False}

def main():
    if os.geteuid()!=0:raise RuntimeError('root required')
    payload=json.load(sys.stdin)
    actions={'audit':audit,'backup':backup,'prepare':prepare,'build':build,'configure_stt':configure_stt,'activate':activate,'check':check,'giga':giga,'rollback':rollback}
    result=actions[payload['action']](payload)
    print(json.dumps({'ok':True,'action':payload['action'],'result':result},ensure_ascii=False))

if __name__=='__main__':
    try:main()
    except Exception as error:
        print(json.dumps({'ok':False,'error_type':type(error).__name__}));sys.exit(1)
