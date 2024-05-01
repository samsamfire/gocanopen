import pytest
import logging
import pathlib

import canopen

EDS_PATH = str(
    pathlib.Path(__file__).parent.absolute().parent.joinpath("pkg/od/base.eds")
)
TEST_ID = 0x10

logger = logging.getLogger(__name__)


@pytest.fixture(scope="session")
def network():
    network = canopen.Network()
    network.connect(
        interface="virtualcan",
        channel="localhost:18889",
        receive_own_messages=True,
    )
    yield network


@pytest.fixture(scope="session")
def node(network) -> canopen.RemoteNode:
    if TEST_ID in network:
        del network[TEST_ID]
    node = network.add_node(TEST_ID, EDS_PATH)
    yield node
