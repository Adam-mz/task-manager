# Task Manager API

HTTP API для управления задачами на Go с хранением данных в PostgreSQL.

## Возможности

- Регистрация, вход по JWT и доступ только к своим задачам.
- Создание, получение, изменение и удаление задач.
- Статусы: `todo`, `in_progress`, `done`.
- Пагинация списка через `limit` и `offset`.
- Фильтрация по статусу.
- Проверка готовности через `/readyz`.
- Логирование HTTP-запросов и корректное завершение работы.

## Технологии

Go, net/http, pgx, PostgreSQL, golang-migrate, Docker Compose.

## Запуск через Docker

Требуется запущенный Docker Desktop с поддержкой Linux-контейнеров.
Команды выполняются из корня проекта.

Перед запуском задайте `JWT_SECRET` в файле `.env` в корне проекта.
Это случайный ключ размером не менее 32 байт **до кодирования**, записанный в Base64.
Сгенерировать 32 случайных байта и получить Base64 можно в PowerShell:

```powershell
$jwtSecretBytes = New-Object byte[] 32
$rng = [System.Security.Cryptography.RandomNumberGenerator]::Create()
$rng.GetBytes($jwtSecretBytes)
$rng.Dispose()
[Convert]::ToBase64String($jwtSecretBytes)
```

Добавьте в `.env` строку, подставив полученное значение:

```dotenv
JWT_SECRET=<сгенерированное_значение_Base64>
```

Docker Compose читает `.env` и передаёт `JWT_SECRET` в API.
Без ключа, при некорректном Base64 или длине ключа менее 32 байт API не запустится.
Файл `.env` исключён через `.gitignore`; не добавляйте его в коммит.

Запустите API:

```powershell
docker compose up -d --build api
```

Порядок запуска:

1. PostgreSQL запускается и проходит проверку готовности.
2. Миграции применяются к базе.
3. После успешного завершения миграций запускается API.

API доступен по адресу http://localhost:8080.

Проверка готовности:

```powershell
Invoke-RestMethod -Uri "http://localhost:8080/readyz"
```

Ожидаемый ответ: `ready`.

Просмотр состояния и логов:

```powershell
docker compose ps -a
docker compose logs --tail=20 api
```

Контейнер `migrate` со статусом `Exited (0)` означает успешное
завершение миграций.

## Запуск на сервере

Нужны Docker с поддержкой Linux-контейнеров и Docker Compose **2.24.4 или новее**.
Создайте `.env.server` рядом с `compose.yaml` и задайте новые серверные секреты:

```dotenv
TASK_SERVER_DB_PASSWORD=<новый_случайный_пароль_в_hex>
TASK_SERVER_JWT_SECRET=<новый_случайный_ключ_Base64>
```

Для пароля БД используйте 32 случайных байта в hex (64 символа): такой пароль
подходит для URL подключения. JWT-ключ должен содержать не менее 32 случайных байт
до кодирования в Base64. `.env.server` исключён из Git.

Запустите из корня проекта:

```bash
docker compose --env-file .env.server -f compose.yaml -f compose.server.yaml up -d --build api
```

PostgreSQL не публикует порт на хосте. `TASK_SERVER_PORT` можно опустить:
по умолчанию API доступен на `127.0.0.1:8080`. Для локальной репетиции можно
добавить `TASK_SERVER_PORT=18080` в `.env.server`.
Проверка готовности: `curl --fail http://127.0.0.1:8080/readyz`, ожидаемый ответ — `ready`.

Пароль применяется при создании нового тома PostgreSQL; в существующей базе
его нужно менять отдельно, сохраняя данные.

## Остановка

```powershell
docker compose stop
```

Данные PostgreSQL хранятся в Docker-томе и сохраняются при остановке
и пересоздании контейнеров.

Команда `docker compose down -v` удаляет том с данными — для обычной
остановки её использовать не нужно.

## Маршруты

| Метод | Путь | Назначение | Авторизация |
|---|---|---|---|
| POST | `/auth/register` | Зарегистрироваться | Без токена |
| POST | `/auth/login` | Войти и получить токен | Без токена |
| GET | `/readyz` | Проверить доступность PostgreSQL | Без токена |
| POST | `/tasks` | Создать задачу | Bearer-токен |
| GET | `/tasks` | Получить список своих задач | Bearer-токен |
| GET | `/tasks/{id}` | Получить свою задачу | Bearer-токен |
| PUT | `/tasks/{id}` | Обновить свою задачу | Bearer-токен |
| DELETE | `/tasks/{id}` | Удалить свою задачу | Bearer-токен |

## Регистрация и вход

`POST /auth/register`, `POST /auth/login` и `GET /readyz` доступны без токена.
Все маршруты `/tasks` требуют заголовок `Authorization: Bearer <access_token>`.
Сервер определяет владельца по токену; передавать `user_id` в запросах не нужно.
Список содержит только задачи текущего пользователя.

Выполняйте следующие команды и примеры запросов к задачам в одном окне PowerShell.
Сначала зарегистрируйте пользователя; пароль должен содержать от 15 до 128 символов:

```powershell
$credentials = @{
    email = "adam@example.com"
    password = "LocalDemoPassword123!"
} | ConvertTo-Json

Invoke-RestMethod `
    -Method Post `
    -Uri "http://localhost:8080/auth/register" `
    -ContentType "application/json" `
    -Body $credentials
```

Успешная регистрация возвращает `201 Created`. Если пользователь уже зарегистрирован,
перейдите сразу ко входу с его email и паролем.

Сохраните ответ входа в `$login`, затем сформируйте заголовок для запросов к задачам:

```powershell
$login = Invoke-RestMethod `
    -Method Post `
    -Uri "http://localhost:8080/auth/login" `
    -ContentType "application/json" `
    -Body $credentials

$headers = @{ Authorization = "Bearer $($login.access_token)" }
Invoke-RestMethod -Uri "http://localhost:8080/tasks" -Headers $headers
```

Вход возвращает `200 OK`, `access_token` сроком на 15 минут и `token_type: "Bearer"`.
После истечения срока снова выполните вход и обновите `$headers`.
Без действительного токена запросы к `/tasks` возвращают `401 Unauthorized`.
Для чужой или отсутствующей задачи чтение, обновление и удаление возвращают `404 Not Found`.

## Параметры списка

Пример: `/tasks?status=done&limit=10&offset=0`.

| Параметр | По умолчанию | Допустимые значения |
|---|---|---|
| `limit` | `20` | От 1 до 100 |
| `offset` | `0` | Целое число от 0 |
| `status` | Без фильтра | `todo`, `in_progress`, `done` |
| `order` | `asc` | `asc`, `desc`; пустое значение означает `asc` |

Пустой `status` означает отсутствие фильтра.
Некорректные параметры возвращают `400 Bad Request`.

По умолчанию задачи сортируются по ID по возрастанию (`order=asc`).
Для сортировки по убыванию используйте `order=desc`.
Неизвестное значение `order` возвращает `400 Bad Request`.

Порядок обработки: фильтрация → сортировка → пагинация.
Пустой результат возвращается как JSON-массив `[]`.

```powershell
Invoke-RestMethod -Uri "http://localhost:8080/tasks?status=done&order=desc&limit=10&offset=0" -Headers $headers
```

## Примеры запросов

Выполняйте команды в одном окне PowerShell:
`$headers` содержит Bearer-токен из примера входа выше,
а `$createdTask` хранит созданную задачу.

### Создание

```powershell
$createdTask = Invoke-RestMethod `
    -Method Post `
    -Uri "http://localhost:8080/tasks" `
    -Headers $headers `
    -ContentType "application/json" `
    -Body '{"title":"Learn Docker","description":"Run API in a container"}'

$createdTask
```

Успешный ответ: `201 Created`. Начальный статус задачи — `todo`.

### Получение по ID

```powershell
Invoke-RestMethod -Uri "http://localhost:8080/tasks/$($createdTask.id)" -Headers $headers
```

### Обновление

```powershell
Invoke-RestMethod `
    -Method Put `
    -Uri "http://localhost:8080/tasks/$($createdTask.id)" `
    -Headers $headers `
    -ContentType "application/json" `
    -Body '{"title":"Learn Docker","description":"Run API in a container","status":"done"}'
```

### Список выполненных задач

```powershell
Invoke-RestMethod -Uri "http://localhost:8080/tasks?status=done&limit=10&offset=0" -Headers $headers
```

### Удаление

```powershell
Invoke-RestMethod `
    -Method Delete `
    -Uri "http://localhost:8080/tasks/$($createdTask.id)" `
    -Headers $headers
```

Успешный ответ: `204 No Content`, без тела ответа.

## Ошибки

Ошибки авторизации и запросов к задачам возвращаются в JSON.

```json
{"error":"invalid task id"}
```

- `400` — некорректные входные данные.
- `401` — токен отсутствует, недействителен или истёк; при входе — неверные email или пароль.
- `404` — задача не найдена или принадлежит другому пользователю.
- `409` — email уже зарегистрирован.
- `500` — внутренняя ошибка сервера.
- `503` — истёк таймаут операции.

Маршрут `/readyz` возвращает текст:
`ready` со статусом `200` или `not ready` со статусом `503`.
Он проверяет соединение с PostgreSQL, но не наличие таблиц.

## Тестирование

Команды выполняются из корня проекта. Для запуска тестов нужен установленный Go версии, указанной в `go.mod`.

Запуск всех тестов:

```powershell
go test -race -count=1 -timeout=60s ./...
```

Интеграционные тесты PostgreSQL используют отдельную базу
`task_manager_test`. Если переменная `TEST_DATABASE_URL` не задана,
эти тесты пропускаются.

### Подготовка тестовой базы

Запустите PostgreSQL и дождитесь готовности:

```powershell
docker compose up -d --wait postgres
```

Создайте тестовую базу один раз:

```powershell
docker compose exec postgres psql -U task_manager -d task_manager -c "CREATE DATABASE task_manager_test;"
```

Если база уже существует, повторять создание не нужно.

Примените миграции к тестовой базе:

```powershell
docker compose run --rm --entrypoint migrate migrate `
    -path=/migrations `
    -database "postgres://task_manager:task_manager_password@postgres:5432/task_manager_test?sslmode=disable" `
    up
```

При повторном запуске сообщение `no change` означает, что все миграции уже применены.

Для уже подготовленной тестовой базы с применёнными миграциями:

```powershell
$env:TEST_DATABASE_URL = "postgres://task_manager:task_manager_password@localhost:5432/task_manager_test?sslmode=disable"

go test -race -count=1 -timeout=60s ./...
```

Запуск только тестов PostgreSQL:

```powershell
go test -race -count=1 -timeout=60s -run '^TestPostgresRepository' -v ./internal/task
```

Тесты проверяют CRUD, обработку ошибок, ограничения базы,
пагинацию и фильтрацию по статусу.

Переменная окружения задаётся для текущего окна PowerShell.
