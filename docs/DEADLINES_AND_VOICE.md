# Дедлайны и голосовой ввод

`simple_llm` принимает обычные формулировки дел и дедлайнов как `create_task`.
«Мне нужно подготовить смету до двадцатого» даёт title «Подготовить смету»,
date `{"type":"day_of_month","day":20}` и time null. Временная часть не входит
в название. Для задачи время необязательно; правила встреч и напоминаний сохранены.
NLU не вычисляет месяц и год: Go выбирает ближайший существующий день в timezone
пользователя, включая сегодняшний. После 20-го выбирается следующий месяц; для
31-го короткие месяцы пропускаются. Неверные числа и несовместимые temporal поля
отклоняются. Legacy SemanticPlan не участвует.

Микрофон использует getUserMedia и MediaRecorder, предпочтительно
`audio/webm;codecs=opus`, затем поддерживаемый MIME. Первое нажатие: «Слушаю…»,
второе останавливает запись: «Распознаю…». Результат Groq вставляется в существующий
черновик. Пользователь проверяет и редактирует его, затем сам нажимает Send/Enter.
Во время записи и распознавания Send отключён. Правка черновика отменяет
незавершённое распознавание; поздний ответ его не перезапишет. Ограничение записи:
2 минуты и 15 МиБ. При ошибке текст остаётся. Автоматического browser
SpeechRecognition fallback нет.

`stt.Transcriber` отделён от authenticated `POST /api/voice/transcribe`.
Принимается один multipart part `file` (также `audio`), MIME audio/webm, audio/ogg,
audio/wav, audio/mpeg или audio/mp4; codecs разрешены. Пустая запись: 400,
слишком большая: 413, неподдерживаемый MIME: 415, STT unavailable: безопасный 503.
Cookie auth и CSRF обязательны. Multipart разрешён только этому маршруту при
явном совпадении Origin с PUBLIC_URL; остальные mutations остаются JSON-only.

Backend configuration:

```dotenv
GENA_NLU_MODE=simple_llm
GENA_SEMANTIC_PROVIDER=gigachat
GENA_STT_PROVIDER=groq
GROQ_STT_MODEL=whisper-large-v3-turbo
```

`GROQ_API_KEY` передаётся отдельно через secret environment, без вывода в отчёты,
Dockerfile/frontend или Git. Клиент Groq переиспользует http.Client, имеет timeout
30 секунд, передаёт context cancellation и запрещает redirects. Response ограничен
64 КиБ; HTTP errors, invalid JSON и пустой transcript не раскрывают provider body.
Форма содержит file, model, language=ru, response_format=json, temperature=0.
[Документация Groq](https://console.groq.com/docs/speech-to-text).

Audio не пишется на диск и не логируется. Используется MultipartReader вместо
ParseMultipartForm. Nginx voice location имеет limit 16 МиБ (с multipart overhead)
и proxy_request_buffering off; остальные маршруты сохраняют прежние ограничения.
Секреты выпуска передаются по SSH stdin в root-owned production.env (0600).
Release backup включает PostgreSQL, env, unit и Nginx; rollback возвращает
конфигурацию и image, сохраняя пользовательские данные.

Проверки: temporal edge cases, Groq mock HTTP errors/timeout/cancellation/response
limits/redirects, endpoint validation/auth/CSRF, frontend state/cancellation/manual
Send и browser UI с явно обозначенным mock capture/STT. Эти mocks не подтверждают
качество реального голоса. `testdata/simple_llm/deadlines.json` — небольшой live
smoke через настоящий GigaChat в disposable базе:

```sh
go run ./cmd/simple-smoke -mode live -input testdata/simple_llm/deadlines.json
go test -count=1 ./...
go vet ./...
cd web
pnpm exec tsc --noEmit
pnpm test
pnpm build
```

До выпуска обязательны пять реальных записей на устройстве и проверка их
transcripts, ручной отправки и создания задачи. Synthetic audio и fixtures не
считаются проверкой качества микрофона.
