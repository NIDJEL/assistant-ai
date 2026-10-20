# Страница авторизации «Гена»

Визуальная система страницы `/auth` реализована в текущем React/Vite-приложении. Двухколоночная сцена сохраняется при ширине больше 900 px; на меньших экранах остаются форма, логотип и компактный Гена. Страница занимает 100dvh без прокрутки документа. Вход и регистрация целиком помещаются на проверенных экранах 320×740–1920×1080: на невысоких экранах сокращаются интервалы и высоты полей. Для экранной клавиатуры и экстремально малой доступной высоты сохраняется прокрутка внутри панели, чтобы элементы формы оставались доступны.

Текст и карточки смещены влево независимо от персонажа, с ограничением по доступному полю экрана. Удалены дополнительные подписи рядом с логотипом, над заголовком, под карточками и нижний футер. Отсутствие пересечения текста с головой персонажа, обрезанных элементов формы и вертикальной прокрутки проверяется в браузерном сценарии.

## Компоненты и движение

- `web/src/pages/Auth/AuthScene.tsx`: фон, персонаж с независимым слоем глаз, три переиспользуемые карточки.
- `web/src/pages/Auth/useAuthMotion.ts`: затухающая пружина для указателя. Координаты записываются в CSS-переменные без React state. После достижения цели `requestAnimationFrame` прекращается.
- `web/src/styles/auth-premium.css`: сцена, адаптивность, появления, взаимодействия и состояния формы. Стили изолированы классом `auth-premium`.

Сохранён существующий механизм CSS/Web Animations и `AutoHeight`. Новые анимационные зависимости не требуются. Частые движения используют transform/translate и opacity. Высота анимируется только при изменении содержимого формы; яркость персонажа меняется только при наведении.

В `AutoHeight` измерения после уведомлений `ResizeObserver` объединены в следующий кадр: вложенные переходы формы больше не запускают цикл доставки наблюдателя. Все браузерные сценарии дополнительно проверяют отсутствие необработанных ошибок страницы.

Появление: фон 0–300 мс, логотип 150–650 мс, строки заголовка 300–900 мс, персонаж 500–1100 мс, карточки 650/770/890 мс с длительностью 360 мс, панель 400–1000 мс. Форма доступна во время появления.

Гена парит на 12 px с периодом 6 с и поворотом ±1°. Моргание использует отдельные глаза: интервалы 7,6 / 6,3 / 5,6 / 6,5 с. Фон движется с периодами 23–29 с. Наклон карточек ограничен 3,5° по каждой оси, подъём — 6 px.

`prefers-reduced-motion` и настройка приложения `data-motion="reduced"` отключают движение. На touch-устройствах нет параллакса и эффектов, требующих hover. При `document.hidden` CSS-анимации приостанавливаются, пружины сбрасываются.

## Авторизация

Сохранены существующие `login`, `register`, проверка паролей, блокировка повторной отправки, версия и содержимое согласий, обработка OAuth-ошибок и переход через `/api/auth/yandex/start`. Добавлены ARIA-связи вкладок и сообщений об ошибках, переключение вкладок стрелками/Home/End. Ссылки документов открывают настоящие маршруты проекта.

В существующем backend нет восстановления пароля по почте. Ссылка сохраняет прежний информационный обработчик; имитация отправки письма не добавлялась.

## Проверка

Из `web/`:

```powershell
pnpm build
pnpm test
# В другом терминале: pnpm dev --port 5173
$env:P1_BASE_URL = 'http://127.0.0.1:5173'
pnpm exec playwright test e2e/auth-scene.spec.ts --reporter=list
node scripts/auth-preview.mjs
```

Браузерные проверки сцены используют контролируемые API-ответы и проверяют размеры 320–1920 px, обе формы, согласия, прежние payload входа и Яндекса, блокировку повторов, клавиатуру, reduced motion, завершение пружин, паузу неактивной вкладки и touch. Они не подтверждают вход в реальный аккаунт провайдера. Существующие интеграционные сценарии с сервером и PostgreSQL остаются в `web/e2e/auth.spec.ts`.

Сборка проходит; 113 unit-тестов и 8 браузерных проверок сцены прошли. Vite сохраняет предупреждение об общем JS-чанке приложения больше 500 kB; новая анимационная библиотека не добавлялась.

Скриншоты сохраняются в `artifacts/auth-premium/`. Для замера добавьте `$env:AUTH_PROFILE='1'` перед запуском `node scripts/auth-preview.mjs`; результат записывается в `performance.json`.

В доступном headless Chromium использовался программный рендерер ANGLE/SwiftShader: на 180 кадрах с движением указателя медиана интервала составила 33,3 мс, p95 — 50 мс, задач главного потока дольше 50 мс не обнаружено. Это не подтверждает 60 FPS. Целевая частота на устройстве с аппаратным ускорением требует отдельной проверки в обычном браузере. Затухающая пружина прекращает запросы кадров в покое; это проверено отдельно от CSS-анимаций сцены.

## Иллюстрации

Использован встроенный **image_gen**, по одному запросу на ассет, с `transparent_background: true`. Результаты конвертированы в WebP с сохранением alpha, без изменения изображения. Четыре файла суммарно занимают 436 058 байт:

| Ассет | Файл в проекте | Размер |
| --- | --- | ---: |
| Персонаж | `web/src/assets/auth/gena.webp` | 84 246 |
| Календарь | `web/src/assets/auth/calendar.webp` | 130 052 |
| Задачи | `web/src/assets/auth/tasks.webp` | 113 398 |
| Интеграции | `web/src/assets/auth/integrations.webp` | 108 362 |

Ниже сохранены финальные запросы, переданные инструменту.

### Персонаж

```text
Use case: stylized-concept. Asset type: transparent PNG hero mascot for premium Russian AI assistant Gena authentication website. Create a single beautiful minimalist friendly 3D white-lavender ghost, rounded soft ceramic balloon material, floating front view turned very slightly left, full body visible, softly scalloped rounded bottom, two tiny rounded arms, left arm waving, right arm holding a dark midnight blue rounded square tablet against belly. On tablet a simple thick cyan-to-indigo smooth wavy line like lowercase n. ABSOLUTELY NO EYES, NO FACE FEATURES on head: keep the upper third front face entirely blank white-lavender so website can animate independent eyes there. Premium polished product render, like Yandex Pay 3D advertising, believable volume, refined blue violet edge lighting from right, white softbox upper left, soft occlusion under arms, clean precise silhouette. Ghost occupies 85 percent of square image, centered, isolated on genuine transparent alpha background. No ground plane, no cast shadow outside silhouette, no extra floating objects, no text, no watermark, no background. One mascot only.
```

### Календарь

```text
Use case: stylized-concept. Asset type: illustration for feature card on dark navy authentication website. A single beautiful premium 3D desk calendar icon, white ceramic rounded square pages with electric blue top header, two lavender rounded binder rings, blue rounded raised date tile with white number 31 on front. Slightly rotated counterclockwise in three quarter perspective. Blue violet sides, soft white studio light top left, realistic material and ambient occlusion. Premium Yandex Pay advertising product render, restrained, no neon. Full object centered, occupies 82% of square, isolated on genuine transparent alpha background, no ground plane, no surrounding shadow, no extra symbols, no text except 31.
```

### Задачи

```text
Use case: stylized-concept. Asset type: illustration for feature card on dark navy authentication website. A single beautiful premium 3D checklist icon, rounded white ceramic clipboard tilted slightly clockwise, three vertically stacked raised electric blue square checkboxes with white check marks, three short recessed lavender horizontal lines alongside them. Small soft golden yellow rounded 3D five point star at bottom right touching clipboard. Violet thick sides, soft white studio light top left, realistic material and ambient occlusion. Premium Yandex Pay advertising product render, restrained, no neon. Full object centered, occupies 82% of square, isolated on genuine transparent alpha background, no ground plane, no surrounding shadow, no text.
```

### Интеграции

```text
Use case: stylized-concept. Asset type: illustration for feature card on dark navy authentication website. A single composed beautiful premium 3D integrations icon: a white soft ceramic rounded square with raised red Cyrillic letter Я, behind and upper left of electric blue rounded square tile with simple white video camera symbol. A smaller lavender rounded square link symbol tile partly behind upper right. Three tiles nestled together in dynamic balanced three quarter perspective. Same style soft ceramic rounded forms, violet blue sides, soft white studio light top left, believable ambient occlusion. Premium Yandex Pay advertising product render, restrained, no neon. Full composition centered, occupies 82% of square, isolated on genuine transparent alpha background, no ground plane, no surrounding shadow, no text except Я.
```
