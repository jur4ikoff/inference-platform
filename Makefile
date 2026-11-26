.DEFAULT_GOAL := help

COMPOSE_PROJECT := inference-platform
COMPOSE := docker compose -p $(COMPOSE_PROJECT) --project-directory . --env-file infra/.env -f infra/docker-compose.yaml
USER_SERVICE := user-service
MODEL_SERVICE := model-service
INFERENCE_SERVICE := inference-service
INFRA_SERVICES := loki tempo otel-collector alloy prometheus grafana
APPLICATION_SERVICES := $(USER_SERVICE) $(MODEL_SERVICE) $(INFERENCE_SERVICE) nginx adminer observability-service
BUF := docker run --rm -v "$(CURDIR)/internal-api:/workspace" -w /workspace bufbuild/buf:1.61.0
SQLC := docker run --rm -v "$(CURDIR):/src" -w /src sqlc/sqlc:1.31.1
PREK ?= prek

.PHONY: help up up-infra up-database up-services bootstrap-admin up-user up-model up-inference locust proto sql-generate sql-check build down restart restart-user restart-model restart-inference ps logs logs-user logs-model logs-inference logs-postgres logs-observability config test lint fmt

help: ## Показать доступные команды
	@printf '%s\n' \
		'Использование: make <цель>' \
		'' \
		'  up                  Поднять инфраструктуру, БД с миграциями и сервисы' \
		'  up-infra            Поднять инфраструктуру наблюдаемости' \
		'  up-database         Поднять PostgreSQL и применить миграции' \
		'  up-services         Поднять БД с миграциями и прикладные сервисы' \
		'  bootstrap-admin     Создать начального администратора из infra/.env' \
		'  up-user             Поднять user-service и его зависимости' \
		'  up-model            Поднять model-service и его зависимости' \
		'  up-inference        Поднять inference-service и его зависимости' \
		'  locust              Поднять сервисы и веб-интерфейс Locust' \
		'  proto               Сгенерировать protobuf- и gRPC-стабы internal-api' \
		'  sql-generate        Сгенерировать Go-обёртки SQL-запросов через sqlc' \
		'  sql-check           Проверить SQL-запросы и схему для sqlc' \
		'  build               Пересобрать образы без запуска контейнеров' \
		'  down                Остановить и удалить контейнеры и сеть (volumes сохраняются)' \
		'  restart             Перезапустить весь стек' \
		'  restart-user        Пересобрать и перезапустить user-service' \
		'  restart-model       Пересобрать и перезапустить model-service' \
		'  restart-inference   Пересобрать и перезапустить inference-service' \
		'  ps                  Показать состояние контейнеров' \
		'  logs                Смотреть логи всего стека' \
		'  logs-user           Смотреть логи user-service' \
		'  logs-model          Смотреть логи model-service' \
		'  logs-inference      Смотреть логи inference-service' \
		'  logs-postgres       Смотреть логи PostgreSQL' \
		'  logs-observability  Смотреть логи Grafana, Loki, Tempo и OTel Collector' \
		'  config              Проверить итоговую Docker Compose-конфигурацию' \
		'  test                Запустить unit- и интеграционные тесты' \
		'  test-unit-allure    Запустить unit-тесты и открыть Allure-отчёт' \
		'  allure-report       Создать HTML-отчёт из результатов Allure' \
		'  allure-open         Открыть результаты последнего прогона в Allure' \
		'  generate-mocks      Сгенерировать тестовые моки inference-service' \
		'  test-repository-integration  Интеграционные тесты репозиториев с PostgreSQL' \
		'  test-pipeline           ЛР №2: unit → integration → E2E в Docker' \
		'  test-integration    Интеграционные тесты сервисов с PostgreSQL' \
		'  test-shuffle        Запустить unit-тесты в случайном порядке' \
		'  test-coverage       Сформировать отчёты покрытия операторов' \
		'  test-branch-coverage  Измерить покрытие условий с gobco' \
		'  lint                Запустить prek вручную для всех отслеживаемых файлов' \
		'  fmt                 Отформатировать Go-код сервисов'

up: up-services ## Поднять инфраструктуру, БД с миграциями и сервисы

up-infra: ## Поднять инфраструктуру наблюдаемости
	$(COMPOSE) up -d --remove-orphans --wait $(INFRA_SERVICES)

up-database: ## Поднять PostgreSQL и применить миграции
	$(COMPOSE) up -d --remove-orphans --wait postgres
	./infra/postgres/apply_migrations.sh

up-services: up-infra up-database ## Поднять БД с миграциями и прикладные сервисы
	$(COMPOSE) up -d --remove-orphans --build --no-deps $(APPLICATION_SERVICES)
	$(COMPOSE) exec -T nginx nginx -s reload

bootstrap-admin: up-database ## Создать начального администратора из infra/.env
	@set -a; . infra/.env; set +a; \
	DATABASE_URL="postgres://$$POSTGRES_USER:$$POSTGRES_PASSWORD@localhost:5432/$$POSTGRES_DB?sslmode=disable" \
	./scripts/bootstrap-admin.sh

up-user: up-database ## Поднять user-service и его зависимости
	$(COMPOSE) up -d --remove-orphans --build $(USER_SERVICE)

up-model: up-database ## Поднять model-service и его зависимости
	$(COMPOSE) up -d --remove-orphans --build $(MODEL_SERVICE)

up-inference: up-database ## Поднять БД с миграциями и inference-service
	$(COMPOSE) up -d --remove-orphans --build $(INFERENCE_SERVICE)

locust: bootstrap-admin up-services ## Поднять сервисы и веб-интерфейс Locust
	@test -f stress-test/.env || cp stress-test/.env.example stress-test/.env
	@mkdir -p stress-test/docker-results
	docker compose --env-file infra/.env --env-file stress-test/.env -f stress-test/docker-compose.yaml up -d --remove-orphans locust-ui
	@printf '%s\n' 'Locust UI: http://localhost:8089'

proto: ## Сгенерировать protobuf- и gRPC-стабы internal-api
	$(BUF) generate

sql-generate: ## Сгенерировать Go-обёртки SQL-запросов
	$(SQLC) generate -f infra/sqlc.yaml

sql-check: ## Проверить SQL-запросы и схему
	$(SQLC) compile -f infra/sqlc.yaml

build: ## Пересобрать образы
	$(COMPOSE) build

down: ## Остановить стек, не удаляя volumes
	$(COMPOSE) down --remove-orphans

restart: down up ## Перезапустить весь стек

restart-user: ## Пересобрать и перезапустить user-service
	$(COMPOSE) up -d --remove-orphans --build --force-recreate $(USER_SERVICE)

restart-model: ## Пересобрать и перезапустить model-service
	$(COMPOSE) up -d --remove-orphans --build --force-recreate $(MODEL_SERVICE)

restart-inference: up-database ## Пересобрать и перезапустить inference-service
	$(COMPOSE) up -d --remove-orphans --build --force-recreate $(INFERENCE_SERVICE)



ps: ## Показать состояние контейнеров
	$(COMPOSE) ps

logs: ## Смотреть логи всего стека
	$(COMPOSE) logs --follow --tail=100

logs-user: ## Смотреть логи user-service
	$(COMPOSE) logs --follow --tail=100 $(USER_SERVICE)

logs-model: ## Смотреть логи model-service
	$(COMPOSE) logs --follow --tail=100 $(MODEL_SERVICE)

logs-inference: ## Смотреть логи inference-service
	$(COMPOSE) logs --follow --tail=100 $(INFERENCE_SERVICE)



logs-postgres: ## Смотреть логи PostgreSQL
	$(COMPOSE) logs --follow --tail=100 postgres

logs-observability: ## Смотреть логи observability-стека
	$(COMPOSE) logs --follow --tail=100 grafana loki tempo otel-collector alloy

config: ## Проверить итоговую Compose-конфигурацию
	$(COMPOSE) config --quiet

test: ## Запустить unit- и интеграционные тесты
	$(MAKE) test-unit
	$(MAKE) test-repository-integration
	$(MAKE) test-integration

lint: ## Запустить проверки prek для всех отслеживаемых файлов
	$(PREK) validate-config prek.toml
	$(PREK) run --stage manual --all-files

format: ## Отформатировать Go-код сервисов
	cd $(USER_SERVICE) && gofmt -w $$(find . -name '*.go' -type f)
	cd $(MODEL_SERVICE) && gofmt -w $$(find . -name '*.go' -type f)

# Изолированная тестовая БД и отчёты не используют конфигурацию приложения.
TEST_COMPOSE := docker compose -p inference-platform-tests -f infra/testing/docker-compose.yaml
TEST_POSTGRES_PORT ?= 55432
TEST_DATABASE_URL := postgres://test:test@127.0.0.1:$(TEST_POSTGRES_PORT)/test?sslmode=disable
TEST_TOOLS := $(CURDIR)/.test-tools
export PATH := $(TEST_TOOLS):$(PATH)
export TEST_POSTGRES_PORT

.PHONY: generate-mocks test-tools test-unit test-db-up test-db-down test-repository-integration test-coverage test-branch-coverage

generate-mocks: ## Сгенерировать тестовые моки inference-service
	cd inference-service && go generate ./internal/service ./internal/repository ./internal/manager

test-tools: ## Установить инструменты тестовых отчётов
	@mkdir -p $(TEST_TOOLS)
	GOBIN=$(TEST_TOOLS) go install github.com/rillig/gobco@v1.3.4
	GOBIN=$(TEST_TOOLS) go install github.com/boumenot/gocover-cobertura@v1.5.0

test-unit: ## Запустить unit-тесты с записью результатов Allure
	./infra/testing/run-go-tests.sh unit

test-db-up: ## Запустить временную тестовую PostgreSQL
	$(TEST_COMPOSE) up -d --wait postgres

test-db-down: ## Остановить тестовую PostgreSQL
	$(TEST_COMPOSE) down

test-repository-integration: test-db-up ## Запустить интеграционные тесты репозиториев с отдельной БД
	USER_TEST_DATABASE_URL='$(TEST_DATABASE_URL)' MODEL_TEST_DATABASE_URL='$(TEST_DATABASE_URL)' INFERENCE_TEST_DATABASE_URL='$(TEST_DATABASE_URL)' ./infra/testing/run-go-tests.sh integration

test-coverage: ## Собрать покрытие операторов и отчёты unit-тестов
	./infra/testing/run-go-tests.sh coverage

test-branch-coverage: ## Собрать отчёт gobco по условиям
	./infra/testing/branch-coverage.sh

.PHONY: test-shuffle test-processes test-unit-offline

test-shuffle: ## Запустить unit-тесты в случайном порядке
	./infra/testing/run-go-tests.sh shuffle

test-processes: ## Зафиксировать процессы тестовых бинарников
	./infra/testing/run-go-tests.sh processes

test-unit-offline: ## Запустить unit-тесты без доступа к сети (macOS)
	sandbox-exec -p '(version 1) (allow default) (deny network*)' env GOPROXY=off GOSUMDB=off $(CURDIR)/infra/testing/run-go-tests.sh unit

ALLURE_RESULTS ?= unit
ALLURE_PORT ?= 8090
.PHONY: allure-report allure-open test-unit-allure

allure-report: ## Создать HTML-отчёт из результатов Allure
	@command -v allure >/dev/null || { echo 'Allure CLI не найден: установите Allure Report 2' >&2; exit 1; }
	@test -n "$(firstword $(wildcard test-results/$(ALLURE_RESULTS)/allure-results/*-result.json))" || { echo 'Результаты Allure не найдены: запустите тесты' >&2; exit 1; }
	allure generate "test-results/$(ALLURE_RESULTS)/allure-results" --clean -o "test-results/allure/$(ALLURE_RESULTS)"

allure-open: ## Открыть результаты последнего прогона в Allure
	@command -v allure >/dev/null || { echo 'Allure CLI не найден: установите Allure Report 2' >&2; exit 1; }
	@test -n "$(firstword $(wildcard test-results/$(ALLURE_RESULTS)/allure-results/*-result.json))" || { echo 'Результаты Allure не найдены: запустите тесты' >&2; exit 1; }
	allure serve --host 127.0.0.1 --port $(ALLURE_PORT) "$(CURDIR)/test-results/$(ALLURE_RESULTS)/allure-results"

test-unit-allure: ## Запустить unit-тесты, собрать и открыть Allure-отчёт
	@command -v allure >/dev/null || { echo 'Allure CLI не найден: установите Allure Report 2' >&2; exit 1; }
	@status=0; $(MAKE) test-unit || status=$$?; \
	$(MAKE) allure-open ALLURE_RESULTS=unit || status=1; exit $$status

.PHONY: test-pipeline test-integration
test-pipeline: ## ЛР №2: unit → integration → E2E в изолированных контейнерах
	./infra/testing/lab2/run.sh full

test-integration: ## Запустить интеграционные тесты сервисов с PostgreSQL в контейнерах
	./infra/testing/lab2/run.sh integration
