# syntax=docker/dockerfile:1
# Build from the repository root so the pkg/ module replaces resolve:
#
#   docker build -f services/storefront/Dockerfile -t example .
#
# GOWORK=off: the image contains only this service plus pkg/, not the whole
# workspace, so the build must rely on the module's own replace directives.
#
# Migrations are NOT baked in or run here — the deploy workflow applies them
# before the new revision receives traffic.

FROM golang:1.25 AS build
WORKDIR /src

COPY pkg/ pkg/
COPY services/storefront/ services/storefront/

WORKDIR /src/services/storefront
ENV CGO_ENABLED=0 GOWORK=off
RUN go build -trimpath -ldflags="-s -w" -o /bin/server ./cmd

# Distroless static: no shell, no libc — the binary is pure Go.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /bin/server /server

# Cloud Run injects PORT; the server reads it and defaults to 8080 locally.
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/server"]
