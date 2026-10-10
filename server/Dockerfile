FROM golang:1.26-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/sameframe ./cmd/sameframe

FROM node:24-bookworm-slim AS node
FROM python:3.13-slim-bookworm AS runtime
RUN apt-get update \
 && apt-get install -y --no-install-recommends ca-certificates libstdc++6 libatomic1 \
 && rm -rf /var/lib/apt/lists/* \
 && pip install --no-cache-dir 'yt-dlp[default]==2026.8.19' \
 && useradd --uid 65532 --create-home --user-group sameframe
COPY --from=node /usr/local/bin/node /usr/local/bin/node
COPY --from=build /out/sameframe /sameframe
ENV SAMEFRAME_YTDLP_PATH=/usr/local/bin/yt-dlp
USER 65532:65532
EXPOSE 8080
ENTRYPOINT ["/sameframe"]
