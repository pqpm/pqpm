# E2E test image for pqpm
FROM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN make build

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates curl procps gosu coreutils \
    && rm -rf /var/lib/apt/lists/*

COPY --from=build /src/bin/pqpmd /usr/local/bin/pqpmd
COPY --from=build /src/bin/pqpm /usr/local/bin/pqpm
COPY test/e2e/entrypoint.sh /entrypoint.sh
COPY test/e2e/run-tests.sh /run-tests.sh
RUN chmod +x /entrypoint.sh /run-tests.sh \
    && mkdir -p /var/run/pqpm /var/log/pqpm/users /var/lib/pqpm \
    && useradd -m -u 1042 -s /bin/bash foliyo

ENTRYPOINT ["/entrypoint.sh"]
CMD ["/run-tests.sh"]
