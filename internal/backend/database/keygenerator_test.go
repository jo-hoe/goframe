package database

import (
	"testing"
)

func Test_generateID(t *testing.T) {
	tests := []struct {
		name string // description of this test case
		// Named input parameters for target function.
		data    []byte
		want    string
		wantErr bool
	}{
		{
			name:    "Test Basic Input",
			data:    []byte("test"),
			want:    "cd240807-1c12-5705-9123-e53533b6f6fa",
			wantErr: false,
		},
		{
			name:    "Test Different Input",
			data:    []byte("different"),
			want:    "b555c8d6-0e33-59ee-8d8d-b1d6dab352d6",
			wantErr: false,
		},
		{
			name:    "Test Nil Input",
			data:    nil,
			want:    "",
			wantErr: true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := generateID(tt.data)
			if got != tt.want {
				t.Errorf("generateID() = %v, want %v", got, tt.want)
			}
			if (err != nil) != tt.wantErr {
				t.Errorf("generateID() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
