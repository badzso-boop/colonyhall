# Build: docker build -f deploy/Dockerfile .
# Base images are pinned by digest (round 40); Dependabot moves the digest
# and the tag together.
FROM node:22-alpine@sha256:1314fb30ec37592d5845e5a160d522c0e680972aa534549940b8d73bde88481a AS web
WORKDIR /web
COPY web/package.json web/package-lock.json ./
COPY web/scripts ./scripts
RUN npm ci --no-audit --no-fund
COPY web/ ./
RUN npm run build

FROM golang:1.27-alpine@sha256:eb61c3873c97416fff0116553fff65d67a3549f72e11d9758849a689cdd8b523 AS build
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

FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/colonyhall /colonyhall
COPY --from=build --chown=65532:65532 /out/data /data
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/colonyhall"]
