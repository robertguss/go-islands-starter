package config

import "testing"

func TestConfig(t *testing.T) {
	t.Setenv("MIGRATION_DATABASE_URL", "")
	valid := Config{DatabaseURL: "postgres://runtime", Origin: "https://app.example.test", ClientID: "client", ClientSecret: "secret", Port: "8080", Domains: []string{"example.test"}}
	if err := valid.Validate(); err != nil {
		t.Fatal(err)
	}
	for _, origin := range []string{"http://app.test", "https://app.test/path", "https://app.test?x=y", "https://user@app.test", "https://app.test#fragment"} {
		c := valid
		c.Origin = origin
		if c.Validate() == nil {
			t.Fatalf("accepted %s", origin)
		}
	}
	c := valid
	c.Port = "0"
	if c.Validate() == nil {
		t.Fatal("invalid port")
	}
	t.Setenv("MIGRATION_DATABASE_URL", "postgres://privileged")
	if valid.Validate() == nil {
		t.Fatal("migration credentials accepted")
	}
}
