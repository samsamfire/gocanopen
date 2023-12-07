// CiA 309-5 implementation
package canopen

import (
	"net/http/httptest"
	"testing"
	"time"

	log "github.com/sirupsen/logrus"
)

var NODE_ID_TEST uint8 = 0x30

func init() {
	// Set the logger to debug
	log.SetLevel(log.WarnLevel)
}

func createGateway() *HTTPGatewayServer {
	network := NewNetwork(nil)
	e := network.Connect("virtualcan", "localhost:18888", 500000)
	if e != nil {
		panic(e)
	}
	e = network.AddNode(NODE_ID_TEST, "testdata/base.eds")
	if e != nil {
		panic(e)
	}
	gateway := NewGateway(1, 1, 100, &network)
	go func() {
		network.Process()
	}()
	return gateway
}

func TestHTTPRead(t *testing.T) {
	gateway := createGateway()
	net := createNetwork()
	defer net.Disconnect()
	defer gateway.network.Disconnect()
	time.Sleep(1 * time.Second)
	ts := httptest.NewServer(gateway.serveMux)
	defer ts.Close()
	client := NewHTTPGatewayClient(ts.URL, "1.0", 1)
	type args struct {
		index    uint16
		subindex uint8
		value    string
	}
	tests := []struct {
		name string
		args args
	}{
		{
			name: "1",
			args: args{0x2001, 0, "0x01"},
		},
		{
			name: "2",
			args: args{0x2002, 0, "0x33"},
		},
		{
			name: "3",
			args: args{0x2003, 0, "0x4444"},
		},
		{
			name: "4",
			args: args{0x2004, 0, "0x55555555"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, _, err := client.Read(NODE_ID_TEST, tt.args.index, tt.args.subindex)
			if err != nil {
				t.Fatalf(tt.name)
			}
			if data != tt.args.value {
				t.Fatalf("expected value %v, got %v", tt.args.value, data)
			}
		})
	}
}

// func TestHandler(t *testing.T) {
// 	gateway := createGateway()
// 	ts := httptest.NewServer(gateway.serveMux)
// 	defer ts.Close()

// 	newreq := func(method, url string, body io.Reader) *http.Request {
// 		r, err := http.NewRequest(method, url, body)
// 		if err != nil {
// 			t.Fatal(err)
// 		}
// 		return r
// 	}

// 	type args struct {
// 		uri string
// 	}

// 	tests := []struct {
// 		name    string
// 		r       *http.Request
// 		wantErr bool
// 	}{
// 		{name: "1 : invalid request (no command)", r: newreq("GET", ts.URL+"/cia309-5/1.0/", nil), wantErr: true},
// 		{name: "2 : invalid request (improper node)", r: newreq("POST", ts.URL+"/cia309-5/1.0/xxx10/all/all/start/", nil), wantErr: true},
// 		{name: "3 : valid sdo read request", r: newreq("GET", ts.URL+"/cia309-5/1.0/10/0x1/0x10/read/", nil), wantErr: false},
// 	}

// 	// tests := []struct {
// 	// 	name    string
// 	// 	args    args
// 	// 	wantErr bool
// 	// }{
// 	// 	{
// 	// 		name:    "1",
// 	// 		args:    args{uri: "/cia309-5/1.0/"},
// 	// 		wantErr: true,
// 	// 	},
// 	// 	{
// 	// 		name:    "2",
// 	// 		args:    args{uri: `/cia309-5/1.0/10/all/all/start/`},
// 	// 		wantErr: false,
// 	// 	},
// 	// 	{
// 	// 		name:    "3",
// 	// 		args:    args{uri: "/cia309-5/1.0/xxx10/all/all/start/"},
// 	// 		wantErr: true,
// 	// 	},
// 	// 	{
// 	// 		name:    "4",
// 	// 		args:    args{uri: "/cia309-5/1.0/10/0x1/0x10/read/"},
// 	// 		wantErr: false,
// 	// 	},
// 	// 	{
// 	// 		name:    "network_to_high",
// 	// 		args:    args{uri: "/cia309-5/1.0/10/333333/0x10/read/"},
// 	// 		wantErr: true,
// 	// 	},
// 	// 	{
// 	// 		name:    "node_to_high",
// 	// 		args:    args{uri: "/cia309-5/1.0/10/1/0x100/read/"},
// 	// 		wantErr: true,
// 	// 	},
// 	// }
// 	for _, tt := range tests {
// 		t.Run(tt.name, func(t *testing.T) {
// 			resp, err := http.DefaultClient.Do(tt.r)
// 			if err != nil {
// 				t.Fatal(err)
// 			}
// 			// Get JSON resonse
// 			jsonRsp := new(HTTPGatewayResponse)
// 			err = json.NewDecoder(resp.Body).Decode(jsonRsp)
// 			fmt.Print(jsonRsp)
// 			if err != nil {
// 				t.Fatal(err)
// 			}
// 			if tt.wantErr && jsonRsp.Error == 0 {
// 				t.Fatal("expecting error in gateway response")
// 			} else if !tt.wantErr && jsonRsp.Error != 0 {
// 				t.Fatal("gateway returned an error (not expected)")
// 			}
// 		})
// 	}
// }
