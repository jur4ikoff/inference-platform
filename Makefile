.DEFAULT_GOAL := help

COMPOSE_PROJECT := inference-platform
COMPOSE := docker compose -p $(COMPOSE_PROJECT) --project-directory . --env-file infra/.env -f infra/docker-compose.yaml
USER_SERVICE := user-service
MODEL_SERVICE := model-service
INFERENCE_SERVICE := inference-service
INFRA_SERVICES := loki tempo otel-collector alloy grafana
APPLICATION_SERVICES := $(USER_SERVICE) $(MODEL_SERVICE) $(INFERENCE_SERVICE) nginx adminer observability-service
BUF := docker run --rm -v "$(CURDIR)/internal-api:/workspace" -w /workspace bufbuild/buf:1.61.0
SQLC := docker run --rm -v "$(CURDIR):/src" -w /src sqlc/sqlc:1.31.1
PREK ?= prek

.PHONY: help up up-infra up-database up-services bootstrap-admin up-user up-model up-inference proto sql-generate sql-check build down restart restart-user restart-model restart-inference ps logs logs-user logs-model logs-inference logs-postgres logs-observability config test lint fmt

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
		'  test-integration    Запустить интеграционные тесты с PostgreSQL' \
		'  test-shuffle        Запустить unit-тесты в случайном порядке' \
		'  test-coverage       Сформировать отчёты покрытия операторов' \
		'  test-branch-coverage  Измерить покрытие условий с gobco' \
		'  lint                Запустить prek вручную для всех отслеживаемых файлов' \
		'  fmt                 Отформатировать Go-код сервисов'

up: ## Поднять инфраструктуру, БД с миграциями и сервисы
	$(MAKE) up-infra
	$(MAKE) up-services

up-infra: ## Поднять инфраструктуру наблюдаемости
	$(COMPOSE) up -d --wait $(INFRA_SERVICES)

up-database: ## Поднять PostgreSQL и применить миграции
	$(COMPOSE) up -d --wait postgres
	./infra/postgres/apply_migrations.sh

up-services: up-database ## Поднять БД с миграциями и прикладные сервисы
	$(COMPOSE) up -d --build --no-deps $(APPLICATION_SERVICES)

bootstrap-admin: up-database ## Создать начального администратора из infra/.env
	@set -a; . infra/.env; set +a; \
	DATABASE_URL="postgres://$$POSTGRES_USER:$$POSTGRES_PASSWORD@localhost:5432/$$POSTGRES_DB?sslmode=disable" \
	./scripts/bootstrap-admin.sh

up-user: ## Поднять user-service и его зависимости
	$(COMPOSE) up -d --build $(USER_SERVICE)

up-model: ## Поднять model-service и его зависимости
	$(COMPOSE) up -d --build $(MODEL_SERVICE)

up-inference: up-database ## Поднять БД с миграциями и inference-service
	$(COMPOSE) up -d --build $(INFERENCE_SERVICE)



proto: ## Сгенерировать protobuf- и gRPC-стабы internal-api
	$(BUF) generate

sql-generate: ## Сгенерировать Go-обёртки SQL-запросов
	$(SQLC) generate -f infra/sqlc.yaml

sql-check: ## Проверить SQL-запросы и схему
	$(SQLC) compile -f infra/sqlc.yaml

build: ## Пересобрать образы
	$(COMPOSE) build

down: ## Остановить стек, не удаляя volumes
	$(COMPOSE) down

restart: down up ## Перезапустить весь стек

restart-user: ## Пересобрать и перезапустить user-service
	$(COMPOSE) up -d --build --force-recreate $(USER_SERVICE)

restart-model: ## Пересобрать и перезапустить model-service
	$(COMPOSE) up -d --build --force-recreate $(MODEL_SERVICE)

restart-inference: up-database ## Пересобрать и перезапустить inference-service
	$(COMPOSE) up -d --build --force-recreate $(INFERENCE_SERVICE)



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

.PHONY: test-tools test-unit test-db-up test-db-down test-integration test-coverage test-branch-coverage

test-tools: ## Установить инструменты тестовых отчётов
	@mkdir -p $(TEST_TOOLS)
	GOBIN=$(TEST_TOOLS) go install gotest.tools/gotestsum@v1.13.0
	GOBIN=$(TEST_TOOLS) go install github.com/rillig/gobco@v1.3.4
	GOBIN=$(TEST_TOOLS) go install github.com/boumenot/gocover-cobertura@v1.5.0

test-unit: ## Запустить unit-тесты и сформировать JUnit-отчёты
	./infra/testing/run-go-tests.sh unit

test-db-up: ## Запустить временную тестовую PostgreSQL
	$(TEST_COMPOSE) up -d --wait postgres

test-db-down: ## Остановить тестовую PostgreSQL
	$(TEST_COMPOSE) down

test-integration: test-db-up ## Запустить интеграционные тесты с отдельной БД
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
.PHONY: allure-report test-unit-allure

allure-report: ## Создать Allure-отчёт из последнего JUnit-прогона
	./infra/testing/allure-report.sh "$(ALLURE_RESULTS)"

test-unit-allure: ## Запустить unit-тесты и создать Allure-отчёт
	@command -v gotestsum >/dev/null || { echo 'gotestsum не найден: выполните make test-tools' >&2; exit 1; }
	@command -v allure >/dev/null || { echo 'Allure CLI не найден: установите Allure Report 2' >&2; exit 1; }
	@status=0; $(MAKE) test-unit || status=$$?; $(MAKE) allure-report ALLURE_RESULTS=unit || status=1; exit $$status
