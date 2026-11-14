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

Нужны Go версии из `go.mod`, Docker с Compose и доступом к daemon. Один раз установите инструменты отчётов командой `make test-tools`; для этого нужен интернет. `gotestsum` создаёт JUnit XML и JSON, `gobco` измеряет покрытие условий, `gocover-cobertura` формирует метрику строк. Инструменты устанавливаются в `.test-tools/` и не изменяют зависимости сервисов. Allure Report 2 устанавливается отдельно.

- `make test` последовательно запускает unit- и интеграционные тесты; для интеграционной части требуется Docker с доступной PostgreSQL.
- `make test-unit` запускает тесты сервисов без `-short`, исключая интеграционные тесты PostgreSQL по префиксам имён в `infra/testing/run-go-tests.sh`. Новые интеграционные тесты `inference-service` должны начинаться с `TestDeploymentIntegration`. Результаты: `test-results/unit/`.
- `make test-integration` поднимает отдельный PostgreSQL и запускает интеграционные тесты репозиториев с явно заданными URL. Unit и интеграционные тесты deployment находятся в одном файле `inference-service/internal/repository/deployment_test.go`; интеграционные функции имеют префикс `TestDeploymentIntegration`, чтобы сценарий запуска мог отделить их от unit-тестов. Результаты: `test-results/integration/`. Порт по умолчанию 55432; изменить можно через `TEST_POSTGRES_PORT`. Тесты самостоятельно готовят изолированные временные таблицы или схему и применяют необходимые миграции.
- `make test-db-down` останавливает тестовый стенд. Данные PostgreSQL хранятся в tmpfs и удаляются при остановке контейнера; приложение и его БД команда не затрагивает.
- `make test-coverage` выполняет unit-тесты с покрытием операторов, сохраняет отчёты `.cover`, `.txt`, `.html`, Cobertura XML, JUnit XML и JSON в `test-results/coverage/`. Конвертер выводит в консоль количество покрытых и учтённых строк.
- `make test-branch-coverage` запускает gobco для пакетов бизнес-логики и репозиториев и записывает результаты в `test-results/branch/`. Gobco измеряет покрытие условий и не измеряет `select`; это не то же самое, что полноценное покрытие всех возможных ветвлений.

Команды возвращают ненулевой код при ошибке теста; результаты пишутся по каждому сервису отдельно. При наличии `gotestsum` создаётся JUnit XML; без него `go test -json` сохраняет события в файл и одновременно выводит текст результатов в консоль. В обоих случаях `python3` преобразует события в HTML-отчёт `*.tests.html`. Дополнительно доступен Allure Report 2 на основе JUnit XML от gotestsum.

### Файлы `infra/testing`

Цели `make test-unit`, `make test-integration`, `make test-coverage`, `make test-shuffle`, `make test-processes` и `make test-unit-offline` вызывают `infra/testing/run-go-tests.sh` из корневого Makefile. Это сценарий запуска, а не тест: он выбирает режим `go test` для `user-service`, `model-service` и `inference-service` и сохраняет результаты каждого сервиса в `test-results/<режим>/`.

| Файл | Назначение |
| --- | --- |
| [`run-go-tests.sh`](infra/testing/run-go-tests.sh) | Запускает тесты трёх Go-модулей, исключает PostgreSQL-тесты из unit-прогона и создаёт JSON, JUnit XML при наличии `gotestsum`, HTML и отчёты покрытия. |
| [`docker-compose.yaml`](infra/testing/docker-compose.yaml) | Поднимает отдельный PostgreSQL для `make test-integration`; данные хранятся в `tmpfs`. |
| [`print-go-test-output.py`](infra/testing/print-go-test-output.py) | Печатает читаемый вывод из потока `go test -json`, когда `gotestsum` не установлен. |
| [`render-go-report.py`](infra/testing/render-go-report.py) | Преобразует сохранённые события `go test -json` в HTML-отчёт `*.tests.html`. |
| [`trace-test-process.sh`](infra/testing/trace-test-process.sh) | Обёртка для `go test -exec`: записывает запущенные тестовые бинарники в `processes.tsv` при `make test-processes`. |
| [`branch-coverage.sh`](infra/testing/branch-coverage.sh) | Запускает `gobco` для пакетов сервисов и репозиториев при `make test-branch-coverage`; отчёты кладёт в `test-results/branch/`. |
| [`allure-report.sh`](infra/testing/allure-report.sh) | Создаёт Allure-отчёт из JUnit XML последнего прогона по `make allure-report` или `make test-unit-allure`. |

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

В файле `repository/deployment_test.go` тесты с префиксом `TestDeploymentIntegration` запускаются через `make test-integration`; остальные входят в unit-прогон. Сгенерированные `*_mock_test.go` содержат только моки. Прямых тестов обработчика для `StopDeployment`, `GetDeploymentByID`, `GetDeploymentByName`, `ListDeployments` и `ListAllDeployments` сейчас нет: соответствующие операции проверяются на уровне сервиса и репозитория, но это не проверяет их gRPC-преобразование и коды ошибок.

## 5. Unit-тесты доступа к данным

Публичные методы репозиториев user-service, model-service и inference-service проверяются с подменой `DBTX`: успешные запросы, ошибки БД и отсутствие записей или пустые списки там, где это применимо. Отдельно проверяются преобразование возвращённых строк, конфликты уникальности, ошибки чтения и закрытие результатов, а также commit/rollback при ошибке записи и аудита.

В user-service репозитории принимают интерфейс `repository.Database`: он объединяет `DBTX` и начало транзакции. В тестах `MockDatabase` и `MockTx` заменяют пул и транзакцию; в приложении используется прежний `pgxpool.Pool`. Тестовые реализации `Row` и `Rows` находятся в `internal/testutil` каждого сервиса. Эти unit-тесты не проверяют выполнение SQL на PostgreSQL — для этого предназначен отдельный интеграционный прогон.

## 6. Проверка запуска и отчётов

Команды выполняются из корня репозитория; для запуска без доступа к Go proxy зависимости и выбранный инструмент должны быть заранее установлены:

- `make test-shuffle` запускает unit-тесты с `-shuffle=on`. Go записывает seed в JSON по каждому пакету; для повторения конкретного порядка используйте `TEST_SHUFFLE_SEED=<seed> make test-shuffle`. `-shuffle` перемешивает верхнеуровневые тесты и бенчмарки внутри пакета, а не гарантирует перемешивание подзапусков таблицы.
- `make test-unit-offline` на macOS запускает тесты через `sandbox-exec` с запретом сетевых операций и `GOPROXY=off GOSUMDB=off`. Нужны доступные локально Go, модули, Python и разрешение ОС на создание sandbox. Команда не устанавливает инструменты и не скачивает зависимости.
- `make test-processes` передаёт `go test` флаг `-exec` с обёрткой `infra/testing/trace-test-process.sh`. Она записывает имя модуля, PID, PPID и путь к тестовому бинарнику в `test-results/processes/processes.tsv`, а затем заменяется им через `exec`. Итоги — в `summary.txt`.
- `make test-coverage` сохраняет HTML и текстовый отчёты `go tool cover` в `test-results/coverage/`; они показывают покрытие операторов Go. `gocover-cobertura` дополнительно формирует `*.cobertura.xml` и выводит оценку покрытия строк. Покрытие ветвлений рассчитывается отдельно.
- `make test-unit-allure` запускает unit-тесты и затем формирует Allure-отчёт в `test-results/allure/unit/`. Установите gotestsum через `make test-tools` и Allure Report 2 CLI (macOS: `brew install allure`). Команда `make allure-report ALLURE_RESULTS=integration` формирует отчёт по последнему интеграционному прогону. Используется встроенный в Allure 2 JUnit XML plugin; без `gotestsum` XML не создаётся. Allure отображает результаты тестов, но не измеряет покрытие кода.
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

