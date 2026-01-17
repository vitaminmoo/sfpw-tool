#!/bin/bash -e
adb bugreport bugreport.zip
unzip bugreport.zip FS/data/misc/bluetooth/logs/btsnoop_hci.log
tshark -T fields -e frame.number -e _ws.col.def_src -e _ws.col.def_dst -e btatt.value -Y 'btatt.value!=""' -r FS/data/misc/bluetooth/logs/btsnoop_hci.log > packets.csv
