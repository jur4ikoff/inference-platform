FROM golang:1.26.5-bookworm AS build

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . ./
RUN CGO_ENABLED=0 go build -trimpath -ldflags='-s -w' -o /user-service ./cmd

FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /user-service /user-service

EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/user-service"]
