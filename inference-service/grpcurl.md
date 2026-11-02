# Ручные запросы через grpcurl

Запустите inference-service, установите `grpcurl` и выполняйте команды по отдельности.
В Compose gRPC доступен с хоста на `localhost:9091`. Для работы с deployments
сервису нужна настроенная PostgreSQL с применёнными миграциями.

## Адрес и проверка сервера

```sh
export GRPC_ADDRESS=localhost:9091
export GRPC_SERVICE=inference.v1.InferenceService

grpcurl -plaintext "$GRPC_ADDRESS" list
grpcurl -plaintext "$GRPC_ADDRESS" describe "$GRPC_SERVICE"
grpcurl -plaintext -d '{"service":"inference.v1.InferenceService"}' \
  "$GRPC_ADDRESS" grpc.health.v1.Health/Check
```

## StartDeployment

Замените проект, UUID модели и владельца, имя и репозиторий на свои значения.
Этот запрос создаёт контейнер, сохраняет запись deployment со статусом `PENDING`
и вызывает Docker Start. Менеджер меняет статус на `STARTED` и добавляет маршрут
`http://demo.qwen-small.localhost:8082/`. Загрузка весов может продолжаться
после запуска контейнера; проверяйте `http://demo.qwen-small.localhost:8082/health`.
UI и API доступны напрямую по `project.deploymentName.<host>`; каждый адрес
использует отдельное хранилище чатов. Проект и имя должны содержать 1–63 строчных
латинских букв, цифр или дефисов и начинаться и заканчиваться буквой или цифрой.
Маршруты `/proxy/...` и адреса с UUID удалены, редиректов нет.

```sh
grpcurl -plaintext -d '{
  "project": "demo",
  "modelId": "0197eb5a-37c6-7e48-8cd6-0c19edcd72f1",
  "userId": "0197eb5a-37c6-7e48-8cd6-0c19edcd72f2",
  "deploymentName": "qwen-small",
  "spec": {
    "providerType": "HUGGING_FACE",
    "providerHuggingFace": {
      "repo": "Qwen/Qwen2.5-0.5B-Instruct-GGUF:Q4_K_M"
    }
  }
}' "$GRPC_ADDRESS" "$GRPC_SERVICE/StartDeployment"
```

Скопируйте UUID из `state.deploymentId` ответа:

```sh
export DEPLOYMENT_ID='UUID_ИЗ_ОТВЕТА'
```

Повторное создание для той же модели в проекте вернёт `AlreadyExists`, если есть
deployment в `PENDING`, `STARTING`, `STARTED`, `RUNNING` или `STOPPING`. После
`STOPPED` или `FAILED` запрос создаст новый deployment; старый останется
в `ListAllDeployments`.

## GetDeploymentByID

```sh
grpcurl -plaintext -d "{\"deploymentId\":\"${DEPLOYMENT_ID}\"}" \
  "$GRPC_ADDRESS" "$GRPC_SERVICE/GetDeploymentByID"
```

В ответе `deployment.name` находится имя со случайным суффиксом. Подставьте его
в следующую переменную; UUID deployment и UUID модели — разные значения.

```sh
export DEPLOYMENT_NAME='qwen-small-abcde'
```

## GetDeploymentByName

```sh
grpcurl -plaintext -d "{\"name\":\"${DEPLOYMENT_NAME}\"}" \
  "$GRPC_ADDRESS" "$GRPC_SERVICE/GetDeploymentByName"
```

## ListDeployments

Возвращает запускающиеся и запущенные модели: `PENDING`, `STARTING`, `STARTED`,
`RUNNING`. Остановленные и завершившиеся ошибкой записи доступны через
`ListAllDeployments`. Фильтрация выполняется до пагинации.

Первая страница, по две записи:

```sh
grpcurl -plaintext -d '{"offset":0,"limit":2}' \
  "$GRPC_ADDRESS" "$GRPC_SERVICE/ListDeployments"
```

Скопируйте `nextPageToken` из ответа в переменную. Например, если вернулось `"2"`:

```sh
export NEXT_PAGE_TOKEN='2'

grpcurl -plaintext -d "{\"limit\":2,\"nextPageToken\":\"${NEXT_PAGE_TOKEN}\"}" \
  "$GRPC_ADDRESS" "$GRPC_SERVICE/ListDeployments"
```

Если токен пуст или отсутствует, следующей страницы нет. Не отправляйте пустой
токен для продолжения: без `offset` это снова запрос первой страницы.
Токен содержит смещение записей и при наличии заменяет `offset`.
Порядок записей — по возрастанию ID; `limit` по умолчанию `20`, максимум `100`.

Запрос со смещением без токена:

```sh
grpcurl -plaintext -d '{"offset":0,"limit":2}' \
  "$GRPC_ADDRESS" "$GRPC_SERVICE/ListDeployments"
```

## ListAllDeployments

Все deployments независимо от статуса, включая `STOPPED` и `FAILED`:

```sh
grpcurl -plaintext -emit-defaults -d '{"offset":0,"limit":2}' \
  "$GRPC_ADDRESS" "$GRPC_SERVICE/ListAllDeployments"
```

Следующая страница использует токен из ответа этого же метода:

```sh
grpcurl -plaintext -emit-defaults -d "{\"limit\":2,\"nextPageToken\":\"${NEXT_PAGE_TOKEN}\"}" \
  "$GRPC_ADDRESS" "$GRPC_SERVICE/ListAllDeployments"
```

Перед выполнением подставьте новый `NEXT_PAGE_TOKEN` из ответа `ListAllDeployments`.
`-emit-defaults` показывает пустой список как `"deployments": []`.

## StopDeployment

Останавливает контейнер выбранного deployment, сохраняя запись в БД:

```sh
grpcurl -plaintext -d "{\"deploymentId\":\"${DEPLOYMENT_ID}\"}" \
  "$GRPC_ADDRESS" "$GRPC_SERVICE/StopDeployment"
```

Успешный ответ — `{}`. Менеджер обновляет статус и убирает маршрут прокси
по событиям Docker и при периодической сверке. Для проверки повторите GetDeploymentByID.

## Проверка ошибок

Некорректный UUID и параметры страницы возвращают `InvalidArgument`:

```sh
grpcurl -plaintext -d '{"deploymentId":"invalid"}' \
  "$GRPC_ADDRESS" "$GRPC_SERVICE/GetDeploymentByID"
grpcurl -plaintext -d '{"offset":-1,"limit":2}' \
  "$GRPC_ADDRESS" "$GRPC_SERVICE/ListDeployments"
grpcurl -plaintext -d '{"limit":101}' \
  "$GRPC_ADDRESS" "$GRPC_SERVICE/ListDeployments"
grpcurl -plaintext -d '{"nextPageToken":"invalid"}' \
  "$GRPC_ADDRESS" "$GRPC_SERVICE/ListDeployments"
```
