"""Build the report exclusively from saved measurements. Never calls a model."""
import collections
import hashlib
import json
import math
import pathlib

root = pathlib.Path(__file__).resolve().parents[1]
out = root / 'artifacts/gigachat'
models = [('Ultra','GigaChat-3-Ultra'),('Max','GigaChat-2-Max'),('Pro','GigaChat-3-Pro'),('Lite','GigaChat-2')]
metric_names = [('action_accuracy','Action'),('kind_accuracy','Kind'),('title_accuracy','Title'),('query_accuracy','Query'),('target_accuracy','Target semantic'),('accepted_target_accuracy','Target accepted + scope'),('changes_accuracy','Changes'),('temporal_extraction_accuracy','Temporal extraction'),('temporal_accuracy','Temporal domain'),('full_semantic_accuracy','Full semantic'),('domain_command_accuracy','domain.Command')]

def load(path):
    path = root / path
    return json.loads(path.read_text(encoding='utf-8-sig')) if path.exists() else None

def metric(report, key, group='metrics', counts=False):
    m = report.get(group,{}).get(key) if report else None
    if not m or not m['total']:
        return 'N/A'
    value = f"{100*m['rate']:.2f}%"
    return f"{m['correct']}/{m['total']} = {value}" if counts else value

def complete(report, count):
    return report is not None and report['total']==count and not report.get('aborted_reason')

def performance(report):
    if not report:
        return {}
    values = [r['normalization'] for r in report.get('results',[]) if r['normalization'].get('total_tokens',0)>0]
    return {'cold_ms':report['latency_ms']['cold'], 'warm':report['latency_ms']['warm'], 'usage_covered_requests':len(values), 'average_input_tokens':sum(r['input_tokens'] for r in values)/len(values) if values else None, 'average_output_tokens':sum(r['output_tokens'] for r in values)/len(values) if values else None, 'average_total_tokens':sum(r['total_tokens'] for r in values)/len(values) if values else None, 'input_tokens':sum(r['input_tokens'] for r in values), 'output_tokens':sum(r['output_tokens'] for r in values), 'total_tokens':sum(r['total_tokens'] for r in values)}

qwen = load('testdata/semantic/qwen_baseline.json')
reports = {label:load(f'artifacts/gigachat/heldout-{model}.json') for label,model in models}
held = {label:report if complete(report,208) else None for label,report in reports.items()}
dev = {label:load(f'artifacts/gigachat/dev-final-{model}.json') or load(f'artifacts/gigachat/dev-{model}.json') for label,model in models}
live_dev = {label:load(f'artifacts/gigachat/dev-{model}.json') for label,model in models}
mandatory = {label:load(f'artifacts/gigachat/mandatory-{model}.json') for label,model in models}
perfs = {label:performance(report) for label,report in held.items()}
for label,p in perfs.items():
    if p:
        p['token_accounting'] = 'API prompt_tokens/completion_tokens/total_tokens; cached-prefix tokens were not collected per held-out request'
protocol = load('artifacts/gigachat/heldout-protocol.json')
assert protocol and all(hashlib.sha256((root/name).read_bytes()).hexdigest()==digest for name,digest in protocol['sha256'].items()), 'Frozen evaluation code/contract changed'

def qualifies(report):
    return report and report['unsafe_mutations']==0 and all(report['core_metrics'].get(key,{}).get('rate',0)>=threshold for key,threshold in [('domain_command_accuracy',.97),('action_accuracy',.99),('title_accuracy',.98),('query_accuracy',.98)])

qualified = [label for label,report in held.items() if qualifies(report)]
ranked = sorted([label for label,report in held.items() if report],key=lambda label:(held[label]['core_metrics']['domain_command_accuracy']['rate'],held[label]['metrics']['domain_command_accuracy']['rate']),reverse=True)
recommendation = next((label for label in ranked if label in qualified),ranked[0] if ranked else None)
table = ['| Metric | Qwen 4B | GigaChat Ultra | Max | Pro | Lite |','|---|---:|---:|---:|---:|---:|']
for key,label in metric_names:
    table.append('| '+label+' | '+metric(qwen,key)+' | '+' | '.join(metric(held[name],key) for name,_ in models)+' |')
table.append('| CORE domain.Command | '+metric(qwen,'domain_command_accuracy','core_metrics')+' | '+' | '.join(metric(held[name],'domain_command_accuracy','core_metrics') for name,_ in models)+' |')
table.append('| Unsafe accepted | '+str(qwen['unsafe_mutations'])+' | '+' | '.join(str(held[name]['unsafe_mutations']) if held[name] else 'N/A' for name,_ in models)+' |')
for key,label in [('p50','Warm p50'),('p95','Warm p95')]:
    values = [f"{qwen['latency_ms']['warm'][key]/1000:.3f} s"]+[f"{perfs[name]['warm'][key]/1000:.3f} s" if held[name] else 'N/A' for name,_ in models]
    table.append('| '+label+' | '+' | '.join(values)+' |')
table.append('| Avg tokens/request (API) | '+f"{qwen['average_total_tokens']:.1f}"+' | '+' | '.join((f"{perfs[name]['average_total_tokens']:.1f}"+('*' if name=='Pro' else '')) if held[name] and perfs[name]['average_total_tokens'] is not None else 'N/A' for name,_ in models)+' |')
table_text = '\n'.join(table)
(out/'comparison.md').write_text(table_text+'\n',encoding='utf-8')

lines = ['# GigaChat SemanticCommand: локальная интеграция и оценка','', '15 октября 2026, Europe/Moscow. Production VPS, production env/services и production DB не использовались. Qwen inference не запускался.','', '## Результат','']
if qualified:
    lines += ['Порог hackathon production достигнут: '+', '.join(qualified)+'. Перед deploy требуется ротация credentials и проверка production environment.']
else:
    lines += ['Ни одна из полностью оценённых моделей не достигла всех заданных CORE-порогов. Рекомендуемый кандидат для дальнейшей локальной работы: **'+str(recommendation)+'**. Production deploy по этим результатам не рекомендуется.']
lines += ['', '## Архитектура и безопасность','', '`Voice/Text → conservative Go fast path → SemanticProvider.Parse → strict SemanticCommand JSON → grounding/safety guards → deterministic Go temporal parsing + domain.Command validation → existing assistant.Service/executor → PostgreSQL`.', '', 'В текущем main отсутствовали прежние semantic-файлы: необходимые SemanticCommand, prompt, temporal helpers, корпуса и scoring восстановлены из `D:/assistantai-backups/20261015-sync/local-source-before.tar.gz`. Domain DTO не менялся. В executor восстановлены pending-slot context и атомарный rename после поиска/validation/conflict checks. Frontend передаёт только source=text|voice; профиль, история, БД и now/timezone модели не передаются.', '', 'Rules fast path ограничен однозначными запросами списка; подтверждения и slot replies остаются в Go. Evaluation вызывает Normalize напрямую, без fast path, чтобы измерять модель. После model output не вызывается полный intent/title parser; Go разрешает temporal roles отдельно. ID извлекаются только из исходных #/№/номер; ID отсутствуют в schema.', '', 'Один reusable http.Client с pooling, context/timeout, ограничением body 1 MiB и запретом redirects. GIGACHAT_MAX_CONCURRENT=1 ограничивает только provider. Очередь учитывает ctx.Done. Semaphore действует внутри одного процесса; нескольким replicas Freemium нужна общая очередь.', '', 'Authorization Key используется приоритетно; Client ID/Secret сохранены локально, не применяются для OAuth. Access token живёт только в памяти вместе с expires_at; single refresh разделяется между goroutines, обновление за минуту до expiry. 401 инвалидирует token и допускает один повтор. 429/5xx/network имеют bounded retry, максимум два повтора; ошибки не включают HTTP body, headers, credential или token.', '', 'TLS verification включена; дополнительный CA скачан с официального HTTPS-источника и подключён через GIGACHAT_CA_FILE, без изменения системного trust store. Не используются InsecureSkipVerify или незашифрованные реальные endpoints.', '', 'GigaChat получает response_format={type:json_schema,schema:SemanticSchema(),strict:true}. Контракт един для GigaChat/Ollama; источник — internal/nlp/semantic.go, точный export с prompt — testdata/semantic/structured_contract.json. Проверяются required fields, unknown/duplicate keys, enum/shape, trailing JSON, SemanticCommand invariants, grounding и guards. Invalid/unavailable output fail-closed.', '', 'Cache содержит только model JSON и usage/latency; key=sha256(provider+model+prompt+schema+source+input), путь artifacts/cache/semantic/gigachat/<hash>.json. Cached JSON повторно декодируется. HTTP/OAuth envelopes и secrets не сохраняются.', '', '`.env` ignored/not tracked; `.env.local`, `.env.*.local`, artifacts/cache/semantic/ ignored. Секретный TXT не копировался в repository и не изменялся. Automated scan сравнивает значения локальных credentials с Git-visible файлами/артефактами без печати значений. **Инцидент сессии:** при первичном чтении секретный TXT ошибочно попал в tool output. Перед production требуется ротация credentials.', '', '## Connectivity и реально доступные модели','']
connection = load('artifacts/gigachat/connectivity.json')
lines += ['OAuth, TLS, models endpoint и structured request: успешно. Строгость Go boundary дополнительно подтверждена обязательным набором и unit tests.', '', 'Account model IDs: '+', '.join('`'+value+'`' for value in connection['models'])+'.', '', f"Connectivity request Ultra: {connection['latency_ms']/1000:.3f} s; input {connection['usage']['prompt_tokens']}, output {connection['usage']['completion_tokens']}, total {connection['usage']['total_tokens']} tokens.", '', 'Для классов сравнения выбраны реальные IDs: Ultra=GigaChat-3-Ultra, Max=GigaChat-2-Max, Pro=GigaChat-3-Pro, Lite=GigaChat-2. GigaChat-2-Pro и Lightning перечислены API; отдельный дополнительный прогон не выполнялся.', '', '## Методика и freeze','', 'DEV 572 восстановлен byte-for-byte; sampling seed=20261015. DEV screening: 72 фразы, включая task/reminder/meeting/list/search/update/delete/complete/recurrence/ASR/ambiguity. Состав: 34 create, 6 list, 10 search, 6 update, 5 delete, 5 complete, 6 help; 4 ASR-tagged. Mandatory тексты исключены из sample. В старом DEV нет полного SemanticCommand gold, поэтому full semantic/changes/temporal extraction на DEV не размечены.', '', 'Held-out 208 SHA-256: `704b0c5324dab12bf5673afeab92df1c7507d7555f1761e619456eb8ab093b1e`. Prompt/schema исходного Qwen baseline сохранены без tuning. После DEV финальный Go mapping повторно проверен только из cache. До первого held-out зафиксированы finalist list, scoring, provider/validation/temporal code hashes в artifacts/gigachat/heldout-protocol.json. Finalists: '+', '.join(protocol['finalists'])+'.', '', 'Held-out не использовался для tuning. После старта held-out prompt/schema/зафиксированный Go code не менялись; повторный inference и компактный prompt experiment не выполнялись. Часы Go: 2026-10-15 12:30:17+03:00, Europe/Moscow. Это синтетические текстовые фразы, не live-ASR и не оценка WER.', '', 'CORE A–D: create task/reminder без recurrence, create meeting, list, search. BONUS: update/delete/complete/recurrence/ambiguity. Action/kind/title/query/target/changes/temporal extraction/full semantic оцениваются на model boundary до Go guards; target accepted + scope, temporal domain и domain.Command — после Go. Сравнение наследует scoring Qwen; title/query требуют нормализованного точного совпадения, clarification wording не оценивается. Unsafe accepted включает мутацию вместо read-only/help, invented ID и неверный mutation target/kind/scope; это проверка данного корпуса, не доказательство универсальной безопасности.', '', 'Latency: concurrency=1. Cold обозначает первый запрос нового клиента с OAuth/новым соединением; hosted model cold-start независимо измерить невозможно. Warm p50/p95 — nearest-rank остальных live requests. Cache hits исключены из latency. API usage отражает возвращённые prompt/completion/total tokens; запросы, завершившиеся timeout до usage, не получают выдуманный token count.', '', '## Mandatory 13','', '| Model | domain.Command | Full semantic | Unsafe accepted |','|---|---:|---:|---:|']
for label,_ in models:
    r=mandatory[label]
    lines.append('| '+label+' | '+metric(r,'domain_command_accuracy',counts=True)+' | '+metric(r,'full_semantic_accuracy',counts=True)+' | '+(str(r['unsafe_mutations']) if r else 'N/A')+' |')
lines += ['', 'Ultra: все 13 сохранённых реальных ответов дополнительно прошли существующий executor на изолированных SQLite fixtures (create/list/search/update/delete confirmation/complete/rename), без новых model calls. CSV 1000 событий проверен в отдельной test DB: import, period list, search и fuzzy search; CSV не изменялся.', '', '## DEV screening','', '| Model | Completed | Action | CORE domain.Command | domain.Command | warm p50 / p95 |','|---|---:|---:|---:|---:|---:|']
for label,_ in models:
    r=dev[label]; live=live_dev[label]
    if complete(r,72):
        latency=live['latency_ms']['warm']
        lines.append(f"| {label} | 72/72 | {metric(r,'action_accuracy',counts=True)} | {metric(r,'domain_command_accuracy','core_metrics',True)} | {metric(r,'domain_command_accuracy',counts=True)} | {latency['p50']/1000:.3f} / {latency['p95']/1000:.3f} s |")
    else:
        lines.append(f"| {label} | {str(r['total'])+'/72' if r else 'diagnostics only'} | N/A | N/A | N/A | N/A |")
max_probe = load('artifacts/gigachat/max-timeout-probe.json')
lines += ['', 'Max: при timeout=30s ответы не завершались; отдельный probe с timeout=90s вернул невалидный SemanticCommand через '+f"{max_probe['results'][0]['latency_ms']/1000:.3f}"+' s. Полный DEV72 не выполнялся после диагностики, полный held-out отсутствует. Lite: три последовательных timeout по 30s, прогон остановлен после 3/72. Их показатели качества не оцениваются как 0/72 и обозначены N/A.', '', '## Frozen held-out 208: сравнение','',table_text,'', 'Qwen quality — сохранённый final Go report; performance/usage — сохранённый исходный live held-out report. Provenance и hashes в testdata/semantic/qwen_baseline.json. Qwen не запускался.', '', '## CORE, BONUS и ошибки','', '| Model | CORE action | CORE title | CORE query | CORE domain | BONUS domain | False unknown | Validation/model errors | Unsafe proposals | Unsafe accepted |','|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|']
for label,_ in models:
    r=held[label]
    if r:
        lines.append('| '+label+' | '+' | '.join([metric(r,'action_accuracy','core_metrics',True),metric(r,'title_accuracy','core_metrics',True),metric(r,'query_accuracy','core_metrics',True),metric(r,'domain_command_accuracy','core_metrics',True),metric(r,'domain_command_accuracy','bonus_metrics',True),str(r['false_unknown_count']),str(r['model_errors']),str(r['unsafe_proposals']),str(r['unsafe_mutations'])])+' |')
for label,_ in models:
    r=held[label]
    if not r:continue
    lines += ['', '### '+label+' remaining failures','', 'Failure categories: '+json.dumps(r['failure_categories'],ensure_ascii=False)+'.']
    strict = sum(row.get('error')=='invalid SemanticCommand JSON' for row in r['results'])
    transport = sum(bool(row.get('error')) and not row['normalization'].get('raw') for row in r['results'])
    mapping = sum(bool(row.get('error')) and bool(row['normalization'].get('raw')) and row.get('error')!='invalid SemanticCommand JSON' for row in r['results'])
    guards = sum(bool(row['normalization'].get('guard_reason')) and not row.get('error') for row in r['results'])
    lines += ['', f'Strict response validation failures: **{strict}**; Go temporal/mapping failures: **{mapping}**; safety guard rejections: **{guards}**; transport/envelope failures: **{transport}**.']
    validation=collections.Counter(row.get('error') or row['normalization'].get('guard_reason') or 'semantic/domain mismatch' for row in r['results'] if not row['full_parse_correct'])
    lines += ['', 'Reasons: '+json.dumps(dict(validation),ensure_ascii=False)+'. Полные cases и outputs сохраняются только в локальном JSON report; по ним prompt/schema не менялись.']
lines += ['', '## Tokens и производительность','', '| Model | First client request | Input avg (API) | Output avg | Total avg (API) | Usage coverage | Input total (API) | Output total |','|---|---:|---:|---:|---:|---:|---:|---:|']
for label,_ in models:
    p=perfs[label]
    if not p:continue
    lines.append(f"| {label} | {p['cold_ms']/1000:.3f} s | {p['average_input_tokens']:.1f} | {p['average_output_tokens']:.1f} | {p['average_total_tokens']:.1f} | {p['usage_covered_requests']}/208 | {p['input_tokens']} | {p['output_tokens']} |")
lines += ['', 'Baseline prompt+schema дают около 2,3 тыс. input tokens/request по connectivity. Полная схема дополнительно передаётся как API response_format; server token accounting может учитывать её иначе. Компактный prompt/schema experiment не выполнялся: сначала сохранён исходный baseline.', '', '## Проверки и production решение','', '`go test -count=1 ./...`: PASS. `go vet ./...`: PASS. Targeted token/strict JSON/timeout/retry/cancellation/semaphore/cache/grounding/ID tests: PASS. Обычные тесты не обращаются к API; live test требует GIGACHAT_INTEGRATION=1. Race не запускался: gcc не найден. Frontend: tsc --noEmit и 27 App/voice tests PASS; полный frontend suite не запускался.', '', 'Критерии: CORE domain >=97%, CORE action >=99%, CORE title/query >=98% каждый, unsafe accepted=0. Выполнили все критерии: '+(', '.join(qualified) if qualified else 'ни одна модель')+'.', '', 'Интеграция и opt-in production override подготовлены. Production не изменён. Следующие настройки описаны в [GIGACHAT_CONFIGURATION.md](GIGACHAT_CONFIGURATION.md). При недостигнутом CORE-пороге нужны улучшения на DEV и новый fresh held-out для новой честной оценки; текущий held-out не становится независимым после повторного использования.', '', 'Источники API: [OAuth](https://developers.sber.ru/docs/ru/gigachat/api/reference/rest/gigachat-api), [structured output](https://developers.sber.ru/docs/ru/gigachat/guides/structured-output), [CA](https://developers.sber.ru/docs/ru/gigachat/certificates).']
accounting = load('artifacts/gigachat/token-accounting-GigaChat-3-Pro.json')
if accounting:
    usage=accounting['usage']
    note=f"* Pro: API total_tokens исключает precached_prompt_tokens. В отдельном DEV/connectivity probe после held-out API вернул prompt={usage['prompt_tokens']}, cached-prefix={usage['precached_prompt_tokens']}, completion={usage['completion_tokens']}, total={usage['total_tokens']}. Полный вход этого probe: {usage['prompt_tokens']+usage['precached_prompt_tokens']} токенов. Поэтому 211,0 API tokens/request нельзя считать полным размером контекста или напрямую сравнивать с Ultra для оценки стоимости. Cached-prefix не был сохранён для каждого held-out request; его среднее на held-out — N/A. Probe не менял prompt/schema и не использовался для повторной оценки held-out."
    lines[lines.index(table_text)+1:lines.index(table_text)+1]=['',note]
    perfs['Pro']['accounting_probe_usage']=usage
(root/'docs/GIGACHAT_SEMANTIC_REPORT.md').write_text('\n'.join(lines)+'\n',encoding='utf-8')
summary = {'recommendation':recommendation,'production_qualified':qualified,'models':connection['models'],'heldout':{name:({'metrics':r['metrics'],'core_metrics':r['core_metrics'],'bonus_metrics':r['bonus_metrics'],'unsafe_mutations':r['unsafe_mutations'],'performance':perfs[name]} if r else None) for name,r in held.items()}}
(out/'summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2)+'\n',encoding='utf-8')
print(json.dumps({'recommended':recommendation,'production_qualified':qualified,'heldout_completed':[name for name,r in held.items() if r]},ensure_ascii=False))
