# syntax=docker/dockerfile:1
# goldgirl runtime. Built with context = the-beach repo root
# (catalog deploy.build.dockerfile = "cmd/hush/Dockerfile").
ARG GO_VERSION=1.26

FROM golang:${GO_VERSION} AS build
WORKDIR /src
COPY . .
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -mod=vendor -trimpath -ldflags="-s -w" -o /out/app ./cmd/hush

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/app /app
EXPOSE 8080
ENTRYPOINT ["/app"]
