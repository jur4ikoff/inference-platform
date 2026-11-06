FROM golang:1.26.5-bookworm AS binaries
WORKDIR /src
COPY utils/go.mod utils/go.sum ./utils/
COPY user-service/go.mod user-service/go.sum ./user-service/
WORKDIR /src/user-service
RUN --mount=type=cache,target=/go/pkg/mod go mod download
WORKDIR /src
COPY utils ./utils
COPY user-service ./user-service
WORKDIR /src/user-service
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -ldflags='-s -w' -o /out/bootstrap-admin-linux-amd64 ./cmd/bootstrap-admin && \
    CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -ldflags='-s -w' -o /out/bootstrap-admin-linux-arm64 ./cmd/bootstrap-admin

FROM nginx:1.27-alpine
COPY --from=binaries /out/ /srv/downloads/
