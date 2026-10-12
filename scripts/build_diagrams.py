"""Generate portable SVG block diagrams without a CDN or diagram service."""

from html import escape
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
OUT = ROOT / "docs/diagrams"
OUT.mkdir(parents=True, exist_ok=True)


def canvas(width, height, title, subtitle):
    return [
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {width} {height}" role="img" aria-label="{escape(title)}">',
        '<defs><marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M 0 0 L 10 5 L 0 10 z" fill="#718a79"/></marker></defs>',
        f'<rect width="{width}" height="{height}" rx="20" fill="#f7f9f3"/>',
        '<g font-family="Segoe UI,Arial,sans-serif" fill="#183b3a">',
        f'<text x="32" y="45" font-size="25" font-weight="600">{escape(title)}</text>',
        f'<text x="32" y="74" font-size="14" fill="#778875">{escape(subtitle)}</text>',
    ]


def box(parts, x, y, width, height, title, lines=(), dark=False):
    fill, ink = ("#183b3a", "#ffffff") if dark else ("#ffffff", "#183b3a")
    parts.append(f'<rect x="{x}" y="{y}" width="{width}" height="{height}" rx="13" fill="{fill}" stroke="#dbe5d3"/>')
    parts.append(
        f'<text x="{x + width / 2}" y="{y + 30}" text-anchor="middle" font-size="16" font-weight="600" fill="{ink}">{escape(title)}</text>'
    )
    for i, line in enumerate(lines):
        font_size = min(12, (width - 18) / max(len(line) * 0.56, 1))
        parts.append(
            f'<text x="{x + width / 2}" y="{y + 55 + i * 21}" text-anchor="middle" font-size="{font_size:.1f}" fill="{ink if dark else "#778875"}">{escape(line)}</text>'
        )


def arrow(parts, points, label=None, lx=None, ly=None, dashed=False):
    pts = " ".join(f"{x},{y}" for x, y in points)
    dash = ' stroke-dasharray="6 5"' if dashed else ""
    parts.append(f'<polyline points="{pts}" fill="none" stroke="#718a79" stroke-width="2" marker-end="url(#arrow)"{dash}/>')
    if label:
        parts.append(f'<text x="{lx}" y="{ly}" font-size="12" fill="#628067">{escape(label)}</text>')


def diamond(parts, x, y, width, height, title):
    pts = f"{x + width / 2},{y} {x + width},{y + height / 2} {x + width / 2},{y + height} {x},{y + height / 2}"
    parts.append(f'<polygon points="{pts}" fill="#eaf1e3" stroke="#b5c9aa"/>')
    parts.append(f'<text x="{x + width / 2}" y="{y + height / 2 + 5}" text-anchor="middle" font-size="14">{escape(title)}</text>')


def save(parts, name):
    (OUT / name).write_text("\n".join(parts + ["</g></svg>"]), encoding="utf-8")


parts = canvas(1200, 540, "01 / Архитектура приложения", "Голос и текст проходят один путь; изменения выполняет только бизнес-логика.")
boxes = [
    (30, "Пользователь", ["Голос или текст", "Свободный русский язык"]),
    (225, "Браузер", ["Speech-to-Text → текст", "История + Text-to-Speech"]),
    (420, "FastAPI", ["POST /api/chat", "Проверка входных данных"]),
    (615, "Интерпретатор", ["Действие, дата, время", "Русский NLP / Ollama"]),
    (810, "Бизнес-логика", ["Уточнения и конфликты", "Проверка перед записью"]),
    (1005, "SQLite", ["События и диалоги", "Атомарные транзакции"]),
]
for x, title, lines in boxes:
    box(parts, x, 140, 165, 115, title, lines, dark=title == "Бизнес-логика")
for i in range(5):
    arrow(parts, [(boxes[i][0] + 165, 197), (boxes[i + 1][0], 197)])
box(parts, 565, 342, 255, 98, "Ollama · опционально", ["Локальная модель → JSON", "Ошибка → русский NLP"])
arrow(parts, [(697, 342), (697, 255)], dashed=True)
box(parts, 880, 342, 275, 98, "Поиск по архиву", ["Словоформы + синонимы", "Опечатки + ранжирование"])
arrow(parts, [(1090, 255), (1090, 342)])
arrow(parts, [(880, 391), (858, 391), (858, 255)])
arrow(parts, [(895, 140), (895, 108), (307, 108), (307, 140)], "Текст ответа и события → озвучивание в браузере", 425, 102)
parts.append(
    '<text x="32" y="502" font-size="13" fill="#778875">SQLite хранится в var/assistant.sqlite3; Docker использует постоянный том rhythm-data.</text>'
)
save(parts, "architecture.svg")

parts = canvas(
    1120,
    1110,
    "02 / Как обрабатывается запрос",
    "Уточнение не создаёт событие. Повторная доставка одного request_id не повторяет действие.",
)
box(parts, 390, 105, 320, 77, "Сообщение + session_id", ["Голос уже преобразован в текст"])
arrow(parts, [(550, 182), (550, 207)])
diamond(parts, 410, 207, 280, 92, "Запрос уже выполнен?")
box(parts, 800, 215, 275, 83, "Вернуть сохранённый ответ", ["Без повторного изменения данных"])
arrow(parts, [(690, 253), (800, 253)], "Да", 735, 245)
arrow(parts, [(550, 299), (550, 332)], "Нет", 561, 319)
box(parts, 390, 332, 320, 90, "Определить действие и поля", ["NLP / Ollama → валидированный Command", "Учесть контекст текущего диалога"])
arrow(parts, [(550, 422), (550, 462)])
diamond(parts, 410, 462, 280, 92, "Создание / изменение?")
box(parts, 800, 466, 275, 90, "Список или поиск", ["Выбрать записи по периоду / теме", "Нормализовать и ранжировать"])
arrow(parts, [(690, 508), (800, 508)], "Нет", 735, 500)
arrow(parts, [(550, 554), (550, 593)], "Да", 561, 577)
diamond(parts, 410, 593, 280, 92, "Данных достаточно?")
box(parts, 35, 593, 280, 100, "Задать уточняющий вопрос", ["Дата, время или номер записи", "Сохранить pending в SQLite"])
arrow(parts, [(410, 639), (315, 639)], "Нет", 353, 630)
arrow(parts, [(175, 593), (175, 143), (390, 143)], "Следующий ответ", 48, 126, dashed=True)
arrow(parts, [(550, 685), (550, 725)], "Да", 561, 710)
diamond(parts, 410, 725, 280, 92, "Есть конфликт встреч?")
box(parts, 800, 728, 275, 96, "Уточнить конфликт", ["Другое время / да / отмена", "Перепроверить перед сохранением"])
arrow(parts, [(690, 771), (800, 771)], "Да", 735, 760)
arrow(parts, [(1075, 771), (1102, 771), (1102, 95), (550, 95), (550, 105)], "Ответ → повторная проверка", 765, 90, dashed=True)
arrow(parts, [(550, 817), (550, 854)], "Нет / подтверждено", 564, 841)
box(parts, 390, 854, 320, 90, "Выполнить действие", ["Параметризованный SQL", "Событие + контекст + ответ атомарно"], dark=True)
arrow(parts, [(550, 944), (550, 979)])
box(parts, 390, 979, 320, 80, "Показать и озвучить ответ", ["Браузер обновляет список событий"])
arrow(parts, [(937, 556), (1089, 556), (1089, 1019), (710, 1019)])
parts.append(
    '<text x="32" y="1090" font-size="12" fill="#778875">Удаление требует отдельного подтверждения; при неоднозначном выборе ассистент спрашивает номер записи.</text>'
)
save(parts, "request-flow.svg")

parts = canvas(
    1200,
    420,
    "03 / Как работает поиск старых записей",
    "Пример: «Найди задачу про контракт с Иваном» находит «Проверить договор с Иваном».",
)
search_boxes = [
    (30, "Запрос", ["Убрать команду и даты", "Оставить тему и имя"]),
    (267, "Словоформы", ["Иваном → Иван", "договору → договор"]),
    (504, "Смысловые группы", ["контракт ↔ договор", "Презинтация → презентация"]),
    (741, "Оценка совпадения", ["Редкие слова имеют больший вес", "Порог + устойчивость к опечаткам"]),
    (978, "Результаты", ["Отсортировать по оценке", "Показать даты, тип и номер"]),
]
for x, title, lines in search_boxes:
    box(parts, x, 155, 193, 115, title, lines, dark=title == "Результаты")
for i in range(4):
    arrow(parts, [(search_boxes[i][0] + 193, 211), (search_boxes[i + 1][0], 211)])
parts.append(
    '<text x="32" y="337" font-size="14" fill="#778875">Поиск включает прошлые и выполненные записи. Фильтр периода применяется только при явном указании.</text>'
)
parts.append(
    '<text x="32" y="366" font-size="13" fill="#778875">Это морфологический поиск со словарём синонимов. Векторные embeddings можно добавить следующим шагом.</text>'
)
save(parts, "search.svg")
print("Generated 3 block diagrams")
