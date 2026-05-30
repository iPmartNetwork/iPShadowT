package dns

import (
	"testing"
)

func TestBuildDNSQuery(t *testing.T) {
	query := buildDNSQuery("www.google.com", 1)

	// Should be a valid DNS query
	if len(query) < 12 {
		t.Fatal("Query too short")
	}

	// Check header flags (standard query, recursion desired)
	if query[2] != 0x01 || query[3] != 0x00 {
		t.Fatal("Invalid flags")
	}

	// Check question count = 1
	if query[4] != 0x00 || query[5] != 0x01 {
		t.Fatal("Question count should be 1")
	}
}

func TestSplitHostname(t *testing.T) {
	tests := []struct {
		input    string
		expected []string
	}{
		{"www.google.com", []string{"www", "google", "com"}},
		{"example.org", []string{"example", "org"}},
		{"a.b.c.d.e", []string{"a", "b", "c", "d", "e"}},
		{"single", []string{"single"}},
	}

	for _, tt := range tests {
		labels := splitHostname(tt.input)
		if len(labels) != len(tt.expected) {
			t.Fatalf("splitHostname(%q): got %v, want %v", tt.input, labels, tt.expected)
		}
		for i, l := range labels {
			if l != tt.expected[i] {
				t.Fatalf("splitHostname(%q)[%d]: got %q, want %q", tt.input, i, l, tt.expected[i])
			}
		}
	}
}

func TestParseDNSResponse(t *testing.T) {
	// Minimal DNS response with one A record (1.2.3.4)
	response := []byte{
		// Header
		0xAB, 0xCD, // Transaction ID
		0x81, 0x80, // Flags: response, recursion available
		0x00, 0x01, // Questions: 1
		0x00, 0x01, // Answers: 1
		0x00, 0x00, // Authority: 0
		0x00, 0x00, // Additional: 0
		// Question: example.com A IN
		0x07, 'e', 'x', 'a', 'm', 'p', 'l', 'e',
		0x03, 'c', 'o', 'm',
		0x00,       // Root
		0x00, 0x01, // Type A
		0x00, 0x01, // Class IN
		// Answer: pointer to name, A record, 1.2.3.4
		0xC0, 0x0C, // Pointer to offset 12 (question name)
		0x00, 0x01, // Type A
		0x00, 0x01, // Class IN
		0x00, 0x00, 0x01, 0x00, // TTL: 256
		0x00, 0x04, // RDLENGTH: 4
		0x01, 0x02, 0x03, 0x04, // RDATA: 1.2.3.4
	}

	ips, err := parseDNSResponse(response)
	if err != nil {
		t.Fatalf("parseDNSResponse failed: %v", err)
	}

	if len(ips) != 1 {
		t.Fatalf("expected 1 IP, got %d", len(ips))
	}

	if ips[0].String() != "1.2.3.4" {
		t.Fatalf("expected 1.2.3.4, got %s", ips[0].String())
	}
}
