FROM docker:28-cli AS docker-cli
FROM golang:1.26.5-bookworm
RUN apt-get update && apt-get install -y --no-install-recommends \
    curl python3 postgresql-client openssl default-jre-headless unzip \
    && rm -rf /var/lib/apt/lists/*
COPY --from=docker-cli /usr/local/bin/docker /usr/local/bin/docker
ARG ALLURE_VERSION=2.46.2
RUN curl --fail --location --retry 3 \
    "https://github.com/allure-framework/allure2/releases/download/${ALLURE_VERSION}/allure-${ALLURE_VERSION}.tgz" \
    | tar xz -C /opt && ln -s "/opt/allure-${ALLURE_VERSION}/bin/allure" /usr/local/bin/allure
WORKDIR /src
COPY utils ./utils
COPY internal-api ./internal-api
COPY user-service ./user-service
COPY model-service ./model-service
COPY inference-service ./inference-service
COPY infra/testing ./infra/testing
COPY infra/postgres/migrations ./infra/postgres/migrations
ARG LAB2_SOURCE_REVISION=unknown
# Run tests and build bootstrap-admin from a Git snapshot of the current worktree.
RUN git init /src && git -C /src add . && \
    git -C /src -c user.name=Lab2 -c user.email=lab2@example.test commit -m "Source ${LAB2_SOURCE_REVISION} with workspace overlay" && \
    git clone --no-local /src /checked-source && rm -rf /src && mv /checked-source /src && \
    git -C /src rev-parse HEAD > /usr/local/share/lab2-snapshot-revision
RUN cd user-service && go mod download && CGO_ENABLED=0 go build -o /usr/local/bin/bootstrap-admin ./cmd/bootstrap-admin
RUN cd model-service && go mod download
RUN cd inference-service && go mod download
ENV OTEL_ENABLED=false DATABASE_URL=postgres://test:test@postgres:5432/lab2?sslmode=disable
ENTRYPOINT ["python3", "-u", "/src/infra/testing/container_runner.py"]
