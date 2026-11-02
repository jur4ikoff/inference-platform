# Internal API

Отдельный Go-модуль с protobuf-контрактами межсервисного gRPC API и сгенерированными
Go-клиентами и серверными интерфейсами. Реализация `InferenceServiceServer`
находится в `inference-service/internal/api`.

## Inference v1

Контракт: `proto/inference/v1/inference.proto`.
Импорт Go: `github.com/jur4ikoff/internal-api/gen/go/inference/v1`.
Полное имя сервиса: `inference.v1.InferenceService`.
Путь RPC: `/inference.v1.InferenceService/<метод>`. Версия задаётся protobuf-пакетом;
HTTP-префикс `/api/v1` для gRPC не используется.

| RPC | Запрос | Ответ |
| --- | --- | --- |
| `StartDeployment` | `project`, `model_id`, `user_id`, `deployment_name`, `spec` | `state` |
| `StopDeployment` | `deployment_id` | пустой ответ после Docker Stop |
| `GetDeploymentByID` | `deployment_id` | `deployment`: полная запись из БД |
| `GetDeploymentByName` | `name` | `deployment`: полная запись из БД |
| `ListDeployments` | `offset`, `limit`, `next_page_token` | `deployments`, `next_page_token` |
| `ListAllDeployments` | `offset`, `limit`, `next_page_token` | `deployments`, `next_page_token` |

`state` содержит `deployment_id`, `model_id`, `project`, `deployment_name` и enum `ModelStatus`.
Enum покрывает все доменные статусы: `PENDING`, `STARTING`, `STARTED`, `RUNNING`,
`STOPPING`, `STOPPED`, `FAILED` и `UNKNOWN` (с префиксом `MODEL_STATUS_`).

Для `StartDeployment` обязательны все пять полей. `model_id` и `user_id` должны быть
ненулевыми UUID. `model-service` передаёт ID владельца, имя и спецификацию из
записи модели. В `deployment_name` клиент model-service по умолчанию передаёт
имя модели. При прямом вызове RPC публичное имя deployment задаёт вызывающий.
`inference-service` преобразует запрос в `StartDeploymentCommand`.
Поле `spec` имеет тип `DeploymentSpec`; настройки Hugging Face описаны
в отдельной структуре `HuggingFaceProvider`. В запросе это вложенный объект:

```json
{
  "provider_type": "HUGGING_FACE",
  "provider_hugging_face": {
    "repo": "Qwen/Qwen2.5-0.5B-Instruct-GGUF:Q4_K_M"
  }
}
```

`model-service` преобразует JSON из записи модели в `DeploymentSpec` перед gRPC-вызовом.
При обновлении контракта нужно пересобрать оба сервиса: запросы старого клиента
с JSON-байтами вместо структуры будут отклонены с `InvalidArgument`.

Для `StopDeployment` передайте ненулевой UUID из `state.deployment_id` ответа
`StartDeployment`. Сервис ищет deployment в БД и ждёт завершения Docker Stop.
Уже остановленный или удалённый контейнер считается успешно остановленным.
Запись и контейнер не удаляются. Статус и маршрут прокси обновляет
`DeploymentManager` по фактическому состоянию Docker; обновление БД может
завершиться после ответа RPC.

`GetDeploymentByID` ищет по ненулевому UUID deployment, `GetDeploymentByName` —
по уникальному `deployments.name`, например `qwen-small-abcde`. Это имя
Docker-контейнера со случайным суффиксом. Поле `deployment_name` содержит публичное
имя deployment и используется в hostname `{project}.{deployment_name}.<host>`.
Проект и публичное имя должны быть DNS-метками из 1–63 строчных латинских букв,
цифр или дефисов, с буквой или цифрой в начале и конце.
Оба RPC возвращают `deployment` с полями `deployment_id`, `container_id`,
`model_id`, `project`, `username`, `name`, `deployment_name`, `status` и `spec`.
Чтение выполняется из PostgreSQL, без обращения к Docker. Пустое имя и
некорректный UUID возвращают `InvalidArgument`, отсутствие записи — `NotFound`.

`ListDeployments` возвращает только `PENDING`, `STARTING`, `STARTED` и `RUNNING`.
`ListAllDeployments` возвращает записи во всех допустимых статусах, включая
остановленные и завершившиеся ошибкой. В обоих методах записи отсортированы
по `id` по возрастанию, а параметры пагинации одинаковы:

- `offset` — число пропускаемых записей; по умолчанию `0`.
- `limit` — размер страницы от `1` до `100`; отсутствующее значение или `0` означает `20`.
- `next_page_token` — строка со смещением, например `"20"`; при наличии заменяет `offset`.

Ответ содержит `deployments` и токен следующей страницы. Передайте возвращённый
`nextPageToken` в следующий запрос с тем же `limit`. Пустой токен (поле может
отсутствовать в protobuf JSON) означает, что следующей страницы нет.
Если смещение выходит за конец списка, возвращается пустая страница.
Некорректный токен, отрицательное эффективное смещение, неверный лимит или
переполнение 32-битного смещения возвращают `InvalidArgument`.
При добавлении или удалении записей между запросами состав страниц может смещаться.
Фильтр статусов применяется в SQL до пагинации. Токены двух методов используются
независимо: продолжайте страницу тем же RPC. Изменение статусов также может
изменить состав страниц `ListDeployments`.

| Ситуация | gRPC-код |
| --- | --- |
| Пустое имя проекта, некорректные UUID, пустое имя модели или некорректный spec | `InvalidArgument` |
| Deployment с указанным UUID отсутствует в БД | `NotFound` |
| Уже есть активный deployment этой модели или публичного имени в проекте | `AlreadyExists` |
| Подключение к БД не настроено | `Unavailable` |
| Истёк срок запроса | `DeadlineExceeded` |
| Запрос отменён | `Canceled` |
| Неожиданная внутренняя ошибка | `Internal` |

## Генерация

Из корня репозитория:

```sh
make -C internal-api check
make proto
make -C internal-api test
```

Нужен Docker. Makefile использует Buf 1.61.0, удалённые плагины
`protoc-gen-go` 1.36.11 и `protoc-gen-go-grpc` 1.5.1.
Версии и параметры зафиксированы в `buf.gen.yaml`.
При генерации требуется доступ к Buf Schema Registry.
Сгенерированные файлы лежат в `gen/go`; вручную их редактировать не нужно.

Другие Go-сервисы подключают этот модуль отдельно. Для локальной разработки
в общем репозитории inference-service использует:

```go
require github.com/jur4ikoff/internal-api v0.0.0

replace github.com/jur4ikoff/internal-api => ../internal-api
```

Клиент создаётся через `inferencev1.NewInferenceServiceClient(connection)`.

## Проверка работающего сервиса

При локальном `go run ./cmd` в inference-service gRPC слушает `localhost:9091`.
Reflection включён по умолчанию, поэтому grpcurl получает схему с сервера:

```sh
grpcurl -plaintext localhost:9091 list
grpcurl -plaintext -d '{"project":"PROJECT_NAME","modelId":"MODEL_ID","userId":"USER_ID","deploymentName":"qwen-small","spec":{"providerType":"HUGGING_FACE","providerHuggingFace":{"repo":"Qwen/Qwen2.5-0.5B-Instruct-GGUF:Q4_K_M"}}}' \
  localhost:9091 inference.v1.InferenceService/StartDeployment
grpcurl -plaintext -d '{"deploymentId":"DEPLOYMENT_ID"}' \
  localhost:9091 inference.v1.InferenceService/GetDeploymentByID
grpcurl -plaintext -d '{"name":"qwen-small-abcde"}' \
  localhost:9091 inference.v1.InferenceService/GetDeploymentByName
grpcurl -plaintext -d '{"offset":0,"limit":20}' \
  localhost:9091 inference.v1.InferenceService/ListDeployments
grpcurl -plaintext -d '{"limit":20,"nextPageToken":"20"}' \
  localhost:9091 inference.v1.InferenceService/ListDeployments
grpcurl -plaintext -emit-defaults -d '{"offset":0,"limit":20}' \
  localhost:9091 inference.v1.InferenceService/ListAllDeployments
grpcurl -plaintext -d '{"deploymentId":"DEPLOYMENT_ID"}' \
  localhost:9091 inference.v1.InferenceService/StopDeployment
grpcurl -plaintext -d '{"service":"inference.v1.InferenceService"}' \
  localhost:9091 grpc.health.v1.Health/Check
```

Для ручного вызова `StartDeployment` подставьте UUID модели и владельца,
а также проект из записи модели в model-service.
В Compose внутренний адрес сервера — `inference-service:9091`.
С хоста тот же сервер доступен напрямую на `localhost:9091`.

Порядок генерации описан в [документации Buf](https://buf.build/docs/generate/usage/),
работа со сгенерированным сервером — в [руководстве gRPC Go](https://grpc.io/docs/languages/go/basics/).
