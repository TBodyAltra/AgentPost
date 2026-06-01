package main

import (
	"strings"
	"testing"
)

func TestNormalizeAgentMessageBodyForDisplay(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		in   string
		want string
	}{
		{
			name: "unicode in json request",
			in:   `{"request":"\u76ee\u6807\u5212\u5b8c\u6210"}`,
			want: "目标划完成",
		},
		{
			name: "literal escapes in request",
			in:   `{"request":"line1\\n\\n1. first\\n2. second"}`,
			want: "line1\n\n1. first\n2. second",
		},
		{
			name: "double encoded json string",
			in:   `"{\"request\":\"\\u76ee\"}"`,
			want: "目",
		},
		{
			name: "plain text",
			in:   "hello",
			want: "hello",
		},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			got := normalizeAgentMessageBodyForDisplay(tc.in)
			if got != tc.want {
				t.Fatalf("got %q, want %q", got, tc.want)
			}
			if strings.Contains(got, `\u`) || strings.Contains(got, `\n`) {
				t.Fatalf("result still contains literal escapes: %q", got)
			}
		})
	}
}
