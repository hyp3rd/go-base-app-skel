# syntax=docker/dockerfile:1

################################################################################
# Create a stage for building the application.
ARG GO_VERSION=1.23.5
# FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS build
FROM --platform=$BUILDPLATFORM golang:${GO_VERSION}-alpine AS builder
WORKDIR /src

# This is the architecture you're building for, which is passed in by the builder.
# Placing it here allows the previous steps to be cached across architectures.
ARG TARGETARCH=amd64

ARG UID=10001

ENV VERSION=""
# Export the BACKEND environment variable to the final image
# cockroachdb | postgresql | patroni | mysql | mariadb | oracle | mssql | clickhouse | monngodb | etc.
ENV BACKEND=""

ENV PG_USER=""
ENV PG_PASSWORD=""

COPY ./configs  /src/configs
COPY ./data     /src/data

RUN touch app-api.log
# Download dependencies as a separate step to take advantage of Docker's caching.
# Leverage a cache mount to /go/pkg/mod/ to speed up subsequent builds.
# Leverage bind mounts to go.sum and go.mod to avoid having to copy them into
# the container.
RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=bind,source=./go.sum,target=go.sum \
    --mount=type=bind,source=./go.mod,target=go.mod \
    go mod download -x
# Create a non-privileged user that the app will run under.
# See https://docs.docker.com/go/dockerfile-user-best-practices/
RUN addgroup -S appgroup && \
    adduser \
    --disabled-password \
    --gecos "" \
    --home "/nonexistent" \
    --shell "/sbin/nologin" \
    --no-create-home \
    --uid "${UID}" \
    appuser -G appgroup  && \
    # prepare the bin folder to receive the build's binaries
    mkdir -p /bin
    # mount the cache
RUN --mount=type=cache,target=/go/pkg/mod/ \
    --mount=type=bind,source=./,target=.,rw \
    # build the app service
    CGO_ENABLED=0 GOARCH=$TARGETARCH go build \
    -ldflags="all=-s -w -X main.Version=$VERSION -X main.BuildTime=$(date +%FT%T%z) -X main.Backend=$BACKEND" \
    -tags=!healthcheck -trimpath -o /bin/app ./cmd/app/main.go && \
    CGO_ENABLED=0 GOARCH=$TARGETARCH go build \
    -ldflags="all=-s -w -X main.Version=$VERSION -X main.BuildTime=$(date +%FT%T%z) -X main.Backend=$BACKEND" \
    -trimpath -tags=healthcheck -o /bin/healthcheck ./cmd/healthcheck/main.go

FROM scratch AS app

ENV BACKEND=${BACKEND}

ENV PG_USER=${PG_USER}
ENV PG_PASSWORD=${PG_PASSWORD}

# Copy the non-root user and group files from the builder stage
COPY --from=builder /etc/passwd /etc/passwd
COPY --from=builder /etc/group /etc/group

# Copy SSL certificates from the builder image to the /etc/ssl/certs directory in the final image
COPY --from=builder /etc/ssl/certs /etc/ssl/certs

# Copy app-api.log file from the builder image to the /app directory in the final image
COPY --chown=appuser:appgroup --from=builder /src/app-api.log /app/app-api.log

# Copy app binary, healthcheck binary, and config directory from the builder image to the /app directory in the final image
COPY --from=builder /bin/app /app/app
COPY --from=builder /bin/healthcheck /app/healthcheck
COPY --from=builder /src/data/* /
COPY --from=builder /src/data/mock/* /
COPY --from=builder /src/configs/ /configs/

# Set the non-root user
USER appuser:appgroup

EXPOSE 9090 50051

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
    CMD ["/app/healthcheck"]

CMD ["/app/app"]
