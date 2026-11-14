# SQL-запросы и генерация Go-кода

`sqlc` 1.31.1 генерирует обёртки для `pgx/v5` по конфигурации
[`../sqlc.yaml`](../sqlc.yaml). Генерация работает через Docker без подключения к БД:

```sh
make sql-check
make sql-generate
```

Команды выполняются из корня репозитория.

## Роли model-service

В `infra/.env` задайте разные случайные шестнадцатеричные пароли
`MODEL_SERVICE_RW_PASSWORD` и `MODEL_SERVICE_RO_PASSWORD` (каждый можно
получить командой `openssl rand -hex 32`). При первом создании хранилища
PostgreSQL выполняет `configure_model_roles.sql` под `POSTGRES_USER` и создаёт
роли `model_service_rw` и `model_service_ro`. Миграция 000009 выдаёт им права
на таблицу `models`. При изменении схемы права обновляются миграцией.

Скрипты в `docker-entrypoint-initdb.d` выполняются только для пустого тома.
Для существующей базы роли должны быть созданы до применения миграции 000009.
Изменение паролей в `infra/.env` само по себе не меняет пароли уже созданных
ролей в PostgreSQL.

`make up-database` запускает primary, применяет миграции и запускает standby.
При первом запуске пустого тома standby скрипт `start_replica.sh` выполняет
`pg_basebackup` от пользователя `postgres` через `gosu`. При следующих запусках
используется сохранённый том. Непустой каталог без полноценной базы и база без
`standby.signal` не перезаписываются автоматически. После запуска проверьте
состояние командой `make replication-check`.

Основной model-service и его зеркало читают и пишут в primary `postgres`.
Два RO-экземпляра model-service читают standby `postgres-2` с той же учётной
записью, что и основной экземпляр. Запись на standby запрещает сам PostgreSQL.
Репликация асинхронная, поэтому чтение через основной API может отставать от
записи. Для чтения из primary после записи используйте `/mirror/api/v1/...`.

Для `inference-service` используются:

- `migrations/` — схема PostgreSQL для sqlc и создания таблиц БД.
- `queries/inference-service/connection.sql` — запрос `CheckConnection`.
- `queries/inference-service/deployments.sql` — создание, поиск deployment и обновление статусов.
- `../../inference-service/internal/infrastructure/dao/` — сгенерированный Go-пакет.
- `../../inference-service/internal/repository/deployment.go` — `DeploymentRepository`, вызывающий сгенерированные запросы.

Запрос `SELECT 1` проверяет соединение и не требует таблиц. SQL-запросы из
`queries/` не применяются как миграции; миграции выполняет отдельный runner.

## Подключение

В `inference-service` переменная `DATABASE_URL` включает подключение к PostgreSQL.
Если она пуста, сервер запускается без БД, а создание deployment возвращает
`Unavailable` до обращения к Docker. Пример для локального запуска:

```sh
cd inference-service
DATABASE_URL='postgres://USER:PASSWORD@localhost:5432/DATABASE?sslmode=disable' go run ./cmd
```

Подставьте свои реквизиты. Для Docker Compose задайте `DATABASE_URL` в
`inference-service/.env`, используя имя хоста `postgres`. Файл `.env`
не загружается автоматически при запуске через `go run`.

Дополнительные настройки:

| Переменная | По умолчанию | Назначение |
| --- | --- | --- |
| `DB_MAX_CONNS` | `10` | Максимальное число соединений |
| `DB_MIN_CONNS` | `0` | Минимальное число соединений |
| `DB_CONNECT_TIMEOUT` | `5s` | Время ожидания проверки подключения при запуске |

В `main` создаётся пул и проверяется соединение через репозиторий:

```go
database := repository.NewDeploymentRepository(pool)
if err := database.Check(ctx); err != nil {
    return err
}
```

В конструктор можно передать `*pgxpool.Pool`, `*pgx.Conn` или `pgx.Tx`.
Пул закрывается после остановки gRPC-сервера.

Создание пула вынесено в общий Go-модуль `github.com/jur4ikoff/utils`,
пакет `utils/postgres`. В `main` используется `postgres.NewPool(...)`.
Для локальной сборки `inference-service/go.mod` содержит замену на `../utils`;
Dockerfile также копирует этот модуль в контекст сборки.

Репозиторий передаётся в `service.NewDeploymentService(repo, cfg.Service, runtimeClient)`
через интерфейс `service.DeploymentRepository`. В методах сервиса он доступен через `s.repo`.

## Сохранение deployment

Перед первым созданием deployment примените миграции через `make up-database`:
`000011_create_deployments_table.sql` создаёт таблицу с частичными уникальными
индексами `(project, model_id)` и `(project, deployment_name)` для активных статусов.
Исходная схема `models` использует `project TEXT` с именем проекта;
`000011` сразу создаёт `deployments.deployment_name` для публичного URL.

Перед обращением к Docker сервис проверяет `ActiveDeploymentExists` по проекту и UUID
модели. Если есть активная запись (`PENDING`, `STARTING`, `STARTED`, `RUNNING`
или `STOPPING`), создание возвращает `AlreadyExists`. `STOPPED` и `FAILED`
не блокируют новый deployment: создаются новые ID и контейнер, история сохраняется.
Частичные уникальные индексы в БД защищают
от одновременных запросов: если другой запрос успел вставить запись после проверки,
созданный вторым запросом контейнер удаляется, а клиент получает `AlreadyExists`.

После успешного `CreateContainer` сервис сохраняет запись в `deployments`:

- `id` — отдельный UUID deployment;
- `container_id` — ID созданного Docker-контейнера;
- `model_id`, `project`, `username`, `name`, `deployment_name`, `status` — данные deployment;
- `spec` — JSONB с `provider_type` и `provider_hugging_face`.

Начальный статус — `PENDING`: после сохранения записи сервис вызывает Docker Start,
а менеджер обновляет статус по фактическому состоянию контейнера.
Поле `username` содержит имя пользователя, переданное при создании deployment. Имя deployment совпадает с именем контейнера.
`project` — имя проекта, `deployment_name` — публичное имя deployment; `name` содержит
имя контейнера со случайным суффиксом. Публичный адрес — `{project}.{deployment_name}.<host>`.
`CreateDeployment` использует `INSERT ... RETURNING` (`:one` в sqlc) и возвращает
`(domain.Deployment, error)`. Репозиторий собирает результат из строки PostgreSQL,
включая десериализацию JSONB; сервис возвращает именно этот результат.
Если запись завершилась ошибкой,
сервис пытается удалить созданный контейнер с отдельным таймаутом 10 секунд
и возвращает ошибку сохранения; ошибка удаления также сохраняется в цепочке ошибок.

`DeploymentManager` читает `ListDeploymentStates` и `GetDeploymentState`, а затем
выполняет `UpdateDeploymentStatus`. Обновление проверяет UUID deployment,
`container_id` и предыдущий статус; при несовпадении возвращается `false`.
Это защищает от перезаписи состояния устаревшими наблюдениями.
Статусы Docker синхронизируются с `STARTED`, `STOPPING` и `STOPPED`.
`DeploymentManager` регистрирует и удаляет маршруты через `ProxyManager`.
Оба компонента и их тесты находятся в `inference-service/internal/manager/`.
Колонка `status` имеет тип `TEXT NOT NULL` без ограничения на перечень значений.
Допустимые статусы проверяются в Go через `domain.Status.Validate()` и
`domain.ParseStatus()` при создании, обновлении и чтении deployment.
Новые значения статусов не ограничены схемой БД. Если новый статус должен
блокировать повторный запуск, его нужно добавить в проверку Go и предикаты
частичных уникальных индексов.

Для остановки `DeploymentService.StopDeployment(ctx, id)` получает запись через
`GetDeploymentByID` по UUID deployment и передаёт её `container_id` в Docker Stop.
Репозиторий возвращает `ErrDeploymentNotFound`, если такой записи нет.
`GetDeploymentByName` читает ту же полную запись по уникальному полю `name`
(имя контейнера со случайным суффиксом). Оба запроса декодируют JSONB в
`domain.DeploymentSpec` и проверяют статус через `domain.ParseStatus()`.

`ListDeployments` читает полные записи со статусами `PENDING`, `STARTING`, `STARTED`
и `RUNNING`. Набор статусов передаётся из Go; SQL применяет фильтр до `ORDER BY id`,
`LIMIT` и `OFFSET`. `ListAllDeployments` использует тот же порядок и пагинацию
без фильтра статусов. Сверка менеджера по-прежнему использует все записи
через `ListDeploymentStates` без пагинации.
Сервис запрашивает `limit + 1` записей: дополнительная строка определяет наличие
следующей страницы. Клиент получает не более `limit` записей и строковый токен
со смещением `offset + limit`, если есть продолжение. `COUNT(*)` не выполняется.

## Добавление запросов

1. Добавьте изменения схемы в миграции в `migrations/`.
2. Добавьте SQL-файл в `queries/inference-service/`. Перед запросом укажите
   аннотацию `-- name: ИмяМетода :one`, `:many` или `:exec`.
3. Выполните `make sql-check` и `make sql-generate`.
4. Вызовите сгенерированный метод через `r.queries` в своём репозитории.
   Преобразование результатов в доменные типы и обработку ошибок разместите там же.
5. Передайте репозиторий в сервис через интерфейс для нужной сущности.

Сгенерированные файлы хранятся в Git. Для сборки приложения `sqlc` не требуется.
При добавлении запросов редактируйте SQL и запускайте генератор.

## Проверка с PostgreSQL

```sh
cd inference-service
INFERENCE_TEST_DATABASE_URL='postgres://USER:PASSWORD@localhost:5432/DATABASE?sslmode=disable' \
  go test ./internal/repository -v
```

Тесты проверяют соединение, чтение имени базы и пользователя, отмену контекста
и сохранение deployment с JSONB. Для проверки записи миграция выполняется
во временной таблице внутри транзакции с последующим откатом; таблицы и данные
приложения не меняются.

## Миграции UserService

Миграции `000001`–`000004` создают пользователей, проекты, роли и участников.
Runner не повторяет миграции, уже записанные в `schema_migrations`.

Интеграционный тест первого спринта запускается на отдельной тестовой БД:

```sh
USER_TEST_DATABASE_URL='postgres://USER:PASSWORD@localhost:5432/TEST_DATABASE?sslmode=disable' \
  go -C user-service test ./internal/repository -run TestUserRepositoryIntegration -v
```

Тест создаёт случайную схему и удаляет её после завершения. Без переменной
интеграционный сценарий пропускается.

Модели сохраняют `username` как текст без внешнего ключа на пользователей.
Миграция `000012` переносит прежний `user_id` в текст без потери значения;
для старых записей это строковое представление UUID, поскольку имя не сохранялось.
