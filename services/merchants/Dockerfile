# syntax=docker/dockerfile:1
# Build from the repository root so the go.work workspace resolves the
# pkg/ module replaces:
#
#   docker build -f services/merchants/Dockerfile -t example .
#
# Migrations are NOT baked in or run here — the deploy workflow applies them
# before the new revision receives traffic.

FROM golang:1.25 AS build
WORKDIR /src

COPY go.work ./
COPY pkg/ pkg/
COPY services/merchants/ services/merchants/

ENV CGO_ENABLED=0
RUN go build -trimpath -ldflags="-s -w" -o /bin/server ./services/merchants/cmd

# Distroless static: no shell, no libc — the binary is pure Go.
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /bin/server /server

# Cloud Run injects PORT; the server reads it and defaults to 8080 locally.
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/server"]
