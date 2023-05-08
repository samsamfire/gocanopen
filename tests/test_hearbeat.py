import pytest
import canopen
import time


def test_heartbeat_time_producer(node: canopen.RemoteNode):
    global counter
    counter = 0

    def hb_callback(*args):
        global counter
        counter += 1

    node.sdo["Producer heartbeat time"].raw = 100
    node.network.subscribe(node.id + 0x700, hb_callback)
    counter = 0
    time.sleep(1)
    assert 9 <= counter <= 11
    node.sdo["Producer heartbeat time"].raw = 500
    counter = 0
    time.sleep(1.0)
    assert 2 <= counter <= 3


def test_disable_heartbeat(node: canopen.RemoteNode):
    node.sdo["Producer heartbeat time"].raw = 0
