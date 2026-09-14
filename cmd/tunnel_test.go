package cmd

import "testing"

func TestParseRoutesAddsRootFallback(t *testing.T) {
	routes, err := parseRoutes([]string{"/api=7000"}, 3000)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 2 || routes[0].Path != "/api" || routes[1].Path != "/" || routes[1].Port != 3000 {
		t.Fatalf("routes = %#v, want /api and root fallback", routes)
	}
}

func TestParseRoutesPreservesExplicitRoot(t *testing.T) {
	routes, err := parseRoutes([]string{"/=4000", "/api=7000"}, 3000)
	if err != nil {
		t.Fatal(err)
	}
	if len(routes) != 2 || routes[0].Port != 4000 {
		t.Fatalf("routes = %#v, want explicit root", routes)
	}
}

func TestParseRoutesRejectsInvalidMapping(t *testing.T) {
	cases := []string{"api=7000", "/api=nope", "/api=0", "/api"}
	for _, value := range cases {
		if _, err := parseRoutes([]string{value}, 3000); err == nil {
			t.Errorf("parseRoutes(%q) succeeded, want error", value)
		}
	}
}
