# syntax=docker/dockerfile:1
# Build from the repository root:
#
#   docker build -f services/admin/Dockerfile -t admin .
#
# Stage 1 builds the SPA; stage 2 embeds it in the Go binary (replacing the
# committed placeholder in internal/web/dist); stage 3 is distroless.

FROM node:22-alpine AS webbuild
WORKDIR /web
COPY services/admin/web/package.json services/admin/web/package-lock.json ./
RUN npm ci
COPY services/admin/web/ ./
RUN npm run build

FROM golang:1.25 AS build
WORKDIR /src
COPY services/admin/ services/admin/
COPY --from=webbuild /web/dist/ services/admin/internal/web/dist/
WORKDIR /src/services/admin
ENV CGO_ENABLED=0 GOWORK=off
RUN go build -trimpath -ldflags="-s -w" -o /bin/server ./cmd

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /bin/server /server
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/server"]
