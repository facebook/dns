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

apt-get install -qq librocksdb7.3
apt-get install -qq librocksdb-dev
