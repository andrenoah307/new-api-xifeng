package common

import "testing"

func TestStripProxyIdSuffixes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "single request id",
			in:   "error message (request id: 202605141428458399333168268d9d6bxJ92st8)",
			want: "error message",
		},
		{
			name: "multiple request ids",
			in:   "`temperature` and `top_p` cannot both be specified. (request id: aaa111) (request id: bbb222)",
			want: "`temperature` and `top_p` cannot both be specified.",
		},
		{
			name: "request_ori_id format",
			in:   "some error (request_ori_id: 50ae253d-5e3b-4b2c-be31-198c49680fca) (request id: 202605141408539495610798268d9d6UcS23nkG)",
			want: "some error",
		},
		{
			name: "fullwidth traceid",
			in:   "upstream error（traceid: cda4fce8ab5baf18d4e8417f61732d5f） (request id: abc123)",
			want: "upstream error",
		},
		{
			name: "mixed all three formats",
			in:   "msg (request_ori_id: uuid-here)（traceid: trace123） (request id: rid1) (request id: rid2)",
			want: "msg",
		},
		{
			name: "preserve provider trace ID inside message",
			in:   `{"error":{"message":"The prompt is too long (trace ID: e881296798f15260248dc38b75e0306c)"}} (request id: abc)`,
			want: `{"error":{"message":"The prompt is too long (trace ID: e881296798f15260248dc38b75e0306c)"}}`,
		},
		{
			name: "preserve provider error ID inside message",
			in:   "resource_exhausted (error ID: 6d19ab581080461e) (request id: xyz)",
			want: "resource_exhausted (error ID: 6d19ab581080461e)",
		},
		{
			name: "no proxy ids to strip",
			in:   "normal error message",
			want: "normal error message",
		},
		{
			name: "empty string",
			in:   "",
			want: "",
		},
		{
			name: "production 3-layer chain example",
			in:   "status_code=400, `temperature` and `top_p` cannot both be specified for this model. Please use only one. (request id: 202605141428458399333168268d9d6bxJ92st8) (request id: 202605141428458380333838268d9d6TxLv9cG1)",
			want: "status_code=400, `temperature` and `top_p` cannot both be specified for this model. Please use only one.",
		},
		{
			name: "production channel 217 gemini with fullwidth traceid",
			in:   `{"error":{"code":"invalid_argument","message":"The prompt is too long (trace ID: fc42398c)"}}` + "（traceid: dfea036eda5caf18） (request id: abc) (request id: def)",
			want: `{"error":{"code":"invalid_argument","message":"The prompt is too long (trace ID: fc42398c)"}}`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := StripProxyIdSuffixes(tt.in)
			if got != tt.want {
				t.Errorf("StripProxyIdSuffixes(%q)\n  got:  %q\n  want: %q", tt.in, got, tt.want)
			}
		})
	}
}
