# Лабораторные работы по WEB 4 курс ИУ7
# Inference Platform
## Цель работы
Сервис для локального инференса ML-моделей, как площадка для НИР и ВКР
B2B AI SAAS

### Какую проблему решает сервис;
Дает возможность локально развернуть модели

## Функциональные требования

- Модель - абстракция над моделью, загруженной в систему
- Проект - единица абстракции в которой запускаются модели
- В одном проекте можно запустить несколько моделей

### Happy path
Пользователь создает модель, после этого имеет возможность ее запустить, остановить, обновить и удалить.

## Нефункциональные требования
- Под каждую модель должен запускаться Docker-контейнер
- Доступ к персональным данным и административным функциям должен выполняться только после аутентификации и проверки прав пользователя; пароли должны храниться исключительно в зашифрованном виде, а передача данных по HTTPS.

## Диаграммы
![alt text](</docs/img/Use-Case.png>)

![alt text](</docs/img/BPMN.png>)

![ER-диаграмма](<docs/img/ER.png>)

## Технологический стек
### backend
- Golang 1.26+
- http для внешнего общения
- grpc для общения между микросервисами

### infra
- Приложение упаковывается в Docker контейнеры
- Для локального развертывания используется Docker-Compose

### Запуск сервисов

Команды выполняются из корня репозитория:

```sh
make up                 # Инфраструктура, PostgreSQL с миграциями, user/model/inference-service
make up-database        # PostgreSQL и миграции
make up-services        # PostgreSQL с миграциями и три прикладных сервиса
make up-infra           # Grafana, Loki, Tempo, OTel Collector и Alloy
make up-inference       # Inference-service и его зависимости
make restart-inference  # Пересборка и перезапуск inference-service
make logs-inference     # Логи inference-service
```

`make up-services` сначала поднимает PostgreSQL, ждёт готовности и применяет миграции,
затем запускает приложения, NGINX и Adminer с `--no-deps`. При ошибке БД или миграций запуск
приложений не выполняется. Мониторинг запускается
отдельно командой `make up-infra`. Для полного запуска используйте `make up`.

Контейнеры настроены на автоматический перезапуск при сбое и запуске Docker.
На macOS включите в Docker Desktop настройку **Settings → General → Start Docker Desktop when you sign in to your computer**. Сначала запустите стек командой `make up`: после этого Docker Desktop восстановит контейнеры при следующем запуске. Команда `make down` удаляет контейнеры, поэтому после неё для повторного запуска нужен `make up`.

Inference-service входит в общий Compose-проект: `make ps`, `make logs`,
`make down` и `make restart` также управляют им.
Inference-service предоставляет gRPC API в Docker-сети
по адресу `inference-service:9091`, с хоста — `localhost:9091`. Проверка состояния — через `grpc.health.v1.Health`.

### Линтеры и форматирование

Проверки настроены в `prek.toml`. Нужны Go 1.26.5 и
[prek](https://prek.j178.dev/installation/) версии 0.4.13 или новее.

```sh
make lint  # Проверить все отслеживаемые файлы
```

`prek` исправляет форматирование Go, пробелы в конце строк и переводы строк
в конце файлов, проверяет синтаксис YAML/TOML и маркеры конфликтов слияния.
`go vet` запускается отдельно для `user-service`, `model-service`,
`inference-service`, `internal-api` и общего модуля `utils`. Go-обёртки, сгенерированные protobuf
и sqlc, исключены из `gofmt`, но участвуют в анализе `go vet`.

Проверки запускаются вручную; Git-хуки не используются. Если ранее был
установлен хук prek, отключите его командой `prek uninstall --hook-type pre-commit`.
Если форматтер изменил файлы, проверьте изменения и повторите `make lint`.
Новые файлы нужно сначала добавить через `git add`, чтобы они попали в проверку.
Для отдельной проверки:

```sh
prek run --stage manual go-vet-inference-service --all-files
```

### HTTP API v1

Все HTTP-маршруты сервисов начинаются с `/api/v1`. Версия пути задаёт версию
контракта API и не зависит от версии сборки сервиса.

| Сервис | Маршрут через NGINX | OpenAPI JSON |
| --- | --- | --- |
| user-service | `/api/v1/auth`, `/api/v1/users`, `/api/v1/projects`, `/api/v1/roles` | `/api/v1/openapi-user.json` |
| model-service | `/api/v1/projects/{project}/models`, `/api/v1/projects/{project}/deployments` | `/api/v1/openapi-model.json` |

NGINX доступен на `http://localhost:8000` (порт задаётся `NGINX_HTTP_PORT` в `infra/.env`).
Он направляет `/api/v1/projects/{project}/models...` и `/api/v1/projects/{project}/deployments...` в model-service, а остальные `/api/v1/...` — в user-service. Авторизацию выполняют сами сервисы. Общий Swagger UI с переключением двух схем доступен на `/api/v1/docs`; схема user-service — `/api/v1/openapi-user.json`, model-service — `/api/v1/openapi-model.json`. Модельные API и UI доступны без JWT по `/inference/{project}/{deployment}/`; потоковые ответы не буферизуются.

Служебные страницы: `/management` содержит ссылки, `/status` показывает страницу со статистикой NGINX (исходные данные stub_status — `/status/data`), `/health` проверяет процесс NGINX, `/ready` возвращает JSON с готовностью user-service и model-service и HTTP-доступностью inference-service: первые два сервиса проверяют базу, а model-service также проверяет gRPC inference-service. Эти данные выводятся на странице `/status`. `/documentation` отображает этот README, его исходный Markdown доступен на `/documentation/raw`, `/` и `/reserved/` — приветственную страницу. На `/legacy` доступны бинарные файлы Linux-утилиты bootstrap-admin для архитектур amd64 и arm64; они собираются вместе с образом NGINX. Это утилита первоначального создания администратора, работающая напрямую с PostgreSQL, а не клиент API.

Администрирование PostgreSQL через Adminer: `http://127.0.0.1:8083/admin/` на компьютере с сервером. Публичный `/admin` возвращает 403; Adminer не имеет собственного опубликованного порта. В форме выберите PostgreSQL и сервер `postgres`; имя БД, пользователь и пароль заданы в `infra/.env` (`POSTGRES_DB`, `POSTGRES_USER`, `POSTGRES_PASSWORD`). Доступ к 8083 ограничен loopback на уровне Docker; для удалённого администрирования используйте SSH-туннель, например `ssh -L 8083:127.0.0.1:8083 user@server`. PostgreSQL также публикуется только на `127.0.0.1:5432`.

Локальный запуск: `make up` из корня репозитория. HTTP-порты сервисов не публикуются на хосте; доступ осуществляется через NGINX. Swagger UI версии 5.34.1 и его ресурсы размещены локально в `nginx/static/swagger`.

Примеры: `POST /api/v1/auth/register`, `POST /api/v1/auth/login`,
`PATCH /api/v1/users/{id}`, `GET /api/v1/projects/{project}/models`.
Прежние HTTP-пути без `/api/v1` больше не обслуживаются.

Регистрация: `POST /api/v1/auth/register`.
Тело запроса: `{"login":"yuri","firstname":"Юрий","lastname":"Попов","email":"user@example.com","password":"password-12345"}`.
Все пять полей обязательны; имя и фамилия не могут быть пустыми или состоять из пробелов.
ID и даты создаются сервером. Роль относится к участию в проекте (`members.role_id`), а не к пользователю.
Ответ `201` содержит `id`, `email`, `firstname`, `lastname`; повторный email возвращает `409`.
Пароль должен содержать не менее 8 символов и не более 72 байт.

Списки используют `offset` (от 0, по умолчанию 0) и `limit` (1–100, по умолчанию 20).
`nextpagetoken` возвращается только в ответе и содержит следующий `offset`, а на последней
странице — `null`. Например, после `?offset=0&limit=20` ответ с `nextpagetoken: 20`
означает запрос следующей страницы с `?offset=20&limit=20`. Фильтры нужно передавать
в каждом запросе. Пустой список возвращается как `[]` внутри объекта ответа.
Некорректные `offset`/`limit` возвращают `422`. Состав страниц может смещаться при
добавлении или удалении записей между запросами.

- Пользователи: `GET /api/v1/users?offset=0&limit=20`, ответ `{"users":[...],"nextpagetoken":20}`.
- Проекты текущего пользователя: `GET /api/v1/projects?offset=0&limit=20`, ответ `{"projects":[...],"nextpagetoken":20}`; фильтр `name` — точное название.
- Роли: `GET /api/v1/roles?offset=0&limit=20`, ответ `{"roles":[...],"nextpagetoken":20}`; фильтр `name` — точное название.
- Модели проекта: `GET /api/v1/projects/{project}/models?offset=0&limit=20`, ответ `{"models":[...],"nextpagetoken":20}`.

Списки пользователей, проектов и ролей требуют `Authorization: Bearer <access_token>`;
без него возвращается `401`. Их порядок — `created_at`, затем `id` по возрастанию;
модели упорядочены по `created_at`, затем `id` по убыванию.

Получение, изменение и удаление проекта используют `GET`, `PATCH`, `DELETE /api/v1/projects/{id}`.
Прежнего маршрута `/projects/list` нет. Получение, создание, изменение и удаление проекта
пока возвращают `501`; список проектов работает через БД и ограничен членством пользователя.

Для существующей БД необходимо применить миграции `000007_align_users_with_user_service.sql` и `000008_remove_user_role.sql`
командой `make up-database`.

Обновление пользователя: `PATCH /api/v1/users/{id}` с bearer-токеном.
Поля: `email`, `password`, `firstname`, `lastname`; все необязательны,
но нужно передать хотя бы одно. Пользователь может менять только свои данные.
Пропущенные поля сохраняются; пустая строка очищает имя или фамилию.
Ответ содержит `id`, `email`, `firstname`, `lastname`, `created_at`, `updated_at`.
Пароль меняется через этот же PATCH и хранится как bcrypt-хеш.

Роли назначаются участникам проекта через `members.role_id`. Помимо `all_permissions`,
контракт ролей содержит отдельные разрешения:

- Проект: `can_read_project`, `can_update_project`, `can_delete_project`.
- Участники и их роли: `can_manage_members`.
- Модели: `can_create_model`, `can_read_model`, `can_update_model`, `can_delete_model`.
- Выполнение моделей: `can_start_model`, `can_stop_model`.

Поля доступны в схемах создания, PATCH и ответа роли. Новые разрешения необязательны
при создании и по умолчанию равны `false`. В PATCH пропущенное поле сохраняет значение,
явное `false` снимает разрешение. Для БД добавлена миграция `000009_add_role_permissions.sql`.
Миграция `000010_remove_model_interaction_permission.sql` удаляет устаревшее
разрешение на вызов модели.
Список ролей работает через БД. Создание, получение, изменение и удаление роли пока
возвращают `501`; проверки разрешений при выполнении операций ещё не реализованы.

Получение, изменение и удаление модели используют `GET`, `PATCH`, `DELETE /api/v1/projects/{project}/models/{model_id}`.
`model_id` — UUID модели, который возвращается при создании. Имя модели можно менять,
обращение к модели выполняется по ID.

Создание модели: `POST /api/v1/projects/{project}/models`.
Тело запроса: `{"name":"demo","description":"...","username":"alice","spec":{"image":"my-model:latest"}}`.
Проект задаётся только в пути; поле `project` в теле создания не принимается.

Inference-service использует gRPC-контракт `inference.v1.InferenceService`.
Путь RPC: `/inference.v1.InferenceService/StartDeployment` (аналогично для остальных методов).
Версия gRPC задаётся protobuf-пакетом `inference.v1`; префикс `/api/v1` относится к HTTP API.

Контракт развёртываний в model-service:

| Метод и путь | Назначение | Успешный ответ по контракту |
| --- | --- | --- |
| `POST /api/v1/projects/{project}/models/{model_id}/deployment` | Создать развёртывание и запустить модель с текущим spec; без тела запроса | `201`, состояние и `Location` |
| `GET /api/v1/projects/{project}/models/{model_id}/deployment` | Получить состояние развёртывания | `200` |
| `DELETE /api/v1/projects/{project}/models/{model_id}/deployment` | Остановить развёртывание и освободить ресурсы, сохранив запись модели | `204`, без тела |
| `GET /api/v1/projects/{project}/deployments?offset=0&limit=20` | Получить страницу развёртываний проекта | `200`, `deployments` и `nextpagetoken` |

Все четыре ручки пока являются заглушками и возвращают `501 Not Implemented`.
Прежние HTTP-ручки `/start` и `/stop` удалены. По контракту у модели одно развёртывание,
повторное создание возвращает `409`, повторный DELETE отсутствующего развёртывания
существующей модели — `204`. Состояния: `starting`, `running`, `stopping`, `failed`.
`nextpagetoken` — следующий числовой offset либо `null` на последней странице.
Внутренние gRPC `StartDeployment` и `StopDeployment` реализованы в inference-service.

Внутренний gRPC-адрес задаётся через `INFERENCE_GRPC_ADDR` (`localhost:9091`
по умолчанию; в Compose — `inference-service:9091`). Значение `REQUEST_TIMEOUT`
в model-service по умолчанию — `90s`. Переменная окружения переопределяет этот
таймаут. Для сборки образа вручную: `docker build -f model-service/Dockerfile .`.

### База данных
- Для хранения пользователей Postgres


## Диаграмма БД
![DB-Диаграмма](<docs/img/DB.png>)

## C4
![C4-l1](<docs/img/C4-l1.png>)

![C4-l2](<docs/img/C4-l2.png>)

![C4-l3](<docs/img/C4-l3.png>)

## Проектирование интерфейса
Сервис представляет собой MPA, где каждая страница посвещенна сущности

### Черновой набросок:
![interface-draft](<docs/interface/1.png>)

## ADR
1. UserService выдаёт JWT; каждый сервис локально проверяет access JWT и права. Nginx отвечает за TLS и маршрутизацию.
2. Изначально приложение проектируется под микросервисную архитектуру, но база данных единая для всех микросервисов для простоты разработки.
3. Выбран архитектурный паттерн Control Plane и Data Plane


## Запуск
```bash
make bootstrap-admin
make up
```

http://localhost:8000/inference/<проект>/<имя-deployment>/

`GET /healthz` (также `/healtz`) возвращает JSON со статусами сервисов и HTTP 503, если хотя бы один из основных сервисов недоступен. Для user-service, model-service, inference-service, NGINX и Adminer проверяется HTTP, для PostgreSQL и сервисов мониторинга — доступность TCP-порта. TCP-проверка не гарантирует готовность приложения; `/ready` отдельно проверяет готовность основных зависимостей.
