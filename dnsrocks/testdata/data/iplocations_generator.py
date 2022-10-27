#!/usr/bin/env python3
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
        print("%\\000\\00{},{},c\\000".format(x, n))
        print("%\\000\\00{},{},ec".format(x, n))
        n = list(n.subnets())
        if len(n) == 1:
            break
        n = n[1]


print("####### start generated ip maps ######")
print_networks("fd76::/16")
print_networks("10.0.0.0/8")
print("####### end generated ip maps ######")
