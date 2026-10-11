# Image produksi: binary statis (template & asset ter-embed) di atas distroless.
# Dev memakai Dockerfile.dev (hot reload), bukan file ini.

# ---------- build ----------
FROM golang:1.26-alpine AS build

ARG TAILWIND_VERSION=v4.1.14
# Diisi otomatis oleh BuildKit (amd64 / arm64).
ARG TARGETARCH

RUN apk add --no-cache curl libstdc++ libgcc \
 && case "$TARGETARCH" in \
      arm64) TW_ARCH=arm64 ;; \
      *)     TW_ARCH=x64 ;; \
    esac \
 && curl -fsSL -o /usr/local/bin/tailwindcss \
      "https://github.com/tailwindlabs/tailwindcss/releases/download/${TAILWIND_VERSION}/tailwindcss-linux-${TW_ARCH}-musl" \
 && chmod +x /usr/local/bin/tailwindcss

WORKDIR /src

# Layer dependency terpisah agar rebuild cepat bila hanya kode yang berubah.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN tailwindcss -i web/tailwind/input.css -o web/static/app.css --minify \
 && CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/rekapin ./cmd/rekapin

# ---------- runtime ----------
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/rekapin /rekapin

ENV ADDR=0.0.0.0:8080
EXPOSE 8080
USER nonroot:nonroot
ENTRYPOINT ["/rekapin"]
CMD ["serve"]
