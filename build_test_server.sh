#!/bin/sh

set -e

mkdir -p ftpd
cd ftpd
curl -O http://download.pureftpd.org/pub/pure-ftpd/releases/pure-ftpd-1.0.36.tar.gz
tar -xzf pure-ftpd-1.0.36.tar.gz
cd pure-ftpd-1.0.36
./configure --with-nonroot --with-puredb --with-tls --with-certfile=pure-ftpd.pem
make clean
make
mv src/pure-ftpd ..

# setup up a goftp user for ftp server
cd ../..
echo "goftp:_.../HVM0l1lcNKVtiKs:`id -u`:`id -g`::`pwd`/testroot::::::::::::" > ftpd/users.txt
ftpd/pure-ftpd-1.0.36/src/pure-pw mkdb ftpd/users.pdb -f ftpd/users.txt
