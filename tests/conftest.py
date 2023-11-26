import pytest
import time
import canopen
import logging
import pathlib
import os
import signal
import subprocess

EDS_PATH = str(
    pathlib.Path(__file__).parent.absolute().parent.joinpath("testdata/base.eds")
)
EXEC_PATH = (
    pathlib.Path(__file__).parent.absolute().parent.joinpath("cmd/canopen/canopen")
)
TEST_ID = 0x10
# The os.setsid() is passed in the argument preexec_fn so
# it's run after the fork() and before  exec() to run the shell.

logger = logging.getLogger(__name__)


# @pytest.fixture(scope="session", autouse=True)
# def go_main():
#     """start go"""
#     cmd = f"{EXEC_PATH} -i vcan0 -p {EDS_PATH} -n {TEST_ID}"
#     logger.info(f"using cmd for go {cmd}")
#     proc = subprocess.Popen(
#         cmd,
#         stdout=subprocess.PIPE,
#         shell=True,
#         preexec_fn=os.setsid,
#     )
#     time.sleep(1.0)  # give time for the server to start
#     yield proc
#     os.killpg(os.getpgid(proc.pid), signal.SIGTERM)  # Send the signal to all the process groups


# @pytest.fixture(autouse=True)
# def check_main_still_alive(go_main):
#     assert go_main.poll() is None


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
