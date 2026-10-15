# Знакомство с Геной и Яндекс Календарь

Главная страница `/` содержит чат Гены (`TodayPage`). Новый пользователь видит приветствие и предложение подключить календарь. Branding: Гена / Ассистент Гена.

Кнопка подключения сразу открывает `/api/auth/yandex/start?calendar=1&source=onboarding`. Backend требует сессию, создаёт одноразовый browser-bound state с PKCE, purpose=calendar и source=onboarding. Допустимые источники — onboarding и integrations; внешние return URL игнорируются.

После успешного callback существующий Calendar.Connect шифрует и сохраняет credentials, ставит initial sync в прежнюю очередь и будит calendar worker. Backend завершает onboarding и возвращает `/?calendar=connected`. Успешное получение credentials имеет состояние pending до окончания sync; пользователь сразу может работать в чате, статус и ошибки sync доступны в «Интеграциях».

«Сделать позже» вызывает существующий `POST /api/auth/onboarding` и сразу открывает `/`. Если календарь уже connected/pending, onboarding завершает знакомство без второго согласия. Пока состояние подключения загружается, кнопка подключения ожидает результат.

Отмена OAuth или ошибка с проверенным state возвращает `/onboarding?error=...`: «Не удалось подключить Яндекс Календарь», «Попробовать ещё раз», «Сделать позже». Нет второго обязательного нажатия в «Интеграциях». При недействительном/истёкшем state источник нельзя доверенно восстановить; незавершённый пользователь возвращается в onboarding по состоянию аккаунта.

Обычная кнопка в «Интеграциях» использует source=integrations (также default для прежних клиентов без source). Её успех/ошибка возвращаются в `/integrations?service=yandex`. Управление, reconnect, manual sync, disconnect и диагностика сохранены. Обычный Яндекс-вход и linking не меняют свои назначения.

Проверки: unit-тесты OnboardingPage, PostgreSQL callback/state/initial sync tests и Playwright OAuth mock. Настоящее согласие в пользовательском аккаунте Яндекса проверяется вручную. [Календарь](YANDEX_CALENDAR.md), [Web Push](WEB_PUSH.md).
