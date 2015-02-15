#!/bin/sh

set -e

mkdir -p ftpd
cd ftpd
curl -O http://download.pureftpd.org/pub/pure-ftpd/releases/pure-ftpd-1.0.36.tar.gz
tar -xzf pure-ftpd-1.0.36.tar.gz
cd pure-ftpd-1.0.36
./configure --with-nonroot
make
mv src/pure-ftpd ..
