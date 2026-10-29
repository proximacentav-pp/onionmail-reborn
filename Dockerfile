FROM golang:1.22-bookworm AS build

WORKDIR /src
COPY go.mod ./
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/om ./cmd/om \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/om-front ./cmd/front \
 && CGO_ENABLED=0 go build -trimpath -ldflags "-s -w" -o /out/om-voucher ./cmd/voucher

FROM debian:bookworm-slim

ENV DEBIAN_FRONTEND=noninteractive
ENV LANG=C.UTF-8

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      ca-certificates gnupg wget curl openssl iproute2 openssh-server \
      tor obfs4proxy default-jre cpulimit wipe libcap2-bin dnsutils \
      net-tools procps util-linux tini \
 && rm -rf /var/lib/apt/lists/*

COPY onionmail_1.12.0-1_all.deb /tmp/onionmail_1.12.0-1_all.deb
COPY onionmail_1.12.0-1_all.deb.sha256 /tmp/onionmail_1.12.0-1_all.deb.sha256
RUN cd /tmp \
 && sha256sum -c onionmail_1.12.0-1_all.deb.sha256 \
 && dpkg -i onionmail_1.12.0-1_all.deb \
 && rm -f onionmail_1.12.0-1_all.deb onionmail_1.12.0-1_all.deb.sha256 \
 && rm -rf /var/lib/apt/lists/*

RUN mkdir -p /opt/seed \
 && cp -a /etc/onionmail /opt/seed/etc-onionmail \
 && rm -f /opt/seed/etc-onionmail/bootToken /opt/seed/etc-onionmail/.service_pass \
 && cp -a /var/lib/onionmail /opt/seed/var-lib-onionmail \
 && mkdir -p /opt/seed/var-onionmail \
 && cp -a /var/onionmail/. /opt/seed/var-onionmail/ \
 && chmod -R a=rX /opt/seed

COPY theme/ /usr/local/share/onionmail/theme/
COPY --from=build /out/om /usr/local/bin/om
COPY --from=build /out/om-front /usr/local/bin/om-front
COPY --from=build /out/om-voucher /usr/local/bin/om-voucher
COPY languages.json /usr/local/share/onionmail/languages.json
COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh /usr/local/bin/om /usr/local/bin/om-front /usr/local/bin/om-voucher

EXPOSE 25 110 80 18000 18001

LABEL org.opencontainers.image.title="OnionMail Reborn" \
      org.opencontainers.image.description="OnionMail 1.12 with webmail, admin panel and Tor hidden service" \
      org.opencontainers.image.version="1.12"

HEALTHCHECK --interval=30s --timeout=5s --start-period=180s --retries=5 \
  CMD curl -fsS http://127.0.0.1:18000/healthz || exit 1

ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/entrypoint.sh"]
