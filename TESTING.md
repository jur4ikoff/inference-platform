# План тестирования

## 1. Определить границы тестирования

Цель: определить публичные методы основных компонентов бизнес-логики и доступа к данным платформы и не более трёх основных тестовых сценариев для каждого метода. Конструкторы, приватные методы, структуры данных, сгенерированный sqlc/protobuf, конфигурация и HTTP/gRPC-адаптеры не включены в обязательный перечень методов. Тестирование адаптеров рассматривается отдельно и не заменяет unit-тесты бизнес-логики или репозитория.

В таблицах U означает unit-тесты, I — интеграционные тесты. Для каждого метода нужно реализовать как минимум позитивный и негативный unit-сценарий. Если метод не возвращает ошибку, негативный сценарий проверяет пограничный или некорректный случай и отсутствие нежелательного эффекта.

### User service: бизнес-логика

Исходники: `user-service/internal/service/`.

| Компонент и публичные методы | Что реализовать |
| --- | --- |
| `UserService.Register` | U: успешная регистрация; некорректные данные; конфликт login/email. |
| `UserService.Login` | U: успешный вход; неизвестный login; неверный пароль. |
| `UserService.Verify`, `PublicJWKSet` | U: успешная проверка токена / получение JWKS; неверный токен; ошибка зависимости. |
| `UserService.UpdateUser` | U: успешное обновление; некорректные данные; запрет доступа. |
| `UserService.DeleteUser`, `GetUser`, `GetUserByLogin`, `ListUsers`, `AuthorizeUser` | U для каждого: успешная операция; запрет доступа; отсутствие пользователя или ошибка репозитория. |
| `BootstrapAdminService.Create` | U: создание администратора; повторное создание; ошибка сохранения. |
| `ProjectTokenService.Create` | U: выпуск токена; отсутствие principal; ошибка получения прав. |
| `ProjectService.ListProjects` | U: успешное получение страницы; некорректная пагинация; запрет доступа. |
| `ProjectService.CreateProject`, `GetProject`, `UpdateProject`, `DeleteProject`, `ListMembers`, `ChangeMember` | U для каждого: успешная операция; некорректные данные или отсутствующая сущность; запрет доступа. |
| `RoleService.CreateRole`, `GetRole`, `ListRoles`, `UpdateRole`, `DeleteRole` | U для каждого: успешная операция; некорректные данные или отсутствие роли; ошибка репозитория. |
| `PlatformRoleService.SetRole` | U: успешная смена роли; запрет доступа; некорректная роль. |

### User service: доступ к данным

Исходники: `user-service/internal/repository/`. Необходимо реализовать изолированные unit-тесты и отдельно интеграционные проверки с PostgreSQL.

| Репозиторий и публичные методы | Что реализовать |
| --- | --- |
| `UserRepository.Create` | U: успешная запись; конфликт данных; ошибка БД. |
| `CreateSaved`, `BootstrapAdmin` | U: успешное сохранение; повтор bootstrap или конфликт данных; ошибка транзакции. |
| `FindByEmail`, `FindByLogin`, `List` | U: получение записи или списка; запись отсутствует или список пуст; ошибка БД. |
| `Update`, `Delete` | U: успешная операция; отсутствие записи; ошибка БД. |
| `GetAuthState`, `UpdatePasswordHashIfCurrent` | U: успешное получение или обновление; отсутствие состояния или устаревший хеш; ошибка БД. |
| `ActorRole`, `SetPlatformRole`, `GetVisible`, `ListVisible` | U: успешная операция; запрет доступа или отсутствие записи; ошибка БД. |
| `ProjectRepository.Create`, `List`, `ProjectTokenGrant`, `Get`, `Update`, `Delete`, `Members`, `ChangeMember` | U для каждого: успешная операция; отсутствие записи или запрет доступа; ошибка БД. |
| `RoleRepository.Create`, `Get`, `List`, `Update`, `Delete` | U для каждого: успешная операция; отсутствие роли или конфликт; ошибка БД. |

### Model service: бизнес-логика и адаптер runtime

Исходники: `model-service/internal/service/`.

| Компонент и публичные методы | Что реализовать |
| --- | --- |
| `ModelService.Create`, `Get`, `Update` | U для каждого: успешная операция; некорректные данные или отсутствующая модель; ошибка зависимости. |
| `ModelService.Delete` | U: успешное удаление; активный deployment; ошибка зависимости. |
| `ModelService.List` | U: получение страницы; некорректная пагинация; ошибка зависимости. |
| `ModelService.Start`, `CreateDeployment`, `GetDeployment`, `StopDeployment` | U для каждого: успешный вызов; отсутствие модели или deployment; ошибка runtime. |
| `ModelService.GetDeploymentsHistory`, `GetActiveDeployments` | U для каждого: получение списка; неверный проект; ошибка runtime. |
| `GRPCModelRuntime.Start`, `CreateDeployment`, `StopDeployment` | U для каждого: успешный RPC; некорректные данные или пустой ответ; ошибка RPC. |
| `GRPCModelRuntime.GetDeployment` | U: успешное получение; deployment отсутствует; ошибка RPC. |
| `GRPCModelRuntime.GetDeploymentsHistory`, `GetActiveDeployments` | U для каждого: получение страниц с фильтрацией; повтор токена; ошибка RPC. |

Дополнительно необходимо проверить доменные правила спецификации модели и допустимости имени.

### Model service: доступ к данным

| Репозиторий и публичные методы | Что реализовать |
| --- | --- |
| `ModelRepository.Create`, `Get`, `List`, `Update`, `Delete` | U для каждого: успешная операция; отсутствие записи или конфликт; ошибка БД. |

### Inference service: бизнес-логика и управление состоянием

Исходники: `inference-service/internal/service/`, `internal/manager/` и `internal/runtime/`.

| Компонент и публичные методы | Что реализовать |
| --- | --- |
| `DeploymentService.StartDeployment` | U: успешный запуск; существующий deployment; ошибка запуска с очисткой ресурсов. |
| `DeploymentService.Init`, `GetDeploymentByID`, `GetDeploymentByName`, `StopDeployment` | U для каждого: успешная операция; неверные данные или отсутствие записи; ошибка зависимости. |
| `DeploymentService.ListDeployments`, `ListAllDeployments` | U для каждого: получение страницы; некорректная пагинация; ошибка репозитория. |
| `DeploymentManager.Init`, `Run`, `Reconcile`, `HandleEvent` | U для каждого: успешная обработка; ошибка зависимости; отмена или устаревшее событие. |
| `ProxyManager.Upsert`, `Remove`, `Prune`, `ServeHTTP` | U для каждого: успешная операция; отсутствие маршрута или некорректный endpoint; ошибка proxy или пограничный случай. |
| `RuntimeService.Close`, `GetContainer`, `ListServiceContainers`, `ContainerEndpoint`, `WatchContainerEvents`, `CreateContainer`, `StartContainer`, `StopContainer`, `RemoveContainer`, `EnsureImage`, `PullImage` | U с подменой Docker API: успешная операция, ошибка Docker или некорректные данные там, где применимо. |
| `DeploymentsCollector.Collect`, `ContainerLimitsCollector.Collect` | U: корректные значения метрик и сигнал об ошибке БД или Docker. |

### Inference service: доступ к данным

| Репозиторий и публичные методы | Что реализовать |
| --- | --- |
| `DeploymentRepository.Check` | U: успешная проверка; ошибка подключения. |
| `ActiveDeploymentExists`, `CreateDeployment` | U для каждого: успешный запрос; конфликт или отсутствие записи; ошибка БД. |
| `GetDeploymentByID`, `GetDeploymentByName`, `ListDeployments`, `ListAllDeployments` | U для каждого: получение данных; пустой результат или отсутствие записи; ошибка БД. |
| `ListDeploymentStates`, `GetDeploymentState`, `UpdateDeploymentStatus` | U для каждого: успешная операция; отсутствие записи или устаревшее состояние; ошибка БД. |

### Остальные сервисы и границы

`internal-api` содержит protobuf-контракт и сгенерированный код, а `utils` — общие инфраструктурные пакеты. API-адаптеры не входят в обязательный перечень методов этого этапа. Внешние API-тесты на async pytest в условии не требуются; при добавлении их следует запускать отдельно. Интеграционные тесты с реальными зависимостями не заменяют unit-тесты.

### Общие требования к реализации

- Для каждого публичного метода реализовать позитивный и негативный unit-сценарий с проверкой результата и ошибок.
- Отделить unit-тесты репозиториев с подменой DBTX от интеграционных тестов с PostgreSQL.
- Для каждого теста указать технику подготовки данных; использовать Data Builder, Object Mother, AAA и представить классический и лондонский стили.
- На следующих этапах настроить проверку случайного порядка, запуск без интернета и отчёты о результатах и покрытии строк и ветвлений.

## 2. Тестовая инфраструктура

Нужны Go версии из `go.mod`, Docker с Compose и доступом к daemon. Один раз установите инструменты отчётов командой `make test-tools`; для этого нужен интернет. `gobco` измеряет покрытие условий, `gocover-cobertura` формирует метрику строк. Для обычного прогона тестов эти инструменты не нужны: официальный Go-адаптер записывает результаты Allure непосредственно из тестов. Инструменты устанавливаются в `.test-tools/` и не изменяют зависимости сервисов. Allure Report 2 устанавливается отдельно.

- `make test` последовательно запускает unit- и интеграционные тесты; для интеграционной части требуется Docker с доступной PostgreSQL.
- `make test-unit` запускает тесты сервисов без `-short`, исключая интеграционные тесты PostgreSQL по префиксам имён в `infra/testing/run-go-tests.sh`. Новые интеграционные тесты `inference-service` должны начинаться с `TestDeploymentIntegration`. Результаты: `test-results/unit/`, данные Allure — в подкаталоге `allure-results/`.
- `make test-repository-integration` поднимает отдельный PostgreSQL и запускает интеграционные тесты репозиториев с явно заданными URL. Unit и интеграционные тесты deployment находятся в одном файле `inference-service/internal/repository/deployment_test.go`; интеграционные функции имеют префикс `TestDeploymentIntegration`, чтобы сценарий запуска мог отделить их от unit-тестов. Результаты: `test-results/integration/`. Порт по умолчанию 55432; изменить можно через `TEST_POSTGRES_PORT`. Тесты самостоятельно готовят изолированные временные таблицы или схему и применяют необходимые миграции.
- `make test-db-down` останавливает тестовый стенд. Данные PostgreSQL хранятся в tmpfs и удаляются при остановке контейнера; приложение и его БД команда не затрагивает.
- `make test-coverage` выполняет unit-тесты с покрытием операторов, сохраняет отчёты `.cover`, `.txt`, `.html`, Cobertura XML и JSON в `test-results/coverage/`, данные Allure — в `test-results/coverage/allure-results/`. Конвертер выводит в консоль количество покрытых и учтённых строк.
- `make test-branch-coverage` запускает gobco для пакетов бизнес-логики и репозиториев и записывает результаты в `test-results/branch/`. Gobco измеряет покрытие условий и не измеряет `select`; это не то же самое, что полноценное покрытие всех возможных ветвлений.

Команды возвращают ненулевой код при ошибке теста. Тесты и табличные подтесты
обёрнуты в `allure.Wrap` из официального пакета
[`github.com/allure-framework/allure-go/commons/gotest`](https://allurereport.org/docs/go/).
Адаптер записывает `*-result.json` прямо во время прогона; преобразование из
JUnit XML не используется. `go test -json` параллельно сохраняет исходные
события, а `python3` формирует отдельный HTML-отчёт `*.tests.html`.

Для запуска unit-тестов и просмотра Allure достаточно одной команды:

```sh
make test-unit-allure
```

Команда запускает тесты и вызывает `allure serve` для результатов прогона.
Отчёт доступен на `http://127.0.0.1:8090/`; сервер работает до `Ctrl+C`.
Отчёт открывается и при падении тестов, чтобы можно было посмотреть ошибки.
Другой порт задаётся через `ALLURE_PORT=8091`.

Для просмотра последнего прогона используйте `make allure-open`,
для интеграционного — `make allure-open ALLURE_RESULTS=integration`.
Отдельная генерация перед просмотром не нужна. Если требуется сохранить
HTML-отчёт, выполните `make allure-report`: он появится в
`test-results/allure/<режим>/`.

`run-go-tests.sh` задаёт абсолютный `ALLURE_RESULTS_DIR`:
`test-results/<режим>/allure-results/`. Перед каждым прогоном этот каталог
очищается, чтобы Allure не смешивал результаты разных запусков.
При прямом запуске из каталога Go-модуля можно указать свой путь:

```sh
ALLURE_RESULTS_DIR="$PWD/allure-results" go test -count=1 ./...
allure serve ./allure-results
```

Флаг `-count=1` отключает кэш тестов, чтобы результаты записывались заново.
Без `ALLURE_RESULTS_DIR` адаптер создаёт `allure-results/` в каждом тестируемом
пакете. Для ошибок используйте `allureCtx.Fatal`/`Errorf`, чтобы сообщение
попало в Allure; для API стандартного `testing.T` доступен `allureCtx.T()`.
У табличного подтеста должна быть собственная обёртка `allure.Wrap`.

### Файлы `infra/testing`

Цели `make test-unit`, `make test-repository-integration`, `make test-coverage`, `make test-shuffle`, `make test-processes` и `make test-unit-offline` вызывают `infra/testing/run-go-tests.sh` из корневого Makefile. Это сценарий запуска, а не тест: он выбирает режим `go test` для `user-service`, `model-service` и `inference-service` и сохраняет результаты каждого сервиса в `test-results/<режим>/`.

| Файл | Назначение |
| --- | --- |
| [`run-go-tests.sh`](infra/testing/run-go-tests.sh) | Запускает тесты трёх Go-модулей, исключает PostgreSQL-тесты из unit-прогона и сохраняет JSON и HTML, собирает записанные тестами результаты Allure и отчёты покрытия. |
| [`docker-compose.yaml`](infra/testing/docker-compose.yaml) | Поднимает отдельный PostgreSQL для `make test-repository-integration`; данные хранятся в `tmpfs`. |
| [`print-go-test-output.py`](infra/testing/print-go-test-output.py) | Печатает читаемый вывод из потока `go test -json`. |
| [`render-go-report.py`](infra/testing/render-go-report.py) | Преобразует сохранённые события `go test -json` в HTML-отчёт `*.tests.html`. |
| [`trace-test-process.sh`](infra/testing/trace-test-process.sh) | Обёртка для `go test -exec`: записывает запущенные тестовые бинарники в `processes.tsv` при `make test-processes`. |
| [`branch-coverage.sh`](infra/testing/branch-coverage.sh) | Запускает `gobco` для пакетов сервисов и репозиториев при `make test-branch-coverage`; отчёты кладёт в `test-results/branch/`. |

Проверка заголовков HTTP, gzip и условных запросов работающего NGINX запускается отдельно: `python3 nginx/tests/check_cache_headers.py`.

## 3. Тестовые данные и моки

В каждом сервисе пакет `internal/testutil` содержит повторно используемые тестовые данные и тестовые подмены зависимостей:

- `user-service`: builders и Object Mother для пользователей, проектов и ролей; моки репозиториев, хешера, менеджера токенов, часов и генератора ID.
- `model-service`: builders и Object Mother для моделей и deployment; моки репозитория модели и runtime.
- `inference-service`: builder и Object Mother для deployment; моки интерфейсов сервиса, менеджера и БД генерируются рядом с тестами соответствующих пакетов.
- Для SQL-репозиториев есть `MockDBTX`, позволяющий тестировать их без реальной БД: в `user-service` и `model-service` он находится в `testutil`, а в `inference-service` сгенерирован рядом с тестами репозитория.

Builders создают валидные значения по умолчанию и позволяют заменить поля через `With…`. `Build` возвращает независимую копию, включая вложенную спецификацию. ID по умолчанию фиксированы, а даты фиксированы в тех фикстурах, где они заданы: для тестов уникальности и нескольких записей нужно явно задать разные значения. Именованные методы Object Mother описывают частые состояния; фикстуры проекта, роли и deployment доступны также как функции.

В `inference-service` моки GoMock лежат рядом с тестами: `internal/service/deployment_mock_test.go`, `internal/manager/deployment_mock_test.go` и `internal/repository/db_mock_test.go`. Пересоздать их можно из корня проекта командой `make generate-mocks`. `internal/testutil/data.go` содержит `DeploymentBuilder` и `DeploymentMother`, а `internal/testutil/rows.go` — тестовую реализацию строк pgx; эти файлы не содержат тестовых функций. В user-service используются внешние тестовые пакеты `service_test`: моки этого сервиса импортируют его типы.

Техника подготовки данных фиксируется **в комментарии непосредственно перед каждым тестом** в формате `// Техника: анализ граничных значений; пустая строка.`; для разных подслучаев одного table-driven теста допускается отдельное поле `technique` в таблице случаев. Используйте названия техник из условия: разбиение на классы эквивалентности, анализ граничных значений, тестирование переходов состояний, комбинаторное тестирование и т. п. Например, «найденный контейнер» — представитель класса данных, а техника его выбора — разбиение на классы эквивалентности. Builder, Object Mother и фикстура описывают способ создания объектов и сами по себе не заменяют указание техники выбора значений.

## 4. Unit-тесты бизнес-логики

Добавлены тесты операций пользователей, проектов, ролей, моделей и deployment, включая проверки ошибок, границ и состояний. Для изоляции зависимостей используются моки `internal/testutil`; `TestCreateModelWithInMemoryRepository` демонстрирует классический стиль с репозиторием в памяти. Новые тесты содержат пометки о технике выбора данных.

Проверки сгруппированы по методам и сценариям. Таблицы используются для вариантов входных данных одного метода, например при валидации команды и переходах состояний; отдельные операции проверяются отдельными тестами. Покрытие операторов само по себе не подтверждает наличие всех ветвей.

### Тесты inference-service

| Файлы в `inference-service/internal/` | Что проверяется | Внешние зависимости |
| --- | --- | --- |
| `api/command_test.go`, `api/handler_test.go` | Преобразование и валидация команды `StartDeployment`, ошибка `AlreadyExists`, сериализация временных меток. | Без запущенного gRPC-сервера; используются protobuf-объекты и подменённый репозиторий. |
| `service/deployment_test.go`, `service/pagination_test.go` | Инициализация образа, запуск с проверкой дубля и очисткой при ошибке, получение и остановка deployment, страницы текущих запусков и истории. | Моки или тестовые реализации репозитория и runtime. |
| `manager/deployment_test.go`, `manager/proxy_test.go` | Сверка статусов, события Docker, переподключение и отмена; маршруты, проверка host/origin и обработка HTML/gzip. | Память, моки, `httptest` и подменённый HTTP transport. |
| `runtime/runtime_test.go` | Создание, чтение, список, запуск, остановка и удаление контейнеров; проверка и загрузка образа, поток событий, адрес контейнера и закрытие клиента. | Подменённый HTTP transport Docker SDK; Docker daemon не требуется. |
| `repository/deployment_test.go` | Unit-сценарии методов репозитория с `MockDBTX`; интеграционные проверки записи, ограничений уникальности, статусов и истории. | Для интеграционных тестов нужен PostgreSQL; сценарии чтения и записи используют временную таблицу в транзакции. |
| `metrics/deployments_test.go`, `metrics/container_limits_test.go` | Значения Prometheus-метрик и сигнал об ошибках БД или Docker. | Тестовые реализации БД и Docker. |

В файле `repository/deployment_test.go` тесты с префиксом `TestDeploymentIntegration` запускаются через `make test-repository-integration`; остальные входят в unit-прогон. Сгенерированные `*_mock_test.go` содержат только моки. Прямых тестов обработчика для `StopDeployment`, `GetDeploymentByID`, `GetDeploymentByName`, `ListDeployments` и `ListAllDeployments` сейчас нет: соответствующие операции проверяются на уровне сервиса и репозитория, но это не проверяет их gRPC-преобразование и коды ошибок.

## 5. Unit-тесты доступа к данным

Публичные методы репозиториев user-service, model-service и inference-service проверяются с подменой `DBTX`: успешные запросы, ошибки БД и отсутствие записей или пустые списки там, где это применимо. Отдельно проверяются преобразование возвращённых строк, конфликты уникальности, ошибки чтения и закрытие результатов, а также commit/rollback при ошибке записи и аудита.

В user-service репозитории принимают интерфейс `repository.Database`: он объединяет `DBTX` и начало транзакции. В тестах `MockDatabase` и `MockTx` заменяют пул и транзакцию; в приложении используется прежний `pgxpool.Pool`. Тестовые реализации `Row` и `Rows` находятся в `internal/testutil` каждого сервиса. Эти unit-тесты не проверяют выполнение SQL на PostgreSQL — для этого предназначен отдельный интеграционный прогон.

## 6. Проверка запуска и отчётов

Команды выполняются из корня репозитория; для запуска без доступа к Go proxy зависимости и выбранный инструмент должны быть заранее установлены:

- `make test-shuffle` запускает unit-тесты с `-shuffle=on`. Go записывает seed в JSON по каждому пакету; для повторения конкретного порядка используйте `TEST_SHUFFLE_SEED=<seed> make test-shuffle`. `-shuffle` перемешивает верхнеуровневые тесты и бенчмарки внутри пакета, а не гарантирует перемешивание подзапусков таблицы.
- `make test-unit-offline` на macOS запускает тесты через `sandbox-exec` с запретом сетевых операций и `GOPROXY=off GOSUMDB=off`. Нужны доступные локально Go, модули, Python и разрешение ОС на создание sandbox. Команда не устанавливает инструменты и не скачивает зависимости.
- `make test-processes` передаёт `go test` флаг `-exec` с обёрткой `infra/testing/trace-test-process.sh`. Она записывает имя модуля, PID, PPID и путь к тестовому бинарнику в `test-results/processes/processes.tsv`, а затем заменяется им через `exec`. Итоги — в `summary.txt`.
- `make test-coverage` сохраняет HTML и текстовый отчёты `go tool cover` в `test-results/coverage/`; они показывают покрытие операторов Go. `gocover-cobertura` дополнительно формирует `*.cobertura.xml` и выводит оценку покрытия строк. Покрытие ветвлений рассчитывается отдельно.
- `make test-unit-allure` запускает unit-тесты и открывает их результаты через `allure serve`. Нужен Allure Report 2 CLI (macOS: `brew install allure`); результаты тестов записываются и без установленного CLI. `make allure-report ALLURE_RESULTS=integration` сохраняет HTML-отчёт по последнему интеграционному прогону. Allure отображает результаты тестов, но не измеряет покрытие кода.
- `make test-branch-coverage` предназначен для отчётов gobco по условиям в `test-results/branch/`. Перед запуском нужен `make test-tools`; gobco не покрывает все виды ветвлений, в том числе `select`.

### Сколько процессов запускают unit-тесты

Из корня репозитория выполните:

```sh
make test-processes
cat test-results/processes/summary.txt
cat test-results/processes/processes.tsv
```

`summary.txt` содержит число запусков по сервисам, а каждая строка `processes.tsv` — модуль, PID, PPID и путь к тестовому бинарнику. Обёртка запускает бинарник через `exec`, поэтому её PID становится PID тестового процесса. Команда использует тот же список пакетов и исключение интеграционных тестов PostgreSQL, что и `make test-unit`.

В Go нет тестовых классов: `go test` запускает отдельный тестовый бинарник для каждого пакета с тестами. Функции `Test...` и подзапуски `t.Run` внутри пакета выполняются в этом бинарнике; файлы внешнего пакета `*_test` тоже включаются в него. `go test -p N` ограничивает число одновременно выполняемых команд сборки и тестовых бинарников, а `go test -parallel N` — число одновременно выполняемых тестов, вызвавших `t.Parallel`, внутри одного бинарника. По умолчанию оба предела зависят от `GOMAXPROCS`. Эти флаги управляют параллелизмом, а не числом бинарников за полный прогон. Флаг `-count=1` в сценарии отключает кэш успешных тестов, чтобы бинарники действительно запустились и попали в журнал. В Go предел одновременно выполняемых тестовых бинарников задаёт флаг -p N. Для команд этого проекта его можно передать так: `GOFLAGS='-p=4' make test-unit`

Подсчёт относится только к запускам тестовых бинарников: в него не входят процессы `go test`, компилятора, возможные дочерние процессы тестов, потоки ОС и горутины.

## 7. ЛР №2: контейнерный pipeline

Из корня репозитория выполните `make test-pipeline`. Нужны Docker с Compose v2, доступ к Docker socket и сеть для первой сборки зависимостей. Хостовые Go, PostgreSQL, Java и Allure не нужны. Один образ runner содержит Go 1.26.5, зависимости из `go.mod`/`go.sum`, Allure 2, curl/tcpdump и собранные штатные бинарники трёх сервисов и bootstrap-admin. В build-контейнер передаётся snapshot текущих исходников (без ключей и `.env`), оформляется как временный Git-репозиторий и клонируется внутри контейнера; бинарники и тесты используют эту же клонированную копию. Так локальные незакоммиченные изменения включены в проверку, а в CI snapshot основан на checkout проверяемого коммита. `commit.txt`, `worktree.txt` и `snapshot-revision.txt` сохраняют исходный HEAD, наличие локальных изменений и ревизию временного snapshot; рабочий Git-репозиторий скрипт не изменяет.

Порядок: миграции и bootstrap → unit ЛР №1 → ровно три `TestServiceIntegration` → один E2E через curl. Новые integration исключены из unit и запускаются с `-count=1`, каждый использует настоящий сервисный слой, репозиторий и PostgreSQL. Только Docker runtime в integration inference-service заменён fake. Отсутствие DSN, пропуск или отсутствие одного из трёх результатов завершают обязательный integration-прогон ошибкой. Unit-тесты запускаются через `make test-unit`, тесты репозиториев — через `make test-repository-integration`, а три теста сервисов — через `make test-integration`.

E2E входит через Nginx, выполняет login, создаёт проект, получает project token, создаёт Hugging Face модель, запускает deployment с `Idempotency-Key`, ждёт running, останавливает и удаляет модель, проверяет 404. `INFERENCE_SERVER_IMAGE` указывает на маленький HTTP fixture, принимающий аргументы llama без скачивания весов и записи в `/cache`. Реальными остаются три сервиса, PostgreSQL, Nginx и Docker.

Каждый вызов создаёт проект `lab2-<время>-<pid>-<случайное число>`, отдельную сеть и PostgreSQL без фиксированных host-портов/имён. Результаты печатаются в начале и сохраняются в `test-results/lab2/<project>/`. При ошибке следующие стадии не выполняются и показаны skipped. Обработчик EXIT/INT/TERM останавливает писателей и только модельные контейнеры сети этого прогона, пересоздаёт тестовую БД, применяет все штатные миграции и seed, проверяет S0 SQL-запросами и удаляет ресурсы Compose. Общие ML-кеши и чужие контейнеры не удаляются. SIGKILL всей машины не обрабатывается.

Артефакты: `summary.json`, `unit/` и `integration/` с Go JSON и native Allure, `allure-report/`, `setup.log`, `cleanup.log` с `S0 verified`, `stand.log`, `runner.log`, `e2e.log`, `traffic.pcap`, читаемый `traffic.txt`. Tcpdump снимает HTTP-трафик curl на интерфейсе тестовой сети перед login и до конца E2E; запись содержит только тестовые пароли/токены. Посмотреть её можно командой `tcpdump -nn -A -r <каталог-прогона>/traffic.pcap`; открыть отчёт — `allure open <каталог-прогона>/allure-report` при наличии локального Allure CLI. Не публикуйте каталог `keys/`; CI исключает его из артефакта.

История лежит отдельно в `test-results/lab2/history/`: следующий отчёт забирает только `history/`, сырые результаты предыдущего прогона не переносятся. Обновление истории защищено блокировкой при конкурентном завершении. Путь можно изменить через `LAB2_HISTORY_DIR`; `LAB2_RESULTS_DIR` должен указывать на новый пустой каталог. `.github/workflows/testing.yaml` запускает тот же runner, всегда выполняет финализацию, сохраняет отчёты/pcap как artifact и переносит историю через GitHub Actions cache. Удалённый запуск CI требует push или ручного workflow_dispatch.

### Демонстрация повторяемости и изоляции

Команда `make test-integration` запускает три интеграционных сценария сервисов. После первой сборки `LAB2_SKIP_BUILD=1` переиспользует runner; при изменении исходников сначала выполните обычную сборку. По три integration выполняются два раза последовательно и два раза одновременно:

```sh
LAB2_SKIP_BUILD=1 ./infra/testing/lab2/run.sh integration
LAB2_SKIP_BUILD=1 ./infra/testing/lab2/run.sh integration
LAB2_SKIP_BUILD=1 ./infra/testing/lab2/run.sh integration > /tmp/lab2-a.log 2>&1 &
a=$!
LAB2_SKIP_BUILD=1 ./infra/testing/lab2/run.sh integration > /tmp/lab2-b.log 2>&1 &
b=$!
wait "$a"
wait "$b"
```

Два последовательных отчёта также демонстрируют тренд Allure. В каждом `integration/<service>.json` должна быть одна запись pass для `TestServiceIntegration`, а `cleanup.log` должен содержать подтверждение S0. Проверить отсутствие оставшихся контейнеров конкретного проекта: `docker ps -a --filter label=com.docker.compose.project=<project>`.

### Демонстрация отказа и прерывания

Контроли ниже предназначены для ручной демонстрации, не добавляют продуктовых сценариев. `LAB2_FAIL_STAGE=integration` вызывает отказ после интеграционных проверок: unit/integration имеют результаты, E2E skipped. `LAB2_FAIL_STAGE=e2e` вызывает отказ после перехода реального deployment в running, до остановки модели. Оба вызова должны завершиться с ненулевым кодом, сохранить отчёт и подтвердить восстановление S0:

```sh
LAB2_SKIP_BUILD=1 LAB2_FAIL_STAGE=integration ./infra/testing/lab2/run.sh full
LAB2_SKIP_BUILD=1 LAB2_FAIL_STAGE=e2e ./infra/testing/lab2/run.sh full
```

Для прерывания с живым deployment задайте отдельные пути, дождитесь `pause-ready` и отправьте TERM внешнему скрипту; он вернёт 143 после финализации:

```sh
LAB2_RESULTS_DIR="$PWD/test-results/lab2/interruption-demo" \
LAB2_SKIP_BUILD=1 LAB2_PAUSE_STAGE=e2e ./infra/testing/lab2/run.sh full > /tmp/lab2-interrupt.log 2>&1 &
pid=$!
while [ ! -f test-results/lab2/interruption-demo/pause-ready ]; do sleep 1; done
kill -TERM "$pid"
wait "$pid"
```

Используйте новый пустой каталог для каждого повтора. Для демонстрации прерывания integration аналогично задайте `LAB2_PAUSE_STAGE=integration`. В `summary.json` прерванная стадия broken, следующие skipped, cleanup passed; в `cleanup.log` видна проверка S0 до удаления БД. Если внешний процесс был остановлен до срабатывания trap, CI вызывает `run.sh finalize` с теми же `LAB2_PROJECT`, `LAB2_RESULTS_DIR` и `LAB2_HISTORY_DIR`.

