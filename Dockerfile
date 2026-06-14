# syntax=docker/dockerfile:1
#
# Multi-stage build for a the-beach example app. The static assets the app serves
# (the generated app.css, the Datastar client, pantry's item images) are embedded
# in the binary via go:embed, so the runtime image only carries the binary — no
# CSS build step, no asset volume. The binary is fully static (pgx, clickhouse-go
# and argon2 are pure Go), so it runs on distroless/static.
#
# Pick the example with the APP build arg: boardwalk | driftbottle | pantry | hush.
#
# Dependencies are vendored (vendor/, tracked in git), so the build needs no
# network, no module proxy, and no SSH — even hush's private Repute module is in
# the tree. All four examples build identically with no extra flags:
#   docker build --build-arg APP=hush -t hush .

ARG GO_VERSION=1.26

FROM golang:${GO_VERSION} AS build
WORKDIR /src

COPY . .
ARG APP=pantry
# -mod=vendor (the default when vendor/ is present) builds straight from the
# vendored tree — no go mod download, no fetches. The build cache mount keeps
# compiles warm across builds.
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/app ./examples/${APP}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
