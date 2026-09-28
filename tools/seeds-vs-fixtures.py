import json, os, collections, sys
FIX='/home/user/estevao-api/spec/fixtures/prayer_books'
DS=sys.argv[1]
def load(p, key):
    if not os.path.exists(p): return None
    d=json.load(open(p)); return d.get(key, []) if isinstance(d, dict) else d
def ds(book, table):
    p=os.path.join(DS,'prayer_books',book,table+'.json')
    return json.load(open(p)) if os.path.exists(p) else []
def canon(v): return json.dumps(v, sort_keys=True, ensure_ascii=False)
rename={'celebration_name':'celebration','category_key':'category'}
books=sorted(os.listdir(os.path.join(DS,'prayer_books')))
fixbooks=sorted(os.listdir(FIX))
print('books in seeds without fixtures:', [b for b in books if b not in fixbooks])
print('fixture dirs without seeds:', [b for b in fixbooks if b not in books])
tables=[('celebrations','celebrations'),('collects','collects'),('lectionary_readings','lectionary_readings'),('liturgical_texts','liturgical_texts'),('preference_categories','preference_categories'),('preference_definitions','preference_definitions'),('psalms','psalms'),('psalm_cycles','psalms')]
tot=collections.Counter(); extra_cols=collections.defaultdict(set); rows=[]
for b in books:
    for table,file in tables:
        fx=load(os.path.join(FIX,b,file+'.json'), table)
        sd=ds(b,table)
        if fx is None:
            rows.append((b,table,len(sd),'no fixture file',None,None,None)); tot['missing_file_rows']+=len(sd); continue
        fields=set()
        for r in fx: fields|=set(r)
        sdfields=set(sd[0]) if sd else set()
        mapped={rename.get(f,f) for f in fields}
        for c in sdfields-mapped-{'prayer_book'}: extra_cols[table].add(c)
        def proj_fx(r): return canon({rename.get(k,k):v for k,v in r.items() if v is not None})
        def proj_sd(r): return canon({k:v for k,v in r.items() if k in mapped and v is not None})
        A=collections.Counter(proj_sd(r) for r in sd); B=collections.Counter(proj_fx(r) for r in fx)
        only_s=sum((A-B).values()); only_f=sum((B-A).values())
        order = [proj_sd(r) for r in sd]==[proj_fx(r) for r in fx]
        rows.append((b,table,len(sd),len(fx),only_s,only_f,order))
        tot['seed_rows']+=len(sd); tot['only_seed']+=only_s; tot['only_fixture']+=only_f
print('%-14s %-24s %7s %9s %9s %9s %s'%('book','table','seed','fixture','onlySeed','onlyFix','sameOrder'))
for r in rows:
    if r[3]=='no fixture file' or r[4] or r[5]:
        print('%-14s %-24s %7s %9s %9s %9s %s'%tuple(str(x) for x in r))
print(dict(tot))
print('seed columns absent from the fixtures:', {k:sorted(v) for k,v in extra_cols.items()})
