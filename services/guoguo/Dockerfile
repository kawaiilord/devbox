FROM golang:1.26-bookworm AS build
WORKDIR /src
COPY go.mod main.go ./
COPY internal ./internal
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/guoguo .

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates ffmpeg tini curl \
 && rm -r /var/lib/apt/lists/* \
 && groupadd --gid 65532 guoguo && useradd --uid 65532 --gid guoguo --no-create-home guoguo \
 && mkdir -p /data /downloads && chown guoguo:guoguo /data /downloads
COPY --from=build /out/guoguo /usr/local/bin/guoguo
USER 65532:65532
WORKDIR /data
ENV SAMEFRAME_BRIDGE=1
EXPOSE 8999
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/guoguo"]
CMD ["-open=false", "-listen", "0.0.0.0:8999", "-data-dir", "/data", "-out", "/downloads", "-ffmpeg", "/usr/bin/ffmpeg"]
