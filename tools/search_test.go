package tools

import "testing"

func TestPoolProgramNames(t *testing.T) {
	tests := []struct {
		fn   func(string) string
		in   string
		want string
	}{
		// FUGR: non-namespaced
		{FugrPoolProgramName, "ZFUGR", "SAPLZFUGR"},
		{FugrPoolProgramName, "Z_ADT_MCP_TEST_FGRP", "SAPLZ_ADT_MCP_TEST_FGRP"},
		// FUGR: namespaced
		{FugrPoolProgramName, "/ACCGO/CAS_SETTLEMENT_UI", "/ACCGO/SAPLCAS_SETTLEMENT_UI"},
		// CLAS: padding to 30 chars + CP
		{ClassPoolProgramName, "ZCL_FOO", "ZCL_FOO=======================CP"},            // 7 + 23 + 2 = 32
		{ClassPoolProgramName, "ZCL_ABAPGIT_AUTH", "ZCL_ABAPGIT_AUTH==============CP"},   // 16 + 14 + 2 = 32
		{ClassPoolProgramName, "ZCL_ABAP2OTEL_COLLECTOR", "ZCL_ABAP2OTEL_COLLECTOR=======CP"}, // 23 + 7 + 2 = 32
		// INTF: padding to 30 chars + IP
		{IntfPoolProgramName, "ZIF_FOO", "ZIF_FOO=======================IP"},              // 7 + 23 + 2 = 32
		{IntfPoolProgramName, "ZIF_ABAPGIT_AJSON", "ZIF_ABAPGIT_AJSON=============IP"},   // 17 + 13 + 2 = 32
	}
	for _, tc := range tests {
		got := tc.fn(tc.in)
		if got != tc.want {
			t.Errorf("fn(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
