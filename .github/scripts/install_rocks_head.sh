#!/bin/bash
echo """
deb http://archive.ubuntu.com/ubuntu/ lunar main restricted
deb http://archive.ubuntu.com/ubuntu/ lunar-updates main restricted
deb http://archive.ubuntu.com/ubuntu/ lunar universe
deb http://archive.ubuntu.com/ubuntu/ lunar-updates universe
deb http://archive.ubuntu.com/ubuntu/ lunar multiverse
deb http://archive.ubuntu.com/ubuntu/ lunar-updates multiverse
""" > /etc/apt/sources.list.d/lunar.list

apt-get update -qq

apt-get install libgflags-dev libsnappy-dev zlib1g-dev libbz2-dev libzstd-dev liblz4-dev
git clone https://github.com/facebook/rocksdb.git
cd rocksdb || exit
PREFIX=/usr make install-shared
