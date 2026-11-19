# Наблюдаемость

Запустите стек командой `make up` и откройте
`http://localhost:8000/monitoring/`. Войдите в Grafana с учётными данными
`GRAFANA_ADMIN_USER` и `GRAFANA_ADMIN_PASSWORD` из `infra/.env`. Эти переменные
создают администратора при первоначальной инициализации Grafana; изменение
пароля в `.env` не сбрасывает пароль уже существующей учётной записи.

В разделе **Dashboards → Inference Platform** находится dashboard
**Inference Platform — сервисы**. Его JSON и provisioning-файлы находятся в
`infra/observability/grafana/` и загружаются Grafana автоматически. У каждого
микросервиса своя строка с графиками доступности и логами:

- **User Service** — HTTP-запросы в секунду и p95 времени ответа.
- **Model Service** — HTTP-запросы в секунду и p95 времени ответа.
- **Inference Service** — частота и p95 unary gRPC-запросов. Потоковый HTTP
  model proxy не входит в эти метрики.

В каждой строке dashboard показаны CPU процесса в процентах одного ядра,
резидентная память и число goroutine; серии разделены по `instance`, поэтому
экземпляры model-service видны отдельно. Это метрики процесса, а не суммарное
потребление всех процессов контейнера.

В каждой группе есть график количества ответов по статусам за последние 30
секунд с разбивкой по экземпляру (`instance`) и коду. У Inference Service
отдельные графики для gRPC и HTTP-прокси. Служебные health- и metrics-запросы
из соответствующих счётчиков исключены. Новые серии появляются
после запросов и как минимум двух опросов Prometheus.

Рядом с загрузкой CPU и памятью показаны фактические лимиты контейнеров из
Docker. Строка «Лимиты контейнеров — все сервисы» содержит также остальные
сервисы Compose. Inference Service экспортирует
`platform_container_cpu_limit_cores` и `platform_container_memory_limit_bytes`;
для неограниченных ресурсов соответствующая серия отсутствует, а
`platform_container_limit_configured` равна 0. Если Docker недоступен,
`platform_container_limits_scrape_success` равна 0.
По умолчанию опрашиваются Compose-проекты основной платформы, `stress-test` и
`inference-platform-tests`; список можно заменить переменной
`CONTAINER_METRICS_PROJECTS` в `inference-service/.env`, перечислив имена через
запятую. Значения потребления относятся к процессу сервиса, а предел — ко
всему контейнеру.

В строке Inference Service дополнительно показаны запущенные Docker-контейнеры
моделей по пользователю и проекту и таблица текущих deployment. Метрики
`inference_deployment_info` и `inference_deployment_container_running` содержат
идентификаторы deployment, модели и контейнера, их имена, пользователя и
проект; `inference_user_running_containers` — количество работающих контейнеров
по пользователю и проекту. Сборщик читает неперешедшие в конечное состояние
deployment из PostgreSQL и проверяет фактическое состояние в Docker. При
ошибке сбора `inference_deployment_metrics_scrape_success` равна 0, и данные
контейнеров за этот опрос не публикуются. Серии отсутствуют, когда подходящих
deployment нет. Метки содержат идентификаторы и доступны всем, у кого есть
доступ к метрикам Prometheus.

Для новых deployment в `username` сохраняется пользователь, инициировавший
запуск. В ранее созданных записях может оставаться автор модели.

Например, model-service экспортирует счётчик `app_http_requests_total` с
метками HTTP-метода и кода ответа и гистограмму
`app_http_request_duration_seconds`. Prometheus опрашивает `/metrics` каждого
сервиса раз в 10 секунд и добавляет метки `job` и `instance`. Адреса
настроены в `infra/observability/prometheus/prometheus.yaml`: по одному
адресу на сервис. `/metrics` доступен внутри
Docker-сети и не проксируется как публичный API.

Alloy читает Docker-логи и отправляет их в Loki с метками `service` и
`container`. На dashboard отображаются логи, которые сервисы действительно
записали: отсутствие логов в строке само по себе не означает неисправность.
Метрика Prometheus `up` показывает успешность опроса, а не полную готовность
сервиса и всех его зависимостей.

User Service, Model Service и Inference Service отправляют трассировки через
OTel Collector в Tempo. В трассировку входят входящие HTTP- и gRPC-запросы,
обращения к PostgreSQL, gRPC-вызовы inference-service, запросы за JWKS и
HTTP-запросы прокси к модели. Контекст передаётся между сервисами по W3C
Trace Context. SQL-текст и параметры запросов к базе в span не записываются.
В логах, записанных в контексте запроса, поля `trace_id` и `span_id` позволяют
найти соответствующую трассировку в Grafana Explore → Tempo; в Loki их можно
искать по значению `trace_id`.

В Compose экспорт включён через `OTEL_ENABLED` и
`OTEL_EXPORTER_OTLP_ENDPOINT`. Вне Compose он по умолчанию выключен; для
отправки трассировок задайте эти переменные. Все сервисы уважают решение о
выборке, полученное от родительского span. Без входящего `traceparent` первый
сервис создаёт новую трассировку; Nginx сам не создаёт span.

Grafana доступна через Nginx с префиксом `/monitoring/`. Если внешний адрес
или порт отличаются от `http://localhost:8000`, установите в `infra/.env`
`GRAFANA_ROOT_URL` на полный внешний URL, например
`http://localhost:9000/monitoring/`, и перезапустите Grafana. Прямой порт
Grafana ограничен localhost; открытый порт Nginx предоставляет экран входа
Grafana. Доступ для анонимных пользователей не включается этой конфигурацией.
