#! /bin/bash

for i in `seq 1 2`;
do
	sudo ip netns del ns${i}	# remove ns if already existed
	sudo ip netns add ns${i}
	sudo ip link add veth${i}_ type veth peer name veth${i}
	sudo ip link set veth${i}_ netns ns${i}
	sudo ip netns exec ns${i} ip link set dev veth${i}_ up
	sudo ip link set dev veth${i} up
	sudo ip netns exec ns${i} ifconfig veth${i}_ 10.0.0.${i}/24
done