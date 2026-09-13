# Third-party data

The application code in this repository is MIT licensed (see `LICENSE`).
The generated language packs under `packs/` are **not** — they derive from the
sources below and inherit their terms.

## Russian

| Source | Used for | Licence |
| --- | --- | --- |
| [OpenCorpora](http://opencorpora.org/) dictionary, via [pymorphy3-dicts-ru](https://github.com/no-plagiarism/pymorphy3-dicts) | the list of valid word forms, and the part-of-speech / case filtering that picks answers | CC BY-SA 4.0 |
| [hermitdave/FrequencyWords](https://github.com/hermitdave/FrequencyWords) (OpenSubtitles 2018) | ranking answers by how common they are | CC BY-SA 4.0 |
| [openrussian](https://en.openrussian.org/) dictionary data, via [Badestrand/russian-dictionary](https://github.com/Badestrand/russian-dictionary) | English meanings, stressed spellings and grammatical gender | CC BY-SA 4.0 |

## English

| Source | Used for | Licence |
| --- | --- | --- |
| [dwyl/english-words](https://github.com/dwyl/english-words) | the list of valid guesses | Unlicense |
| [hermitdave/FrequencyWords](https://github.com/hermitdave/FrequencyWords) (OpenSubtitles 2018) | ranking answers by how common they are | CC BY-SA 4.0 |
| [Princeton WordNet 3.0](https://wordnet.princeton.edu/) | definitions and parts of speech | [WordNet 3.0 licence](https://wordnet.princeton.edu/license-and-commercial-use) (BSD-style) |
| Hunspell `en_US` dictionary (SCOWL) | build-time filter only, to drop proper nouns; no part of it is redistributed | MIT-like (SCOWL) |

Because the frequency and dictionary data are CC BY-SA 4.0, `packs/ru.json` and
`packs/en.json` are distributed under **CC BY-SA 4.0** with attribution to the
sources above.

No word list from the New York Times' Wordle is used, copied or derived from.
