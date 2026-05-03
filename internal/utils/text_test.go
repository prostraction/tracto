package utils

import (
	"testing"
)

func TestSanitizeBytes(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected string
	}{
		{
			name:     "clean ascii",
			input:    []byte("hello world"),
			expected: "hello world",
		},
		{
			name:     "valid utf8",
			input:    []byte("привет мир 😊"),
			expected: "привет мир 😊",
		},
		{
			name:     "invalid utf8 sequence",
			input:    []byte("hello\xffworld"),
			expected: "hello\ufffdworld",
		},
		{
			name:     "control chars removed",
			input:    []byte("hello\x01\x1fworld"),
			expected: "helloworld",
		},
		{
			name:     "tabs expanded",
			input:    []byte("hello\tworld"),
			expected: "hello    world",
		},
		{
			name:     "crlf normalized",
			input:    []byte("hello\r\nworld"),
			expected: "hello\nworld",
		},
		{
			name:     "cr normalized",
			input:    []byte("hello\rworld"),
			expected: "hello\nworld",
		},
		{
			name:     "special spaces normalized",
			input:    []byte("hello\u2028world\u2029"),
			expected: "hello\u2028world\u2029",
		},
		{
			name:     "zero width chars removed",
			input:    []byte("hello\u200b\ufeffworld"),
			expected: "hello\u200b\ufeffworld",
		},
		{
			name:     "del and above removed",
			input:    []byte("hello\x7f\x1fworld"),
			expected: "helloworld",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := SanitizeBytes(tc.input)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestSanitizeText(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "clean ascii",
			input:    "hello world",
			expected: "hello world",
		},
		{
			name:     "valid utf8",
			input:    "привет мир 😊",
			expected: "привет мир 😊",
		},
		{
			name:     "invalid utf8 sequence",
			input:    "hello\xffworld",
			expected: "hello\ufffdworld",
		},
		{
			name:     "control chars removed",
			input:    "hello\x01\x1fworld",
			expected: "helloworld",
		},
		{
			name:     "tabs expanded",
			input:    "hello\tworld",
			expected: "hello    world",
		},
		{
			name:     "crlf normalized",
			input:    "hello\r\nworld",
			expected: "hello\nworld",
		},
		{
			name:     "cr normalized",
			input:    "hello\rworld",
			expected: "hello\nworld",
		},
		{
			name:     "special spaces normalized",
			input:    "hello\u2028world\u2029",
			expected: "hello\nworld\n",
		},
		{
			name:     "zero width chars removed",
			input:    "hello\u200b\ufeffworld",
			expected: "helloworld",
		},
		{
			name:     "del and above removed",
			input:    "hello\x7f\x9fworld",
			expected: "hello\ufffdworld",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := SanitizeText(tc.input)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestStripJSONComments(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "no comments",
			input:    `{"key": "value"}`,
			expected: `{"key": "value"}`,
		},
		{
			name:     "line comment at end",
			input:    `{"key": "value"} // comment`,
			expected: `{"key": "value"} `,
		},
		{
			name:     "line comment in middle",
			input:    "{\n  // comment\n  \"key\": \"value\"\n}",
			expected: "{\n  \n  \"key\": \"value\"\n}",
		},
		{
			name:     "comment inside string",
			input:    `{"key": "http://example.com"}`,
			expected: `{"key": "http://example.com"}`,
		},
		{
			name:     "escaped quote in string",
			input:    `{"key": "http://example.com\" // not comment"}`,
			expected: `{"key": "http://example.com\" // not comment"}`,
		},
		{
			name:     "escaped backslash before quote",
			input:    `{"key": "http://example.com\\" // comment}`,
			expected: `{"key": "http://example.com\\" `,
		},
		{
			name:     "multiple comments",
			input:    `// c1` + "\n" + `{"a": 1} // c2` + "\n" + `// c3`,
			expected: "\n" + `{"a": 1} ` + "\n",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result := StripJSONComments(tc.input)
			if result != tc.expected {
				t.Errorf("expected %q, got %q", tc.expected, result)
			}
		})
	}
}
