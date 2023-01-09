package canopen

import (
	"reflect"
	"testing"
)

func TestCANopenError_Error(t *testing.T) {
	tests := []struct {
		name  string
		error CANopenError
		want  string
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.error.Error(); got != tt.want {
				t.Errorf("CANopenError.Error() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestNewCANModule(t *testing.T) {
	type args struct {
		can     interface{}
		rxArray []CANrxMsg
		txArray []CANtxMsg
	}
	tests := []struct {
		name string
		args args
		want *CANModule
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NewCANModule(tt.args.can, tt.args.rxArray, tt.args.txArray); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("NewCANModule() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestCANModule_SetRxFilters(t *testing.T) {
	type fields struct {
		CAN               interface{}
		RxArray           []CANrxMsg
		TxArray           []CANtxMsg
		CANerrorstatus    uint16
		CANnormal         bool
		UseCANrxFilters   bool
		BufferInhibitFlag bool
		FirstCANtxMessage bool
		CANtxCount        uint32
		ErrOld            uint32
	}
	tests := []struct {
		name   string
		fields fields
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canmodule := &CANModule{
				CAN:               tt.fields.CAN,
				RxArray:           tt.fields.RxArray,
				TxArray:           tt.fields.TxArray,
				CANerrorstatus:    tt.fields.CANerrorstatus,
				CANnormal:         tt.fields.CANnormal,
				UseCANrxFilters:   tt.fields.UseCANrxFilters,
				BufferInhibitFlag: tt.fields.BufferInhibitFlag,
				FirstCANtxMessage: tt.fields.FirstCANtxMessage,
				CANtxCount:        tt.fields.CANtxCount,
				ErrOld:            tt.fields.ErrOld,
			}
			canmodule.SetRxFilters()
		})
	}
}

func TestCANModule_Send(t *testing.T) {
	type fields struct {
		CAN               interface{}
		RxArray           []CANrxMsg
		TxArray           []CANtxMsg
		CANerrorstatus    uint16
		CANnormal         bool
		UseCANrxFilters   bool
		BufferInhibitFlag bool
		FirstCANtxMessage bool
		CANtxCount        uint32
		ErrOld            uint32
	}
	type args struct {
		buffer []CANtxMsg
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantResult COResult
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canmodule := &CANModule{
				CAN:               tt.fields.CAN,
				RxArray:           tt.fields.RxArray,
				TxArray:           tt.fields.TxArray,
				CANerrorstatus:    tt.fields.CANerrorstatus,
				CANnormal:         tt.fields.CANnormal,
				UseCANrxFilters:   tt.fields.UseCANrxFilters,
				BufferInhibitFlag: tt.fields.BufferInhibitFlag,
				FirstCANtxMessage: tt.fields.FirstCANtxMessage,
				CANtxCount:        tt.fields.CANtxCount,
				ErrOld:            tt.fields.ErrOld,
			}
			if gotResult := canmodule.Send(tt.args.buffer); gotResult != tt.wantResult {
				t.Errorf("CANModule.Send() = %v, want %v", gotResult, tt.wantResult)
			}
		})
	}
}

func TestCANModule_ClearSyncPDOs(t *testing.T) {
	type fields struct {
		CAN               interface{}
		RxArray           []CANrxMsg
		TxArray           []CANtxMsg
		CANerrorstatus    uint16
		CANnormal         bool
		UseCANrxFilters   bool
		BufferInhibitFlag bool
		FirstCANtxMessage bool
		CANtxCount        uint32
		ErrOld            uint32
	}
	tests := []struct {
		name       string
		fields     fields
		wantResult COResult
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canmodule := &CANModule{
				CAN:               tt.fields.CAN,
				RxArray:           tt.fields.RxArray,
				TxArray:           tt.fields.TxArray,
				CANerrorstatus:    tt.fields.CANerrorstatus,
				CANnormal:         tt.fields.CANnormal,
				UseCANrxFilters:   tt.fields.UseCANrxFilters,
				BufferInhibitFlag: tt.fields.BufferInhibitFlag,
				FirstCANtxMessage: tt.fields.FirstCANtxMessage,
				CANtxCount:        tt.fields.CANtxCount,
				ErrOld:            tt.fields.ErrOld,
			}
			if gotResult := canmodule.ClearSyncPDOs(); gotResult != tt.wantResult {
				t.Errorf("CANModule.ClearSyncPDOs() = %v, want %v", gotResult, tt.wantResult)
			}
		})
	}
}

func TestCANModule_Process(t *testing.T) {
	type fields struct {
		CAN               interface{}
		RxArray           []CANrxMsg
		TxArray           []CANtxMsg
		CANerrorstatus    uint16
		CANnormal         bool
		UseCANrxFilters   bool
		BufferInhibitFlag bool
		FirstCANtxMessage bool
		CANtxCount        uint32
		ErrOld            uint32
	}
	tests := []struct {
		name       string
		fields     fields
		wantResult COResult
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canmodule := &CANModule{
				CAN:               tt.fields.CAN,
				RxArray:           tt.fields.RxArray,
				TxArray:           tt.fields.TxArray,
				CANerrorstatus:    tt.fields.CANerrorstatus,
				CANnormal:         tt.fields.CANnormal,
				UseCANrxFilters:   tt.fields.UseCANrxFilters,
				BufferInhibitFlag: tt.fields.BufferInhibitFlag,
				FirstCANtxMessage: tt.fields.FirstCANtxMessage,
				CANtxCount:        tt.fields.CANtxCount,
				ErrOld:            tt.fields.ErrOld,
			}
			if gotResult := canmodule.Process(); gotResult != tt.wantResult {
				t.Errorf("CANModule.Process() = %v, want %v", gotResult, tt.wantResult)
			}
		})
	}
}

func TestCANModule_TxBufferInit(t *testing.T) {
	type fields struct {
		CAN               interface{}
		RxArray           []CANrxMsg
		TxArray           []CANtxMsg
		CANerrorstatus    uint16
		CANnormal         bool
		UseCANrxFilters   bool
		BufferInhibitFlag bool
		FirstCANtxMessage bool
		CANtxCount        uint32
		ErrOld            uint32
	}
	type args struct {
		index    uint32
		ident    uint32
		rtr      bool
		length   uint8
		syncFlag bool
	}
	tests := []struct {
		name       string
		fields     fields
		args       args
		wantResult COResult
		wantMsg    *CANtxMsg
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canmodule := &CANModule{
				CAN:               tt.fields.CAN,
				RxArray:           tt.fields.RxArray,
				TxArray:           tt.fields.TxArray,
				CANerrorstatus:    tt.fields.CANerrorstatus,
				CANnormal:         tt.fields.CANnormal,
				UseCANrxFilters:   tt.fields.UseCANrxFilters,
				BufferInhibitFlag: tt.fields.BufferInhibitFlag,
				FirstCANtxMessage: tt.fields.FirstCANtxMessage,
				CANtxCount:        tt.fields.CANtxCount,
				ErrOld:            tt.fields.ErrOld,
			}
			gotResult, gotMsg := canmodule.TxBufferInit(tt.args.index, tt.args.ident, tt.args.rtr, tt.args.length, tt.args.syncFlag)
			if gotResult != tt.wantResult {
				t.Errorf("CANModule.TxBufferInit() gotResult = %v, want %v", gotResult, tt.wantResult)
			}
			if !reflect.DeepEqual(gotMsg, tt.wantMsg) {
				t.Errorf("CANModule.TxBufferInit() gotMsg = %v, want %v", gotMsg, tt.wantMsg)
			}
		})
	}
}

func TestCANModule_RxBufferInit(t *testing.T) {
	type fields struct {
		CAN               interface{}
		RxArray           []CANrxMsg
		TxArray           []CANtxMsg
		CANerrorstatus    uint16
		CANnormal         bool
		UseCANrxFilters   bool
		BufferInhibitFlag bool
		FirstCANtxMessage bool
		CANtxCount        uint32
		ErrOld            uint32
	}
	type args struct {
		index    uint32
		ident    uint32
		mask     uint32
		rtr      bool
		object   interface{}
		callback func(object interface{}, message interface{})
	}
	tests := []struct {
		name   string
		fields fields
		args   args
		want   CANopenError
	}{
		// TODO: Add test cases.
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canmodule := &CANModule{
				CAN:               tt.fields.CAN,
				RxArray:           tt.fields.RxArray,
				TxArray:           tt.fields.TxArray,
				CANerrorstatus:    tt.fields.CANerrorstatus,
				CANnormal:         tt.fields.CANnormal,
				UseCANrxFilters:   tt.fields.UseCANrxFilters,
				BufferInhibitFlag: tt.fields.BufferInhibitFlag,
				FirstCANtxMessage: tt.fields.FirstCANtxMessage,
				CANtxCount:        tt.fields.CANtxCount,
				ErrOld:            tt.fields.ErrOld,
			}
			if got := canmodule.RxBufferInit(tt.args.index, tt.args.ident, tt.args.mask, tt.args.rtr, tt.args.object, tt.args.callback); got != tt.want {
				t.Errorf("CANModule.RxBufferInit() = %v, want %v", got, tt.want)
			}
		})
	}
}
