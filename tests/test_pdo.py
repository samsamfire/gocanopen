import pytest
import time
import canopen
from canopen.sdo import SdoAbortedError


# def test_read_tpdo_communication_parameter(remote_node : canopen.RemoteNode):
#     cob_id_raw = remote_node.sdo.upload(0x1800,0x1)
#     transmission_type_raw = remote_node.sdo.upload(0x1800,0x2)
#     inihibit_time_raw = remote_node.sdo.upload(0x1800,0x03)

# def test_write_tpdo_communication_parameter(remote_node : canopen.RemoteNode):
#     # Disable first
#     remote_node.sdo.download(0x1800,0x1,0x80000180.to_bytes(length=4,byteorder="little"))
#     # Write new transmission type
#     assert  len(remote_node.sdo.upload(0x1800,0x2)) == 1
#     remote_node.sdo.download(0x1800,0x2,int(10).to_bytes(length=1,byteorder="little"))


def test_tpdo_communication_parameter(remote_node: canopen.RemoteNode):
    pdo = remote_node.tpdo[1]
    # Test enable transmission
    pdo.enabled = True
    pdo.save()
    timestamp = pdo.wait_for_reception(timeout=1)
    assert timestamp != None
    # Test disable transmission
    pdo.enabled = False
    pdo.save()
    timestamp = pdo.wait_for_reception(timeout=1)
    assert timestamp == None


def test_tpdo_transmission_type(remote_node: canopen.RemoteNode):
    pdo = remote_node.tpdo[1]
    # Enable tpdo
    pdo.enabled = True
    pdo.trans_type = 1
    pdo.save()
    SYNC_PERIOD = 10000 / 1e6
    # Put a sync value of 0.01s (in Us)
    remote_node.sdo.download(
        0x1006, 0x0, int(10000).to_bytes(length=4, byteorder="little")
    )
    # Check delta between pdo reception
    timestamp = pdo.wait_for_reception(timeout=0.1)
    for i in range(5):
        new_timestamp = pdo.wait_for_reception(timeout=0.1)
        assert abs(new_timestamp - timestamp) == pytest.approx(
            SYNC_PERIOD * pdo.trans_type, abs=1e-3
        )
        timestamp = new_timestamp

    # Change transmission type to be 5 (every 5th sync)
    pdo.trans_type = 5
    pdo.save()
    timestamp = pdo.wait_for_reception(timeout=0.1)
    for i in range(5):
        new_timestamp = pdo.wait_for_reception(timeout=0.1)
        assert abs(new_timestamp - timestamp) == pytest.approx(
            SYNC_PERIOD * pdo.trans_type, abs=1e-2
        )
        timestamp = new_timestamp
