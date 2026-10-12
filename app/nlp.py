import re
from datetime import datetime

from .dates import extract
from .schemas import Command

MEETING = r"\b(?:встреч[ауи]|созвон\w*|совещан\w*|демо\b|интервью\b)"


def infer_kind(text: str):
    if re.search(r"\b(?:напомни\w*|напоминан\w*)", text):
        return "reminder"
    if re.search(r"\bзадач\w*\s+и\s+встреч\w*", text):
        return None
    if re.search(r"\bзадач\w*", text):
        return "task"
    if re.search(MEETING, text):
        return "meeting"
    return None


def parse(text: str, now: datetime) -> Command:
    low = text.lower().replace("ё", "е").strip()
    temporal = extract(low, now)
    cleaned = temporal.clean(text)
    kind = infer_kind(low)
    cmd = Command(
        kind=kind,
        date=temporal.date,
        time=temporal.time,
        period_start=temporal.start,
        period_end=temporal.end,
        duration_minutes=temporal.duration,
        error=temporal.error,
    )
    target = re.search(r"(?:#|номер\s*|№\s*)(\d+)", low)
    if target:
        cmd.target_id = int(target[1]) if int(target[1]) > 0 else None
        cleaned = re.sub(r"(?:#|номер\s*|№\s*)\d+", "", cleaned).strip()
    if re.search(r"\b(?:удали\w*|удалить|отмени\w*|убери\w*)", low):
        cmd.action = "delete"
    elif re.search(r"\b(?:выполнен\w*|выполнил\w*|сделан\w*|заверши\w*|закончи\w*)", low):
        cmd.action = "complete"
    elif re.search(r"\b(?:перенеси\w*|перенести|перенос\w*|сдвинь|измени\w*)", low):
        cmd.action = "update"
    elif re.search(r"\b(?:найди|найти|отыщи|поищи|поиск|когда|раньше|когда-то|была|были|истори\w*)", low):
        cmd.action = "search"
    elif re.search(r"\b(?:что|какие|покажи|показать|список|планы|запланировано|есть\s+ли|расписание)\b", low):
        cmd.action = "list"
    elif re.search(
        r"\b(?:напомни\w*|поставь|поставить|добавь|добавить|создай|создать|запиши|записать|нужно|надо|купить|подготовить|отправить|позвонить|проверить|запланируй\w*)\b",
        low,
    ) or (kind == "meeting" and temporal.date):
        cmd.action = "create"
    if cmd.action == "create":
        cmd.kind = kind or "task"
        cleaned = re.sub(
            r"(?i)\b(?:напомни\w*|поставь|поставить|добавь|добавить|создай|создать|запиши|записать|запланируй\w*)\s*(?:мне\s*)?(?:(?:новую?\s+)?(?:задачу|напоминание)\s*)?(?:чтобы\s*)?",
            "",
            cleaned,
        )
        cleaned = re.sub(r"(?i)\b(?:мне|у меня|нужно|надо)\b", "", cleaned)
        cleaned = re.sub(r"(?i)\b(?:до|на|в|во|к)\s*$", "", cleaned)
        cmd.title = re.sub(r"\s+", " ", cleaned).strip(" ,.!?:;-")
        cmd.title = re.sub(r"(?i)^встречу\b", "Встреча", cmd.title)
        if cmd.title:
            cmd.title = cmd.title[0].upper() + cmd.title[1:]
    else:
        cleaned = re.sub(
            r"(?i)\b(?:найди|найти|отыщи|поищи|поиск|когда-то|когда|раньше|была|был|были|ставил|ставила|я|у меня|мои|мою|мне|задач\w*|напоминан\w*|событи\w*|выполнен\w*|выполнил\w*|сделан\w*|заверши\w*|закончи\w*|отметь|как|удали\w*|удалить|отмени\w*|убери\w*|перенеси\w*|перенести|измени\w*|а|ее|его|их|это|эту|этот|на|про|по|пожалуйста)\b",
            " ",
            cleaned,
        )
        cmd.query = re.sub(r"\s+", " ", cleaned).strip(" ,.!?:;-")
    return cmd
