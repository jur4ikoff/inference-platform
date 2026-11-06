# Inference Service

Go-сервис для создания deployment моделей в Docker, наблюдения за контейнерами
и HTTP-проксирования запросов к запущенным моделям.

## Компоненты

- `internal/service/deployment.go` — создание контейнера и записи deployment через sqlc.
- `internal/manager/deployment.go` — события Docker, сверка статусов и управление маршрутами.
- `internal/manager/proxy.go` — `ProxyManager`: реестр маршрутов и HTTP reverse proxy.
- `internal/runtime/runtime.go` — Docker API, параметры контейнеров и определение endpoint.
- `internal/repository/deployment.go` — запросы к PostgreSQL и преобразование доменных типов.
- `cmd/main.go` — создание зависимостей, запуск HTTP/gRPC и завершение приложения.

`RuntimeService` с одним Docker-клиентом создаётся в `main` и передаётся сервису
и менеджеру через интерфейсы. При `SIGINT`/`SIGTERM` HTTP и gRPC получают до
15 секунд на завершение запросов. Затем приложение завершает менеджер и закрывает
Docker-клиент и пул PostgreSQL. Контейнеры моделей продолжают работать; при
следующем запуске менеджер сверяет их состояние и восстанавливает маршруты.
Пока сервис остановлен, HTTP proxy недоступен.

## Проект и модель

`project` — строковое имя проекта во всём контракте model-service и inference-service.
В gRPC, HTTP DTO и таблице `models` используется `project`, без UUID проекта.
`model_id` остаётся UUID модели, `deployment_name` — публичное имя deployment.

Deployment хранит `id`, `container_id`, `model_id`, `project`, `username`, `name`,
`deployment_name`, `status` и `spec` (JSONB). `name` — имя Docker-контейнера со случайным
суффиксом, например `qwen-small-abcde`; публичный URL использует `project` и
`deployment_name`: `http://localhost:8000/inference/demo/qwen-small/`.
`username` содержит имя пользователя, указанное при создании модели.

Перед созданием сервис проверяет активные deployments по `(project, model_id)`.
`PENDING`, `STARTING`, `STARTED`, `RUNNING` и `STOPPING` блокируют повторный запуск.
После `STOPPED` или `FAILED` можно создать новый deployment с новым ID и контейнером;
старая запись сохраняется в истории. Частичные уникальные индексы защищают
от одновременного создания активных deployments и конфликтов публичных маршрутов.
Индекс `deployments_project_deployment_name_key` проверяет уникальность
`(project, deployment_name)` при INSERT и переходе в активный статус.
После успешного INSERT возвращается строка PostgreSQL через `RETURNING`.
При ошибке сохранения сервис пытается удалить новый контейнер с таймаутом 10 секунд.

## HTTP-прокси

Один HTTP-сервер слушает `HTTP_ADDR` (по умолчанию `:8080`). В Docker Compose
он доступен с хоста через NGINX на `http://localhost:8000/inference/`.

`PROXY_PUBLIC_URL` задаёт внешний адрес proxy, по умолчанию `http://localhost:8082`. В стандартном Compose HTTP-порт inference-service не опубликован; NGINX пересылает запросы внутри Docker-сети.
Интерфейс и API доступны по пути `/<project>/<deployment_name>/`:

```text
http://localhost:8000/inference/ml/qwen-small/
  -> http://qwen-small-abcde:8080/
```

Прокси выбирает контейнер по проекту и имени deployment, удаляет этот префикс
перед отправкой запроса и передаёт его в `X-Forwarded-Prefix`. Например:

```sh
curl http://localhost:8000/inference/ml/qwen-small/health
curl http://localhost:8000/inference/ml/qwen-small/v1/chat/completions \
  -H 'Content-Type: application/json' \
  -d '{"model":"Qwen/model","messages":[{"role":"user","content":"Hello"}],"stream":true}'
```

Прокси добавляет в HTML интерфейса bootstrap, который изолирует базы IndexedDB
и ключи localStorage по паре `(project, deployment_name)` и направляет
корневые API-запросы UI в нужный префикс. Новые пространства имён не переносят
историю со старых поддоменов. Это разделение данных UI, а не граница безопасности:
все страницы имеют один origin и могут обращаться к данным друг друга. Для
разграничения пользователей необходимы авторизация и изоляция на сервере.

Маршрут появляется, когда контейнер работает и deployment имеет статус
`STARTED` или `RUNNING`, и удаляется при остановке или паузе контейнера. После
перезапуска сервиса маршруты восстанавливаются при сверке с Docker. Неизвестные
и неактивные маршруты возвращают 404; ошибка соединения с backend — 502.

Проект и имя deployment должны содержать от 1 до 63 строчных латинских букв,
цифр или `-` и начинаться и заканчиваться буквой или цифрой. Точки, `_` и
заглавные буквы отклоняются с `InvalidArgument` до создания контейнера.
Неизвестные и неактивные маршруты
возвращают 404. Ошибка соединения с backend возвращает 502. `STARTED` фиксирует
запуск контейнера; модель в этот момент ещё может загружать веса.

В общей Docker-сети модели доступны по именам контейнеров на порту `8080`.
При заданном `DOCKER_NETWORK` порты моделей на хосте не публикуются.
При запуске Go-процесса на хосте используется фактический опубликованный порт
из Docker Inspect; `RUNTIME_PORT_HOST` задаёт его при создании контейнера.

## Наблюдение за статусами

`DeploymentManager` запускается из `main` при подключённой БД. Он слушает события
контейнеров проекта `inference-models` и обновляет PostgreSQL:

- контейнер работает — `STARTED`;
- получен стоп-сигнал — `STOPPING`;
- контейнер завершился или удалён — `STOPPED`.

До открытия HTTP/gRPC-портов `main` синхронно вызывает `DeploymentManager.Init`.
Менеджер читает все deployments из БД без пагинации, проверяет каждый контейнер
через Docker Inspect, обновляет статусы и восстанавливает маршруты работающих
моделей. При ошибке стартовой сверки API не запускается. В логе
`initial deployment reconciliation completed` указано число проверенных записей.
После этого менеджер слушает события и продолжает периодическую сверку.
Если контейнер уже удалён, запись deployment сохраняется со статусом `STOPPED`
и остаётся в `ListAllDeployments`. Стартовая сверка не запускает остановленные модели.

Созданный, но ещё не запущенный контейнер сохраняет `PENDING`.
`StartDeployment` сохраняет deployment и вызывает Docker Start; менеджер
наблюдает фактические события запуска и остановки контейнеров. Если Docker Start
возвращает ошибку, сервис пытается перевести начальную запись в `FAILED`,
не перезаписывая более свежий статус менеджера.

После обрыва потока менеджер переподключается с последней обработанной позиции.
При подключении и каждые 5 секунд он сверяет контейнеры с БД. Ошибка Docker
не считается остановкой контейнера. `STOPPING` определяется по событиям сигналов,
поскольку Docker Inspect не предоставляет отдельного состояния остановки в процессе.

Обновление статуса проверяет предыдущее значение и `container_id`.
В БД статус хранится как `TEXT NOT NULL`; допустимые значения проверяются в Go
через `Status.Validate()` и `ParseStatus()`.

## gRPC

Контракт и сгенерированные типы — в [internal-api](../internal-api/README.md).
Готовые команды для ручных запросов — в [grpcurl.md](grpcurl.md).
`StartDeployment` принимает `project`, `model_id`, `username`, `deployment_name` и `spec`.
В protobuf JSON поле называется `deploymentName`; адрес имеет вид
`http://localhost:8000/inference/{project}/{deployment_name}/` при настройках по умолчанию.
Реализация создаёт контейнер, сохраняет запись deployment со статусом `PENDING`
и запускает контейнер. `STARTED` и маршрут прокси выставляет `DeploymentManager`
по фактическому состоянию Docker. RPC возвращает первоначально сохранённую запись,
поэтому ответ может ещё содержать `PENDING`; актуальный статус доступен через Get.
В ответе `state.deployment_id` возвращается UUID созданного deployment.

`GetDeploymentByID` принимает `deployment_id`, а `GetDeploymentByName` — `name`
deployment, например `qwen-small-abcde`. Оба возвращают полную запись PostgreSQL
в поле `deployment`, включая `container_id`, `status` и типизированный `spec`.
Пустое имя или некорректный UUID дают `InvalidArgument`, отсутствие записи —
`NotFound`. Статус читается из БД, которую обновляет `DeploymentManager`.

`StopDeployment` принимает `deployment_id`, находит запись в PostgreSQL и
останавливает её контейнер через Docker API. Запрос ждёт завершения Docker Stop;
Docker использует таймаут остановки контейнера или свой стандартный таймаут.
Повторная остановка уже остановленного или удалённого контейнера успешна.
Отсутствующая запись возвращает `NotFound`, некорректный UUID — `InvalidArgument`.
Статусы и маршруты обновляет `DeploymentManager` по событиям Docker и при сверке;
обновление БД может произойти после завершения RPC.

```sh
grpcurl -plaintext -d '{"deploymentId":"DEPLOYMENT_UUID"}' \
  localhost:9091 inference.v1.InferenceService/StopDeployment
```

`ListDeployments` возвращает запускающиеся и запущенные deployments: `PENDING`,
`STARTING`, `STARTED` и `RUNNING`. `ListAllDeployments` возвращает записи во всех
статусах, включая `STOPPING`, `STOPPED`, `FAILED` и `UNKNOWN`.
`ListDeployments` читает записи в порядке возрастания `id`. `ListAllDeployments` возвращает сначала новые запуски по времени создания; у записей до миграции время неизвестно и их взаимный порядок не отражает порядок запуска.
Принимают `offset` (по умолчанию `0`), `limit` (по умолчанию `20`, максимум `100`)
и строковый `next_page_token`. Токен содержит смещение следующей страницы и
при наличии заменяет `offset`. Ответ содержит `deployments` и `next_page_token`;
пустой токен означает конец списка. Для продолжения передайте токен из ответа
в следующий запрос с тем же лимитом. Неверные параметры дают `InvalidArgument`.
Фильтр статусов применяется до `LIMIT`/`OFFSET`. Токен продолжения нужно
использовать в том же методе, который его вернул.

```sh
grpcurl -plaintext -d '{"offset":0,"limit":20}' \
  localhost:9091 inference.v1.InferenceService/ListDeployments
grpcurl -plaintext -d '{"limit":20,"nextPageToken":"20"}' \
  localhost:9091 inference.v1.InferenceService/ListDeployments
grpcurl -plaintext -emit-defaults -d '{"offset":0,"limit":20}' \
  localhost:9091 inference.v1.InferenceService/ListAllDeployments
```

Сервер также предоставляет `grpc.health.v1.Health` и reflection.

Спецификация модели:

```json
{
  "provider_type": "HUGGING_FACE",
  "provider_hugging_face": {"repo": "Qwen/Qwen2.5-0.5B-Instruct-GGUF:Q4_K_M"}
}
```

Образ задаётся через `INFERENCE_SERVER_IMAGE`. При запуске `Init` использует
локальный образ, если он уже есть в Docker. При отсутствии скачивает его до
начала обслуживания запросов. Ошибка скачивания или обращения к Docker
останавливает запуск сервиса.

Для обновления образа с тем же тегом скачайте его вручную и перезапустите сервис:

```sh
docker pull ghcr.io/ggml-org/llama.cpp:server
```

## Запуск и конфигурация

Нужны Docker, Go 1.26.5 и PostgreSQL с применёнными миграциями.
Из корня репозитория:

```sh
make up-inference
```

`make up-inference`, `make restart-inference` и `make up-services` сначала
ожидают готовности PostgreSQL и применяют миграции. При ошибке этого этапа
сервисы не запускаются. При прямом `docker compose up inference-service`
Compose ожидает `postgres: service_healthy`; миграции отдельно выполняет Makefile.

В исходных миграциях `models.project` имеет тип `TEXT`, а `deployments`
сразу содержит поле `deployment_name` для публичного URL.
Описание sqlc и миграций — в [infra/postgres](../infra/postgres/README.md).
Миграции автоматически при запуске Go-процесса не применяются.

Основные переменные перечислены в `.env.example`:

```env
GRPC_ADDR=:9091
HTTP_ADDR=:8080
PROXY_PUBLIC_URL=http://localhost:8082
INFERENCE_SERVER_IMAGE=ghcr.io/ggml-org/llama.cpp:server
DOCKER_NETWORK=inference-platform_default
RUNTIME_PORT_HOST=18081
DEPLOYMENT_MANAGER_SYNC_INTERVAL=5s
DEPLOYMENT_MANAGER_OPERATION_TIMEOUT=5s
DEPLOYMENT_MANAGER_RECONNECT_INTERVAL=1s
```

`DATABASE_URL` включает подключение к БД. Без него HTTP/gRPC стартуют,
но создание deployment возвращает `Unavailable`, менеджер не запускается,
реестр HTTP-маршрутов остаётся пустым. При `go run` файл `.env` нужно загрузить
в окружение самостоятельно. Для локального запуска рядом с Compose-сервисами
выберите свободный `HTTP_ADDR`, например `:8082`.
Если меняется внешний порт прокси, обновите и `PROXY_PUBLIC_URL`.

## Проверки

```sh
go test ./...
go test -race ./internal/manager ./internal/runtime
```

Все проверки проекта: `make lint`. Для интеграционных тестов репозитория задайте
`INFERENCE_TEST_DATABASE_URL`. Они используют изолированные таблицы и схемы
с откатом транзакции и не меняют данные приложения.

В `ListDeployments` и `ListAllDeployments` поля `started_at` и `updated_at` содержат время создания записи запуска и последнего изменения её статуса соответственно. Для записей, созданных до соответствующих миграций, неизвестные значения отсутствуют.
