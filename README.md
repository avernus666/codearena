# AvernaArena

AvernaArena — платформа соревновательного программирования: аккаунты, каталог алгоритмических задач, отправка решений, история проверок, таблица рейтинга и очередь поиска соперника. Репозиторий включает frontend, Go API, PostgreSQL и отдельный Rust judge, который запускает Python-решения в одноразовых Docker-контейнерах с ограничениями ресурсов.

## Состояние реализации

- Аккаунты: регистрация/вход/выход, серверная сессия в HttpOnly cookie, хеширование пароля PBKDF2-HMAC-SHA256, валидация и ограничения запросов.
- Каталог задач и тестовые примеры загружаются из PostgreSQL.
- Отправления сохраняются в БД и отображаются в истории пользователя.
- Judge: Python 3.12; лимит кода 64 KiB, контейнер без сети, read-only root filesystem, лимит памяти 128 MiB, CPU 0.5, ограничение процессов, drop capabilities, no-new-privileges и timeout.
- Рейтинг показывает реальные зарегистрированные аккаунты; стартовый рейтинг 1200. Автоматическое изменение рейтинга по завершении дуэли пока не реализовано.
- Matchmaking создаёт комнаты и сохраняет очередь в PostgreSQL. Участники видят общий редактор через сохраняемые события и опрос API; это polling-синхронизация, а не WebSocket. Первое принятое решение завершает дуэль. В рейтинговом режиме обновляются Elo-подобный рейтинг и W/L; в casual рейтинг не меняется.
- Docker Compose, health checks для БД/frontend/judge, CI для Node.js, Go и Rust.

## Требования

- Docker Engine и Docker Compose v2.
- Для judge нужен доступ к Docker daemon через `/var/run/docker.sock`. Это даёт сервису judge полномочия Docker-хоста. Для публичного production-развёртывания запускайте judge на отдельной изолированной машине/VM с отдельным Docker daemon, не на хосте с секретами или другими сервисами. Не выставляйте порт judge наружу.

## Локальный запуск

1. Скопируй `.env.example` в `.env`.
2. Замени `POSTGRES_PASSWORD` на длинный пароль и `SESSION_SECRET` на случайную строку не менее 32 байт.
3. Запусти из корня репозитория:

   ```bash
   docker compose up --build -d
   ```

4. Открой `http://localhost:8080`.
5. Проверка API: `http://localhost:8080/healthz`.
6. Логи: `docker compose logs -f api judge frontend`.
7. Остановка: `docker compose down`. Чтобы также удалить базу данных, используй `docker compose down -v`.

При первом запуске Docker скачает базовые образы, включая `python:3.12-alpine`, используемый для изолированной проверки. Для скачивания образов нужен интернет. SQL-файл в `backend/migrations/001_init.sql` автоматически применяется только при создании нового тома PostgreSQL. Если база уже создана и схему надо сбросить в локальной среде, останови compose и выполни `docker compose down -v`, затем запусти заново; команда удалит все локальные данные.

## Локальная разработка

Frontend:

```bash
cd frontend
npm install
npm run check
npm test
npm run dev
```

Для отдельной разработки через Vite сначала подними Compose, чтобы API-прокси был доступен на `http://localhost:8080`, затем запускай Vite на `http://localhost:5173`. `vite.config.js` проксирует `/api` и `/healthz` на Compose frontend reverse proxy.

Go API:

```bash
cd backend
go mod tidy
go test ./...
go vet ./...
go run ./cmd/api
```

API требует PostgreSQL и переменные `DATABASE_URL`, `SESSION_SECRET`; адрес по умолчанию `:8080`.

Rust judge:

```bash
cd judge
cargo test
cargo run
```

Judge требует доступный Docker daemon и слушает `:8090`. Не публикуй его порт в интернет.

## API

Все API-маршруты используют префикс `/api/v1`. Дополнительные room-маршруты: `GET /rooms/{id}/events?after=<event-id>` и `POST /rooms/{id}/events` (типы `editor_update`, `message`, payload до 16 KiB):

| Метод | Путь | Назначение |
|---|---|---|
| GET | `/tasks` | Опубликованные задачи |
| POST | `/auth/register` | Создание аккаунта |
| POST | `/auth/login` | Вход |
| POST | `/auth/logout` | Выход |
| GET | `/auth/me` | Текущий пользователь |
| GET | `/leaderboard` | Таблица рейтинга |
| POST | `/submissions` | Создать отправление |
| GET | `/submissions` | История текущего пользователя |
| POST | `/matchmaking/queue` | Встать в очередь (`ranked` или `casual`) |
| DELETE | `/matchmaking/queue` | Покинуть очередь |
| GET | `/matchmaking/status` | Статус очереди |
| GET | `/rooms/{id}` | Данные комнаты для её участников |

Регистрация JSON: `{"handle":"Averna","email":"user@example.com","password":"a-long-password"}`. Вход: `{"login":"Averna","password":"a-long-password"}`. Отправление: `{"taskId":"sum-two","language":"python","code":"a,b=map(int,input().split())\\nprint(a+b)"}`.

## Архитектура

- `frontend/` — responsive React UI, Vite, Nginx, API integration.
- `backend/cmd/api/` — HTTP API, сессии, matchmaking и репозиторные запросы к PostgreSQL.
- `backend/migrations/` — SQL-схема и стартовый каталог задач.
- `judge/` — Rust HTTP worker и запуск Python-кода в отдельном контейнере на каждый тест.
- `.github/workflows/ci.yml` — frontend checks/tests/build, Go tests/vet/build, Rust check/tests и Compose config validation.

## Известные границы

Это запускаемая базовая соревновательная платформа, но не законченный турнирный продукт: WebSocket-транспорт (редактор синхронизируется polling-запросами), таймер и система отмены матча, турниры, восстановление пароля, email verification, rate limiting на уровне reverse proxy, админ-модерация и интеграционные тесты с реальным PostgreSQL не реализованы. Judge поддерживает только Python 3.12 и небольшой встроенный набор тестов для задач из начальной миграции. Для production нужно вынести runner на отдельную изолированную VM, добавить лимиты частоты запросов, мониторинг, резервное копирование и проверку зависимостей.
