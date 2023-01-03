package canopen

// import "log"

// type CO_SDO_state_t uint8

// const (
// 	/**
// 	 * - SDO client may start new download to or upload from specified node,
// 	 * specified index and specified subindex. It can start normal or block
// 	 * communication.
// 	 * - SDO server is waiting for client request. */
// 	CO_SDO_ST_IDLE CO_SDO_state_t = 0x00
// 	/**
// 	 * - SDO client or server may send SDO abort message in case of error:
// 	 *  - byte 0: @b 10000000 binary.
// 	 *  - byte 1..3: Object index and subIndex.
// 	 *  - byte 4..7: #CO_SDO_abortCode_t. */
// 	CO_SDO_ST_ABORT = 0x01

// 	/**
// 	 * - SDO client: Node-ID of the SDO server is the same as node-ID of this node,
// 	 *   SDO client is the same device as SDO server. Transfer data directly without
// 	 *   communication on CAN.
// 	 * - SDO server does not use this state. */
// 	CO_SDO_ST_DOWNLOAD_LOCAL_TRANSFER = 0x10
// 	/**
// 	 * - SDO client initiates SDO download:
// 	 *  - byte 0: @b 0010nnes binary: (nn: if e=s=1, number of data bytes, that do
// 	 *    @b not contain data; e=1 for expedited transfer; s=1 if data size is
// 	 *    indicated.)
// 	 *  - byte 1..3: Object index and subIndex.
// 	 *  - byte 4..7: If e=1, expedited data are here. If e=0 s=1, size of data for
// 	 *    segmented transfer is indicated here.
// 	 * - SDO server is in #CO_SDO_ST_IDLE state and waits for client request. */
// 	CO_SDO_ST_DOWNLOAD_INITIATE_REQ = 0x11
// 	/**
// 	 * - SDO client waits for response.
// 	 * - SDO server responses:
// 	 *  - byte 0: @b 01100000 binary.
// 	 *  - byte 1..3: Object index and subIndex.
// 	 *  - byte 4..7: Reserved.
// 	 * - In case of expedited transfer communication ends here. */
// 	CO_SDO_ST_DOWNLOAD_INITIATE_RSP = 0x12
// 	/**
// 	 * - SDO client sends SDO segment:
// 	 *  - byte 0: @b 000tnnnc binary: (t: toggle bit, set to 0 in first segment;
// 	 *    nnn: number of data bytes, that do @b not contain data; c=1 if this is the
// 	 *    last segment).
// 	 *  - byte 1..7: Data segment.
// 	 * - SDO server waits for segment. */
// 	CO_SDO_ST_DOWNLOAD_SEGMENT_REQ = 0x13
// 	/**
// 	 * - SDO client waits for response.
// 	 * - SDO server responses:
// 	 *  - byte 0: @b 001t0000 binary: (t: toggle bit, set to 0 in first segment).
// 	 *  - byte 1..7: Reserved.
// 	 * - If c was set to 1, then communication ends here. */
// 	CO_SDO_ST_DOWNLOAD_SEGMENT_RSP = 0x14

// 	/**
// 	 * - SDO client: Node-ID of the SDO server is the same as node-ID of this node,
// 	 *   SDO client is the same device as SDO server. Transfer data directly without
// 	 *   communication on CAN.
// 	 * - SDO server does not use this state. */
// 	CO_SDO_ST_UPLOAD_LOCAL_TRANSFER = 0x20
// 	/**
// 	 * - SDO client initiates SDO upload:
// 	 *  - byte 0: @b 01000000 binary.
// 	 *  - byte 1..3: Object index and subIndex.
// 	 *  - byte 4..7: Reserved.
// 	 * - SDO server is in #CO_SDO_ST_IDLE state and waits for client request. */
// 	CO_SDO_ST_UPLOAD_INITIATE_REQ = 0x21
// 	/**
// 	 * - SDO client waits for response.
// 	 * - SDO server responses:
// 	 *  - byte 0: @b 0100nnes binary: (nn: if e=s=1, number of data bytes, that do
// 	 *    @b not contain data; e=1 for expedited transfer; s=1 if data size is
// 	 *    indicated).
// 	 *  - byte 1..3: Object index and subIndex.
// 	 *  - byte 4..7: If e=1, expedited data are here. If e=0 s=1, size of data for
// 	 *    segmented transfer is indicated here.
// 	 * - In case of expedited transfer communication ends here. */
// 	CO_SDO_ST_UPLOAD_INITIATE_RSP = 0x22
// 	/**
// 	 * - SDO client requests SDO segment:
// 	 *  - byte 0: @b 011t0000 binary: (t: toggle bit, set to 0 in first segment).
// 	 *  - byte 1..7: Reserved.
// 	 * - SDO server waits for segment request. */
// 	CO_SDO_ST_UPLOAD_SEGMENT_REQ = 0x23
// 	/**
// 	 * - SDO client waits for response.
// 	 * - SDO server responses with data:
// 	 *  - byte 0: @b 000tnnnc binary: (t: toggle bit, set to 0 in first segment;
// 	 *    nnn: number of data bytes, that do @b not contain data; c=1 if this is the
// 	 *    last segment).
// 	 *  - byte 1..7: Data segment.
// 	 * - If c is set to 1, then communication ends here. */
// 	CO_SDO_ST_UPLOAD_SEGMENT_RSP = 0x24

// 	/**
// 	 * - SDO client initiates SDO block download:
// 	 *  - byte 0: @b 11000rs0 binary: (r=1 if client supports generating CRC on
// 	 *    data; s=1 if data size is indicated.)
// 	 *  - byte 1..3: Object index and subIndex.
// 	 *  - byte 4..7: If s=1, then size of data for block download is indicated here.
// 	 * - SDO server is in #CO_SDO_ST_IDLE state and waits for client request. */
// 	CO_SDO_ST_DOWNLOAD_BLK_INITIATE_REQ = 0x51
// 	/**
// 	 * - SDO client waits for response.
// 	 * - SDO server responses:
// 	 *  - byte 0: @b 10100r00 binary: (r=1 if server supports generating CRC on
// 	 *    data.)
// 	 *  - byte 1..3: Object index and subIndex.
// 	 *  - byte 4: blksize: Number of segments per block that shall be used by the
// 	 *    client for the following block download with 0 < blksize < 128.
// 	 *  - byte 5..7: Reserved. */
// 	CO_SDO_ST_DOWNLOAD_BLK_INITIATE_RSP = 0x52
// 	/**
// 	 * - SDO client sends 'blksize' segments of data in sequence:
// 	 *  - byte 0: @b cnnnnnnn binary: (c=1 if no more segments to be downloaded,
// 	 *    enter SDO block download end phase; nnnnnnn is sequence number of segment,
// 	 *    1..127.
// 	 *  - byte 1..7: At most 7 bytes of segment data to be downloaded.
// 	 * - SDO server reads sequence of 'blksize' blocks. */
// 	CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_REQ = 0x53
// 	/**
// 	 * - SDO client waits for response.
// 	 * - SDO server responses:
// 	 *  - byte 0: @b 10100010 binary.
// 	 *  - byte 1: ackseq: sequence number of last segment that was received
// 	 *    successfully during the last block download. If ackseq is set to 0 the
// 	 *    server indicates the client that the segment with the sequence number 1
// 	 *    was not received correctly and all segments shall be retransmitted by the
// 	 *    client.
// 	 *  - byte 2: Number of segments per block that shall be used by the client for
// 	 *    the following block download with 0 < blksize < 128.
// 	 *  - byte 3..7: Reserved.
// 	 * - If c was set to 1, then communication enters SDO block download end phase.
// 	 */
// 	CO_SDO_ST_DOWNLOAD_BLK_SUBBLOCK_RSP = 0x54
// 	/**
// 	 * - SDO client sends SDO block download end:
// 	 *  - byte 0: @b 110nnn01 binary: (nnn: number of data bytes, that do @b not
// 	 *    contain data)
// 	 *  - byte 1..2: 16 bit CRC for the data set, if enabled by client and server.
// 	 *  - byte 3..7: Reserved.
// 	 * - SDO server waits for client request. */
// 	CO_SDO_ST_DOWNLOAD_BLK_END_REQ = 0x55
// 	/**
// 	 * - SDO client waits for response.
// 	 * - SDO server responses:
// 	 *  - byte 0: @b 10100001 binary.
// 	 *  - byte 1..7: Reserved.
// 	 * - Block download successfully ends here.
// 	 */
// 	CO_SDO_ST_DOWNLOAD_BLK_END_RSP = 0x56

// 	/**
// 	 * - SDO client initiates SDO block upload:
// 	 *  - byte 0: @b 10100r00 binary: (r=1 if client supports generating CRC on
// 	 *    data.)
// 	 *  - byte 1..3: Object index and subIndex.
// 	 *  - byte 4: blksize: Number of segments per block with 0 < blksize < 128.
// 	 *  - byte 5: pst - protocol switch threshold. If pst > 0 and size of the data
// 	 *    in bytes is less or equal pst, then the server may switch to the SDO
// 	 *    upload protocol #CO_SDO_ST_UPLOAD_INITIATE_RSP.
// 	 *  - byte 6..7: Reserved.
// 	 * - SDO server is in #CO_SDO_ST_IDLE state and waits for client request. */
// 	CO_SDO_ST_UPLOAD_BLK_INITIATE_REQ = 0x61
// 	/**
// 	 * - SDO client waits for response.
// 	 * - SDO server responses:
// 	 *  - byte 0: @b 11000rs0 binary: (r=1 if server supports generating CRC on
// 	 *    data; s=1 if data size is indicated. )
// 	 *  - byte 1..3: Object index and subIndex.
// 	 *  - byte 4..7: If s=1, then size of data for block upload is indicated here.
// 	 * - If enabled by pst, then server may alternatively response with
// 	 *   #CO_SDO_ST_UPLOAD_INITIATE_RSP */
// 	CO_SDO_ST_UPLOAD_BLK_INITIATE_RSP = 0x62
// 	/**
// 	 * - SDO client sends second initiate for SDO block upload:
// 	 *  - byte 0: @b 10100011 binary.
// 	 *  - byte 1..7: Reserved.
// 	 * - SDO server waits for client request. */
// 	CO_SDO_ST_UPLOAD_BLK_INITIATE_REQ2 = 0x63
// 	/**
// 	 * - SDO client reads sequence of 'blksize' blocks.
// 	 * - SDO server sends 'blksize' segments of data in sequence:
// 	 *  - byte 0: @b cnnnnnnn binary: (c=1 if no more segments to be uploaded,
// 	 *    enter SDO block upload end phase; nnnnnnn is sequence number of segment,
// 	 *    1..127.
// 	 *  - byte 1..7: At most 7 bytes of segment data to be uploaded. */
// 	CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ = 0x64
// 	/**
// 	 * - SDO client responses:
// 	 *  - byte 0: @b 10100010 binary.
// 	 *  - byte 1: ackseq: sequence number of last segment that was received
// 	 *    successfully during the last block upload. If ackseq is set to 0 the
// 	 *    client indicates the server that the segment with the sequence number 1
// 	 *    was not received correctly and all segments shall be retransmitted by the
// 	 *    server.
// 	 *  - byte 2: Number of segments per block that shall be used by the server for
// 	 *    the following block upload with 0 < blksize < 128.
// 	 *  - byte 3..7: Reserved.
// 	 * - SDO server waits for response.
// 	 * - If c was set to 1 and all segments were successfull received, then
// 	 *   communication enters SDO block upload end phase. */
// 	CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_CRSP = 0x65
// 	/**
// 	 * - SDO client waits for server request.
// 	 * - SDO server sends SDO block upload end:
// 	 *  - byte 0: @b 110nnn01 binary: (nnn: number of data bytes, that do @b not
// 	 *    contain data)
// 	 *  - byte 1..2: 16 bit CRC for the data set, if enabled by client and server.
// 	 *  - byte 3..7: Reserved. */
// 	CO_SDO_ST_UPLOAD_BLK_END_SREQ = 0x66
// 	/**
// 	 * - SDO client responses:
// 	 *  - byte 0: @b 10100001 binary.
// 	 *  - byte 1..7: Reserved.
// 	 * - SDO server waits for response.
// 	 * - Block download successfully ends here. Note that this communication ends
// 	 *   with client response. Client may then start next SDO communication
// 	 *   immediately.
// 	 */
// 	CO_SDO_ST_UPLOAD_BLK_END_CRSP = 0x67
// )

// type SDO_Client struct {
// 	OD     *OD_t
// 	nodeId uint8
// 	/** Object dictionary interface for locally transferred object */
// 	OD_IO                OD_io
// 	CANdevRx             *CO_CANmodule_t
// 	CANdevRxIdx          uint16
// 	CANdevTx             *CO_CANmodule_t
// 	CANdevTxIdx          uint16
// 	CANtxBuff            *CO_CANtx_t
// 	COB_IDClientToServer uint32
// 	COB_IDServerToClient uint32
// 	// OD_1280_extension    OD_extension_t
// 	nodeIDOfTheSDOServer uint8
// 	valid                bool
// 	index                uint16
// 	subindex             uint8
// 	finished             bool
// 	sizeInd              int
// 	sizeTran             int
// 	state                CO_SDO_state_t
// 	SDOtimeoutTime_us    uint32
// 	timeoutTimer         uint32
// 	bufFifo              []uint8
// 	buf                  []uint8
// 	CANrxNew             *uint8
// 	CANrxData            []uint8
// 	// /** From CO_SDOclient_initCallbackPre() or NULL */
// 	// void (*pFunctSignal)(void *object);
// 	// /** From CO_SDOclient_initCallbackPre() or NULL */
// 	// void *functSignalObject;
// 	toggle                  uint8
// 	block_SDOtimeoutTime_us uint32
// 	block_timeoutTimer      uint32
// 	block_seqno             uint8
// 	block_blksize           uint8
// 	block_noData            uint8
// 	block_crcEnabled        bool
// 	block_dataUploadLast    []uint8
// 	block_crc               uint16
// }

// type CANMessage struct {
// 	dlc  uint8
// 	data []uint8
// }

// /*
//  * Read received message from CAN module.
//  *
//  * Function will be called (by CAN receive interrupt) every time, when CAN
//  * message with correct identifier will be received. For more information and
//  * description of parameters see file CO_driver.h.
//  */

// func (client *SDO_Client) Receive(msg *CANMessage) {
// 	data := msg.data
// 	dlc := msg.dlc
// 	if client.state != CO_SDO_ST_IDLE && dlc == 8 && (client.CANrxNew == nil || data[0] == 0x80) {
// 		if data[0] == 0x80 || (client.state != CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ && client.state != CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_CRSP) {
// 			client.CANrxData = data[0:7]
// 			*client.CANrxNew = 1
// 		} else if client.state == CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ {
// 			var state CO_SDO_state_t
// 			state = CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ
// 			var seqno uint8 = data[0] & 0x7F
// 			client.timeoutTimer = 0
// 			client.block_timeoutTimer = 0
// 			/* verify if sequence number is correct */
// 			if seqno <= client.block_blksize && seqno == client.block_seqno+1 {
// 				client.block_seqno = seqno
// 				/* Is this the last segment ?*/
// 				if (data[0] & 0x80) != 0 {
// 					/* copy data to temporary buffer, because we don't know the
// 					 * number of bytes not containing data */
// 					client.block_dataUploadLast = data[1:8]
// 					client.finished = true
// 					state = CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_CRSP
// 				} else {
// 					/* Copy data. There is always enough space in fifo buffer,
// 					 * because block_blksize was calculated before */
// 					/*TODO : check that using slices is ok compared to using a fifo*/
// 					client.bufFifo = append(client.bufFifo, data[1:8]...)
// 					client.sizeTran += 7
// 					if seqno == client.block_blksize {
// 						state = CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_CRSP
// 					}
// 				}
// 			} else if seqno != client.block_seqno && client.block_seqno != 0 {
// 				/*If message is duplicate or sequence didn't start yet, ignore
// 				 * it. Otherwise seqno is wrong, so break sub-block. Data after
// 				 * last good seqno will be re-transmitted. */
// 				state = CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_CRSP
// 				log.Printf("sub-block, rx WRONG: sequno=%02X, previous=%02X", seqno, client.block_seqno)
// 			} else {
// 				log.Printf("sub-block, rx ignored: sequno=%02X, expected=%02X", seqno, client.block_seqno+1)
// 			}

// 			/* Is exit from sub-block receive state? */
// 			if state != CO_SDO_ST_UPLOAD_BLK_SUBBLOCK_SREQ {
// 				/* Processing will continue in another thread, so make memory
// 				 * barrier here with CO_FLAG_CLEAR() call. */
// 				client.state = state
// 				client.CANrxNew = nil
// 				/* Optional signal to RTOS, which can resume task, which handles
// 				 * SDO client processing. */
// 				// if (SDO_C->pFunctSignal != NULL) {
// 				//     SDO_C->pFunctSignal(SDO_C->functSignalObject);
// 				// }
// 			}

// 		}
// 	}
// }

// /******************************************************************************/

// // CO_ReturnError_t CO_SDOclient_init(CO_SDOclient_t *SDO_C,
// //     OD_t *OD,
// //     OD_entry_t *OD_1280_SDOcliPar,
// //     uint8_t nodeId,
// //     CO_CANmodule_t *CANdevRx,
// //     uint16_t CANdevRxIdx,
// //     CO_CANmodule_t *CANdevTx,
// //     uint16_t CANdevTxIdx,
// //     uint32_t *errInfo)
// // {
// // /* verify arguments */
// // if (SDO_C == NULL || OD_1280_SDOcliPar == NULL
// // || OD_getIndex(OD_1280_SDOcliPar) < OD_H1280_SDO_CLIENT_1_PARAM
// // || OD_getIndex(OD_1280_SDOcliPar) > (OD_H1280_SDO_CLIENT_1_PARAM + 0x7F)
// // || CANdevRx==NULL || CANdevTx==NULL
// // ) {
// // return CO_ERROR_ILLEGAL_ARGUMENT;
// // }

// // /* Configure object variables */
// // #if (CO_CONFIG_SDO_CLI) & CO_CONFIG_SDO_CLI_LOCAL
// // SDO_C->OD = OD;
// // SDO_C->nodeId = nodeId;
// // #endif
// // SDO_C->CANdevRx = CANdevRx;
// // SDO_C->CANdevRxIdx = CANdevRxIdx;
// // SDO_C->CANdevTx = CANdevTx;
// // SDO_C->CANdevTxIdx = CANdevTxIdx;
// // #if (CO_CONFIG_SDO_CLI) & CO_CONFIG_FLAG_CALLBACK_PRE
// // SDO_C->pFunctSignal = NULL;
// // SDO_C->functSignalObject = NULL;
// // #endif

// // /* prepare circular fifo buffer */
// // CO_fifo_init(&SDO_C->bufFifo, SDO_C->buf,
// // CO_CONFIG_SDO_CLI_BUFFER_SIZE + 1);

// // /* Get parameters from Object Dictionary (initial values) */
// // uint8_t maxSubIndex, nodeIDOfTheSDOServer;
// // uint32_t COB_IDClientToServer, COB_IDServerToClient;
// // ODR_t odRet0 = OD_get_u8(OD_1280_SDOcliPar, 0, &maxSubIndex, true);
// // ODR_t odRet1 = OD_get_u32(OD_1280_SDOcliPar, 1, &COB_IDClientToServer, true);
// // ODR_t odRet2 = OD_get_u32(OD_1280_SDOcliPar, 2, &COB_IDServerToClient, true);
// // ODR_t odRet3 = OD_get_u8(OD_1280_SDOcliPar, 3, &nodeIDOfTheSDOServer, true);

// // if (odRet0 != ODR_OK || maxSubIndex != 3
// // || odRet1 != ODR_OK || odRet2 != ODR_OK || odRet3 != ODR_OK
// // ) {
// // if (errInfo != NULL) *errInfo = OD_getIndex(OD_1280_SDOcliPar);
// // return CO_ERROR_OD_PARAMETERS;
// // }

// // #if (CO_CONFIG_SDO_CLI) & CO_CONFIG_FLAG_OD_DYNAMIC
// // SDO_C->OD_1280_extension.object = SDO_C;
// // SDO_C->OD_1280_extension.read = OD_readOriginal;
// // SDO_C->OD_1280_extension.write = OD_write_1280;
// // ODR_t odRetE = OD_extension_init(OD_1280_SDOcliPar,
// //       &SDO_C->OD_1280_extension);
// // if (odRetE != ODR_OK) {
// // if (errInfo != NULL) *errInfo = OD_getIndex(OD_1280_SDOcliPar);
// // return CO_ERROR_OD_PARAMETERS;
// // }

// // /* set to zero to make sure CO_SDOclient_setup() will reconfigure CAN */
// // SDO_C->COB_IDClientToServer = 0;
// // SDO_C->COB_IDServerToClient = 0;
// // #endif

// // CO_SDO_return_t cliSetupRet = CO_SDOclient_setup(SDO_C,
// //                       COB_IDClientToServer,
// //                       COB_IDServerToClient,
// //                       nodeIDOfTheSDOServer);

// // if (cliSetupRet != CO_SDO_RT_ok_communicationEnd) {
// // return CO_ERROR_ILLEGAL_ARGUMENT;
// // }

// // return CO_ERROR_NO;
// // }
