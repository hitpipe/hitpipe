"""Compare unfiltered measure.py outputs, including complete corpus equality.
Usage: python3 tools/sourcefacts/compare.py OLD_DIR NEW_DIR OUTPUT_JSON
No third-party statistics dependency; report descriptive spread, not significance.
"""
import hashlib
import json
import pathlib
import statistics
import sys

old, new, target = map(pathlib.Path, sys.argv[1:])


def load_samples(root):
    result = {}
    for line in (root/'bench.stdout').read_text().splitlines():
        fields = line.split()
        if not fields or not fields[0].startswith('BenchmarkDecode/'):
            continue
        values = {unit:float(fields[fields.index(unit)-1]) for unit in ['ns/op','B/op','allocs/op']}
        result.setdefault(fields[0].split('/',1)[1], []).append(values)
    assert len(result)==13 and all(len(v)==10 for v in result.values()), 'incomplete or changed workload'
    return result


def summary(samples, metric):
    values = [s[metric] for s in samples]
    return dict(median=statistics.median(values), mean=statistics.mean(values), min=min(values), max=max(values), stdev=statistics.stdev(values))


environments=[json.loads((r/'environment.json').read_text()) for r in [old,new]]
for key in ['go','go_env','cpu','logical_cpu_count','platform']:
    assert environments[0][key]==environments[1][key], key+' differs'
corpora=[(r/'corpus.json').read_bytes() for r in [old,new]]
assert corpora[0]==corpora[1], 'differential outputs differ'
samples=[load_samples(r) for r in [old,new]]
assert samples[0].keys()==samples[1].keys()
rows=[]
for name in sorted(samples[0]):
    row=dict(name=name, sample_count=10, raw_samples={'old':samples[0][name],'new':samples[1][name]}, metrics={})
    for metric in ['ns/op','B/op','allocs/op']:
        a,b=[summary(s[name],metric) for s in samples]
        row['metrics'][metric]=dict(old=a,new=b,median_change_percent=100*(b['median']/a['median']-1))
    rows.append(row)
result=dict(old_sha=environments[0]['source_sha'],new_sha=environments[1]['source_sha'],
            environment=environments[0],corpus_records=len(json.loads(corpora[0])),corpus_equal=True,
            corpus_sha256=hashlib.sha256(corpora[0]).hexdigest(),rows=rows,
            limits=['Descriptive local microbenchmarks, ten samples each; no statistical significance or end-to-end claim.',
                    'Sequential old then new runs on the same machine; CPU affinity/governor/background load are not controlled.',
                    'CPU/alloc profiles are diagnostics and excluded from timing samples. Every sample retained.'])
target.write_text(json.dumps(result,indent=2)+'\n')
for row in rows:
    m=row['metrics']
    print(row['name'], ' '.join(f"{k}: {v['old']['median']:g} -> {v['new']['median']:g} ({v['median_change_percent']:+.2f}%)" for k,v in m.items()))
print('Exact corpus equality:',result['corpus_records'],'records;',result['corpus_sha256'])
