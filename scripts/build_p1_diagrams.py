"""Generate the P1 Go/React and manual-send voice diagrams as native SVG."""

from html import escape
from pathlib import Path

OUT = Path(__file__).resolve().parent.parent / "docs/diagrams"


def canvas(width, height, title, subtitle):
    return [
        f'<svg xmlns="http://www.w3.org/2000/svg" viewBox="0 0 {width} {height}" role="img" aria-label="{escape(title)}">',
        '<defs><marker id="arrow" viewBox="0 0 10 10" refX="9" refY="5" markerWidth="7" markerHeight="7" orient="auto-start-reverse"><path d="M0 0 L10 5 L0 10Z" fill="#6286cd"/></marker></defs>',
        f'<rect width="{width}" height="{height}" rx="22" fill="#f2f7ff"/>',
        '<g font-family="Segoe UI,Arial,sans-serif" fill="#182653">',
        f'<text x="32" y="46" font-size="27" font-weight="700">{escape(title)}</text>',
        f'<text x="32" y="78" font-size="15" fill="#6076a5">{escape(subtitle)}</text>',
    ]


def box(parts, x, y, width, height, title, lines=(), accent=False):
    ink = "#ffffff" if accent else "#172755"
    parts.append(f'<rect x="{x}" y="{y}" width="{width}" height="{height}" rx="16" fill="{"#0875ff" if accent else "#ffffff"}" stroke="#dce8fc"/>')
    parts.append(f'<text x="{x + width / 2}" y="{y + 31}" text-anchor="middle" font-size="17" font-weight="650" fill="{ink}">{escape(title)}</text>')
    for i, line in enumerate(lines):
        parts.append(f'<text x="{x + width / 2}" y="{y + 59 + 23 * i}" text-anchor="middle" font-size="13" fill="{ink if accent else "#6076a5"}">{escape(line)}</text>')


def arrow(parts, points, label="", x=0, y=0, dashed=False):
    coords = " ".join(f"{a},{b}" for a, b in points)
    dash = ' stroke-dasharray="7 5"' if dashed else ""
    parts.append(f'<polyline points="{coords}" fill="none" stroke="#6286cd" stroke-width="2" marker-end="url(#arrow)"{dash}/>')
    if label:
        parts.append(f'<text x="{x}" y="{y}" font-size="13" fill="#496baa">{escape(label)}</text>')


def save(parts, filename):
    (OUT / filename).write_text("\n".join(parts + ["</g></svg>"]), encoding="utf-8")


def main():
    OUT.mkdir(parents=True, exist_ok=True)
    parts = canvas(1200, 740, "Ритм / P1: React → Go → SQLite", "Реальные события из API. Python reference остаётся в репозитории.")
    nodes = [
        (30, "React в браузере", ["8 страниц + widgets", "Draft, STT и TTS"]),
        (265, "Typed API client", ["Chat / history / CRUD", "Понятные ошибки"]),
        (500, "Go HTTP API", ["DTO и validation", "SPA routes + assets"]),
        (735, "Go use cases", ["Диалог и idempotency", "Конфликты и поиск"]),
        (970, "SQLite", ["WAL / FK / timeout", "Event + state + history"]),
    ]
    for x, title, lines in nodes:
        box(parts, x, 140, 200, 120, title, lines, accent=title == "Go use cases")
    for i in range(4):
        arrow(parts, [(nodes[i][0] + 200, 200), (nodes[i + 1][0], 200)])
    box(parts, 630, 350, 300, 112, "Hybrid interpreter", ["Parser → confidence → validation", "До BEGIN IMMEDIATE"])
    box(parts, 260, 350, 290, 112, "Ollama · опционально", ["HTTP → structured Command", "Низкая confidence / fallback"])
    arrow(parts, [(550, 402), (630, 402)], "JSON", 571, 390, True)
    arrow(parts, [(835, 350), (835, 260)])
    arrow(parts, [(1085, 140), (1085, 112), (130, 112), (130, 140)], "Ответ, события и состояние → React → TTS", 412, 105)
    box(parts, 50, 564, 280, 105, "Node build stage", ["TypeScript + Vite → web/dist", "Node нужен только при сборке"])
    box(parts, 450, 564, 240, 105, "Go build stage", ["go build → server binary"])
    box(parts, 850, 564, 300, 105, "Production runtime", ["Go binary + React static assets", "Без Node / Python / FastAPI"], True)
    arrow(parts, [(330, 618), (395, 618), (395, 530), (1000, 530), (1000, 564)])
    arrow(parts, [(690, 618), (850, 618)])
    parts.append('<text x="32" y="711" font-size="14" fill="#6076a5">LLM и будущие calendar HTTP requests выполняются вне SQLite write transaction.</text>')
    save(parts, "p1-architecture.svg")

    parts = canvas(1000, 1070, "Ритм / P1: диктовка остаётся черновиком", "Только явный Send или Enter вызывает POST /api/chat.")
    box(parts, 310, 115, 430, 93, "Нажать microphone", ["Включить recognition: ru-RU"])
    box(parts, 310, 265, 430, 93, "recognition.onresult", ["Partial/final объединяются без дублей"])
    box(parts, 310, 420, 430, 110, "Редактируемый draft", ["Можно изменить текст клавиатурой", "Можно продолжить диктовку"])
    box(parts, 32, 275, 222, 110, "Пауза / onend", ["HTTP request отсутствует", "Черновик сохраняется"])
    arrow(parts, [(310, 310), (254, 310)])
    arrow(parts, [(143, 385), (143, 475), (310, 475)])
    arrow(parts, [(525, 208), (525, 265)])
    arrow(parts, [(525, 358), (525, 420)])
    parts.append('<polygon points="525,590 730,655 525,720 320,655" fill="#e2efff" stroke="#a6c8fc"/>')
    parts.append('<text x="525" y="660" text-anchor="middle" font-size="18" font-weight="600">Send / Enter?</text>')
    arrow(parts, [(525, 530), (525, 590)])
    arrow(parts, [(320, 655), (143, 655), (143, 475)], "Нет", 218, 641)
    box(parts, 310, 785, 430, 95, "Остановить recognition", ["Late onend/result игнорируются"])
    arrow(parts, [(525, 720), (525, 785)], "Да", 539, 756)
    box(parts, 310, 938, 430, 100, "Один POST /api/chat", ["Mutex + UUID request_id", "Ответ Go → conversation → TTS"], True)
    arrow(parts, [(525, 880), (525, 938)])
    save(parts, "p1-voice-flow.svg")


if __name__ == "__main__":
    main()
