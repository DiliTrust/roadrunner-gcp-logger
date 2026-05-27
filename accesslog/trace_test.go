package accesslog

import "testing"

func TestParseCloudTraceContext(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		wantTrace  string
		wantSpan   string
		wantSampl  bool
		wantHasSam bool
		wantOK     bool
	}{
		{
			name:       "trace span sampled",
			in:         "abcdef0123456789abcdef0123456789/123;o=1",
			wantTrace:  "abcdef0123456789abcdef0123456789",
			wantSpan:   "000000000000007b",
			wantSampl:  true,
			wantHasSam: true,
			wantOK:     true,
		},
		{
			name:       "trace span not sampled",
			in:         "deadbeef/1;o=0",
			wantTrace:  "deadbeef",
			wantSpan:   "0000000000000001",
			wantSampl:  false,
			wantHasSam: true,
			wantOK:     true,
		},
		{
			name:      "trace only",
			in:        "abcdef",
			wantTrace: "abcdef",
			wantOK:    true,
		},
		{
			name:      "trace and span no options",
			in:        "cafef00d/9",
			wantTrace: "cafef00d",
			wantSpan:  "0000000000000009",
			wantOK:    true,
		},
		{
			name:   "empty",
			in:     "",
			wantOK: false,
		},
		{
			name:      "non-numeric span ignored",
			in:        "feedface/notanumber;o=1",
			wantTrace: "feedface",
			// span ignored, but parse still succeeds
			wantSpan:   "",
			wantHasSam: true,
			wantSampl:  true,
			wantOK:     true,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tc, ok := parseCloudTraceContext(c.in)
			if ok != c.wantOK {
				t.Fatalf("ok=%v want %v", ok, c.wantOK)
			}
			if tc.traceID != c.wantTrace {
				t.Errorf("traceID=%q want %q", tc.traceID, c.wantTrace)
			}
			if tc.spanID != c.wantSpan {
				t.Errorf("spanID=%q want %q", tc.spanID, c.wantSpan)
			}
			if tc.hasSampled != c.wantHasSam {
				t.Errorf("hasSampled=%v want %v", tc.hasSampled, c.wantHasSam)
			}
			if tc.sampled != c.wantSampl {
				t.Errorf("sampled=%v want %v", tc.sampled, c.wantSampl)
			}
		})
	}
}

func TestParseTraceparent(t *testing.T) {
	cases := []struct {
		name       string
		in         string
		wantTrace  string
		wantSpan   string
		wantSampl  bool
		wantHasSam bool
		wantOK     bool
	}{
		{
			name:       "sampled",
			in:         "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
			wantTrace:  "4bf92f3577b34da6a3ce929d0e0e4736",
			wantSpan:   "00f067aa0ba902b7",
			wantSampl:  true,
			wantHasSam: true,
			wantOK:     true,
		},
		{
			name:       "not sampled",
			in:         "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-00",
			wantTrace:  "4bf92f3577b34da6a3ce929d0e0e4736",
			wantSpan:   "00f067aa0ba902b7",
			wantSampl:  false,
			wantHasSam: true,
			wantOK:     true,
		},
		{
			name:   "wrong version",
			in:     "01-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01",
			wantOK: false,
		},
		{
			name:   "missing flags",
			in:     "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7",
			wantOK: false,
		},
		{
			name:   "short trace id",
			in:     "00-abcd-00f067aa0ba902b7-01",
			wantOK: false,
		},
		{
			name:   "non-hex trace id",
			in:     "00-zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz-00f067aa0ba902b7-01",
			wantOK: false,
		},
		{
			name:   "short span id",
			in:     "00-4bf92f3577b34da6a3ce929d0e0e4736-deadbeef-01",
			wantOK: false,
		},
		{
			name:   "empty",
			in:     "",
			wantOK: false,
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			tc, ok := parseTraceparent(c.in)
			if ok != c.wantOK {
				t.Fatalf("ok=%v want %v", ok, c.wantOK)
			}
			if !c.wantOK {
				return
			}
			if tc.traceID != c.wantTrace {
				t.Errorf("traceID=%q want %q", tc.traceID, c.wantTrace)
			}
			if tc.spanID != c.wantSpan {
				t.Errorf("spanID=%q want %q", tc.spanID, c.wantSpan)
			}
			if tc.hasSampled != c.wantHasSam {
				t.Errorf("hasSampled=%v want %v", tc.hasSampled, c.wantHasSam)
			}
			if tc.sampled != c.wantSampl {
				t.Errorf("sampled=%v want %v", tc.sampled, c.wantSampl)
			}
		})
	}
}
