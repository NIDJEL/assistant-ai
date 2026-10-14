FROM node:24.19.0-bookworm-slim AS frontend
WORKDIR /workspace/web
ENV CI=true
RUN npm install --global pnpm@11.25.0
COPY web/package.json web/pnpm-lock.yaml web/pnpm-workspace.yaml ./
RUN pnpm install --frozen-lockfile
COPY web ./
RUN pnpm build

FROM golang:1.27.2-bookworm AS build
WORKDIR /workspace
COPY go.mod go.sum ./
RUN go mod download
COPY cmd ./cmd
COPY internal ./internal
COPY migrations ./migrations
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /workspace/server ./cmd/server \
    && mkdir -p /workspace/var

FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
ENV APP_ROOT=/app APP_ADDR=0.0.0.0:8000
COPY --from=build /workspace/server /app/server
COPY --chown=65532:65532 --from=build /workspace/var /app/var
COPY --from=frontend /workspace/web/dist /app/web/dist
COPY docs/diagrams ./docs/diagrams
COPY docs/p1-architecture.html ./docs/p1-architecture.html
COPY data ./data
USER 65532:65532
EXPOSE 8000
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 CMD ["/app/server", "-healthcheck"]
ENTRYPOINT ["/app/server"]
