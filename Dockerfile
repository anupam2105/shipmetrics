# syntax=docker/dockerfile:1

# ---- build stage ----
FROM golang:1.26.5-alpine AS build

WORKDIR /src

# Fetch modules separately so the layer caches when only source changes.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Statically link so the distroless base image (no libc) can execute us.
ARG BUILD_VERSION=dev
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags "-s -w -X main.buildVersion=${BUILD_VERSION}" \
    -o /out/shipmetrics ./cmd/shipmetrics

# ---- runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/shipmetrics /usr/local/bin/shipmetrics

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/usr/local/bin/shipmetrics"]
