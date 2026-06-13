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
# hush depends on the private module github.com/ThirdCoastInteractive/Repute. To
# build it, pass GOPRIVATE and forward your SSH agent:
#   docker build --ssh default --build-arg APP=hush \
#     --build-arg GOPRIVATE=github.com/ThirdCoastInteractive -t hush .
# The other three examples build with no extra flags.

ARG GO_VERSION=1.26

FROM golang:${GO_VERSION} AS build
WORKDIR /src

# Private modules are fetched over SSH via a forwarded agent; public deps use the
# module proxy. These lines are inert unless GOPRIVATE is set and the build runs
# with `--ssh default`.
ARG GOPRIVATE=
ENV GOPRIVATE=${GOPRIVATE} \
    GIT_SSH_COMMAND="ssh -o StrictHostKeyChecking=accept-new"
RUN git config --global url."git@github.com:".insteadOf "https://github.com/"

COPY . .
ARG APP=pantry
# Only the target app's import graph is compiled, so the three public examples
# (boardwalk, driftbottle, pantry) pull no private modules and build with no SSH.
# hush's graph pulls the private Repute module, so build it with `--ssh default`
# + GOPRIVATE — the ssh mount below forwards the agent. The cache mounts keep the
# module + build caches warm across builds.
RUN --mount=type=ssh \
    --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/app ./examples/${APP}

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
