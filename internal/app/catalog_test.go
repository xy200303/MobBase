package app

import "testing"

func TestCatalogRefreshOptions(t *testing.T) {
	refresh, err := parseRefresh([]string{"--refresh"}, "mob catalog")
	if err != nil || !refresh {
		t.Fatalf("refresh: %t %v", refresh, err)
	}
	if _, err := parseRefresh([]string{"--api", "35"}, "mob catalog"); err == nil {
		t.Fatal("expected invalid catalog option")
	}
}
