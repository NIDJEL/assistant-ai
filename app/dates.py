"""Explicit Russian date/time extraction. All periods are half-open, in APP_TIMEZONE."""

import re
from dataclasses import dataclass, field
from datetime import datetime, timedelta

NUMBERS = {
    "один": 1,
    "одну": 1,
    "одна": 1,
    "два": 2,
    "две": 2,
    "три": 3,
    "четыре": 4,
    "пять": 5,
    "шесть": 6,
    "семь": 7,
    "восемь": 8,
    "девять": 9,
    "десять": 10,
    "одиннадцать": 11,
    "двенадцать": 12,
    "тринадцать": 13,
    "четырнадцать": 14,
    "пятнадцать": 15,
    "шестнадцать": 16,
    "семнадцать": 17,
    "восемнадцать": 18,
    "девятнадцать": 19,
    "двадцать": 20,
    "полчаса": 0.5,
}
NUMBER_RE = r"(?:\d+|двадцать\s+(?:один|два|три)|" + "|".join(NUMBERS) + r")"
WEEKDAYS = [r"понедельник", r"вторник", r"сред[ау]", r"четверг", r"пятниц[ау]", r"суббот[ау]", r"воскресенье"]
MONTHS = {
    "января": 1,
    "февраля": 2,
    "марта": 3,
    "апреля": 4,
    "мая": 5,
    "июня": 6,
    "июля": 7,
    "августа": 8,
    "сентября": 9,
    "октября": 10,
    "ноября": 11,
    "декабря": 12,
}


def number(value: str) -> float:
    if value.isdigit():
        return int(value)
    return sum(NUMBERS.get(word, 0) for word in value.split())


@dataclass
class Temporal:
    date: str | None = None
    time: str | None = None
    start: str | None = None
    end: str | None = None
    duration: int = 60
    error: str | None = None
    spans: list = field(default_factory=list)

    def clean(self, text):
        for start, end in sorted(self.spans, reverse=True):
            text = text[:start] + " " + text[end:]
        return re.sub(r"\s+", " ", text).strip(" ,.!?;:-")


def extract(text: str, now: datetime) -> Temporal:
    text = text.lower().replace("ё", "е")
    result = Temporal()
    day = now.date()
    dates = []

    def remember(match, target):
        dates.append(target)
        result.spans.append(match.span())

    # Relative hours/minutes must retain minute precision; no fabricated midnight.
    relative = re.search(r"через\s+(" + NUMBER_RE + r")\s*(час\w*|минут\w*|дн\w*|день|недел\w*)", text)
    if relative:
        count = number(relative[1])
        unit = relative[2]
        if count <= 0 or count > 366:
            result.error = "Укажите интервал от 1 до 366 дней (или часов/минут)."
        else:
            delta = (
                timedelta(hours=count)
                if unit.startswith("час")
                else timedelta(minutes=count)
                if unit.startswith("мин")
                else timedelta(days=count * (7 if unit.startswith("нед") else 1))
            )
            target = now + delta
            remember(relative, target.date())
            if unit.startswith(("час", "мин")):
                result.time = target.strftime("%H:%M")

    for m in re.finditer(r"\b(\d{4}-\d{2}-\d{2}|\d{1,2}\.\d{1,2}(?:\.\d{4})?)\b", text):
        try:
            target = datetime.strptime(m[1], "%Y-%m-%d" if "-" in m[1] else "%d.%m.%Y" if m[1].count(".") == 2 else "%d.%m").date()
            if m[1].count(".") == 1:
                target = target.replace(year=now.year)
                if target < day:
                    target = target.replace(year=now.year + 1)
            remember(m, target)
        except ValueError:
            result.spans.append(m.span())
            result.error = "Такой даты нет в календаре. Укажите дату в формате ДД.ММ.ГГГГ."

    month_re = r"\b(\d{1,2})\s+(" + "|".join(MONTHS) + r")(?:\s+(\d{4})(?:\s+года)?)?\b"
    for m in re.finditer(month_re, text):
        try:
            target = day.replace(year=int(m[3] or now.year), month=MONTHS[m[2]], day=int(m[1]))
            if not m[3] and target < day:
                target = target.replace(year=now.year + 1)
            remember(m, target)
        except ValueError:
            result.spans.append(m.span())
            result.error = "Такой даты нет в календаре. Уточните день и месяц."

    for m in re.finditer(r"\b(послезавтра|позавчера|сегодня|завтра|вчера)\b", text):
        remember(m, day + timedelta(days={"сегодня": 0, "завтра": 1, "послезавтра": 2, "вчера": -1, "позавчера": -2}[m[1]]))
    for weekday, pattern in enumerate(WEEKDAYS):
        m = re.search(r"\b(?:(?:в|во|до)\s+)?(?:(следующ\w*|эт\w*)\s+)?(" + pattern + r")\b", text)
        if m:
            offset = (weekday - day.weekday()) % 7
            if m[1] and m[1].startswith("следующ"):
                offset = 7 - day.weekday() + weekday
            remember(m, day + timedelta(days=offset))

    week = re.search(r"\b(?:(?:на|за|до конца)\s+)?(?:(следующ\w*|эт\w*|текущ\w*|прошл\w*)\s+)?недел[юие]\b", text)
    if week:
        monday = day - timedelta(days=day.weekday())
        shift = 7 if week[1] and week[1].startswith("следующ") else -7 if week[1] and week[1].startswith("прошл") else 0
        result.start = (monday + timedelta(days=shift)).isoformat()
        result.end = (monday + timedelta(days=shift + 7)).isoformat()
        result.spans.append(week.span())
        if "до конца" in week[0]:
            dates.append(monday + timedelta(days=6))
    weekend = re.search(r"\b(?:(?:на|за)\s+)?выходны[ех]\b", text)
    if weekend:
        saturday = day + timedelta(days=-1 if day.weekday() == 6 else (5 - day.weekday()) % 7)
        result.start, result.end = saturday.isoformat(), (saturday + timedelta(days=2)).isoformat()
        result.spans.append(weekend.span())

    year = re.search(r"\b(?:за|в)\s+(\d{4})\s*(?:год\w*)?", text)
    if year and not any(a <= year.start() < b for a, b in result.spans):
        y = int(year[1])
        if 1900 <= y < 9999:
            result.start, result.end = f"{y}-01-01", f"{y + 1}-01-01"
            result.spans.append(year.span())
    if len(set(dates)) > 1:
        result.error = "В запросе несколько разных дат. Укажите одну дату события."
    if dates:
        result.date = dates[0].isoformat()
        if not result.start:
            result.start = result.date
            result.end = (dates[0] + timedelta(days=1)).isoformat()

    duration = re.search(r"\b(?:на|длительностью)\s+(?:(" + NUMBER_RE + r")\s*)?(час\w*|минут\w*)\b", text)
    if duration:
        result.duration = int(number(duration[1] or "один") * (60 if duration[2].startswith("час") else 1))
        result.spans.append(duration.span())
        if not 5 <= result.duration <= 1440:
            result.error = "Длительность встречи должна быть от 5 минут до 24 часов."
            result.duration = 60

    clock = re.search(r"\b(?:(?:в|к)\s+)?(\d{1,2}):(\d{2})(?:\s*(утра|вечера|дня|ночи))?\b", text)
    if not clock:
        clock = re.search(r"\b(?:в|к)\s+(" + NUMBER_RE + r")(?:\s+(утра|вечера|дня|ночи))?\b", text)
        if clock and any(a <= clock.start() < b or a <= clock.end() - 1 < b for a, b in result.spans):
            clock = None
        if clock:
            hour, minute, meridiem = int(number(clock[1])), 0, clock[2]
    else:
        hour, minute, meridiem = int(clock[1]), int(clock[2]), clock[3]
    if clock:
        if meridiem in {"вечера", "дня"} and hour < 12:
            hour += 12
        elif meridiem == "ночи" and hour == 12:
            hour = 0
        result.spans.append(clock.span())
        if hour > 23 or minute > 59:
            result.error = "Время должно быть от 00:00 до 23:59. Уточните время."
        else:
            result.time = f"{hour:02}:{minute:02}"
    # Bare time is useful in a clarification turn.
    if not clock and re.fullmatch(NUMBER_RE + r"(?:\s*(утра|вечера|дня|ночи))?[.!]?", text.strip()):
        bare = extract("в " + text.strip(), now)
        result.time, result.error = bare.time, bare.error
    return result
