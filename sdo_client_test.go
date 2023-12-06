package canopen

import (
	"os"
	"testing"

	log "github.com/sirupsen/logrus"
)

func init() {
	// Set the logger to debug
	log.SetLevel(log.DebugLevel)
}

func TestSDOReadExpedited(t *testing.T) {
	network := createNetwork()
	data := make([]byte, 10)
	for i := 0; i < 8; i++ {
		_, err := network.sdoClient.ReadRaw(NODE_ID_TEST, 0x2001+uint16(i), 0, data)
		if err != nil {
			t.Fatal(err)
		}
	}
}

// func TestSDOLocal(t *testing.T) {
// 	network := createNetwork()
// 	_, err := network.CreateNode(0x55, "./testdata/base.eds")
// 	if err != nil {
// 		t.Fatal(err)
// 	}
// 	data := []byte{0x10}
// 	for i := 0; i < 8; i++ {
// 		err := network.sdoClient.WriteRaw(0x55, 0x2001+uint16(i), 0, data, false)
// 		if err != nil {
// 			t.Fatal(err)
// 		}
// 	}
// }

func TestSDOReadBlock(t *testing.T) {
	network := createNetwork()
	_, err := network.sdoClient.ReadAll(NODE_ID_TEST, 0x1021, 0)
	if err != SDO_ABORT_NONE {
		t.Fatal(err)
	}

}

func TestSDOWriteBlock(t *testing.T) {
	network := createNetwork()
	data := []byte("some random string some random string some random string some random string some random stringsome random string some random string")
	node := network.Nodes[NODE_ID_TEST]
	node.OD.AddFile(0x3333, "File entry", "./here.txt", os.O_RDWR|os.O_CREATE, os.O_RDWR|os.O_CREATE)
	err := network.sdoClient.WriteRaw(NODE_ID_TEST, 0x3333, 0, data, false)
	if err != nil {
		t.Fatal(err)
	}
}
