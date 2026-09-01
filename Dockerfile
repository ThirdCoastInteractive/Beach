# syntax=docker/dockerfile:1
#
# Multi-stage build for a Beach example app. The static assets the app serves
# (the generated app.css, the Datastar client, pantry's item images) are embedded
# in the binary via go:embed, so the runtime image only carries the binary — no
# CSS build step, no asset volume. The binary is fully static (pgx, clickhouse-go
# and argon2 are pure Go), so it runs on distroless/static.
#
# Pick the app with the APP build arg — the package path to build, e.g.
# cmd/examples/boardwalk | cmd/examples/driftbottle | cmd/examples/pantry.
#
# Dependencies are vendored (vendor/, tracked in git), so the build needs no
# network, no module proxy, and no SSH:
#   docker build --build-arg APP=cmd/examples/pantry -t pantry .

ARG GO_VERSION=1.26

FROM golang:${GO_VERSION} AS build
WORKDIR /src

COPY . .
ARG APP=cmd/examples/pantry
# -mod=vendor (the default when vendor/ is present) builds straight from the
# vendored tree — no go mod download, no fetches. The build cache mount keeps
# compiles warm across builds.
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/app ./${APP}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
