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

FROM nginx:1.27-alpine AS headers-module
ARG HEADERS_MORE_VERSION=0.38
RUN apk add --no-cache build-base linux-headers pcre2-dev zlib-dev openssl-dev \
    && wget -q "https://nginx.org/download/nginx-${NGINX_VERSION}.tar.gz" -O /tmp/nginx.tar.gz \
    && wget -q "https://github.com/openresty/headers-more-nginx-module/archive/refs/tags/v${HEADERS_MORE_VERSION}.tar.gz" -O /tmp/headers-more.tar.gz \
    && tar -xzf /tmp/nginx.tar.gz -C /tmp \
    && tar -xzf /tmp/headers-more.tar.gz -C /tmp \
    && cd "/tmp/nginx-${NGINX_VERSION}" \
    && ./configure --with-compat --add-dynamic-module="/tmp/headers-more-nginx-module-${HEADERS_MORE_VERSION}" \
    && make modules

FROM nginx:1.27-alpine
COPY --from=headers-module /tmp/nginx-*/objs/ngx_http_headers_more_filter_module.so /usr/lib/nginx/modules/
COPY --from=binaries /out/ /srv/downloads/
