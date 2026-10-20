# Russian normalization data

`russian.json` contains dictionary-backed word forms and top lemmas exported from the original reference implementation (retained in Git history and the pre-cleanup backup).

Sources: the supplied 1000-event CSV, the shared parity corpus, pymorphy3 and `pymorphy3-dicts-ru` 2.4.417150.4580142 (OpenCorpora source revision 417150). Lexeme inflections are expanded using the reference analyzer; they are not inferred using lowercase alone.

- https://github.com/no-plagiarism/pymorphy3
- https://github.com/no-plagiarism/pymorphy3-dicts
- https://opencorpora.org/dict.php

Unknown vocabulary uses the Unicode Russian Snowball stemmer. The shipped lexicon covers the corpus/CSV vocabulary and its inflections; it does not claim to replace the full OpenCorpora morphological analyzer for every Russian word. `SearchEngine` remains replaceable for future richer morphology/vector retrieval.

`synonyms.json` supplements the original dictionary with curated related vocabulary for presentations, contracts, calls, budgets and design. Aliases use the same normalization as queries, including unknown inflections. These groups affect retrieval suggestions; they do not count as exact matches for mutation target selection.
