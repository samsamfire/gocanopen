import pytest
import canopen
from .conftest import TEST_ID, EDS_PATH


@pytest.fixture
def node(network) -> canopen.RemoteNode:
    node = network.add_node(TEST_ID, EDS_PATH)
    yield node


def test_sdo_expedited_download(node: canopen.RemoteNode):
    assert node.sdo["UNSIGNED8 value"].raw == 0x10


def test_sdo_expedited_upload(node: canopen.RemoteNode):
    TEST_VALUES = [10, 22, 89, 253]
    for value in TEST_VALUES:
        # write
        node.sdo["UNSIGNED8 value"].raw = value
        # then read
        assert node.sdo["UNSIGNED8 value"].raw == value


def test_sdo_expedited_upload_errors(node: canopen.RemoteNode):
    with pytest.raises(
        canopen.SdoAbortedError,
        match="Data type does not match, length of service parameter too high",
    ):
        node.sdo["UNSIGNED8 value"].raw = bytes([1, 1])


def test_sdo_segmented_download(node: canopen.RemoteNode):
    assert node.sdo["Manufacturer device name"].raw == "Im a CANopen device"
