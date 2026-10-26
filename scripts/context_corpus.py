"""Author separate DEV/fresh multi-turn corpora; never read the old 208 holdout.

This script freezes both sets before any live evaluation. Do not regenerate or
tune against context_heldout after evaluation. JSONL is the reviewable source.
"""
import hashlib
import json
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
NOW = "2026-10-16T12:30:17+03:00"


def step(message, kind, action, pending=False, event=None, **extra):
    expected = dict(turn_type=kind, action=action, pending=pending)
    if event:
        expected["event"] = event
    expected.update(extra)
    return dict(message=message, expect=expected)


def event(title, kind="meeting", date="2026-10-17", time="10:00", **extra):
    return dict(title=title, kind=kind, date=date, time=time, **extra)


def seed(key, title, kind="meeting", date="2026-10-17", time="10:00"):
    return dict(key=key, title=title, kind=kind, date=date, time=time)


def scenario(id, category, turns, seeds=None, mandatory=False):
    return dict(id=id, category=category, now=NOW, seeds=seeds or [], turns=turns, mandatory=mandatory)


def build_dev():
    out = []
    out.append(scenario("mandatory-pizza", "slot_replies", [
        step("Напомни вытащить пиццу из духовки", "command", "clarify", True),
        step("Ну через 10 минут", "slot_reply", "create", event=event("Вытащить пиццу из духовки", "reminder", "2026-10-16", "12:40")),
    ], mandatory=True))
    out.append(scenario("mandatory-halfhour", "conversational_fillers", [
        step("Напомни позвонить Ивану", "command", "clarify", True),
        step("Ну давай через полчаса", "slot_reply", "create", event=event("Позвонить Ивану", "reminder", "2026-10-16", "13:00")),
    ], mandatory=True))
    out.append(scenario("mandatory-spoken-clock", "slot_replies", [
        step("Добавь встречу с Алисой завтра", "command", "clarify", True),
        step("Часов в десять", "slot_reply", "create", event=event("Встреча с Алисой")),
    ], mandatory=True))
    out.append(scenario("mandatory-pronoun", "pronouns", [
        step("Добавь встречу с Алисой завтра в 10", "command", "create", event=event("Встреча с Алисой", save_as="created")),
        step("А перенеси её на 11", "reference", "update", reference="last", event=event("Встреча с Алисой", time="11:00", same_as="created")),
    ], mandatory=True))
    out.append(scenario("mandatory-ordinal", "ordinal_choice", [
        step("Покажи встречи на завтра", "command", "list", count=2),
        step("Вторую перенеси на 16", "reference", "update", reference="second", event=event("Созвон с командой", time="16:00", same_as="b")),
    ], [seed("a", "Встреча с Алисой"), seed("b", "Созвон с командой", time="15:00")], True))
    out.append(scenario("mandatory-correction", "correction", [
        step("Добавь встречу завтра в 10 с Иваном", "command", "create", event=event("Встреча с Иваном", save_as="created")),
        step("Нет, лучше послезавтра", "reference", "update", reference="last", event=event("Встреча с Иваном", date="2026-10-18", same_as="created")),
    ], mandatory=True))
    out.append(scenario("mandatory-refinement", "refinement", [
        step("Найди встречи с Алисой", "command", "search", count=2),
        step("А только за прошлую неделю?", "refinement", "search", count=1, event=event("Встреча с Алисой", date="2026-10-06")),
    ], [seed("a", "Встреча с Алисой", date="2026-10-06"), seed("b", "Встреча с Алисой"), seed("c", "Созвон с Борисом", date="2026-10-07")], True))
    out.append(scenario("mandatory-new-command", "new_command_while_pending", [
        step("Напомни позвонить Ивану", "command", "clarify", True),
        step("Добавь встречу с Никитой завтра в 15", "command", "create", event=event("Встреча с Никитой", time="15:00")),
    ], mandatory=True))
    out.append(scenario("mandatory-cancel-delete", "cancellation", [
        step("Удали задачу проверить архив", "command", "clarify", True),
        step("нет", "cancellation", "cancel"),
    ], [seed("a", "Проверить архив", "task", "", "")], True))
    out.append(scenario("mandatory-chat", "chat", [step("Привет", "chat", "help"), step("Спасибо", "chat", "help")], mandatory=True))
    out.append(scenario("mandatory-timer-pizza", "timer", [
        step("Поставь таймер на 10 минут чтобы я вытащил пиццу из духовки", "command", "create", event=event("Вытащить пиццу из духовки", "reminder", "2026-10-16", "12:40", save_as="timer")),
        step("Нет, лучше через час", "reference", "update", reference="last", event=event("Вытащить пиццу из духовки", "reminder", "2026-10-16", "13:30", same_as="timer")),
    ], mandatory=True))
    out.append(scenario("mandatory-stopwatch", "timer", [
        step("Засеки 5 минут проверить чай", "command", "create", event=event("Проверить чай", "reminder", "2026-10-16", "12:35")), step("Спасибо", "chat", "help"),
    ], mandatory=True))
    out.append(scenario("mandatory-duration", "timer", [
        step("Создай встречу на 30 минут завтра в 10", "command", "create", event=event("Встреча", duration_minutes=30)), step("Спасибо", "chat", "help"),
    ], mandatory=True))
    out.append(scenario("dev-rename", "correction", [
        step("Добавь встречу с Алисой завтра в 10", "command", "create", event=event("Встреча с Алисой", save_as="created")),
        step("И не с Алисой, а с Марией", "reference", "update", reference="last", event=event("Встреча с Марией", same_as="created")),
    ]))
    out.append(scenario("dev-chain", "correction", [
        step("Добавь встречу с Алисой завтра", "command", "clarify", True),
        step("В 10", "slot_reply", "create", event=event("Встреча с Алисой", save_as="created")),
        step("А перенеси её на пятницу", "reference", "update", reference="last", event=event("Встреча с Алисой", date="2026-10-16", same_as="created")),
        step("На 15", "reference", "update", reference="last", event=event("Встреча с Алисой", date="2026-10-16", time="15:00", same_as="created")),
    ]))
    out.append(scenario("dev-reminder-update", "correction", [
        step("Добавь встречу с Алисой завтра в 10", "command", "create", event=event("Встреча с Алисой", save_as="created")),
        step("И напомни за 30 минут", "reference", "update", reference="last", event=event("Встреча с Алисой", reminder_minutes=30, same_as="created")),
    ]))
    out.append(scenario("dev-delete-confirm", "confirmation", [
        step("Добавь задачу проверить архив", "command", "create", event=dict(title="Проверить архив", kind="task", save_as="created")),
        step("Удали её", "reference", "clarify", True, reference="last"),
        step("да", "confirmation", "delete"),
    ]))
    out.append(scenario("dev-ambiguous-pronoun", "ambiguity", [
        step("Покажи встречи на завтра", "command", "list", count=2),
        step("Удали её", "reference", "clarify"),
    ], [seed("a", "Встреча с Алисой"), seed("b", "Созвон с командой", time="15:00")]))
    out.append(scenario("dev-stale", "ambiguity", [
        step("Добавь задачу проверить архив", "command", "create", event=dict(title="Проверить архив", kind="task")),
        dict(**step("Удали её", "reference", "clarify"), advance_minutes=180),
    ]))
    out.append(scenario("dev-list-filter", "refinement", [
        step("Что у меня завтра?", "command", "list", count=2),
        step("А только встречи?", "refinement", "list", count=1, event=event("Встреча с Алисой")),
        step("А после 15?", "refinement", "clarify"),
    ], [seed("a", "Встреча с Алисой"), seed("b", "Купить хлеб", "task", time="16:00")]))
    out.append(scenario("dev-choose", "ordinal_choice", [
        step("Удали встречу с Алисой", "command", "clarify", True),
        step("вторую", "choice", "clarify", True, reference="second"),
        step("нет", "cancellation", "cancel"),
    ], [seed("a", "Встреча с Алисой"), seed("b", "Встреча с Алисой", time="15:00")]))
    out.append(scenario("dev-negated", "ambiguity", [
        step("Напомни проверить духовку", "command", "clarify", True),
        step("Не добавляй завтра в 10", "command", "clarify", True),
    ]))
    out.append(scenario("dev-restart", "slot_replies", [
        step("Напомни проверить чай", "command", "clarify", True),
        dict(**step("Через 5 минут", "slot_reply", "create", event=event("Проверить чай", "reminder", "2026-10-16", "12:35")), restart=True),
    ]))
    out.append(scenario("dev-timer-order", "timer", [
        step("Поставь таймер минут на 12 проверить пирог", "command", "create", event=event("Проверить пирог", "reminder", "2026-10-16", "12:42")), step("Спасибо", "chat", "help"),
    ]))
    return out


def build_holdout():
    # Fresh phrases and subjects, separated from DEV. Freeze before inference.
    out = []
    names = ["Денисом", "Ольгой", "Софьей", "Максимом", "Еленой", "Тимуром", "Романом", "Полиной", "Глебом", "Викторией"]
    subjects = ["полить цветы", "проверить суп", "закрыть окно", "выключить утюг", "покормить кота", "забрать заказ", "проверить тесто", "вынести мусор", "проверить почту", "проветрить комнату"]
    for i in range(10):
        subject = subjects[i]
        title = subject[0].upper() + subject[1:]
        name = names[i]
        meeting = f"Встреча с {name}"
        delay = i + 6
        out.append(scenario(f"fresh-slot-{i}", "slot_replies", [
            step(f"Напомни {subject}", "command", "clarify", True),
            step(f"Через {delay} минут", "slot_reply", "create", event=event(title, "reminder", "2026-10-16", f"12:{30+delay:02d}")),
        ]))
        out.append(scenario(f"fresh-filler-{i}", "conversational_fillers", [
            step(f"Напомни {subject}", "command", "clarify", True),
            step(["Ну тогда давай через час", "Ладно, пусть будет через полчаса", "А давай-ка через 20 минут", "Хорошо, минут через десять", "Ну, пожалуй, через 15 минут"][i % 5], "slot_reply", "create", event=event(title, "reminder", "2026-10-16", ["13:30", "13:00", "12:50", "12:40", "12:45"][i % 5])),
        ]))
        out.append(scenario(f"fresh-pronoun-{i}", "pronouns", [
            step(f"Добавь встречу с {name} завтра в 10", "command", "create", event=event(meeting, save_as="created")),
            step(["Её перенеси на 12", "А эту встречу передвинь на 12", "Перенеси эту на 12", "Сдвинь её на 12", "А её давай на 12"][i % 5], "reference", "update", reference="last", event=event(meeting, time="12:00", same_as="created")),
        ]))
        out.append(scenario(f"fresh-ordinal-{i}", "ordinal_choice", [
            step("Покажи встречи на завтра", "command", "list", count=2),
            step(["Первую перенеси на 13", "Вот эту вторую перенеси на 13"][i % 2], "reference", "update", reference=["first", "second"][i % 2], event=event([meeting, "Созвон с редакцией"][i % 2], time="13:00", same_as=["a", "b"][i % 2])),
        ], [seed("a", meeting), seed("b", "Созвон с редакцией", time="16:00")]))
        out.append(scenario(f"fresh-correction-{i}", "correction", [
            step(f"Добавь встречу с {name} завтра в 10", "command", "create", event=event(meeting, save_as="created")),
            step(["Не, я имел в виду послезавтра", "Нет, давай лучше послезавтра", "Тогда лучше послезавтра", "А всё-таки послезавтра", "Нет, дату поменяем на послезавтра"][i % 5], "reference", "update", reference="last", event=event(meeting, date="2026-10-18", same_as="created")),
        ]))
        out.append(scenario(f"fresh-refinement-{i}", "refinement", [
            step(f"Найди встречи с {name}", "command", "search", count=2),
            step(["А за прошлую неделю?", "Оставь только за прошлую неделю", "А покажи только прошлую неделю", "Меня интересует только прошлая неделя", "А если только за прошлую неделю?"][i % 5], "refinement", "search", count=1, event=event(meeting, date="2026-10-08")),
        ], [seed("a", meeting, date="2026-10-08"), seed("b", meeting), seed("c", "Встреча с бухгалтером", date="2026-10-09")]))
        out.append(scenario(f"fresh-new-{i}", "new_command_while_pending", [
            step(f"Напомни {subject}", "command", "clarify", True),
            step(f"Создай встречу с {name} завтра в 16", "command", "create", event=event(meeting, time="16:00")),
        ]))
        out.append(scenario(f"fresh-cancel-{i}", "cancellation", [
            step(f"Удали задачу {subject}", "command", "clarify", True),
            step(["нет", "отмена", "не надо", "стоп", "Нет, не удаляй"][i % 5], "cancellation", "cancel"),
        ], [seed("a", title, "task", "", "")]))
        out.append(scenario(f"fresh-confirm-{i}", "confirmation", [
            step(f"Удали задачу {subject}", "command", "clarify", True),
            step(["да", "подтверждаю", "да, удали", "да пожалуйста", "подтверждаю удаление"][i % 5], "confirmation", "delete"),
        ], [seed("a", title, "task", "", "")]))
        out.append(scenario(f"fresh-ambiguous-{i}", "ambiguity", [
            step("Покажи встречи на завтра", "command", "list", count=2),
            step(["Перенеси её на 17", "Удали эту", "Заверши её", "Её на послезавтра", "И напомни о ней за час"][i % 5], "reference", "clarify"),
        ], [seed("a", meeting), seed("b", "Созвон с редакцией", time="16:00")]))
        out.append(scenario(f"fresh-timer-{i}", "timer", [
            step([f"Засеки {delay} минут {subject}", f"Таймер на {delay} минут {subject}", f"Поставь таймер минут на {delay} {subject}"][i % 3], "command", "create", event=event(title, "reminder", "2026-10-16", f"12:{30+delay:02d}")),
            step("Спасибо", "chat", "help"),
        ]))
    return out


def main():
    target = ROOT / "testdata" / "semantic"
    freeze = target / "context_manifest.json"
    if freeze.exists():
        raise SystemExit("Context corpora are already frozen. Do not overwrite held-out data.")
    manifest = dict(protocol="DEV can inform changes; fresh holdout runs once after DEV tuning; never tune on holdout.", old_208_used=False, files={})
    for name, rows in [("context_dev.jsonl", build_dev()), ("context_heldout.jsonl", build_holdout())]:
        content = "".join(json.dumps(row, ensure_ascii=False, separators=(",", ":")) + "\n" for row in rows).encode()
        (target / name).write_bytes(content)
        manifest["files"][name] = dict(conversations=len(rows), turns=sum(len(row["turns"]) for row in rows), sha256=hashlib.sha256(content).hexdigest())
    freeze.write_text(json.dumps(manifest, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")
    print(json.dumps(manifest, ensure_ascii=False, indent=2))


if __name__ == "__main__":
    main()
