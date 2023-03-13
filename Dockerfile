# syntax = docker/dockerfile:1.3
FROM golang:1.20
WORKDIR /src
COPY . .
RUN --mount=type=cache,id=gobuild,target=/root/.cache/go-build \
    make DIST_PATH=/bin

FROM gcr.io/distroless/base
COPY --from=0 /bin/httpbingo /bin/httpbingo
CMD ["/bin/httpbingo"]
