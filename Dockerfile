# Standalone Prometheus-Mimir signal collector image: a static pure-Go binary,
# built reproducibly with -trimpath, on a base with no shell and no package
# manager. With no arguments (or --sdk-stdio) the binary serves one claim: it
# reads one collector-sdk/v1alpha1 request from stdin and writes one result to
# stdout.
#
#   docker run -i IMAGE < request.json    serves one claim from stdin
FROM golang:1.26.10-alpine@sha256:a2012b4a1d1f9f7e9f44e0d39b805a901e856b1dd80b4e6ee4338db547050308 AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags='-s -w -buildid=' -o /out/collector ./cmd/collector

# Base digests resolved 2026-09-17; re-resolve them when bumping either base.
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab
COPY --from=build /out/collector /usr/local/bin/collector
USER 65532:65532
ENTRYPOINT ["/usr/local/bin/collector"]
CMD ["--sdk-stdio"]
