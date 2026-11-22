FROM --platform=$BUILDPLATFORM docker.io/library/golang:1.27.2-bookworm@sha256:5cf287a799e6b94384bad13d16b14904c531f51ba65792237e122ce42b392f61 AS build
WORKDIR /src/services/processing
ENV GOTOOLCHAIN=local GOWORK=off
COPY services/processing/go.mod services/processing/go.sum ./
COPY services/platform/go.mod ../platform/go.mod
RUN go mod download && go mod verify
COPY services/platform ../platform
COPY services/processing/cmd/processing ./cmd/processing
COPY services/processing/internal ./internal

ARG TARGETOS
ARG TARGETARCH
ARG SOURCE_SHA
RUN test "${#SOURCE_SHA}" -eq 40 && \
    case "$SOURCE_SHA" in *[!0-9a-f]*) exit 1;; esac && \
    CGO_ENABLED=0 GOOS="$TARGETOS" GOARCH="$TARGETARCH" go build -trimpath -buildvcs=false \
        -ldflags "-X main.revision=$SOURCE_SHA" -o /out/processing ./cmd/processing && \
    touch -d '@0' /out/processing

FROM scratch
ARG SOURCE_SHA
LABEL org.opencontainers.image.title="HitPipe Processing" \
      org.opencontainers.image.source="https://github.com/hitpipe/hitpipe" \
      org.opencontainers.image.revision="$SOURCE_SHA"
COPY --from=build /out/processing /processing
USER 10001:10001
EXPOSE 8443 9443 9444
ENTRYPOINT ["/processing"]
CMD ["-config", "/etc/processing/processing.json"]
