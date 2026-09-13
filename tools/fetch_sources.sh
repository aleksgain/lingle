#!/usr/bin/env bash
# Fetches the corpora the word lists and definitions are generated from.
# Only needed when regenerating packs; the generated packs are committed.
set -euo pipefail
cd "$(dirname "$0")"
mkdir -p sources

clone() {
  if [ -d "sources/$2" ]; then
    echo "  sources/$2 already present"
  else
    echo "  cloning $2"
    git clone --depth 1 "$1" "sources/$2"
  fi
}

clone https://github.com/hermitdave/FrequencyWords.git frequency-words
clone https://github.com/dwyl/english-words.git english-words
clone https://github.com/Badestrand/russian-dictionary.git russian-dictionary

# WordNet, for the English definitions. Fetched straight from the nltk_data
# repository because nltk's own downloader refuses to run behind a proxy.
if [ -d sources/nltk_data/corpora/wordnet ]; then
  echo "  sources/nltk_data/corpora/wordnet already present"
else
  echo "  downloading wordnet"
  mkdir -p sources/nltk_data/corpora
  curl -sSL -o sources/nltk_data/corpora/wordnet.zip \
    https://raw.githubusercontent.com/nltk/nltk_data/gh-pages/packages/corpora/wordnet.zip
  (cd sources/nltk_data/corpora && unzip -qo wordnet.zip)
fi

echo "done"
