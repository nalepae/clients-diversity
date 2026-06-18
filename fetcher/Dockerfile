# syntax=docker/dockerfile:1

# --- build the static cl-dist daemon binary ---
FROM golang:1.25-alpine AS build
WORKDIR /src
COPY go.mod ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -o /out/cl-dist .
# Empty data dir, staged so the runtime stage can own it as the nonroot user.
RUN mkdir -p /data

# --- distroless runtime ---
# static: no libc, no shell — just the static binary + CA certs (for HTTPS to
# the beacon node) and tzdata (for the UTC slot/date math).
FROM gcr.io/distroless/static-debian12:nonroot

COPY --from=build /out/cl-dist /usr/local/bin/cl-dist

# /data owned by the nonroot user (UID 65532). A fresh named volume mounted here
# inherits this ownership from the image, so the nonroot daemon can write the
# store (and its atomic temp file) without a root chown step.
COPY --from=build --chown=65532:65532 /data /data

# The binary is a daemon: it fetches on startup, then re-fetches at 01:00 UTC
# daily (only finalized days) — no cron, no scheduler. It writes the store to
# OUTPUT, which lives under the shared data volume. DATA_DIR is the mount point;
# override both OUTPUT and the volume mount to relocate the store on the host.
ENV OUTPUT=/data/data.json
VOLUME ["/data"]

ENTRYPOINT ["/usr/local/bin/cl-dist"]
