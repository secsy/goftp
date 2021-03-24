#!/bin/bash -ex

if [ "$(uname)" == "Darwin" ]; then
  cflags=-I/usr/local/opt/openssl/include
  ldflags=-L/usr/local/opt/openssl/lib
fi

goftp_dir=`pwd`

mkdir -p ftpd
cd ftpd

ftpd_dir=`pwd`

proftp_version='1.3.7a'

test -f proftpd-${proftp_version}.tar.gz || curl -O ftp://ftp.proftpd.org/distrib/source/proftpd-${proftp_version}.tar.gz
tar -xzf proftpd-${proftp_version}.tar.gz
cd proftpd-${proftp_version}

CFLAGS=$cflags LDFLAGS=$ldflags ./configure --with-modules=mod_tls --disable-ident

make
mv proftpd ..

cd ..

cat > proftpd.conf <<CONF
ServerType standalone
Port 2124
DefaultAddress 127.0.0.1
AuthUserFile $ftpd_dir/users.txt
AuthOrder mod_auth_file.c
PidFile /dev/null
ScoreboardFile /dev/null
User `id -u -n`
Group `id -g -n`
AllowStoreRestart on
AllowOverwrite on
UseReverseDNS off
RequireValidShell off
TLSOptions NoSessionReuseRequired
<IfModule mod_tls.c>
  TLSEngine on
  TLSRSACertificateFile $ftpd_dir/server.cert
  TLSRSACertificateKeyFile $ftpd_dir/server.key
</IfModule>
CONF

# generate a key and certificate
openssl req -x509 -nodes -newkey rsa:4096 -keyout server.key -out server.cert -days 365 -subj '/C=US/ST=California/O=GoFTP'
cat server.key server.cert > pure-ftpd.pem

pure_ftpd_version='1.0.49'

test -f pure-ftpd-${pure_ftpd_version}.tar.gz || curl -O https://download.pureftpd.org/pub/pure-ftpd/releases/pure-ftpd-${pure_ftpd_version}.tar.gz
tar -xzf pure-ftpd-${pure_ftpd_version}.tar.gz
cd pure-ftpd-${pure_ftpd_version}

# build normal binary with explicit tls support
CFLAGS=$cflags LDFLAGS=$ldflags ./configure --with-nonroot --with-puredb --with-tls --with-certfile=$ftpd_dir/pure-ftpd.pem
make clean
make
mv src/pure-ftpd ..

# build separate binary with implicit tls
CFLAGS=$cflags LDFLAGS=$ldflags ./configure --with-nonroot --with-puredb --with-tls --with-certfile=$ftpd_dir/pure-ftpd.pem --with-implicittls
make clean
make
mv src/pure-ftpd ../pure-ftpd-implicittls

cd ..

# setup up a goftp user for ftp server
if [ "$(uname)" == "Darwin" ]; then
  echo "goftp:_.../HVM0l1lcNKVtiKs:`id -u`:`id -g`::$goftp_dir/testroot/./::::::::::::" > users.txt
elif [ "$(expr substr $(uname -s) 1 5)" == "Linux" ]; then
  echo "goftp:\$1\$salt\$IbAl9EugC.V4mMOY6YMYE0:`id -u`:`id -g`::$goftp_dir/testroot/./::::::::::::" > users.txt
elif [ "$(expr substr $(uname -s) 1 10)" == "MINGW32_NT" ]; then
  echo "Doesn't support windows yet"
  exit 1
fi

chmod 600 users.txt

# generate puredb user db file
pure-ftpd-${pure_ftpd_version}/src/pure-pw mkdb users.pdb -f users.txt
