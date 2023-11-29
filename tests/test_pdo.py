import logging
from typing import List
import time

import pytest
import canopen
from canopen import RemoteNode
from canopen.pdo.base import Map

logger = logging.getLogger(__name__)

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


@pytest.fixture()
def first_pdos(first_rpdo, first_tpdo) -> List[Map]:
    return [first_rpdo, first_tpdo]


def test_map_pdo(node_configured):
    for pdo in node_configured.pdo.values():
        for variable in VARIABLES_TO_MAP:
            pdo.clear()
            pdo.add_variable(f"{variable} value")
            pdo.save()
            pdo.read()
            assert pdo[f"{variable} value"] is not None


def test_map_pdo_multiple(first_pdos: List[Map]):
    for pdo in first_pdos:
        pdo.clear()
        for _ in range(8):
            pdo.add_variable("UNSIGNED8 value")
        pdo.save()
        pdo.clear()
        for _ in range(4):
            pdo.add_variable("UNSIGNED16 value")
        pdo.save()
        pdo.clear()
        for _ in range(2):
            pdo.add_variable("UNSIGNED32 value")
        pdo.save()


def test_disable_pdo(first_rpdo: Map, first_tpdo: Map):
    for pdo in [first_tpdo, first_rpdo]:
        pdo.enabled = False
        pdo.save()
        pdo.read()
        assert pdo.enabled == False


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


def test_rpdo_receive(node_configured: canopen.RemoteNode, first_rpdo: Map):
    first_rpdo.clear()
    first_rpdo.add_variable("REAL64 value")
    first_rpdo.trans_type = 1
    first_rpdo.enabled = True
    first_rpdo.cob_id = 0x310
    first_rpdo.save()
    first_rpdo["REAL64 value"].raw = 1.554
    first_rpdo.transmit()
    time.sleep(0.2)
    assert node_configured.sdo["REAL64 value"].raw == 1.554


def test_rpdo_receive_consistency(node_configured: canopen.RemoteNode, first_rpdo: Map):
    first_rpdo.clear()
    first_rpdo.add_variable("INTEGER64 value")
    first_rpdo.trans_type = 1
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
    received_bytes = []

    def tpdo_receiver(map: Map):
        raw_val = map["UNSIGNED64 value"].raw
        received_bytes.append(raw_val)

    first_tpdo.clear()
    first_tpdo.add_variable("UNSIGNED64 value")
    node_configured.sdo["Communication cycle period"].raw = 1000
    node_configured.sdo["UNSIGNED64 value"].raw = 0xAA_AA_AA_AA_AA_AA_AA_A
    first_tpdo.trans_type = 1
    first_tpdo.enabled = True
    first_tpdo.cob_id = 0x190
    first_tpdo.save()
    first_tpdo.add_callback(tpdo_receiver)
    toggle: bool = True
    for _ in range(100):
        if toggle:
            node_configured.sdo["UNSIGNED64 value"].raw = 0xAA_AA_AA_AA_AA_AA_AA_A
            toggle = False
        else:
            node_configured.sdo["UNSIGNED64 value"].raw = 0xBB_BB_BB_BB_BB_BB_BB_B
            toggle = True

    for raw_val in received_bytes:
        assert (
            raw_val == 0xAA_AA_AA_AA_AA_AA_AA_A or raw_val == 0xBB_BB_BB_BB_BB_BB_BB_B
        )


def test_tpdo_transmission_type(first_tpdo: Map, node_configured: RemoteNode):
    first_tpdo.clear()
    first_tpdo.enabled = True
    first_tpdo.trans_type = 1
    first_tpdo.add_variable("UNSIGNED64 value")
    first_tpdo.save()
    SYNC_PERIOD_S = 0.01
    # Put a sync value of 0.01s (convert in Us)
    node_configured.sdo.download(
        0x1006, 0x0, int(SYNC_PERIOD_S * 1e6).to_bytes(length=4, byteorder="little")
    )
    # Check delta between pdo reception
    timestamp = first_tpdo.wait_for_reception(timeout=0.1)
    for _ in range(5):
        new_timestamp = first_tpdo.wait_for_reception(timeout=0.1)
        assert abs(new_timestamp - timestamp) == pytest.approx(
            SYNC_PERIOD_S * first_tpdo.trans_type, abs=1e-3
        )
        timestamp = new_timestamp

    # Change transmission type to be 5 (every 5th sync)
    first_tpdo.trans_type = 5
    first_tpdo.save()
    first_tpdo.read()
    assert first_tpdo.trans_type == 5
    timestamp = first_tpdo.wait_for_reception(timeout=0.1)
    for i in range(5):
        new_timestamp = first_tpdo.wait_for_reception(timeout=0.1)
        assert abs(new_timestamp - timestamp) == pytest.approx(
            SYNC_PERIOD_S * first_tpdo.trans_type, abs=1e-2
        )
        timestamp = new_timestamp


def test_tpdo_transmission_type(first_tpdo: Map):
    first_tpdo.clear()
    first_tpdo.add_variable("UNSIGNED64 value")
    first_tpdo.enabled = True
    first_tpdo.inhibit_time = 11000
    first_tpdo.event_timer = 9999
    first_tpdo.trans_type = 188
    first_tpdo.save()
    first_tpdo.read()
    assert first_tpdo.inhibit_time == 11000
    assert first_tpdo.event_timer == 9999
    assert first_tpdo.trans_type == 188
