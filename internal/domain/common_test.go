package domain

import "testing"

func TestCalculateMeta(t *testing.T) {
	tests := []struct {
		name         string
		total        int64
		page         int
		limit        int
		wantPage     int
		wantPerPage  int
		wantLastPage int
	}{
		{
			name:         "exact division total divides evenly by limit",
			total:        30,
			page:         1,
			limit:        10,
			wantPage:     1,
			wantPerPage:  10,
			wantLastPage: 3,
		},
		{
			name:         "uneven division total does not divide evenly by limit",
			total:        25,
			page:         1,
			limit:        10,
			wantPage:     1,
			wantPerPage:  10,
			wantLastPage: 3,
		},
		{
			name:         "single partial page",
			total:        5,
			page:         1,
			limit:        10,
			wantPage:     1,
			wantPerPage:  10,
			wantLastPage: 1,
		},
		{
			name:         "zero total still reports one page",
			total:        0,
			page:         1,
			limit:        10,
			wantPage:     1,
			wantPerPage:  10,
			wantLastPage: 1,
		},
		{
			name:         "zero page defaults to one",
			total:        20,
			page:         0,
			limit:        10,
			wantPage:     1,
			wantPerPage:  10,
			wantLastPage: 2,
		},
		{
			name:         "negative page defaults to one",
			total:        20,
			page:         -3,
			limit:        10,
			wantPage:     1,
			wantPerPage:  10,
			wantLastPage: 2,
		},
		{
			name:         "zero limit defaults to ten",
			total:        25,
			page:         1,
			limit:        0,
			wantPage:     1,
			wantPerPage:  10,
			wantLastPage: 3,
		},
		{
			name:         "negative limit defaults to ten",
			total:        25,
			page:         1,
			limit:        -5,
			wantPage:     1,
			wantPerPage:  10,
			wantLastPage: 3,
		},
		{
			name:         "page beyond last page is preserved",
			total:        20,
			page:         99,
			limit:        10,
			wantPage:     99,
			wantPerPage:  10,
			wantLastPage: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CalculateMeta(tt.total, tt.page, tt.limit)
			if got == nil {
				t.Fatal("CalculateMeta returned nil")
			}
			if got.Total != tt.total {
				t.Errorf("Total = %d, want %d", got.Total, tt.total)
			}
			if got.CurrentPage != tt.wantPage {
				t.Errorf("CurrentPage = %d, want %d", got.CurrentPage, tt.wantPage)
			}
			if got.PerPage != tt.wantPerPage {
				t.Errorf("PerPage = %d, want %d", got.PerPage, tt.wantPerPage)
			}
			if got.LastPage != tt.wantLastPage {
				t.Errorf("LastPage = %d, want %d", got.LastPage, tt.wantLastPage)
			}
		})
	}
}

func TestContainsAny(t *testing.T) {
	tests := []struct {
		name string
		s    string
		xs   []string
		want bool
	}{
		{name: "exact substring match", s: "cpu usage high", xs: []string{"usage"}, want: true},
		{name: "no match", s: "cpu usage high", xs: []string{"disk"}, want: false},
		{name: "match is case insensitive", s: "CPU Usage High", xs: []string{"usage"}, want: true},
		{name: "pattern is case insensitive", s: "cpu usage", xs: []string{"USAGE"}, want: true},
		{name: "wildcard prefix matches", s: "memory.used_gb", xs: []string{"memory.*"}, want: true},
		{name: "wildcard prefix does not match other", s: "cpu.usage_percent", xs: []string{"memory.*"}, want: false},
		{name: "bare star matches everything", s: "anything at all", xs: []string{"*"}, want: true},
		{name: "empty patterns never match", s: "cpu", xs: nil, want: false},
		{name: "empty haystack never matches", s: "", xs: []string{"cpu"}, want: false},
		{name: "any of several patterns matches", s: "disk full", xs: []string{"cpu", "disk"}, want: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ContainsAny(tt.s, tt.xs); got != tt.want {
				t.Errorf("ContainsAny(%q, %v) = %v, want %v", tt.s, tt.xs, got, tt.want)
			}
		})
	}
}
