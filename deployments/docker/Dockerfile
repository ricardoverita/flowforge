FROM golang:1.27.1-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
ARG SERVICE=api
RUN --mount=type=cache,target=/go/pkg/mod,sharing=locked --mount=type=cache,target=/root/.cache/go-build,sharing=locked \
    GOMAXPROCS=2 CGO_ENABLED=0 go build -p 2 -trimpath -ldflags="-s -w" -o /out/flowforge ./cmd/${SERVICE}

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=build /out/flowforge /flowforge
COPY --from=build /src/migrations /app/migrations
USER 65532:65532
EXPOSE 8080 8081 8082
ENTRYPOINT ["/flowforge"]
