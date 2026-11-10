# The front end is built here and embedded by the Go build below; the image has
# no Node at runtime. The version is .node-version (make verify checks they agree).
FROM --platform=$BUILDPLATFORM node:24.22.0-alpine AS web
WORKDIR /src/web
COPY web/package.json web/package-lock.json web/.npmrc ./
RUN npm ci
COPY web/ ./
# The editor imports the document schema it shares with the Go bridge; its
# packages resolve from web/node_modules (vite dedupe), so only the file is needed.
COPY internal/docroom/interop/schema.mjs /src/internal/docroom/interop/schema.mjs
COPY docs/licenses/fonts /src/docs/licenses/fonts
# vite.config.ts writes to ../internal/web/dist.
RUN npm run build

# The embedded runtime (the libraries pages import) is rebuilt from its lockfile
# and must equal the files committed under internal/jsruntime/v1: the binary
# carries exactly what was reviewed, and the build fails when it would not.
FROM --platform=$BUILDPLATFORM node:24.22.0-alpine AS runtime
WORKDIR /src/runtime
COPY runtime/package.json runtime/package-lock.json runtime/.npmrc ./
RUN npm ci
COPY runtime/ ./
COPY internal/jsruntime/v1 /src/internal/jsruntime/v1
RUN npm run check

FROM --platform=$BUILDPLATFORM golang:1.27-alpine AS build
ARG TARGETOS TARGETARCH
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=web /src/internal/web/dist ./internal/web/dist
# Copied from the stage that proved it equal to a fresh build.
COPY --from=runtime /src/internal/jsruntime/v1 ./internal/jsruntime/v1
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags="-s -w" -o /codexartifacts ./cmd/codexartifacts

RUN mkdir /data

FROM gcr.io/distroless/static-debian12:nonroot
ARG VERSION=dev
ARG REVISION=unknown
LABEL org.opencontainers.image.source="https://github.com/iml885203/code-x-artifacts" \
      org.opencontainers.image.version="${VERSION}" \
      org.opencontainers.image.revision="${REVISION}" \
      org.opencontainers.image.licenses="MIT"
COPY LICENSE THIRD_PARTY_NOTICES.md /licenses/
COPY docs/licenses /licenses/third-party/
COPY --chown=65532:65532 --from=build /data /data
WORKDIR /data
COPY --from=build /codexartifacts /codexartifacts
HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 CMD ["/codexartifacts", "healthcheck"]
ENTRYPOINT ["/codexartifacts"]
