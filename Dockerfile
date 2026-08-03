FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates && \
    rm -rf /var/lib/apt/lists/*

COPY bin/arb-core /usr/local/bin/arb-core
COPY config/default.textproto /etc/arb/config.textproto

EXPOSE 50051

ENTRYPOINT ["arb-core", "-config=/etc/arb/config.textproto"]
