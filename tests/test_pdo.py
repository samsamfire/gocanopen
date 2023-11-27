import logging

import pytest
import canopen
from canopen.pdo.base import Map

logger = logging.getLogger(__name__)


@pytest.fixture(scope="module")
def node_configured(node: canopen.RemoteNode) -> canopen.RemoteNode:
    node.pdo.read()
    yield node


@pytest.fixture()
def first_tpdo(node_configured) -> Map:
    return node_configured.tpdo[1]


@pytest.fixture()
def first_rpdo(node_configured) -> Map:
    return node_configured.rpdo[1]


def test_disable_pdo(first_rpdo: Map, first_tpdo: Map):
    # Configuring a pdo that is enabled should throw an exception
    first_tpdo.clear()
    first_tpdo.enabled = False
    first_tpdo.save()
    assert not first_tpdo.enabled
    first_rpdo.enabled = False
    first_tpdo.clear()
    first_rpdo.save()
    assert not first_rpdo.enabled


VARIABLES_TO_MAP = [
    "UNSIGNED8",
    "UNSIGNED16",
    "UNSIGNED32",
    "UNSIGNED64",
    "INTEGER8",
    "INTEGER16",
    "INTEGER32",
    "INTEGER64",
    "REAL32",
    "REAL64",
]


def test_map_tpdo(first_tpdo: Map):
    for variable in VARIABLES_TO_MAP:
        first_tpdo.clear()
        first_tpdo.add_variable(f"{variable} value")
        first_tpdo.save()
        # first_tpdo.read()
        assert first_tpdo[f"{variable} value"] is not None


def test_map_rpdo(first_rpdo: Map):
    for variable in VARIABLES_TO_MAP:
        first_rpdo.clear()
        first_rpdo.add_variable(f"{variable} value")
        first_rpdo.save()
        first_rpdo.read()
        assert first_rpdo[f"{variable} value"] is not None


def test_map_pdo_invalid_length(first_tpdo: Map):
    first_tpdo.add_variable("UNSIGNED64 value")
    first_tpdo.add_variable("UNSIGNED8 value")
    with pytest.raises(canopen.SdoAbortedError, match="length exceeded"):
        first_tpdo.save()


def test_map_pdo_not_mappable_var(first_tpdo: Map):
    first_tpdo.add_variable("DOMAIN value")
    with pytest.raises(canopen.SdoAbortedError, match="cannot be mapped"):
        first_tpdo.save()


def test_tpdo_transmission(first_tpdo: Map):
    first_tpdo.clear()
    first_tpdo.save()
    first_tpdo.add_variable("UNSIGNED64 value")
    first_tpdo.trans_type = 1
    first_tpdo.enabled = True
    first_tpdo.save()
    assert first_tpdo.wait_for_reception(timeout=1) is not None


import time


def test_rpdo_receive(node_configured: canopen.RemoteNode, first_rpdo: Map):
    first_rpdo.clear()
    first_rpdo.add_variable("REAL64 value")
    # first_rpdo.trans_type = 1
    first_rpdo.enabled = True
    first_rpdo.cob_id = 0x310
    first_rpdo.save()
    first_rpdo["REAL64 value"].raw = 1.554
    for i in range(10):
        time.sleep(0.1)
        first_rpdo.transmit()
    assert node_configured.sdo["REAL64 value"].raw == 1.554


def test_rpdo_receive_consistency(node_configured: canopen.RemoteNode, first_rpdo: Map):
    first_rpdo.clear()
    first_rpdo.add_variable("INTEGER64 value")
    # first_rpdo.trans_type = 1
    first_rpdo.enabled = True
    first_rpdo.cob_id = 0x310
    first_rpdo.save()
    first_rpdo["INTEGER64 value"].raw = 8989
    first_rpdo.start(period=0.01)
    time.sleep(0.1)
    for i in range(100):
        first_rpdo["INTEGER64 value"].raw = 1_111_111
        first_rpdo.update()
        sdo_raw = node_configured.sdo["INTEGER64 value"].raw
        assert sdo_raw == 1_111_111 or sdo_raw == 8989
        first_rpdo["INTEGER64 value"].raw = 8989
        first_rpdo.update()
        sdo_raw = node_configured.sdo["INTEGER64 value"].raw
        assert sdo_raw == 1_111_111 or sdo_raw == 8989
        time.sleep(0.03)


def test_tpdo_receive_consistency(node_configured: canopen.RemoteNode, first_tpdo: Map):
    def tpdo_receiver(map: Map):
        raw_val = map["UNSIGNED64 value"].raw
        assert (
            raw_val == 0xAA_AA_AA_AA_AA_AA_AA_A or raw_val == 0xBB_BB_BB_BB_BB_BB_BB_B
        )

    first_tpdo.clear()
    first_tpdo.clear()
    first_tpdo.add_variable("UNSIGNED64 value")
    node_configured.sdo["Communication cycle period"].raw = 1000
    first_tpdo.trans_type = 1
    first_tpdo.enabled = True
    first_tpdo.cob_id = 0x190
    first_tpdo.save()
    first_tpdo["UNSIGNED64 value"].raw = 8989
    first_tpdo.add_callback(tpdo_receiver)
    for i in range(100):
        node_configured.sdo["UNSIGNED64 value"].raw = 0xAA_AA_AA_AA_AA_AA_AA_A
        node_configured.sdo["UNSIGNED64 value"].raw = 0xBB_BB_BB_BB_BB_BB_BB_B


# def test_tpdo_transmission_type(first)


# def test_tpdo_communication_parameter(remote_node: canopen.RemoteNode):
#     pdo = remote_node.tpdo[1]
#     # Test enable transmission
#     pdo.enabled = True
#     pdo.save()
#     timestamp = pdo.wait_for_reception(timeout=1)
#     assert timestamp != None
#     # Test disable transmission
#     pdo.enabled = False
#     pdo.save()
#     timestamp = pdo.wait_for_reception(timeout=1)
#     assert timestamp == None


# def test_tpdo_transmission_type(remote_node: canopen.RemoteNode):
#     pdo = remote_node.tpdo[1]
#     # Enable tpdo
#     pdo.enabled = True
#     pdo.trans_type = 1
#     pdo.save()
#     SYNC_PERIOD = 10000 / 1e6
#     # Put a sync value of 0.01s (in Us)
#     remote_node.sdo.download(0x1006, 0x0, int(10000).to_bytes(length=4, byteorder="little"))
#     # Check delta between pdo reception
#     timestamp = pdo.wait_for_reception(timeout=0.1)
#     for i in range(5):
#         new_timestamp = pdo.wait_for_reception(timeout=0.1)
#         assert abs(new_timestamp - timestamp) == pytest.approx(SYNC_PERIOD * pdo.trans_type, abs=1e-3)
#         timestamp = new_timestamp

#     # Change transmission type to be 5 (every 5th sync)
#     pdo.trans_type = 5
#     pdo.save()
#     timestamp = pdo.wait_for_reception(timeout=0.1)
#     for i in range(5):
#         new_timestamp = pdo.wait_for_reception(timeout=0.1)
#         assert abs(new_timestamp - timestamp) == pytest.approx(SYNC_PERIOD * pdo.trans_type, abs=1e-2)
#         timestamp = new_timestamp
