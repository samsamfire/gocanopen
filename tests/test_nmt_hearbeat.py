import pytest
import canopen
from canopen import nmt
import time


@pytest.fixture
def prepared_node(node: canopen.RemoteNode) -> canopen.RemoteNode:
    node.nmt.state = "OPERATIONAL"
    node.sdo["Producer heartbeat time"].raw = 100
    yield node


def test_nmt_stop(prepared_node: canopen.RemoteNode):
    prepared_node.nmt.state = "STOPPED"
    time.sleep(0.2)  # Let the time for the node to actually stop
    state = prepared_node.nmt.wait_for_heartbeat(timeout=1.0)
    assert state == "STOPPED"
    # Check that we can't access node via SDO anymore
    with pytest.raises(canopen.SdoCommunicationError):
        _ = prepared_node.sdo["Producer heartbeat time"].raw


def test_nmt_pre_operational(prepared_node: canopen.RemoteNode):
    prepared_node.nmt.state = "PRE-OPERATIONAL"
    time.sleep(0.2)  # Let the time for the node to to be actually in preop
    state = prepared_node.nmt.wait_for_heartbeat(timeout=1.0)
    assert state == "PRE-OPERATIONAL"


# RESET and RESET COMMUNICATION are handled by user rather than the stack


def test_nmt_reset_comm(prepared_node: canopen.RemoteNode):
    prepared_node.nmt.state = "RESET COMMUNICATION"
    # give time for the application to restart
    for _ in range(3):
        state = prepared_node.nmt.wait_for_heartbeat(timeout=1.0)
    assert state == "OPERATIONAL"


def test_nmt_reset_node(prepared_node: canopen.RemoteNode):
    prepared_node.nmt.state = "RESET"
    for _ in range(3):
        state = prepared_node.nmt.wait_for_heartbeat(timeout=1.0)
    assert state == "OPERATIONAL"
    time.sleep(0.2)


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
    with pytest.raises(nmt.NmtError):
        node.nmt.wait_for_heartbeat(timeout=2)
