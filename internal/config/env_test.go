package config

import (
	"testing"
)

func TestExpandEnvString(t *testing.T) {
	t.Setenv("TEST_GATEWAY_TOKEN", "secret-token")
	t.Setenv("TEST_GATEWAY_HOST", "localhost")
	t.Setenv("TEST_GATEWAY_EMPTY", "")

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{
			name:  "plain string",
			input: "127.0.0.1:9001",
			want:  "127.0.0.1:9001",
		},
		{
			name:  "required env exists",
			input: "${TEST_GATEWAY_TOKEN}",
			want:  "secret-token",
		},
		{
			name:  "env in middle of string",
			input: "http://${TEST_GATEWAY_HOST}:8080",
			want:  "http://localhost:8080",
		},
		{
			name:  "default value when env missing",
			input: "${TEST_GATEWAY_MISSING:127.0.0.1:9001}",
			want:  "127.0.0.1:9001",
		},
		{
			name:  "default value when env empty",
			input: "${TEST_GATEWAY_EMPTY:default-value}",
			want:  "default-value",
		},
		{
			name:    "missing env without default",
			input:   "${TEST_GATEWAY_MISSING}",
			wantErr: true,
		},
		{
			name:    "missing closing brace",
			input:   "${TEST_GATEWAY_TOKEN",
			wantErr: true,
		},
		{
			name:    "empty expression",
			input:   "${}",
			wantErr: true,
		},
		{
			name:    "empty variable name with default",
			input:   "${:default}",
			wantErr: true,
		},
		{
			name:  "multiple expressions",
			input: "http://${TEST_GATEWAY_HOST}:${TEST_GATEWAY_PORT:8080}",
			want:  "http://localhost:8080",
		},
		{
			name:  "dollar not expression",
			input: "$TEST_GATEWAY_TOKEN",
			want:  "$TEST_GATEWAY_TOKEN",
		},
		{
			name:  "default contains colon",
			input: "${TEST_GATEWAY_ADDR:127.0.0.1:9001}",
			want:  "127.0.0.1:9001",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := expandEnvString(tt.input)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if got != tt.want {
				t.Fatalf("got %q, want %q", got, tt.want)
			}
		})
	}
}

func TestSplitEnvExpression(t *testing.T) {
	tests := []struct {
		name             string
		input            string
		wantName         string
		wantDefaultValue string
		wantHasDefault   bool
	}{
		{
			name:           "name only",
			input:          "TOKEN",
			wantName:       "TOKEN",
			wantHasDefault: false,
		},
		{
			name:             "name with default",
			input:            "ADDR:127.0.0.1:9001",
			wantName:         "ADDR",
			wantDefaultValue: "127.0.0.1:9001",
			wantHasDefault:   true,
		},
		{
			name:             "empty default",
			input:            "TOKEN:",
			wantName:         "TOKEN",
			wantDefaultValue: "",
			wantHasDefault:   true,
		},
		{
			name:             "default contains colon",
			input:            "URL:http://localhost:8081",
			wantName:         "URL",
			wantDefaultValue: "http://localhost:8081",
			wantHasDefault:   true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotName, gotDefaultValue, gotHasDefault := splitEnvExpression(tt.input)

			if gotName != tt.wantName {
				t.Fatalf("name got %q, want %q", gotName, tt.wantName)
			}

			if gotDefaultValue != tt.wantDefaultValue {
				t.Fatalf("default value got %q, want %q", gotDefaultValue, tt.wantDefaultValue)
			}

			if gotHasDefault != tt.wantHasDefault {
				t.Fatalf("hasDefault got %v, want %v", gotHasDefault, tt.wantHasDefault)
			}
		})
	}
}
