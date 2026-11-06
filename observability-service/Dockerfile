FROM golang:1.26.5-bookworm AS build
WORKDIR /src
COPY observability-service/main.go ./main.go
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /observability-service ./main.go
FROM scratch
COPY --from=build /observability-service /observability-service
USER 65532:65532
ENTRYPOINT ["/observability-service"]
