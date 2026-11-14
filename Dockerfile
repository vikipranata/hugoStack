# 2-stage build: static Go binary in a minimal debian image with git.
FROM golang:1.27 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/hugostack ./cmd/server

FROM debian:trixie-slim
# git for the deploy workflow, ca-certificates for HTTPS remotes. No wget/curl: the image is a server, not a client.
RUN apt-get update \
    && apt-get install -y --no-install-recommends git ca-certificates \
    && rm -rf /var/lib/apt/lists/*
# The binary reads ui/templates + ui/static from its working directory.
COPY --from=build /out/hugostack /app/hugostack
COPY ui /app/ui
WORKDIR /app
# Bind-mounted repos are host-owned; the container runs as root, so git's
# "dubious ownership" guard would refuse every operation. safe.directory=*
# is the standard container workaround. It must live in the system config
# (not env): git only honors safe.directory from system/global config, and
# env-set values do not reach the upload-pack children spawned for
# local-path remotes.
RUN printf '[safe]\n\tdirectory = *\n' > /etc/gitconfig
ENTRYPOINT ["/app/hugostack"]
# "server" is accepted and ignored: the binary is a single-purpose server.
CMD ["server"]
