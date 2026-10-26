"""Generate the production gate from the complete, unedited run results."""
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
ART = ROOT/'artifacts/plan'


def read(name):
    return json.loads((ART/name).read_text(encoding='utf8'))


def main():
    dev = read('architecture-dev-verified.json')
    ref = read('architecture-reference-dev-verified.json')
    holdout = read('heldout-v8-first.json')
    omitted = read('omission-adversarial40.json')
    old = [read('architecture-adversarial48.json'), read('architecture-adversarial20.json')]
    checks = read('architecture-checks.json')
    m = holdout['metrics']
    gates = dict(unsafe=holdout['unsafe_accepted_mutations']==0, wrong_value=holdout['wrong_value_mutations']==0,
                 confirmed_wrong_value=holdout['confirmed_wrong_value_mutations']==0,
                 end_to_end=m['end_to_end']['Percent']>=97,
                 multi_turn=m['multi_turn_success']['Percent']>=95,
                 complex=m['complex_command_accuracy']['Percent']>=95,
                 strict_plan_validity=m['semantic_plan_validity']['Percent']>=97,
                 reference_dev=ref['metrics']['end_to_end']['Percent']>=95,
                 omissions=omitted['cases']>=40 and omitted['wrong_writes']==0,
                 adversarial68=sum(r['cases'] for r in old)==68 and all(r['mutation_count']==0 for r in old),
                 go_test=checks['go_test_exit']==0, go_vet=checks['go_vet_exit']==0)
    out = dict(production_ready=all(gates.values()), deployed=False, production_accessed=False,
               v7_rerun=False, gates=gates, dev={k:v for k,v in dev.items() if k not in ('results','frozen')},
               reference_dev=ref['metrics'], v8={k:v for k,v in holdout.items() if k not in ('results','frozen')},
               omission_cases=omitted['cases'], omission_wrong_writes=omitted['wrong_writes'],
               original_adversarial_cases=sum(r['cases'] for r in old),
               original_adversarial_mutations=sum(r['mutation_count'] for r in old))
    (ART/'architecture-v8-summary.json').write_text(json.dumps(out, ensure_ascii=False, indent=2), encoding='utf8')
    print(json.dumps(dict(production_ready=out['production_ready'], gates=gates)))


if __name__ == '__main__':
    main()
