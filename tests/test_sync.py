import canopen
from canopen.pdo.base import Map
import time
import logging

import pytest

from .conftest import TEST_ID, EDS_PATH


logger = logging.getLogger(__name__)

DEFAULT_SYNC_ID = 0x80
DEFAULT_SYNC_COMM_PERIOD_MS = 1000_000


@pytest.fixture
def node_sync(network: canopen.Network) -> canopen.RemoteNode:
    if TEST_ID in network:
        del network[TEST_ID]
    node = network.add_node(TEST_ID, EDS_PATH)
    if DEFAULT_SYNC_ID in network.subscribers:
        network.unsubscribe(DEFAULT_SYNC_ID)
    # Disable the sync and default to 1 second time
    ENABLE_SYNC = 0x80 | (0 << 30)
    node.sdo["COB-ID SYNC message"].raw = ENABLE_SYNC
    node.sdo["Communication cycle period"].raw = DEFAULT_SYNC_COMM_PERIOD_MS
    yield node


def test_sync_communication_cycle_period(node_sync: canopen.RemoteNode):
    global counter
    counter = 0

    def sync_receiver(*args):
        global counter
        counter += 1

    PERIOD_MS = 1000 * 1000
    node_sync.sdo["Communication cycle period"].raw = PERIOD_MS
    assert node_sync.sdo["Communication cycle period"].raw == PERIOD_MS
    ENABLE_SYNC = 0x80 | (1 << 30)
    node_sync.sdo["COB-ID SYNC message"].raw = ENABLE_SYNC
    assert node_sync.sdo["COB-ID SYNC message"].raw == ENABLE_SYNC
    node_sync.network.subscribe(0x80, sync_receiver)
    time.sleep(1.2)
    assert counter == 1
    node_sync.sdo["Communication cycle period"].raw = PERIOD_MS / 10
    counter = 0
    time.sleep(1.0)
    assert 9 <= counter <= 11
    node_sync.sdo["Communication cycle period"].raw = PERIOD_MS / 100
    counter = 0
    time.sleep(1.0)
    assert 90 <= counter <= 110


def test_sync_change_cobid(node_sync: canopen.RemoteNode):
    global counter
    counter = 0

    def sync_receiver(*args):
        global counter
        counter += 1

    NEW_COB_ID = 0x91
    node_sync.sdo["COB-ID SYNC message"].raw = 1 << 30 | NEW_COB_ID

    node_sync.network.subscribe(NEW_COB_ID, sync_receiver)
    time.sleep(1.2)
    assert counter == 1


def test_sync_change_cobid_errors(node_sync: canopen.RemoteNode):
    with pytest.raises(canopen.SdoAbortedError, match="parameter exceeded"):
        node_sync.sdo["COB-ID SYNC message"].raw = 0x101


def test_synchronous_overflow(node_sync: canopen.RemoteNode):
    with pytest.raises(canopen.SdoAbortedError, match="Data can not be transferred"):
        _ = node_sync.sdo["Synchronous counter overflow value"].raw = 100

    # Can only be changed if cycle period is 0
    node_sync.sdo["Communication cycle period"].raw = 0
    SYNCHRONOUS_OVERFLOW = 100
    node_sync.sdo["Synchronous counter overflow value"].raw = SYNCHRONOUS_OVERFLOW
    assert node_sync.sdo["Synchronous counter overflow value"].raw == SYNCHRONOUS_OVERFLOW

    for WRONG_VALUE in [1, 245]:
        with pytest.raises(canopen.SdoAbortedError, match="parameter exceeded"):
            _ = node_sync.sdo["Synchronous counter overflow value"].raw = WRONG_VALUE


def test_sync_tpdo_start_value(node_sync: canopen.RemoteNode):
    global counter
    counter = 0

    def sync_receiver(*args):
        global counter
        counter += 1

    SYNC_PERIOD_MS = 10
    node_sync.pdo.read()
    tpdo: Map = node_sync.tpdo[1]
    tpdo.clear()
    tpdo.add_variable("UNSIGNED64 value")
    tpdo.cob_id = 0x211
    tpdo.trans_type = 1
    tpdo.enabled = True
    tpdo.save()
    # Add a callback to this tpdo
    tpdo.add_callback(sync_receiver)
    node_sync.sdo["COB-ID SYNC message"].raw = 1 << 30 | 0x80
    node_sync.sdo["Communication cycle period"].raw = 0
    node_sync.sdo["Synchronous counter overflow value"].raw = 200
    # Enable Sync with a period of 100ms
    node_sync.sdo["Communication cycle period"].raw = SYNC_PERIOD_MS * 1000
    counter = 0
    tpdo.sync_start_value = 100  # i.e start pdo emission  after 100 syncs
    tpdo.trans_type = 1  # Send every sync
    tpdo.save()
    time.sleep(1.0)
    assert 0 <= counter <= 10  # Should not have received anything yet
    time.sleep(1.0)
    assert 90 <= counter <= 110


def test_sync_tpdo(node_sync: canopen.RemoteNode):
    global counter
    counter = 0

    def sync_receiver(*args):
        global counter
        counter += 1

    SYNC_PERIOD_MS = 100
    node_sync.pdo.read()
    # Enable Sync with a period of 100ms
    node_sync.sdo["COB-ID SYNC message"].raw = 1 << 30 | 0x80
    node_sync.sdo["Communication cycle period"].raw = SYNC_PERIOD_MS * 1000
    tpdo: Map = node_sync.tpdo[1]
    tpdo.clear()
    tpdo.add_variable("UNSIGNED64 value")
    tpdo.cob_id = 0x211
    tpdo.trans_type = 1
    tpdo.enabled = True
    tpdo.save()
    # Add a callback to this tpdo
    tpdo.add_callback(sync_receiver)
    counter = 0
    time.sleep(1.0)
    assert 9 <= counter <= 11
    node_sync.sdo["Communication cycle period"].raw = SYNC_PERIOD_MS * 100
    counter = 0
    time.sleep(1.0)
    assert 90 <= counter <= 111
    # Update the transmission type
    tpdo.trans_type = 10
    tpdo.save()
    counter = 0
    time.sleep(1.0)
    assert 9 <= counter <= 11
    # Update the synchronous start
    node_sync.sdo[0x1019].raw = 1
    tpdo.sync_start_value = 200  # i.e
    tpdo.trans_type = 1
    tpdo.save()
    counter = 0
    time.sleep(1.0)
    assert 0 <= counter <= 10
    time.sleep(1.0)
    assert 90 <= counter <= 110
