package canopen

type httpSDOTimeoutRequest struct {
	Value string `json:"value"`
}

type httpSDOWriteRequest struct {
	Value    string `json:"value"`
	Datatype string `json:"datatype"`
}

type httpSDOReadResponse struct {
	Sequence int    `json:"sequence"`
	Response string `json:"response"`
	Data     string `json:"data"`
	Length   int    `json:"length,omitempty"`
}
