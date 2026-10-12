"""Morphology + synonym concepts + typo tolerance; no model download at startup."""

import math
import re
from collections import Counter
from functools import lru_cache

import pymorphy3
from rapidfuzz.fuzz import ratio

MORPH = pymorphy3.MorphAnalyzer()
STOP = set(
    "в на с со и у по про для от до а я мой мне это задача встреча напоминание когда быть ставить найти показать купить нужно есть ли что какой запланировать".split()
)
GROUPS = [
    {"созвон", "звонок", "позвонить", "телефон", "связаться"},
    {"отчёт", "отчет", "отчётность", "report"},
    {"договор", "контракт", "соглашение"},
    {"презентация", "слайд", "слайды", "доклад"},
    {"бюджет", "финансы", "финансовый", "расход"},
    {"дизайн", "дизайнер", "макет"},
    {"клиент", "заказчик"},
    {"команда", "коллега", "коллеги"},
]


@lru_cache(maxsize=20000)
def lemma(word):
    return MORPH.parse(word)[0].normal_form.replace("ё", "е")


CONCEPTS = {lemma(word): f"concept_{i}" for i, group in enumerate(GROUPS) for word in group}


@lru_cache(maxsize=10000)
def tokens(text):
    words = [lemma(w) for w in re.findall(r"[а-яёa-z0-9]+", text.lower())]
    normalized = []
    for word in words:
        if word in STOP:
            continue
        concept = CONCEPTS.get(word)
        if concept is None and len(word) >= 6:
            best = max(CONCEPTS, key=lambda candidate: ratio(word, candidate))
            if ratio(word, best) >= 88:
                concept = CONCEPTS[best]
        normalized.append(concept or word)
    return tuple(normalized)


def search(events: list[dict], query: str, limit=30):
    wanted = set(tokens(query))
    if not wanted:
        return []
    docs = [set(tokens(event["title"])) for event in events]
    freq = Counter(token for doc in docs for token in doc)
    weights = {t: math.log((len(docs) + 1) / (freq[t] + 1)) + 1 for t in wanted}
    scored = []
    for event, doc in zip(events, docs):
        total = sum(weights.values())
        hit = 0.0
        for token in wanted:
            similarity = (
                1
                if token in doc
                else max(
                    (ratio(token, d) / 100 for d in doc if not token.startswith("concept_") and not d.startswith("concept_")), default=0
                )
            )
            hit += weights[token] * (similarity if similarity >= 0.78 else 0)
        score = hit / total
        if score >= 0.55:
            scored.append({**event, "score": round(score, 3)})
    return sorted(scored, key=lambda e: (-e["score"], e["date"] or "9999", e["time"] or "", e["id"]))[:limit]
