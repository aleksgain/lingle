# -*- coding: utf-8 -*-
"""Assemble the finished language packs that the server loads at start-up.

Reads the intermediate lists produced by build_<lang>.py, applies the
blocklist, fixes a deterministic puzzle order and writes packs/<code>.json
plus packs/index.json.

Word lists are stored as one flat string of fixed-width words rather than a
JSON array: same data, roughly half the bytes.
"""
import hashlib, json, os, random, sys

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
OUT = os.path.join(ROOT, 'packs')

# Packs are read by the server and never served to the browser, so both word
# lists are stored in the clear as flat fixed-width strings.

def load_definitions(code):
    path = os.path.join(ROOT, 'tools', f'.{code}_defs.json')
    if not os.path.exists(path):
        return {}
    return json.load(open(path, encoding='utf-8'))


def load_blocklist(code):
    path = os.path.join(ROOT, 'tools', 'blocklist', f'{code}.txt')
    if not os.path.exists(path):
        return set()
    return {l.strip().lower() for l in open(path, encoding='utf-8')
            if l.strip() and not l.startswith('#')}

LANGS = {
    'ru': {
        'name': 'Русский',
        'englishName': 'Russian',
        'flag': '🇷🇺',
        'dir': 'ltr',
        'normalize': {'ё': 'е'},
        'keyboard': [
            list('йцукенгшщзхъ'),
            list('фывапролджэ'),
            list('ячсмитьбю'),
        ],
        'strings': {
            'title': 'Лингл',
            'howToPlay': 'Как играть',
            'stats': 'Статистика',
            'settings': 'Настройки',
            'notEnough': 'Слишком мало букв',
            'notInList': 'Такого слова нет в словаре',
            'hardModeViolation': 'В сложном режиме нужно использовать подсказки',
            'hardModeLocked': 'Сложный режим можно включить только до первой попытки',
            'won': ['Гениально!', 'Великолепно!', 'Впечатляет!', 'Отлично!', 'Неплохо!', 'Уф, успели!'],
            'lost': 'Загаданное слово:',
            'played': 'Сыграно',
            'winPct': '% побед',
            'streak': 'Серия',
            'maxStreak': 'Рекорд',
            'distribution': 'Распределение попыток',
            'nextWord': 'Следующее слово',
            'share': 'Поделиться',
            'copied': 'Результат скопирован',
            'copyFailed': 'Не удалось скопировать',
            'theme': 'Тёмная тема',
            'colorblind': 'Режим для дальтоников',
            'hardMode': 'Сложный режим',
            'hardModeHint': 'Открытые подсказки нужно использовать в следующих попытках',
            'language': 'Язык',
            'rulesIntro': 'Угадайте слово из 5 букв за 6 попыток.',
            'rulesLine1': 'Каждая попытка должна быть существующим словом.',
            'rulesLine2': 'После каждой попытки цвет плиток покажет, насколько вы близки.',
            'rulesCorrect': 'Буква <b>{a}</b> есть в слове и стоит на своём месте.',
            'rulesPresent': 'Буква <b>{a}</b> есть в слове, но на другом месте.',
            'rulesAbsent': 'Буквы <b>{a}</b> в слове нет.',
            'rulesYo': 'Буква «ё» вводится как «е».',
            'close': 'Закрыть',
            'enterKey': 'ВВОД',
            'meaning': 'Значение',
            'pos': {'noun': 'сущ.', 'verb': 'гл.', 'adjective': 'прил.', 'adverb': 'нареч.'},
            'note': {'masculine': 'м. р.', 'feminine': 'ж. р.', 'neuter': 'ср. р.'},
            'offline': 'Нет связи с сервером',
            'serverError': 'Ошибка сервера, попробуйте ещё раз',
            'exampleCorrect': 'парус',
            'exampleAbsent': 'тропа',
        },
    },
    'en': {
        'name': 'English',
        'englishName': 'English',
        'flag': '🇬🇧',
        'dir': 'ltr',
        'normalize': {},
        'keyboard': [
            list('qwertyuiop'),
            list('asdfghjkl'),
            list('zxcvbnm'),
        ],
        'strings': {
            'title': 'Lingle',
            'howToPlay': 'How to play',
            'stats': 'Statistics',
            'settings': 'Settings',
            'notEnough': 'Not enough letters',
            'notInList': 'Not in word list',
            'hardModeViolation': 'Hard mode: you must use the revealed hints',
            'hardModeLocked': 'Hard mode can only be changed before the first guess',
            'won': ['Genius', 'Magnificent', 'Impressive', 'Splendid', 'Great', 'Phew'],
            'lost': 'The word was:',
            'played': 'Played',
            'winPct': 'Win %',
            'streak': 'Streak',
            'maxStreak': 'Max streak',
            'distribution': 'Guess distribution',
            'nextWord': 'Next word in',
            'share': 'Share',
            'copied': 'Copied to clipboard',
            'copyFailed': 'Could not copy',
            'theme': 'Dark theme',
            'colorblind': 'Colour-blind mode',
            'hardMode': 'Hard mode',
            'hardModeHint': 'Any revealed hint must be used in subsequent guesses',
            'language': 'Language',
            'rulesIntro': 'Guess the 5-letter word in 6 tries.',
            'rulesLine1': 'Each guess must be a real word.',
            'rulesLine2': 'After each guess the tile colours show how close you were.',
            'rulesCorrect': '<b>{a}</b> is in the word and in the right spot.',
            'rulesPresent': '<b>{a}</b> is in the word but in the wrong spot.',
            'rulesAbsent': '<b>{a}</b> is not in the word.',
            'rulesYo': '',
            'close': 'Close',
            'enterKey': 'ENTER',
            'meaning': 'Meaning',
            'pos': {'noun': 'noun', 'verb': 'verb', 'adjective': 'adjective', 'adverb': 'adverb'},
            'note': {'masculine': 'masculine', 'feminine': 'feminine', 'neuter': 'neuter'},
            'offline': 'Cannot reach the server',
            'serverError': 'Server error, please try again',
            'exampleCorrect': 'crane',
            'exampleAbsent': 'ghost',
        },
    },
}

EPOCH = '2026-01-01'   # day 0 of the puzzle sequence, UTC

def build(code):
    raw_path = os.path.join(ROOT, 'tools', f'.{code}_raw.json')
    if not os.path.exists(raw_path):
        print(f'  ! {raw_path} missing, run build_{code}.py first', file=sys.stderr)
        return None
    raw = json.load(open(raw_path, encoding='utf-8'))
    cfg = LANGS[code]
    blocked = load_blocklist(code)
    definitions = load_definitions(code)

    guesses = sorted(set(raw['guesses']))
    answers = [w for w in raw['answers'] if w not in blocked]
    # every answer must also be an acceptable guess
    gset = set(guesses)
    answers = [w for w in answers if w in gset]

    length = len(guesses[0])
    assert all(len(w) == length for w in guesses), 'ragged guess list'
    assert all(len(w) == length for w in answers), 'ragged answer list'

    # Deterministic puzzle order: seeded from the language code and the exact
    # contents of the list, so the same list always yields the same sequence
    # and every word is used once before any repeats.
    digest = hashlib.sha256((code + '|' + ''.join(answers)).encode()).hexdigest()
    order = list(answers)
    random.Random(int(digest[:16], 16)).shuffle(order)

    pack = {
        'code': code,
        'name': cfg['name'],
        'englishName': cfg['englishName'],
        'flag': cfg['flag'],
        'dir': cfg['dir'],
        'length': length,
        'tries': 6,
        'epoch': EPOCH,
        'normalize': cfg['normalize'],
        'keyboard': cfg['keyboard'],
        'strings': cfg['strings'],
        'answerCount': len(order),
        'guessCount': len(guesses),
        'answers': ''.join(order),
        'guesses': ''.join(guesses),
        # Only answers need a definition, and it is never sent to a browser
        # until that day's game is over.
        'definitions': {w: definitions[w] for w in order if w in definitions},
    }
    path = os.path.join(OUT, f'{code}.json')
    json.dump(pack, open(path, 'w', encoding='utf-8'), ensure_ascii=False, separators=(',', ':'))
    size = os.path.getsize(path)
    defined = sum(1 for w in order if w in definitions)
    print(f'  {code}: {len(guesses):>6} guesses, {len(order):>5} answers, '
          f'{defined:>5} definitions, {size/1024:.0f} KB  '
          f'({len(order)/365.25:.1f} years of puzzles)')
    return {k: pack[k] for k in ('code', 'name', 'englishName', 'flag', 'length', 'dir')}

if __name__ == '__main__':
    os.makedirs(OUT, exist_ok=True)
    index = []
    for code in sys.argv[1:] or sorted(LANGS):
        meta = build(code)
        if meta:
            index.append(meta)
    json.dump({'languages': index}, open(os.path.join(OUT, 'index.json'), 'w', encoding='utf-8'),
              ensure_ascii=False, indent=2)
    print(f'  index.json: {[m["code"] for m in index]}')
