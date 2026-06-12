package providers

import (
	"context"
	"sort"
	"testing"
)

// TestChatStream_SparseAndOutOfOrderToolCalls verifies that sparse (e.g. index 0
// then 2 with 1 missing) and out-of-order tool_call delta indexes do not panic
// and produce all expected ToolCalls. The zai-coding/glm-5.1 path emits such
// deltas; the old parse loop indexed accumulators[0..len-1] and nil-deref'd on
// gaps, dropping high indexes and bypassing the no-silence guarantee.
func TestChatStream_SparseAndOutOfOrderToolCalls(t *testing.T) {
	tests := []struct {
		name        string
		chunks      []string
		wantNames   []string // tool names expected, sorted by index (ascending)
		wantArgPath map[string]string
	}{
		{
			name: "sparse_index_0_then_2",
			chunks: []string{
				`data: {"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]}}]}` + "\n\n",
				// gap: no index 1 ever arrives
				`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":2,"id":"call_c","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"c.txt\"}"}}]}}]}` + "\n\n",
				`data: {"choices":[{"index":0,"finish_reason":"tool_calls","delta":{}}]}` + "\n\n",
				"data: [DONE]\n\n",
			},
			wantNames:   []string{"read_file", "write_file"},
			wantArgPath: map[string]string{"read_file": "a.txt", "write_file": "c.txt"},
		},
		{
			name: "out_of_order_index_2_then_0",
			chunks: []string{
				`data: {"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":2,"id":"call_c","type":"function","function":{"name":"write_file","arguments":"{\"path\":\"c.txt\"}"}}]}}]}` + "\n\n",
				`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"read_file","arguments":"{\"path\":\"a.txt\"}"}}]}}]}` + "\n\n",
				`data: {"choices":[{"index":0,"finish_reason":"tool_calls","delta":{}}]}` + "\n\n",
				"data: [DONE]\n\n",
			},
			wantNames:   []string{"read_file", "write_file"},
			wantArgPath: map[string]string{"read_file": "a.txt", "write_file": "c.txt"},
		},
		{
			name: "single_high_index_only",
			chunks: []string{
				`data: {"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":5,"id":"call_hi","type":"function","function":{"name":"list_dir","arguments":"{\"path\":\"/hi\"}"}}]}}]}` + "\n\n",
				`data: {"choices":[{"index":0,"finish_reason":"tool_calls","delta":{}}]}` + "\n\n",
				"data: [DONE]\n\n",
			},
			wantNames:   []string{"list_dir"},
			wantArgPath: map[string]string{"list_dir": "/hi"},
		},
		{
			name: "interleaved_arg_fragments_sparse",
			chunks: []string{
				`data: {"choices":[{"index":0,"delta":{"role":"assistant","tool_calls":[{"index":0,"id":"call_a","type":"function","function":{"name":"read_file","arguments":"{\"pa"}}]}}]}` + "\n\n",
				`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":3,"id":"call_d","type":"function","function":{"name":"write_file","arguments":"{\"pa"}}]}}]}` + "\n\n",
				`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"th\":\"a.txt\"}"}}]}}]}` + "\n\n",
				`data: {"choices":[{"index":0,"delta":{"tool_calls":[{"index":3,"function":{"arguments":"th\":\"d.txt\"}"}}]}}]}` + "\n\n",
				`data: {"choices":[{"index":0,"finish_reason":"tool_calls","delta":{}}]}` + "\n\n",
				"data: [DONE]\n\n",
			},
			wantNames:   []string{"read_file", "write_file"},
			wantArgPath: map[string]string{"read_file": "a.txt", "write_file": "d.txt"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := newOpenAISSEServer(t, tt.chunks)
			p := newTestOpenAIProvider(server.URL)

			req := ChatRequest{
				Model:    "gpt-4",
				Messages: []Message{{Role: "user", Content: "sparse tools"}},
			}

			// Must not panic on sparse/out-of-order indexes.
			result, err := p.ChatStream(context.Background(), req, nil)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(result.ToolCalls) != len(tt.wantNames) {
				t.Fatalf("expected %d tool calls, got %d (%+v)", len(tt.wantNames), len(result.ToolCalls), result.ToolCalls)
			}

			gotNames := make([]string, len(result.ToolCalls))
			for i, tc := range result.ToolCalls {
				gotNames[i] = tc.Name
				if tc.ParseError != "" {
					t.Errorf("tool %q: unexpected ParseError %q", tc.Name, tc.ParseError)
				}
				if wantPath, ok := tt.wantArgPath[tc.Name]; ok {
					if got, _ := tc.Arguments["path"].(string); got != wantPath {
						t.Errorf("tool %q: path = %q, want %q", tc.Name, got, wantPath)
					}
				}
			}

			// Output is emitted in ascending-index order, matching wantNames.
			wantSorted := append([]string(nil), tt.wantNames...)
			sort.Strings(wantSorted)
			gotSorted := append([]string(nil), gotNames...)
			sort.Strings(gotSorted)
			for i := range wantSorted {
				if wantSorted[i] != gotSorted[i] {
					t.Errorf("tool names mismatch: got %v, want %v", gotNames, tt.wantNames)
					break
				}
			}

			if result.FinishReason != "tool_calls" {
				t.Errorf("FinishReason = %q, want %q", result.FinishReason, "tool_calls")
			}
		})
	}
}
