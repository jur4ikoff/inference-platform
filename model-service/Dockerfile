FROM golang:1.26.5-bookworm AS build

WORKDIR /src/model-service

COPY internal-api /src/internal-api
COPY model-service/go.mod model-service/go.sum ./
RUN go mod download

COPY model-service/cmd ./cmd
COPY model-service/internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /model-service ./cmd

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /model-service /model-service

EXPOSE 8081
USER nonroot:nonroot
ENTRYPOINT ["/model-service"]
