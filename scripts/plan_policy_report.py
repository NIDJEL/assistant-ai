"""Summarize local software evaluations without rerunning any inference."""
import collections
import hashlib
import json
from pathlib import Path

ROOT=Path(__file__).resolve().parents[1]
ART=ROOT/'artifacts/plan'


def load(name):
    return json.loads((ART/name).read_text(encoding='utf8'))


def metric(report,key):
    m=report['metrics'][key]
    return f"{m['Correct']}/{m['Total']} = {m['Percent']:.2f}%"


def main():
    reports={name:load(path) for name,path in {
        'policy':'policy-dev-final-corrected.json', 'context':'context-dev-final.json',
        'legacy_reference':'policy-reference-final.json','bare_reference':'bare-reference-dev.json','v7':'heldout-v7-first.json'}.items()}
    focused_correct=sum(reports[n]['metrics']['end_to_end']['Correct'] for n in ['context','bare_reference'])
    focused_total=sum(reports[n]['Turns'] for n in ['context','bare_reference'])
    focused_percent=100*focused_correct/focused_total
    inventory=load('policy-inventory.json')
    v7=reports['v7']
    gate={
        'unsafe_zero':v7['unsafe_accepted_mutations']==0,
        'wrong_value_zero':v7['wrong_value_mutations']==0,
        'confirmed_wrong_value_zero':v7['confirmed_wrong_value_mutations']==0,
        'end_to_end_97':v7['metrics']['end_to_end']['Percent']>=97,
        'multi_turn_95':v7['metrics']['multi_turn_success']['Percent']>=95,
        'complex_95':v7['metrics']['complex_command_accuracy']['Percent']>=95,
        'legacy_reference_95':reports['legacy_reference']['metrics']['end_to_end']['Percent']>=95,
        'focused_reference_95':focused_percent>=95,
        'go_test_pass':'FAIL' not in (ART/'policy-go-test-final.log').read_text(encoding='utf8'),
        'go_vet_pass':not (ART/'policy-go-vet-final.log').read_text(encoding='utf8').strip(),
    }
    failures={}
    for name,r in reports.items():
        failures[name]=[dict(conversation=c['ID'],category=c['Category'],turn=i,message=t['message'],
                             error=t.get('error'),evidence_field=t.get('evidence_field'),
                             evidence_reason=t.get('evidence_reason'),checks=[k for k,v in t['checks'].items() if not v])
                        for c in r['results'] for i,t in enumerate(c['Turns']) if not t['pass']]
    guard_by_category={}
    for name,r in reports.items():
        guard_by_category[name]=dict(collections.Counter(c['Category'] for c in r['results'] for t in c['Turns'] if t.get('evidence_reason')))
    summary=dict(production_ready=all(gate.values()),gate=gate,
                 datasets={name:{k:v for k,v in r.items() if k not in ['results','frozen']} for name,r in reports.items()},
                 inventory=inventory,evidence_by_category=guard_by_category,failures=failures,
                 focused_reference_combined=dict(Correct=focused_correct,Total=focused_total,Percent=focused_percent),
                 adversarial_original=load('policy-adversarial-original48.json'),
                 adversarial_new=load('policy-adversarial-new20.json'),
                 legacy_root_causes=load('reference-legacy-root-causes.json'),
                 production_touched=False,v6_rerun=False,verifier_added=False)
    (ART/'policy-v7-summary.json').write_text(json.dumps(summary,ensure_ascii=False,indent=2),encoding='utf8')
    ready='YES' if summary['production_ready'] else 'NO'
    lines=['# Semantic evidence policy v2 и fresh v7','',f'**Production ready: {ready}. Deploy не выполнялся.** Production/VPS, production DB/env не затронуты. Frozen v6 не перезапускался и не переоценивался. Второго LLM verifier нет.','',
           '## Изменения','',
           'Централизованная policy проверяет provenance пользовательских ограничений mutation. Нейтральные any/null не требуют evidence; optional create/read-only evidence не блокирует команду. Canonical timer title допускается, произвольные придуманные названия по-прежнему отклоняются language grounding. Unicode EqualFold сохраняет границы слов и пробелы. Старые value mismatch guards сохранены.','',
           'Search normalization удаляет только пустые title/changes/schedule. Заполненный несовместимый payload отклоняется; kind/query модельных планов не исправляются. Read-only ordinal/last выбирает только свежий показанный результат. Safe clarification сохраняет свежий список и его фильтры; evidence mismatch mutation отзывает destructive pending. У неизвестных слов read-only поиск получает второй Snowball stem, как уже делал строгий TitleMatches. Mutation selection не изменён.','',
           '## Evidence policy','',
           'Реестр: 33 семейства полей, включая 24 потенциальных пользовательских ограничения mutation и 9 create/presentation/kind полей. В таблице — конкретные ненулевые значения; any/null переводят соответствующее поле в NotApplicable. Индекс исключения считается одним wildcard семейством. Kind mutation проверяется в composite batch.scope либо по типу выбранной записи, отдельная цитата kind не требуется.','',
           '| Операция | Required | OptionalNonBlocking | NotApplicable |','|---|---:|---:|---:|']
    for op,row in inventory.items():
        c=row['counts'];lines.append(f"| {op} | {c['Required']} | {c['OptionalNonBlocking']} | {c['NotApplicable']} |")
    lines+=['','Матрица полей: [policy-inventory.json](../artifacts/plan/policy-inventory.json).','',
            '## DEV','',
            '| Корпус | Диалоги / ходы | End-to-end | Multi-turn | Unsafe / wrong / confirmed wrong |','|---|---:|---:|---:|---:|']
    for name in ['policy','context','bare_reference','legacy_reference']:
        r=reports[name]
        lines.append(f"| {name} | {r['Conversations']} / {r['Turns']} | {metric(r,'end_to_end')} | {metric(r,'multi_turn_success')} | {r['unsafe_accepted_mutations']} / {r['wrong_value_mutations']} / {r['confirmed_wrong_value_mutations']} |")
    lines+=['',f'Focused reference суммарно: **{focused_correct}/{focused_total} = {focused_percent:.2f}%**, 56 conversations. Основные 50 дополнены 6 диалогами с буквальными короткими «её», «его», «ту» при одном или нескольких результатах.','',
            'Новые независимые DEV: `scripts/plan_policy_dev.py`. Первые результаты сохранены; финальные прогоны используют окончательный prompt/runtime. Offline raw-response replay финальным evaluator подтверждает accuracy без новых inference; сетевые latency берутся только из live runs. В policy gold исправлены только 8 Count-проверок: batch complete возвращает реально изменённые задачи, уже выполненная задача сохраняется и проверяется отдельно. Сообщения, seeds и ожидаемые write values не менялись; [аудит исправления](../artifacts/plan/policy-dev-gold-correction.json).','',
            'Таймеры: '+str(guard_by_category['policy'].get('timer',0))+' evidence rejections на 12 новых timer conversations. Neutral any/read/shift: '+str(sum(v for k,v in guard_by_category['policy'].items() if k.startswith('neutral_')))+' evidence rejections.','',
            'Старый reference 29/40 разобран по исходным raw plans: 6 wrong kind; 2 incompatible search payload; 2 последующих потери результата из-за неуспешного поиска; 1 model semantic failure. Wrong reference и strict JSON shape failure среди этих 11 ошибок отсутствуют (payload compatibility учитывается отдельно). [Классификация](../artifacts/plan/reference-legacy-root-causes.json). Слово «проверка» в runtime/prompt не hardcoded.','',
            'Adversarial: исходные 48/48 + новые 20/20, **68/68 PASS**. Все проходят strict decoder → Service → disposable SQLite → последующее «да»; mutation count=0. Новые случаи: explicit status mismatch при optional any proof, подмена требуемого поля optional proof, case day mismatch, duplicate required proof, exclusion kind mismatch.','',
            '## Единственный fresh v7','',
            f"{v7['Conversations']} conversations / {v7['Turns']} turns. Генератор `scripts/plan_fresh_v7.py` создан после завершения DEV; предыдущие holdout и их model outputs не читаются. Corpus SHA256: `{v7['input_sha256']}`.", '',
            'До inference заморожены corpus, prompt, schema, code и evaluator: `testdata/semantic/plan_heldout_v7_protocol.json`, снимок `artifacts/plan/v7-frozen-source.zip`. Проверка до/после и хешей снимка: `artifacts/plan/v7-freeze-verification.json`. После получения результатов код, prompt, schema, evaluator и corpus не менялись; повторного inference/replay v7 нет.','',
            '| Метрика | v7 |','|---|---:|']
    for label,key in [('End-to-end','end_to_end'),('Multi-turn','multi_turn_success'),('Complex','complex_command_accuracy'),('Routing','routing_accuracy'),('Strict plan validity','semantic_plan_validity')]:
        lines.append(f'| {label} | {metric(v7,key)} |')
    for label,key in [('Unsafe','unsafe_accepted_mutations'),('Wrong-value','wrong_value_mutations'),('Confirmed wrong-value','confirmed_wrong_value_mutations'),('Evidence rejections','evidence_guard_rejections'),('Evidence value mismatches','evidence_value_mismatches'),('Clarifications','clarification_turns'),('Verifier calls','verifier_calls')]:
        lines.append(f'| {label} | {v7[key]} |')
    lines+=['','Evidence breakdown v7:','']
    if v7.get('evidence_rejection_breakdown'):
        lines += [f'- `{k}`: {v}' for k,v in sorted(v7['evidence_rejection_breakdown'].items())]
    else:lines+=['Нет evidence rejection.']
    lines+=['','| Latency | n | p50 ms | p95 ms |','|---|---:|---:|---:|']
    for key,values in v7['latency_ms'].items():
        lines.append(f"| {key} | {values['n']:.0f} | {values.get('p50',0):.3f} | {values.get('p95',0):.3f} |")
    lines += ['',f"Logical ParsePlan calls: {v7['semantic_calls_per_turn']*v7['Turns']:.0f}; network inference requests: {v7['network_inference_requests']}. Network latency отделена от cache и fast path. Verifier round trips: 0.",'',
              '## Неуспешные ходы v7','']
    for f in failures['v7']:
        lines.append(f"- `{f['conversation']}` / turn {f['turn']+1}: «{f['message']}» — {', '.join(f['checks'])}; error={f['error']}; guard={f['evidence_field']}:{f['evidence_reason']}.")
    if not failures['v7']:lines+=['Нет.']
    lines+=['','## Production gate','', '| Проверка | Результат |','|---|---|']
    lines += [f"| {k} | {'PASS' if v else 'FAIL'} |" for k,v in gate.items()]
    lines+=['',f'**Production ready: {ready}.** Решение основано на единственном frozen v7 и обоих reference DEV. Пороги не снижались. Все safe refusals остаются в accuracy; модельные semantic/schema ошибки не исправлялись post-holdout.','',
            'Отчёты: [policy DEV](../artifacts/plan/policy-dev-final-corrected.json), [context DEV](../artifacts/plan/context-dev-final.json), [старый reference](../artifacts/plan/policy-reference-final.json), [fresh v7](../artifacts/plan/heldout-v7-first.json), [машинная сводка](../artifacts/plan/policy-v7-summary.json).']
    (ROOT/'docs/SEMANTIC_POLICY_V7_REPORT.md').write_text('\n'.join(lines)+'\n',encoding='utf8')
    print(json.dumps(dict(production_ready=ready,gate=gate,v7=v7['metrics'],guard=v7.get('evidence_rejection_breakdown'),failures={k:len(v) for k,v in failures.items()}),ensure_ascii=False))


if __name__=='__main__':main()
