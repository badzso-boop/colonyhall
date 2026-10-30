# Build: docker build -f deploy/Dockerfile .
FROM node:22-alpine AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
COPY web/scripts ./scripts
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.27-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# The PWA build from the web stage, embedded by the web package.
COPY --from=web /web/dist ./web/dist
ARG VERSION=dev
RUN CGO_ENABLED=0 go build -trimpath \
      -ldflags "-s -w -X colonyhall/internal/server.Version=${VERSION}" \
      -o /out/colonyhall ./cmd/colonyhall
# The file store's mount point, owned by nonroot: Docker copies it into a new
# named volume, which would otherwise be root-owned and unwritable.
RUN mkdir -p /out/data/files

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/colonyhall /colonyhall
COPY --from=build --chown=65532:65532 /out/data /data
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/colonyhall"]
