# syntax=docker/dockerfile:1

FROM --platform=$BUILDPLATFORM golang:1.25-alpine AS build
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY . .
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH \
    go build -trimpath -ldflags "-s -w -X main.version=$VERSION" -o /out/DigitalSanDook ./cmd/digitalsandook
# empty data dir owned by the unprivileged user of the final image
RUN mkdir /data

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.title="DigitalSanDook" \
      org.opencontainers.image.description="Single-binary, secure, multi-user remote file manager" \
      org.opencontainers.image.source="https://github.com/TinToSer/DigitalSandook" \
      org.opencontainers.image.licenses="MIT"
COPY --from=build /out/DigitalSanDook /DigitalSanDook
COPY --from=build --chown=65532:65532 /data /data
VOLUME /data
EXPOSE 8443
USER 65532:65532
ENTRYPOINT ["/DigitalSanDook"]
CMD ["-data", "/data"]
