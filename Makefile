.DEFAULT_GOAL := help

COMPOSE_PROJECT := inference-platform
COMPOSE := docker compose -p $(COMPOSE_PROJECT) --project-directory . --env-file infra/.env -f infra/docker-compose.yaml
USER_SERVICE := user-service
MODEL_SERVICE := model-service
INFERENCE_SERVICE := inference-service
TRAVEL_SERVICE := travel-service
DATA_COLLECTOR_SERVICE := data-collector-service
INFRA_SERVICES := loki tempo otel-collector alloy grafana
APPLICATION_SERVICES := $(USER_SERVICE) $(MODEL_SERVICE) $(INFERENCE_SERVICE)
BUF := docker run --rm -v "$(CURDIR)/internal-api:/workspace" -w /workspace bufbuild/buf:1.61.0
SQLC := docker run --rm -v "$(CURDIR):/src" -w /src sqlc/sqlc:1.31.1
PREK ?= prek

.PHONY: help up up-infra up-database up-services up-user up-model up-inference up-travel up-data-collector proto sql-generate sql-check build down restart restart-user restart-model restart-inference restart-travel restart-data-collector ps logs logs-user logs-model logs-inference logs-travel logs-data-collector logs-postgres logs-observability config test lint fmt

help: ## Показать доступные команды
	@printf '%s\n' \
		'Использование: make <цель>' \
		'' \
		'  up                  Поднять инфраструктуру, БД с миграциями и сервисы' \
		'  up-infra            Поднять инфраструктуру наблюдаемости' \
		'  up-database         Поднять PostgreSQL и применить миграции' \
		'  up-services         Поднять БД с миграциями и прикладные сервисы' \
		'  up-user             Поднять user-service и его зависимости' \
		'  up-model            Поднять model-service и его зависимости' \
		'  up-inference        Поднять inference-service и его зависимости' \
		'  up-travel           Поднять открытый travel-service' \
		'  up-data-collector   Поднять внутренний gRPC data-collector-service' \
		'  proto               Сгенерировать protobuf- и gRPC-стабы internal-api' \
		'  sql-generate        Сгенерировать Go-обёртки SQL-запросов через sqlc' \
		'  sql-check           Проверить SQL-запросы и схему для sqlc' \
		'  build               Пересобрать образы без запуска контейнеров' \
		'  down                Остановить и удалить контейнеры и сеть (volumes сохраняются)' \
		'  restart             Перезапустить весь стек' \
		'  restart-user        Пересобрать и перезапустить user-service' \
		'  restart-model       Пересобрать и перезапустить model-service' \
		'  restart-inference   Пересобрать и перезапустить inference-service' \
		'  restart-travel      Пересобрать и перезапустить travel-service' \
		'  restart-data-collector Пересобрать и перезапустить data-collector-service' \
		'  ps                  Показать состояние контейнеров' \
		'  logs                Смотреть логи всего стека' \
		'  logs-user           Смотреть логи user-service' \
		'  logs-model          Смотреть логи model-service' \
		'  logs-inference      Смотреть логи inference-service' \
		'  logs-travel         Смотреть логи travel-service' \
		'  logs-data-collector Смотреть логи data-collector-service' \
		'  logs-postgres       Смотреть логи PostgreSQL' \
		'  logs-observability  Смотреть логи Grafana, Loki, Tempo и OTel Collector' \
		'  config              Проверить итоговую Docker Compose-конфигурацию' \
		'  test                Запустить Go-тесты сервисов' \
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

up-user: ## Поднять user-service и его зависимости
	$(COMPOSE) up -d --build $(USER_SERVICE)

up-model: ## Поднять model-service и его зависимости
	$(COMPOSE) up -d --build $(MODEL_SERVICE)

up-inference: up-database ## Поднять БД с миграциями и inference-service
	$(COMPOSE) up -d --build $(INFERENCE_SERVICE)

up-travel: ## Поднять открытый travel-service
	$(COMPOSE) up -d --build $(TRAVEL_SERVICE)

up-data-collector: ## Поднять внутренний gRPC data-collector-service
	$(COMPOSE) up -d --build $(DATA_COLLECTOR_SERVICE)

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

restart-travel: ## Пересобрать и перезапустить travel-service
	$(COMPOSE) up -d --build --force-recreate $(TRAVEL_SERVICE)

restart-data-collector: ## Пересобрать и перезапустить data-collector-service
	$(COMPOSE) up -d --build --force-recreate $(DATA_COLLECTOR_SERVICE)

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

logs-travel: ## Смотреть логи travel-service
	$(COMPOSE) logs --follow --tail=100 $(TRAVEL_SERVICE)

logs-data-collector: ## Смотреть логи data-collector-service
	$(COMPOSE) logs --follow --tail=100 $(DATA_COLLECTOR_SERVICE)

logs-postgres: ## Смотреть логи PostgreSQL
	$(COMPOSE) logs --follow --tail=100 postgres

logs-observability: ## Смотреть логи observability-стека
	$(COMPOSE) logs --follow --tail=100 grafana loki tempo otel-collector alloy

config: ## Проверить итоговую Compose-конфигурацию
	$(COMPOSE) config --quiet

test: ## Запустить Go-тесты сервисов
	cd $(USER_SERVICE) && go test ./...
	cd $(MODEL_SERVICE) && go test ./...

lint: ## Запустить проверки prek для всех отслеживаемых файлов
	$(PREK) validate-config prek.toml
	$(PREK) run --stage manual --all-files

format: ## Отформатировать Go-код сервисов
	cd $(USER_SERVICE) && gofmt -w $$(find . -name '*.go' -type f)
	cd $(MODEL_SERVICE) && gofmt -w $$(find . -name '*.go' -type f)
