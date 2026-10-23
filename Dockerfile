# syntax=docker/dockerfile:1.7
# Build multi-stage com cache do BuildKit:
#   - /go/pkg/mod       → módulos baixados (só refaz quando go.mod/go.sum mudam)
#   - /root/.cache/go-build → cache de compilação (rebuild incremental)
# Os caches ficam no builder, fora das camadas da imagem.

ARG GO_VERSION=1.26

FROM golang:${GO_VERSION}-alpine AS deps
WORKDIR /src
ENV CGO_ENABLED=0 GOFLAGS=-trimpath
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download && go mod verify

FROM deps AS build
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go build -ldflags="-s -w" -o /out/prazo ./cmd/prazo \
 && mkdir -p /out/data

# docker build --target test .
FROM build AS test
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go vet ./... && go test ./...

# Imagem final: sem shell, sem gerenciador de pacotes, usuário não-root (65532).
FROM gcr.io/distroless/static-debian12:nonroot AS final
COPY --from=build /out/prazo /usr/local/bin/prazo
COPY --from=build --chown=65532:65532 /out/data /data
USER 65532:65532
VOLUME ["/data"]
EXPOSE 8080
ENTRYPOINT ["/usr/local/bin/prazo"]
CMD ["serve"]
