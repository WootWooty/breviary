# Dockerfile — multi-stage build for Breviary
# Usage: docker build -t breviary .
#        docker run --rm -v $PWD:/workspace breviary run /workspace/runbook.yaml

# Stage 1: Build
FROM golang:1.25-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/breviary ./cmd/breviary/

# Stage 2: Runtime
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata bash

WORKDIR /workspace
COPY --from=builder /out/breviary /usr/local/bin/breviary

# Healthcheck
EXPOSE 8080
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -qO- http://localhost:8080/api/v1/health || exit 1

ENTRYPOINT ["breviary"]
CMD ["--help"]