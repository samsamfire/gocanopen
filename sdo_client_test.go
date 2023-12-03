package canopen

import (
	"os"
	"testing"
)

func TestSDOReadBlock(t *testing.T) {
	network := createNetwork()
	_, err := network.sdoClient.ReadAll(NODE_ID_TEST, 0x1021, 0)
	if err != SDO_ABORT_NONE {
		t.Fatal(err)
	}

}

func TestSDOWriteBlock(t *testing.T) {
	network := createNetwork()
	data := []byte(
		"this is some unimportant data",
	)
	node := network.Nodes[NODE_ID_TEST]
	node.OD.AddFile(0x3333, "File entry", "./here.txt", os.O_RDWR|os.O_CREATE, os.O_RDWR|os.O_CREATE)
	err := network.sdoClient.WriteRaw(NODE_ID_TEST, 0x3333, 0, data, false)
	if err != nil {
		t.Fatal(err)
	}
}
