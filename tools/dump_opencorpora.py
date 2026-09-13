"""Cache every 5-letter word form known to the OpenCorpora dictionary."""
import json, os, re, sys
import pymorphy3

HERE = os.path.dirname(os.path.abspath(__file__))
LENGTH = int(sys.argv[1]) if len(sys.argv) > 1 else 5

morph = pymorphy3.MorphAnalyzer()
pattern = re.compile(r'^[а-яё]{%d}$' % LENGTH)
words = sorted(w for w in morph.dictionary.words.iterkeys('') if pattern.match(w))
json.dump(words, open(os.path.join(HERE, '.oc5.json'), 'w', encoding='utf-8'),
          ensure_ascii=False)
print(f'cached {len(words)} word forms of length {LENGTH}')
