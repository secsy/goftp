#!/bin/bash

set -e

mkdir -p ftpd
cd ftpd
curl -O http://download.pureftpd.org/pub/pure-ftpd/releases/pure-ftpd-1.0.36.tar.gz
tar -xzf pure-ftpd-1.0.36.tar.gz
cd pure-ftpd-1.0.36

# build normal binary with explicit tls support
./configure --with-nonroot --with-puredb --with-tls --with-certfile=pure-ftpd.pem
make clean
make
mv src/pure-ftpd ..

# build separate binary with implicit tls
./configure --with-nonroot --with-puredb --with-tls --with-certfile=pure-ftpd.pem --with-implicittls
make clean
make
mv src/pure-ftpd ../pure-ftpd-implicittls

cd ../..

# setup up a goftp user for ftp server
if [ "$(uname)" == "Darwin" ]; then
  echo "goftp:_.../HVM0l1lcNKVtiKs:`id -u`:`id -g`::`pwd`/testroot/./::::::::::::" > ftpd/users.txt
elif [ "$(expr substr $(uname -s) 1 5)" == "Linux" ]; then
  echo "goftp:\$1\$salt\$IbAl9EugC.V4mMOY6YMYE0:`id -u`:`id -g`::`pwd`/testroot/./::::::::::::" > ftpd/users.txt
elif [ "$(expr substr $(uname -s) 1 10)" == "MINGW32_NT" ]; then
  echo "Doesn't support windows yet"
  exit 1
fi

# generate puredb user db file
ftpd/pure-ftpd-1.0.36/src/pure-pw mkdb ftpd/users.pdb -f ftpd/users.txt
