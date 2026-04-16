FROM golang:1.26.1 AS builder
ARG TARGETOS
ARG TARGETARCH
ARG ARKD_VERSION=master
WORKDIR /app
RUN git clone --branch ${ARKD_VERSION} --single-branch https://github.com/arkade-os/arkd.git
RUN cd arkd && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /app/bin/arkd ./cmd/arkd
RUN cd arkd/pkg/ark-cli && CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} go build -o /app/bin/ark main.go

FROM alpine:3.20
RUN apk update && apk upgrade
WORKDIR /app
COPY --from=builder /app/bin/* /app/
ENV PATH="/app:${PATH}"
ENV ARKD_DATADIR=/app/data
VOLUME /app/data
ENTRYPOINT [ "arkd" ]
