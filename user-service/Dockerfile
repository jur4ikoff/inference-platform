FROM golang:1.26.5-bookworm AS build

WORKDIR /src

COPY utils/go.mod utils/go.sum ./utils/
COPY user-service/go.mod user-service/go.sum ./user-service/
WORKDIR /src/user-service
RUN go mod download

WORKDIR /src
COPY utils ./utils
COPY user-service ./user-service
WORKDIR /src/user-service
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /user-service ./cmd && \
    CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /bootstrap-admin ./cmd/bootstrap-admin

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /user-service /user-service
COPY --from=build /bootstrap-admin /bootstrap-admin

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/user-service"]
