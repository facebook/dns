#!/bin/bash
echo """
deb http://archive.ubuntu.com/ubuntu/ kinetic main restricted
deb http://archive.ubuntu.com/ubuntu/ kinetic-updates main restricted
deb http://archive.ubuntu.com/ubuntu/ kinetic universe
deb http://archive.ubuntu.com/ubuntu/ kinetic-updates universe
deb http://archive.ubuntu.com/ubuntu/ kinetic multiverse
deb http://archive.ubuntu.com/ubuntu/ kinetic-updates multiverse
""" > /etc/apt/sources.list.d/kinetic.list

apt-get update -qq

apt-get install libgflags-dev
apt-get install libsnappy-dev
apt-get install zlib1g-dev
apt-get install libbz2-dev
apt-get install libzstd-dev
apt-get install liblz4-dev
cd ../../rocksdb
make static_lib