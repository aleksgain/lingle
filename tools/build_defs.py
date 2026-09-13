# -*- coding: utf-8 -*-
"""Build the definition tables that are shown once a puzzle is over.

Only answer words need an entry, so these stay small. Definitions are looked
up at build time and embedded in the pack: the running container makes no
outbound requests.

  ru : openrussian - English translation, stressed spelling, gender
  en : WordNet     - first non-proper-noun sense
"""
import csv, json, os, re, sys

HERE = os.path.dirname(os.path.abspath(__file__))
SOURCES = os.environ.get('LINGLE_SOURCES', os.path.join(HERE, 'sources'))

ACUTE = '́'   # combining acute accent


def fold_ru(word):
    return word.strip().lower().replace('ё', 'е')


def stress_marks(accented):
    """openrussian marks stress with an apostrophe after the vowel; turn that
    into a real combining accent so it renders as е́ rather than e'."""
    if not accented:
        return ''
    return accented.strip().replace("'", ACUTE).replace('`', '')


def clean_gloss(text, limit=120):
    text = re.sub(r'\s+', ' ', (text or '')).strip(' ;,')
    if len(text) <= limit:
        return text
    cut = text[:limit].rsplit(';', 1)[0].rsplit(',', 1)[0].rstrip(' ;,')
    return (cut or text[:limit].rstrip()) + '…'


def build_ru(answers):
    path = os.path.join(SOURCES, 'russian-dictionary', 'nouns.csv')
    if not os.path.exists(path):
        print(f'  ! {path} missing; run fetch_sources.sh', file=sys.stderr)
        return {}

    wanted = set(answers)
    out = {}
    csv.field_size_limit(10 ** 7)
    with open(path, encoding='utf-8', newline='') as f:
        for row in csv.DictReader(f, delimiter='\t'):
            key = fold_ru(row.get('bare') or '')
            if key not in wanted or key in out:
                continue
            gloss = clean_gloss(row.get('translations_en'))
            if not gloss:
                continue
            entry = {'gloss': gloss, 'pos': 'noun'}
            accented = stress_marks(row.get('accented'))
            # Only worth storing when it actually adds a stress mark or a ё.
            if (accented and accented != key
                    and fold_ru(accented.replace(ACUTE, '')) == key):
                entry['accented'] = accented
            gender = (row.get('gender') or '').strip()
            if gender in ('m', 'f', 'n'):
                entry['note'] = {'m': 'masculine', 'f': 'feminine', 'n': 'neuter'}[gender]
            out[key] = entry
    return out


POS_NAME = {'n': 'noun', 'v': 'verb', 'a': 'adjective', 's': 'adjective', 'r': 'adverb'}


def build_en(answers):
    os.environ.setdefault('NLTK_DATA', os.path.join(SOURCES, 'nltk_data'))
    try:
        from nltk.corpus import wordnet as wn
    except ImportError:
        print('  ! nltk is not installed; skipping English definitions', file=sys.stderr)
        return {}

    out = {}
    for word in answers:
        try:
            senses = wn.synsets(word)
        except LookupError:
            print('  ! WordNet data not found; run fetch_sources.sh', file=sys.stderr)
            return {}
        for sense in senses:
            # Skip proper nouns: WordNet models them as instances, and the
            # most frequent sense of e.g. "crane" is a novelist.
            if sense.instance_hypernyms():
                continue
            if not any(l.name().lower() == word for l in sense.lemmas()):
                continue
            gloss = clean_gloss(sense.definition())
            if not gloss:
                continue
            out[word] = {'gloss': gloss, 'pos': POS_NAME.get(sense.pos(), '')}
            break
    return out


BUILDERS = {'ru': build_ru, 'en': build_en}

if __name__ == '__main__':
    codes = sys.argv[1:] or sorted(BUILDERS)
    for code in codes:
        raw_path = os.path.join(HERE, f'.{code}_raw.json')
        if not os.path.exists(raw_path):
            print(f'  ! {raw_path} missing, run build_{code}.py first', file=sys.stderr)
            continue
        answers = json.load(open(raw_path, encoding='utf-8'))['answers']
        defs = BUILDERS[code](answers)
        json.dump(defs, open(os.path.join(HERE, f'.{code}_defs.json'), 'w', encoding='utf-8'),
                  ensure_ascii=False)
        pct = 100 * len(defs) / max(1, len(answers))
        print(f'  {code}: {len(defs)}/{len(answers)} answers have a definition ({pct:.0f}%)')
        for w in answers[:3]:
            if w in defs:
                d = defs[w]
                print(f'      {d.get("accented", w)} [{d["pos"]}] {d["gloss"]}')
