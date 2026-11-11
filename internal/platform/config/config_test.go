package config

import "testing"

func TestMultitenantNeedsBaseDomain(t *testing.T) {
	t.Setenv("COLONY_STORE", "memory")
	t.Setenv("COLONY_MULTITENANT", "1")
	t.Setenv("COLONY_BASE_DOMAIN", "")
	if _, err := FromEnv(); err == nil {
		t.Fatal("multi-tenant mode without a base domain must not start")
	}
	t.Setenv("COLONY_BASE_DOMAIN", " Colonyhall.Example.com. ")
	c, err := FromEnv()
	if err != nil || !c.Multitenant || c.BaseDomain != "colonyhall.example.com" {
		t.Fatalf("config: %+v %v", c, err)
	}
	t.Setenv("COLONY_MULTITENANT", "")
	if c, err := FromEnv(); err != nil || c.Multitenant {
		t.Fatalf("single-tenant is the default: %+v %v", c, err)
	}
}
