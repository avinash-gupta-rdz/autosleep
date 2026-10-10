# syntax=docker/dockerfile:1.6

# --- build stage ---
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Cache deps before copying source.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# Static build so we can use a minimal base image.
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /out/autosleep .

# --- runtime stage ---
FROM gcr.io/distroless/static-debian12:nonroot

WORKDIR /app
COPY --from=builder /out/autosleep /app/autosleep

USER nonroot:nonroot
EXPOSE 8080

ENTRYPOINT ["/app/autosleep"]
