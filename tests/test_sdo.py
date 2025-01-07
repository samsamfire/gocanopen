import pytest
import canopen
import struct
from canopen.objectdictionary import datatypes
from canopen.sdo.client import BlockDownloadStream, SdoCommunicationError
import time

import logging

logging.basicConfig(level=logging.DEBUG)
logger = logging.getLogger(__name__)


@pytest.fixture(scope="module")
def node(node: canopen.RemoteNode) -> canopen.RemoteNode:
    node.pdo.read()
    for pdo in node.pdo.values():
        pdo.enabled = False
        pdo.save()
    yield node


def test_sdo_expedited_upload_download_uint8(node: canopen.RemoteNode):
    for value in [10, 22, 89, 253]:
        node.sdo["UNSIGNED8 value"].raw = value
        assert node.sdo["UNSIGNED8 value"].raw == value
        assert node.sdo["UNSIGNED8 value"].od.data_type == datatypes.UNSIGNED8


def test_sdo_expedited_upload_download_uint16(node: canopen.RemoteNode):
    for value in [0x100, 0x200, 0x8989]:
        node.sdo["UNSIGNED16 value"].raw = value
        assert node.sdo["UNSIGNED16 value"].raw == value
        assert node.sdo["UNSIGNED16 value"].od.data_type == datatypes.UNSIGNED16


def test_sdo_expedited_upload_download_uint32(node: canopen.RemoteNode):
    for value in [0x222222, 0x88888888, 0x56]:
        node.sdo["UNSIGNED32 value"].raw = value
        assert node.sdo["UNSIGNED32 value"].raw == value
        assert node.sdo["UNSIGNED32 value"].od.data_type == datatypes.UNSIGNED32


def test_sdo_segmented_upload_download_uint64(node: canopen.RemoteNode):
    for value in [0x222222, 0x88888888, 0x9988888899888888]:
        node.sdo["UNSIGNED64 value"].raw = value
        assert node.sdo["UNSIGNED64 value"].raw == value
        assert node.sdo["UNSIGNED64 value"].od.data_type == datatypes.UNSIGNED64


def test_sdo_expedited_upload_download_int8(node: canopen.RemoteNode):
    for value in [-10, 50, 100]:
        node.sdo["INTEGER8 value"].raw = value
        assert node.sdo["INTEGER8 value"].raw == value
        assert node.sdo["INTEGER8 value"].od.data_type == datatypes.INTEGER8


def test_sdo_expedited_upload_download_int16(node: canopen.RemoteNode):
    for value in [-1000, -9999, 0]:
        node.sdo["INTEGER16 value"].raw = value
        assert node.sdo["INTEGER16 value"].raw == value
        assert node.sdo["INTEGER16 value"].od.data_type == datatypes.INTEGER16


def test_sdo_expedited_upload_download_int32(node: canopen.RemoteNode):
    for value in [-100056666, -89123743, 46512]:
        node.sdo["INTEGER32 value"].raw = value
        assert node.sdo["INTEGER32 value"].raw == value
        assert node.sdo["INTEGER32 value"].od.data_type == datatypes.INTEGER32


def test_sdo_segmented_upload_download_int64(node: canopen.RemoteNode):
    for value in [0x222222, 0x88888888, 0x99888889888888]:
        node.sdo["INTEGER64 value"].raw = value
        assert node.sdo["INTEGER64 value"].raw == value
        assert node.sdo["INTEGER64 value"].od.data_type == datatypes.INTEGER64


def test_sdo_expedited_upload_download_float32(node: canopen.RemoteNode):
    for value in [1.5, 3.4, 8952.65]:
        node.sdo["REAL32 value"].raw = value
        pytest.approx(node.sdo["REAL32 value"].raw) == value
        assert node.sdo["REAL32 value"].od.data_type == datatypes.REAL32


def test_sdo_segmented_upload_download_float64(node: canopen.RemoteNode):
    for value in [1.5, 3.4, 8952.65, 3.14159]:
        node.sdo["REAL64 value"].raw = value
        assert pytest.approx(node.sdo["REAL64 value"].raw) == value
        assert node.sdo["REAL64 value"].od.data_type == datatypes.REAL64


def test_sdo_segmented_upload_download_string(node: canopen.RemoteNode):
    for value in ["a string value9", "anotherstring8", "tinystr"]:
        node.sdo["VISIBLE STRING value"].raw = value
        assert node.sdo["VISIBLE STRING value"].raw == value


def test_sdo_segmented_force_download_upload(node: canopen.RemoteNode):
    DATA = b"A"
    node.sdo.download(0x2005, 0x0, b"A", force_segment=True)
    assert node.sdo.upload(0x2005, 0x0) == b"A"
    node.sdo.download(0x2006, 0x0, b"AB", force_segment=True)
    assert node.sdo.upload(0x2006, 0x0) == b"AB"


def test_sdo_access_read_only(node: canopen.RemoteNode):
    with pytest.raises(canopen.SdoAbortedError, match="read only object"):
        node.sdo["READ ONLY"].raw = 0x1


def test_sdo_access_write_only(node: canopen.RemoteNode):
    with pytest.raises(canopen.SdoAbortedError, match="write only object"):
        raw = node.sdo["WRITE ONLY"].raw


def test_sdo_access_read_write(node: canopen.RemoteNode):
    node.sdo["READ WRITE"].raw = 100
    assert node.sdo["READ WRITE"].raw == 100


def test_sdo_wrong_size(node: canopen.RemoteNode):
    for data_type in ["UNSIGNED8", "INTEGER8"]:
        with pytest.raises(canopen.SdoAbortedError, match="parameter too high"):
            node.sdo[f"{data_type} value"].raw = bytes([1, 2])
    for data_type in ["UNSIGNED16", "INTEGER16"]:
        with pytest.raises(canopen.SdoAbortedError, match="parameter too high"):
            node.sdo[f"{data_type} value"].raw = bytes([1, 2, 3])
        with pytest.raises(canopen.SdoAbortedError, match="parameter too low"):
            node.sdo[f"{data_type} value"].raw = bytes([1])
    for data_type in ["UNSIGNED32", "INTEGER32"]:
        with pytest.raises(canopen.SdoAbortedError, match="parameter too high"):
            node.sdo[f"{data_type} value"].raw = bytes([1, 2, 3, 4, 5])
        with pytest.raises(canopen.SdoAbortedError, match="parameter too low"):
            node.sdo[f"{data_type} value"].raw = bytes([1, 2, 3])


def test_sdo_index_not_exist(node: canopen.RemoteNode):
    with pytest.raises(canopen.SdoAbortedError, match="Object does not exist"):
        _ = node.sdo.upload(0x1555, 0x0)


def test_sdo_subindex_not_exist(node: canopen.RemoteNode):
    with pytest.raises(canopen.SdoAbortedError, match="Subindex does not exist"):
        _ = node.sdo.upload(0x1200, 0x27)


def test_sdo_block_download(node: canopen.RemoteNode):
    NB_LINES = 100
    LINE = b"123456"
    with node.sdo["DOMAIN value"].open(
        mode="wb",
        block_transfer=True,
        request_crc_support=True,
        size=len(LINE) * NB_LINES,
    ) as f:
        for _ in range(NB_LINES):
            f.write(LINE)


def test_sdo_block_download_big_block(node: canopen.RemoteNode):
    NB_LINES = 100000
    LINE = b"123456"
    with node.sdo["DOMAIN value"].open(
        mode="wb", block_transfer=True, request_crc_support=True, size=NB_LINES * len(LINE)
    ) as f:
        for _ in range(NB_LINES):
            f.write(LINE)


def test_sdo_block_upload_bye(node: canopen.RemoteNode):
    with node.sdo["DOMAIN value"].open(
        mode="rb",
        block_transfer=True,
        request_crc_support=True,
    ) as f:
        f.readlines()


def test_sdo_block_download_multi_blocks(node: canopen.RemoteNode):
    NB_LINES = 1000
    LINE = b"123456"
    with node.sdo["DOMAIN value"].open(
        mode="wb",
        block_transfer=True,
        request_crc_support=True,
        size=len(LINE) * NB_LINES,
    ) as f:
        for _ in range(NB_LINES):
            f.write(LINE)


def test_sdo_block_download_no_size(node: canopen.RemoteNode):
    with node.sdo["DOMAIN value"].open(
        mode="wb", block_transfer=True, size=None, request_crc_support=True
    ) as f:
        # End before transmitting everything
        for _ in range(2):
            f.write(b"123456")
        f.raw.size = 10
        f.raw.write(b"25")


def test_sdo_block_download_invalid_blksize(node: canopen.RemoteNode):
    with node.sdo["DOMAIN value"].open(
        mode="wb", block_transfer=True, size=None, request_crc_support=True
    ) as f:
        # End before transmitting everything
        for _ in range(2):
            f.write(b"123456")
        f.raw.size = 10
        f.raw.write(b"25")


def test_sdo_block_download_crc_error(node: canopen.RemoteNode):
    NB_LINES = 100
    LINE = b"123456"
    with pytest.raises(canopen.SdoAbortedError, match="CRC error"):
        with node.sdo["DOMAIN value"].open(
            mode="wb",
            block_transfer=True,
            request_crc_support=True,
            size=len(LINE) * NB_LINES,
        ) as f:
            for i in range(NB_LINES):
                f.write(LINE)
                if i == NB_LINES - 2:
                    # Mess up CRC
                    f.raw._crc.process(b"randomdata")


def test_sdo_block_download_invalid_size(node: canopen.RemoteNode):
    NB_LINES = 100
    LINE = b"123456"
    with pytest.raises(canopen.SdoAbortedError, match="too high"):
        with node.sdo["DOMAIN value"].open(
            mode="wb",
            block_transfer=True,
            request_crc_support=True,
            size=len(LINE) * NB_LINES,
        ) as f:
            for _ in range(NB_LINES + 10):
                f.write(LINE)
    with pytest.raises(canopen.SdoAbortedError, match="too low"):
        with node.sdo["DOMAIN value"].open(
            mode="wb",
            block_transfer=True,
            request_crc_support=True,
            size=len(LINE) * NB_LINES,
        ) as f:
            # End before transmitting everything
            for _ in range(2):
                f.write(LINE)
            f.raw.send(LINE, end=True)


def test_sdo_block_download_retransmit(node: canopen.RemoteNode):
    import io

    BUFFER = 150 * b"123456789abcd"
    INPUT_STREAM = io.BytesIO(BUFFER)
    stream = BlockDownloadStream(node.sdo, index=0x200F, subindex=0x0, size=len(BUFFER))
    counter = 0
    while True:
        chunk = INPUT_STREAM.read(7)
        if not chunk:
            break
        if counter == 15:
            # Send frame with invalid sequence number
            request = bytearray(8)
            request[0] = stream._seqno + 2
            stream.sdo_client.send_request(request)
        stream.write(chunk)
        counter += 1
    stream.close()


def test_sdo_segmented_timeout(node: canopen.RemoteNode):
    def mock_close():
        return None

    # !! This is a limitation of canopen that does not check for SDO aborts without calling "read_response"
    # In the case of a block download, this means that aborts are not correctly handled
    # This starts a block download and hangs on purpose
    f = node.sdo["UNSIGNED64 value"].open(
        mode="rb", block_transfer=False, request_crc_support=True
    )
    f.raw.close = mock_close
    time.sleep(1.1)
    with pytest.raises(canopen.SdoAbortedError, match="Timeout"):
        while True:
            response = f.raw.sdo_client.read_response()

    f = node.sdo["UNSIGNED64 value"].open(
        mode="wb", block_transfer=False, request_crc_support=True
    )
    f.raw.close = mock_close
    time.sleep(1.1)
    with pytest.raises(canopen.SdoAbortedError, match="Timeout"):
        while True:
            response = f.raw.sdo_client.read_response()


def test_sdo_block_timeout(node: canopen.RemoteNode):
    from canopen.sdo.client import SdoClient

    SdoClient.RESPONSE_TIMEOUT = 1.5

    def mock_close():
        return None

    # !! This is a limitation of canopen that does not check for SDO aborts without calling "read_response"
    # In the case of a block download, this means that aborts are not correctly handled
    # This starts a block download and hangs on purpose
    f = node.sdo["DOMAIN value"].open(
        mode="wb", block_transfer=True, request_crc_support=True, size=1000
    )
    f.raw.close = mock_close
    time.sleep(2.0)
    with pytest.raises(canopen.SdoAbortedError, match="Timeout"):
        while True:
            _ = f.raw.sdo_client.read_response()

    f = node.sdo["DOMAIN value"].open(
        mode="rb", block_transfer=True, request_crc_support=True, size=1000
    )
    f.raw.close = mock_close
    time.sleep(2.0)
    with pytest.raises(canopen.SdoAbortedError, match="Timeout"):
        while True:
            _ = f.raw.sdo_client.read_response()


def test_sdo_block_upload_invalid_blksize(node: canopen.RemoteNode):
    from canopen.sdo.client import BlockUploadStream

    BlockUploadStream.blksize = 128
    with pytest.raises(canopen.SdoAbortedError, match="Invalid block size"):
        with node.sdo["DOMAIN value"].open(
            mode="rb", block_transfer=True, request_crc_support=True, size=1000
        ) as f:
            pass

    BlockUploadStream.blksize = 0
    with pytest.raises(canopen.SdoAbortedError, match="Invalid block size"):
        with node.sdo["DOMAIN value"].open(
            mode="rb", block_transfer=True, request_crc_support=True, size=1000
        ) as f:
            pass
    BlockUploadStream.blksize = 127


def test_sdo_block_upload_crc_invalid(node: canopen.RemoteNode):
    from canopen.sdo.client import BlockUploadStream

    with pytest.raises(canopen.SdoCommunicationError, match="CRC"):
        stream = BlockUploadStream(
            node.sdo, index=0x200F, subindex=0x0, request_crc_support=True
        )
        counter = 0
        while stream._done != True:
            counter += 1
            stream.read(7)
            if counter == 1:
                # Mess up CRC
                stream._crc.process(b"randomdata")
            stream.close()

    # Do some dummy reads
    for value in [10, 22, 89, 253]:
        node.sdo["UNSIGNED8 value"].raw = value
        assert node.sdo["UNSIGNED8 value"].raw == value
        assert node.sdo["UNSIGNED8 value"].od.data_type == datatypes.UNSIGNED8


def _retransmit(self):
    logger.info(
        "Only %d sequences were received. Requesting retransmission", self._ackseq
    )
    end_time = time.time() + self.sdo_client.RESPONSE_TIMEOUT
    self._ack_block()
    while time.time() < end_time:
        response = self.sdo_client.read_response()
        (res_command,) = struct.unpack_from("B", response)
        seqno = res_command & 0x7F
        if seqno == 1:
            # We should be back in sync
            self._ackseq = seqno
            return response
    raise SdoCommunicationError("Some data were lost and could not be retransmitted")


def test_sdo_block_upload_retransmit(node: canopen.RemoteNode, monkeypatch):
    from canopen.sdo.client import BlockUploadStream

    monkeypatch.setattr(BlockUploadStream, "_retransmit", _retransmit)

    stream = BlockUploadStream(
        node.sdo, index=0x200F, subindex=0x0, request_crc_support=True
    )
    counter = 0
    done = False

    while stream._done != True:

        # Mess up sequence number in order to trigger retransmit
        if not done and stream._ackseq >= 50:
            # Patch read function fake a wrong block to trigger retransmit
            stream.sdo_client.responses.get()
            stream.sdo_client.responses.put(
                bytes([80, 255, 255, 255, 255, 255, 255, 255])
            )
            done = True
        counter += 1
        stream.read(7)


def test_sdo_block_download_upload(node: canopen.RemoteNode):
    LINE = b"this is some fake bin data\n"
    STRING_BINARY = b""
    for i in range(111):
        STRING_BINARY += LINE
    # Write some data then read back
    with node.sdo["DOMAIN value"].open(
        mode="wb",
        block_transfer=True,
        size=len(STRING_BINARY),
        request_crc_support=True,
    ) as f:
        f.write(STRING_BINARY)
    READ_BINARY = b""

    with node.sdo["DOMAIN value"].open(
        mode="rb",
        block_transfer=True,
        size=len(STRING_BINARY),
        request_crc_support=True,
    ) as f:
        READ_BINARY = f.read()
    assert READ_BINARY == STRING_BINARY

    with node.sdo["DOMAIN value"].open(
        mode="rb",
        block_transfer=True,
        request_crc_support=True,
    ) as f:
        READ_BINARY = f.read()
