# Task Manager API

HTTP API для управления задачами на Go с хранением данных в PostgreSQL.

## Возможности

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

## Остановка

```powershell
docker compose stop
```

Данные PostgreSQL хранятся в Docker-томе и сохраняются при остановке
и пересоздании контейнеров.

Команда `docker compose down -v` удаляет том с данными — для обычной
остановки её использовать не нужно.

## Маршруты

| Метод | Путь | Назначение |
|---|---|---|
| POST | `/tasks` | Создать задачу |
| GET | `/tasks` | Получить список задач |
| GET | `/tasks/{id}` | Получить задачу |
| PUT | `/tasks/{id}` | Обновить задачу |
| DELETE | `/tasks/{id}` | Удалить задачу |
| GET | `/readyz` | Проверить доступность PostgreSQL |

## Параметры списка

Пример: `/tasks?status=done&limit=10&offset=0`.

| Параметр | По умолчанию | Допустимые значения |
|---|---|---|
| `limit` | `20` | От 1 до 100 |
| `offset` | `0` | Целое число от 0 |
| `status` | Без фильтра | `todo`, `in_progress`, `done` |

Пустой `status` означает отсутствие фильтра.
Некорректные параметры возвращают `400 Bad Request`.

Задачи сортируются по ID по возрастанию.
Фильтрация применяется до пагинации.
Пустой результат возвращается как JSON-массив `[]`.

## Примеры запросов

Выполняйте команды в одном окне PowerShell:
переменная `$createdTask` хранит созданную задачу.

### Создание

```powershell
$createdTask = Invoke-RestMethod `
    -Method Post `
    -Uri "http://localhost:8080/tasks" `
    -ContentType "application/json" `
    -Body '{"title":"Learn Docker","description":"Run API in a container"}'

$createdTask
```

Успешный ответ: `201 Created`. Начальный статус задачи — `todo`.

### Получение по ID

```powershell
Invoke-RestMethod -Uri "http://localhost:8080/tasks/$($createdTask.id)"
```

### Обновление

```powershell
Invoke-RestMethod `
    -Method Put `
    -Uri "http://localhost:8080/tasks/$($createdTask.id)" `
    -ContentType "application/json" `
    -Body '{"title":"Learn Docker","description":"Run API in a container","status":"done"}'
```

### Список выполненных задач

```powershell
Invoke-RestMethod -Uri "http://localhost:8080/tasks?status=done&limit=10&offset=0"
```

### Удаление

```powershell
Invoke-RestMethod `
    -Method Delete `
    -Uri "http://localhost:8080/tasks/$($createdTask.id)"
```

Успешный ответ: `204 No Content`, без тела ответа.

## Ошибки

Ошибки, обработанные обработчиками задач, возвращаются в JSON.

```json
{"error":"invalid task id"}
```

- `400` — некорректные входные данные.
- `404` — задача не найдена.
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
