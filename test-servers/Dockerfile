# The FTP servers the test suite runs against.
#
# One image, three roles, chosen by which command the container runs:
# pure-ftpd with explicit TLS, pure-ftpd with implicit TLS, and proftpd.
#
# Why an image at all: these servers used to be compiled on whatever
# machine ran the tests, by a script that fetched 2015 tarballs over
# plain FTP and patched their C with perl — including a workaround for a
# segfault on macOS. That is not a test dependency, it is a second
# project. Here the versions are pinned, the build is the same everywhere
# and nothing lands on the developer's machine.
#
# pure-ftpd is still built from source, because implicit TLS is a
# compile-time option no distribution packages. It is built twice from
# the same tree for that reason and no other.

# ---------------------------------------------------------------------
FROM debian:bookworm-slim AS pure-ftpd-build

# Pinned deliberately. A test suite whose servers drift is a test suite
# whose failures cannot be attributed.
ARG PURE_FTPD_VERSION=1.0.54

RUN apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends \
        build-essential curl libssl-dev ca-certificates \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /src
# https, not the plain FTP the old script used: the tarball is the whole
# server, and fetching it over a channel nothing authenticates is a poor
# way to pin a version.
RUN curl -fsSLO "https://download.pureftpd.org/pub/pure-ftpd/releases/pure-ftpd-${PURE_FTPD_VERSION}.tar.gz" \
    && tar -xzf "pure-ftpd-${PURE_FTPD_VERSION}.tar.gz"

WORKDIR /src/pure-ftpd-${PURE_FTPD_VERSION}

# Explicit TLS — the ordinary build.
RUN ./configure --with-nonroot --with-puredb --with-tls --with-certfile=/etc/ssl/private/ftpd.pem >/dev/null \
    && make -j"$(nproc)" >/dev/null \
    && cp src/pure-ftpd /pure-ftpd \
    && cp src/pure-pw /pure-pw

# Implicit TLS — same source, one more configure flag, which is the only
# reason this is built from source rather than installed.
RUN make clean >/dev/null \
    && ./configure --with-nonroot --with-puredb --with-tls --with-implicittls --with-certfile=/etc/ssl/private/ftpd.pem >/dev/null \
    && make -j"$(nproc)" >/dev/null \
    && cp src/pure-ftpd /pure-ftpd-implicittls

# ---------------------------------------------------------------------
FROM debian:bookworm-slim

RUN apt-get update -qq && DEBIAN_FRONTEND=noninteractive apt-get install -y -qq --no-install-recommends \
        proftpd-basic proftpd-mod-crypto openssl ca-certificates \
    && rm -rf /var/lib/apt/lists/*

COPY --from=pure-ftpd-build /pure-ftpd /pure-ftpd-implicittls /pure-pw /usr/local/bin/

# The user the tests log in as. Its home is the shared testroot, which
# the compose file mounts.
ARG FTP_USER=goftp
ARG FTP_PASSWORD=rocks
ARG FTP_UID=2000

RUN groupadd -g ${FTP_UID} ${FTP_USER} \
    && useradd -u ${FTP_UID} -g ${FTP_UID} -d /testroot -s /usr/sbin/nologin ${FTP_USER} \
    && mkdir -p /testroot

# A fresh certificate at build time, rather than a key committed to the
# repository. The old script embedded one that expired in 2015 — the
# tests skip verification, so nobody noticed, which is exactly the
# problem with keeping it.
RUN mkdir -p /etc/ssl/private \
    && openssl req -x509 -newkey rsa:2048 -sha256 -days 3650 -nodes \
        -keyout /etc/ssl/private/ftpd.key -out /etc/ssl/private/ftpd.crt \
        -subj "/CN=localhost/O=goftp test servers" \
        -addext "subjectAltName=DNS:localhost,IP:127.0.0.1,IP:::1" 2>/dev/null \
    && cat /etc/ssl/private/ftpd.key /etc/ssl/private/ftpd.crt > /etc/ssl/private/ftpd.pem \
    && chmod 600 /etc/ssl/private/ftpd.* \
    && cp /etc/ssl/private/ftpd.crt /etc/ssl/private/ftpd.key /etc/proftpd/

# pure-ftpd authenticates from a puredb file, which has to be generated
# from a password nobody types interactively.
RUN printf '%s:%s:%s:%s::/testroot/./::::::::::::\n' \
        "${FTP_USER}" "$(openssl passwd -1 ${FTP_PASSWORD})" "${FTP_UID}" "${FTP_UID}" > /etc/pureftpd.passwd \
    && chmod 600 /etc/pureftpd.passwd \
    && /usr/local/bin/pure-pw mkdb /etc/pureftpd.pdb -f /etc/pureftpd.passwd

COPY proftpd.conf /etc/proftpd/proftpd.conf
RUN printf '%s:%s:%s:%s::/testroot:/bin/false\n' \
        "${FTP_USER}" "$(openssl passwd -1 ${FTP_PASSWORD})" "${FTP_UID}" "${FTP_UID}" > /etc/proftpd/users.txt \
    && chmod 600 /etc/proftpd/users.txt

EXPOSE 2121 2122 2124
