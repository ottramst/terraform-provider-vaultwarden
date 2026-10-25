package vaultwarden

import "testing"

func TestIsSupportedVersion(t *testing.T) {
	tests := []struct {
		version string
		want    bool
		wantErr bool
	}{
		{version: "1.37.4", want: true},
		{version: "1.36.0", want: true},
		{version: "2.0.0", want: true},
		{version: "1.37.4-abc123", want: true},
		{version: "1.35.8", want: false},
		{version: "1.32.5", want: false},
		{version: "0.40.0", want: false},
		{version: "1.37", wantErr: true},
		{version: "main", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.version, func(t *testing.T) {
			got, err := IsSupportedVersion(tt.version)
			if (err != nil) != tt.wantErr {
				t.Fatalf("IsSupportedVersion(%q) error = %v, wantErr %t", tt.version, err, tt.wantErr)
			}
			if got != tt.want {
				t.Errorf("IsSupportedVersion(%q) = %t, want %t", tt.version, got, tt.want)
			}
		})
	}
}
