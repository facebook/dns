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

apt-get install libgflags-dev libsnappy-dev zlib1g-dev libbz2-dev libzstd-dev liblz4-dev
git clone https://github.com/facebook/rocksdb.git
cd rocksdb || exit
PREFIX=/usr make install-shared
