FROM golang:1.26.5-bookworm AS build

WORKDIR /src/inference-service

COPY internal-api /src/internal-api
COPY utils /src/utils
COPY inference-service/go.mod inference-service/go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    for attempt in 1 2 3; do \
        go mod download && exit 0; \
        if [ "$attempt" -eq 3 ]; then exit 1; fi; \
        echo "Dependency download failed; retrying in 5 seconds ($attempt/3)" >&2; \
        sleep 5; \
    done

COPY inference-service/cmd ./cmd
COPY inference-service/internal ./internal

RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -o /inference-service ./cmd

FROM docker:28-cli

COPY --from=build /inference-service /inference-service

EXPOSE 8080 9091
ENTRYPOINT ["/inference-service"]
