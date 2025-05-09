#!/usr/bin/env python3
# Copyright (c) Meta Platforms, Inc. and affiliates.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

# Script to generate a bunch of subnets so we can populate the data file with
# a bunch of ip maps

import ipaddress


DFT_ID = 1
MAX_ID = 5


def print_networks(ipn):
    n = ipaddress.ip_network(ipn)
    cpt = 0
    while True:
        cpt += 1
        x = cpt % MAX_ID + 1
        print(f"%\\000\\00{x},{n},c\\000")
        print(f"%\\000\\00{x},{n},ec")
        n = list(n.subnets())
        if len(n) == 1:
            break
        n = n[1]


print("####### start generated ip maps ######")
print_networks("fd76::/16")
print_networks("10.0.0.0/8")
print("####### end generated ip maps ######")
