# -*- coding: utf-8 -*-
"""Russian language pack builder.

guesses : every 5-letter word form in the OpenCorpora dictionary, ё-normalised
answers : frequency-ranked 5-letter nouns in nominative singular
"""
import json, re, sys
from collections import defaultdict
import pymorphy3

import os
HERE = os.path.dirname(os.path.abspath(__file__))
SOURCES = os.environ.get('LINGLE_SOURCES', os.path.join(HERE, 'sources'))
FREQ = os.path.join(SOURCES, 'frequency-words', 'content', '2018', 'ru', 'ru_full.txt')
OC5  = os.path.join(HERE, '.oc5.json')
CYR  = re.compile(r'^[а-яё]{5}$')
NOSTART = 'ьъы'

def norm(w):
    return w.replace('ё', 'е')

# ---- frequency -------------------------------------------------------
freq = defaultdict(int)
with open(FREQ, encoding='utf-8') as f:
    for line in f:
        p = line.split()
        if len(p) == 2 and CYR.match(p[0].lower()):
            freq[norm(p[0].lower())] += int(p[1])

# ---- guess list ------------------------------------------------------
oc = json.load(open(OC5, encoding='utf-8'))
guesses = sorted({norm(w) for w in oc if w[0] not in NOSTART})

# ---- answer list -----------------------------------------------------
morph = pymorphy3.MorphAnalyzer()
BAD = {'Surn','Name','Patr','Geox','Orgn','Trad','Abbr','Erro',
       'Infr','Slng','Arch','Dist','Anph','Ms-f'}
gset = set(guesses)

answers = []
for w in sorted(freq, key=lambda x: -freq[x]):
    if w not in gset or w[0] in NOSTART:
        continue
    best = None
    for p in morph.parse(w):
        t = p.tag
        if t.POS != 'NOUN' or any(g in t for g in BAD):
            continue
        if 'nomn' not in t or ('sing' not in t and 'Pltm' not in t):
            continue
        if norm(p.normal_form) != w:
            continue
        best = p
        break
    if best is None or best.score < 0.3:
        continue
    answers.append(w)


FLOOR = int(sys.argv[1]) if len(sys.argv) > 1 else 50
kept = [w for w in answers if freq[w] >= FLOOR]
print(f"guesses={len(guesses)}  answer candidates={len(answers)}  kept={len(kept)}", file=sys.stderr)
print("head:", ' '.join(kept[:15]), file=sys.stderr)
print("mid :", ' '.join(kept[len(kept)//2:len(kept)//2+15]), file=sys.stderr)
print("tail:", ' '.join(kept[-15:]), file=sys.stderr)
print(f"tail frequency floor: {freq[kept[-1]]}", file=sys.stderr)

json.dump({'guesses': guesses, 'answers': kept},
          open(os.path.join(HERE, '.ru_raw.json'), 'w', encoding='utf-8'), ensure_ascii=False)
