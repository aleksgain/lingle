# -*- coding: utf-8 -*-
"""English language pack builder.

guesses : 5-letter entries from a large open dictionary
answers : frequency-ranked 5-letter common words that hunspell knows in
          lower case (which filters out proper nouns), minus plurals and
          regular past tenses
"""
import json, re, sys
from collections import defaultdict

import os
HERE = os.path.dirname(os.path.abspath(__file__))
SOURCES = os.environ.get('LINGLE_SOURCES', os.path.join(HERE, 'sources'))
ALPHA = os.path.join(SOURCES, 'english-words', 'words_alpha.txt')
FREQ  = os.path.join(SOURCES, 'frequency-words', 'content', '2018', 'en', 'en_full.txt')
HUN   = os.environ.get('LINGLE_HUNSPELL', '/usr/share/hunspell/en_US.dic')
W5 = re.compile(r'^[a-z]{5}$')

allwords = {w.strip().lower() for w in open(ALPHA, encoding='utf-8')}
guesses = sorted(w for w in allwords if W5.match(w))

# hunspell stems, keeping case so proper nouns can be told apart
hun_lower, hun_any = set(), set()
with open(HUN, encoding='utf-8', errors='ignore') as f:
    next(f)
    for line in f:
        stem = line.split('/')[0].strip()
        if not stem.isalpha():
            continue
        hun_any.add(stem.lower())
        if stem[0].islower():
            hun_lower.add(stem.lower())

freq = defaultdict(int)
for line in open(FREQ, encoding='utf-8'):
    p = line.split()
    if len(p) == 2:
        freq[p[0].lower()] += int(p[1])

def is_plural(w):
    return w.endswith('s') and not w.endswith('ss') and w[:-1] in allwords

def is_past(w):
    return w.endswith('ed') and (w[:-1] in allwords or w[:-2] in allwords)

FLOOR = int(sys.argv[1]) if len(sys.argv) > 1 else 300
answers = [w for w in sorted(guesses, key=lambda x: -freq[x])
           if freq[w] >= FLOOR
           and w in hun_lower
           and not is_plural(w)
           and not is_past(w)]

print(f"guesses={len(guesses)}  answers={len(answers)}", file=sys.stderr)
print("head:", ' '.join(answers[:15]), file=sys.stderr)
print("mid :", ' '.join(answers[len(answers)//2:len(answers)//2+15]), file=sys.stderr)
print("tail:", ' '.join(answers[-15:]), file=sys.stderr)
json.dump({'guesses': guesses, 'answers': answers},
          open(os.path.join(HERE, '.en_raw.json'), 'w', encoding='utf-8'))
